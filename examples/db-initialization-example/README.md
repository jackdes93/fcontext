# Database Initialization Example

Ví dụ hoàn chỉnh về cách dùng `job.Hub + WorkerPool` để orchestrate database initialization tasks theo thứ tự:

1. **Phase 1**: Job "CheckTable" - Kiểm tra table tồn tại, nếu không thì tạo
2. **Phase 2**: Job "InsertData" - Insert dữ liệu mẫu (chạy sau Phase 1 thành công)
3. **Phase 3**: Query dữ liệu để verify

## 📋 Kiến trúc

```
Service Start
      ↓
Phase 1: CheckTable Job
    ├─ Kiểm tra table 'users' tồn tại?
    ├─ Nếu không → Tạo table
    └─ OnComplete → Phase 2
      ↓
Phase 2: InsertData Job (chạy sau Phase 1 thành công)
    ├─ Insert sample data
    └─ OnComplete → Phase 3
      ↓
Phase 3: Query & Display
      ↓
✨ Service initialization completed
```

## 🎯 Thành Phần

### 1. **DatabaseService**
```go
type DatabaseService struct {
    db *sql.DB
    mu sync.Mutex
}

// Phương thức:
- CheckAndCreateTable(ctx)  // Check + tạo table
- InsertSampleData(ctx)      // Insert dữ liệu mẫu
- QueryAllUsers(ctx)         // Lấy tất cả users
```

### 2. **InitializationEngine**
```go
type InitializationEngine struct {
    hub   job.Hub
    pool  *WorkerPool
    db    *DatabaseService
}

// Phương thức:
- RunCheckTableJob(ctx)      // Chạy job check table
- RunInsertDataJob(ctx)      // Chạy job insert data
```

### 3. **WorkerPool**
- Kiểm soát concurrency (max 2 workers)
- Quản lý goroutine lifecycle

## 🏃 Chạy Example

### Chuẩn bị

```bash
# Cài đặt SQLite driver
go get github.com/mattn/go-sqlite3
```

### Chạy

```bash
cd examples/db-initialization-example
make run
```

### Output mẫu

```
🚀 Database Initialization Service
==================================

📍 Phase 1: Checking and creating tables...

[Worker-0] ⚙️  Bắt đầu xử lý job
🔍 Checking if 'users' table exists...
✅ Table 'users' already exists
✅ [CheckTable-1] Table check completed!

📍 Phase 2: Inserting sample data...

[Worker-0] ⚙️  Bắt đầu xử lý job
📝 Inserting sample data...
  ✓ Inserted: Alice
  ✓ Inserted: Bob
  ✓ Inserted: Charlie
✅ [InsertData-1] Data insertion completed!

📍 Phase 3: Querying data...

📊 Users in database:
  [1] Alice (alice@example.com) - Age: 28
  [2] Bob (bob@example.com) - Age: 32
  [3] Charlie (charlie@example.com) - Age: 25

✨ Service initialization completed successfully!
```

## 🔑 Key Features

| Feature | Mô tả |
|---------|------|
| **Sequential Jobs** | Phase 1 → Phase 2 (chạy theo thứ tự) |
| **Thread-Safe DB** | Mutex + atomic operations |
| **Retry Logic** | Tự động retry nếu fail |
| **Callbacks** | OnRetry, OnComplete, OnPermanent |
| **Error Handling** | Xử lý lỗi database gracefully |
| **Timeout** | 10s timeout per job |

## 📊 Workflow Chi Tiết

### Bước 1: Main khởi động
```go
engine := NewInitializationEngine(2, dbService) // Pool size = 2
```

### Bước 2: Chạy Check Table Job
```go
checkTableJob, _ := engine.RunCheckTableJob(context.Background())

// Job sẽ:
// 1. Execute() → database.CheckAndCreateTable()
// 2. Nếu fail → Retry dengan delay (1s, 2s)
// 3. OnComplete → Callback triggered
```

### Bước 3: Chờ Job hoàn thành
```go
for {
    state := checkTableJob.State()
    if state == job.StateCompleted || state == job.StateRetryFailed {
        break
    }
    time.Sleep(100 * time.Millisecond)
}
```

### Bước 4: Chạy Insert Data Job (chỉ nếu check table thành công)
```go
if checkTableJob.State() == job.StateCompleted {
    engine.RunInsertDataJob(context.Background())
}
```

### Bước 5: Query verification
```go
dbService.QueryAllUsers(context.Background())
```

## 🛣️ Use Cases Thực Tế

### Migration Pattern
```go
// Job 1: Backup data
// Job 2: Alter table
// Job 3: Migrate data
// Job 4: Verify integrity
```

### Multi-Tenant Setup
```go
// Loop qua tất cả tenant:
for _, tenant := range tenants {
    // Job: CheckAndCreateTables(tenant)
    // Job: InsertDefaultData(tenant)
    // Job: VerifyData(tenant)
}
```

### Service Bootstrapping
```go
// Job 1: Check DB connection
// Job 2: Run migrations
// Job 3: Load seed data
// Job 4: Warm up cache
// Job 5: Start service
```

## ⚙️ Tuning

### Tăng pool size (handle nhiều job parallel)
```go
engine := NewInitializationEngine(5, dbService) // 5 workers
```

### Tùy chỉnh retry strategy
```go
job.WithRetries([]time.Duration{
    500*time.Millisecond,
    2*time.Second,
    5*time.Second,
})
```

### Tăng timeout
```go
job.WithTimeout(30*time.Second) // 30 seconds
```

## 🧪 Testing

```go
// Test CheckAndCreateTable
func TestCheckAndCreateTable(t *testing.T) {
    db, _ := NewDatabaseService(":memory:") // In-memory SQLite
    err := db.CheckAndCreateTable(context.Background())
    assert.NoError(t, err)
}

// Test InsertSampleData
func TestInsertSampleData(t *testing.T) {
    db, _ := NewDatabaseService(":memory:")
    db.CheckAndCreateTable(context.Background())
    err := db.InsertSampleData(context.Background())
    assert.NoError(t, err)
}
```

## 📚 So sánh: Trước vs Sau

### ❌ Trước (Manual sequential)
```go
// Phase 1
if err := createTableIfNotExists(); err != nil {
    log.Fatal(err)
}

// Phase 2
if err := insertSampleData(); err != nil {
    log.Fatal(err)
}

// Vấn đề:
// - Không có retry logic
// - Khó mở rộng
// - Khó monitoring
// - Khó test
```

### ✅ Sau (job.Hub + WorkerPool)
```go
engine := NewInitializationEngine(2, dbService)

checkTableJob, _ := engine.RunCheckTableJob(ctx)
// Chờ...
engine.RunInsertDataJob(ctx)

// Lợi ích:
// + Automatic retry
// + Lifecycle callbacks
// + Easy to scale
// + Easy to monitor
// + Easy to test
```

## 🚀 Production Considerations

1. **Database Connection Pool** - Tăng pool size nếu có nhiều jobs
2. **Timeout Configuration** - Tùy chỉnh dựa trên DB size
3. **Error Logging** - Log tất cả errors cho debugging
4. **Graceful Shutdown** - Call `engine.Stop()` trước khi exit
5. **Health Checks** - Verify data sau initialization

## 📖 Tham khảo

- [job/GUIDE.md](../../job/GUIDE.md) - Job package usage
- [job/types.go](../../job/types.go) - Job API reference
- [SQLite Go Driver](https://github.com/mattn/go-sqlite3)
