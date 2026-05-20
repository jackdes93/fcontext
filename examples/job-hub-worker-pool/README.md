# Job Hub + Worker Pool Example

Ví dụ hoàn chỉnh về cách dùng `job.Hub` kết hợp `WorkerPool` để xây dựng hệ thống xử lý job asynchronous có scalability tốt.

## 📋 Kiến trúc

```
PubSub Message
      ↓
  Engine (Subscribe)
      ↓
  [Tạo Job]
      ↓
  job.Hub.Submit()
      ↓
  WorkerPool.Submit()
      ↓
  Worker Goroutine (max: poolSize)
      ↓
  j.RunWithRetry() → Execute + Retry
      ↓
  OnComplete / OnPermanent Callback
```

## 🎯 Các Thành Phần

### 1. **WorkerPool** - Kiểm soát concurrency
```go
pool := NewWorkerPool(3) // Max 3 goroutine đồng thời

// Submit job vào worker pool
pool.Submit(jobObj)

// Kiểm tra trạng thái
count := pool.GetRunningCount()
```

**Lợi ích:**
- Giới hạn số goroutine → Tiết kiệm memory
- Tránh context switch quá nhiều
- Backpressure: Queue đầy → reject job

### 2. **job.Hub** - Quản lý job types
```go
hub := job.NewHub(func(j job.Job) bool {
    return pool.Submit(j)  // Custom submission logic
})

// Tạo job
j, _ := hub.Create("ProcessLicense", handler,
    job.WithTimeout(5*time.Second),
    job.WithRetries([]time.Duration{1s, 2s}),
    job.WithJitter(0.2),
    job.WithOnRetry(...),
    job.WithOnComplete(...),
)

hub.Submit(j)
```

### 3. **Engine** - Kết nối PubSub + Hub
```go
engine := NewEngineWithJobHub(3)

engine.SubscribeTopic("license.response",
    licenseHandler,
    webhookHandler,
)

engine.Stop()
```

## 🏃 Chạy Example

```bash
cd examples/job-hub-worker-pool
go run main.go
```

## 📊 Output mẫu

```
🚀 Job Hub + Worker Pool Example
================================

✅ Subscribe topic: license.response với 2 handler(s)
✅ Subscribe topic: device.disconnect với 1 handler(s)

[Worker-0] Bắt đầu xử lý job
  🔍 Xử lý license từ message: message-1
  🌐 Gửi webhook cho message: message-1
📊 Worker pool: 2 job(s) đang chạy

[Worker-1] Bắt đầu xử lý job
  🔌 Xử lý disconnect từ message: message-1
📊 Worker pool: 3 job(s) đang chạy

✅ Hoàn thành job 'ProcessLicense'
[Worker-0] Bắt đầu xử lý job
  🔍 Xử lý license từ message: message-2

📊 Worker pool: 2 job(s) đang chạy
...
```

## 🔑 Key Features

| Feature | Mô tả |
|---------|------|
| **Concurrency Control** | WorkerPool giới hạn số worker |
| **Automatic Retry** | Exponential backoff + jitter |
| **Lifecycle Callbacks** | OnRetry, OnComplete, OnPermanent |
| **Timeout Handling** | Context-based timeout |
| **Thread-Safe** | Mutex + atomic operations |
| **Scalable** | Có thể handle hàng ngàn message |

## ⚙️ Tuning

### Tăng pool size (handle nhiều job)
```go
engine := NewEngineWithJobHub(10) // 10 workers
```

### Tùy chỉnh retry strategy
```go
job.WithRetries([]time.Duration{
    100*time.Millisecond,  // Retry 1
    500*time.Millisecond,  // Retry 2
    2*time.Second,         // Retry 3
    5*time.Second,         // Retry 4
})
```

### Buffer size queue
```go
taskChan: make(chan job.Job, maxSize*2) // 2x buffer
```

## 🚨 Error Handling

Job sẽ retry tối đa theo `Retries` config nếu:
- Handler return error
- Context timeout
- Goroutine panic (job sẽ track lỗi)

Nếu retry hết lần → `OnPermanent` callback được gọi

## 📚 So sánh với asyncjob

| Tính năng | asyncjob | job.Hub |
|-----------|----------|---------|
| Retry | ❌ | ✅ Exponential backoff |
| Jitter | ❌ | ✅ ±20% variance |
| Callbacks | ❌ | ✅ 3x callbacks |
| Hub | ❌ | ✅ Multiple types |
| Thread-Safe | ⚠️ | ✅ Mutex + atomic |
| Error Tracking | ❌ (TODO) | ✅ LastError() |

## 🎓 Học thêm

- [job/GUIDE.md](../../job/GUIDE.md) - Usage patterns
- [job/USECASE.md](../../job/USECASE.md) - Real-world examples
- [job/types.go](../../job/types.go) - API reference
