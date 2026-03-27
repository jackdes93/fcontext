# Job Package - Usage Guide

## Tổng Quan

Package `job` cung cấp một hệ thống quản lý job linh hoạt, hỗ trợ retry tự động, timeout, và callback lifecycle events.

## Cấu Trúc Cơ Bản

Package có hai phần chính:

1. **Job Interface** (`types.go`): Định nghĩa cơ bản cho một job độc lập
2. **Hub Interface** (`hub.go`): Quản lý nhiều job types và submit chúng vào worker pool

---

## Phần 1: Sử Dụng Job Interface

### 1.1 Tạo một Job Đơn Giản

```go
// Định nghĩa handler function
handler := func(ctx context.Context) error {
    fmt.Println("Executing task...")
    return nil
}

// Tạo job
job := job.New(handler,
    job.WithName("my-task"),
    job.WithTimeout(5 * time.Second),
)

// Chạy job
ctx := context.Background()
if err := job.Execute(ctx); err != nil {
    log.Printf("Job failed: %v", err)
}

// Kiểm tra trạng thái
fmt.Printf("State: %v\n", job.State())
```

### 1.2 Xử Lý Error và Retry

```go
handler := func(ctx context.Context) error {
    // Giả sử có 30% chance thất bại
    if rand.Float32() < 0.3 {
        return errors.New("random failure")
    }
    fmt.Println("Success!")
    return nil
}

callCount := 0
job := job.New(handler,
    job.WithName("flaky-task"),
    job.WithTimeout(10 * time.Second),
    // Retry schedule: sau 100ms, 500ms, 1s
    job.WithRetries([]time.Duration{
        100 * time.Millisecond,
        500 * time.Millisecond,
        1 * time.Second,
    }),
    // Callback khi retry
    job.WithOnRetry(func(idx int, nextDelay time.Duration, lastErr error) {
        callCount++
        fmt.Printf("Retry #%d scheduled in %v. Error: %v\n", idx, nextDelay, lastErr)
    }),
    // Callback khi không thể retry nữa
    job.WithOnPermanent(func(lastErr error) {
        fmt.Printf("Job failed permanently: %v\n", lastErr)
    }),
    // Callback khi thành công
    job.WithOnComplete(func() {
        fmt.Println("Job completed successfully!")
    }),
)

ctx := context.Background()
// RunWithRetry tự động retry nếu có lỗi
if err := job.RunWithRetry(ctx); err != nil {
    fmt.Printf("Final error: %v\n", err)
}

fmt.Printf("Total retries: %d\n", callCount)
```

### 1.3 Timeout và Context Cancellation

```go
// Timeout sẽ tự động được áp dụng
job := job.New(handler,
    job.WithTimeout(2 * time.Second),
)

ctx := context.Background()
job.Execute(ctx)

if job.State() == job.StateTimeout {
    fmt.Println("Job timed out!")
}

// Hoặc sử dụng context có deadline riêng
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

job.Execute(ctx)

if job.State() == job.StateFailed {
    fmt.Printf("Job failed with error: %v\n", job.LastError())
}
```

### 1.4 Jitter (Ngẫu Nhiên Hóa Delay)

```go
// Jitter giúp tránh thundering herd problem
job := job.New(handler,
    job.WithRetries([]time.Duration{
        1 * time.Second,
        2 * time.Second,
        4 * time.Second,
    }),
    job.WithJitter(0.2),  // ±20% jitter
)

// Nếu retry delay là 1s, nó sẽ nằm trong khoảng [800ms, 1200ms]
ctx := context.Background()
job.RunWithRetry(ctx)
```

### 1.5 State Transitions

Job có 6 states:

```
StateInit (0) 
    ↓
StateRunning (1)
    ├─→ StateCompleted (4) [thành công]
    ├─→ StateFailed (2) [lỗi, có thể retry]
    ├─→ StateTimeout (3) [quá timeout]
    └─→ StateRetryFailed (5) [retry hết cơ hội]
```

---

## Phần 2: Sử Dụng Hub Interface

Hub được sử dụng để quản lý nhiều job types và submit chúng vào worker pool.

### 2.1 Tạo Hub và Worker Pool

```go
import (
    "github.com/fcontext/job"
    "github.com/fcontext/worker" // Hoặc worker pool của bạn
)

// Giả sử có worker pool
pool := worker.NewPool(10) // 10 workers

// Tạo hub với submit function
hub := job.NewHub(func(j job.Job) bool {
    // Submit job vào pool
    return pool.Submit(j)
})
```

### 2.2 Tạo JobHandler

JobHandler là interface để wrap các loại xử lý khác nhau:

```go
// Email handler
type EmailJobHandler struct {
    To      string
    Subject string
    Body    string
}

func (h *EmailJobHandler) Handle(ctx context.Context) error {
    // Gửi email
    return sendEmail(ctx, h.To, h.Subject, h.Body)
}

func (h *EmailJobHandler) Type() string {
    return "email"
}

// Database handler
type DBJobHandler struct {
    Query string
    Args  []interface{}
}

func (h *DBJobHandler) Handle(ctx context.Context) error {
    // Chạy query
    return executeQuery(ctx, h.Query, h.Args...)
}

func (h *DBJobHandler) Type() string {
    return "database"
}
```

### 2.3 Tạo và Submit Job

```go
// Tạo email job
emailHandler := &EmailJobHandler{
    To:      "user@example.com",
    Subject: "Welcome",
    Body:    "Welcome to our service!",
}

emailJob, err := hub.Create("email", emailHandler,
    job.WithName("welcome-email"),
    job.WithTimeout(30 * time.Second),
    job.WithRetries([]time.Duration{
        1 * time.Second,
        5 * time.Second,
        1 * time.Minute,
    }),
    job.WithOnComplete(func() {
        log.Println("Email sent successfully")
    }),
    job.WithOnPermanent(func(lastErr error) {
        log.Printf("Failed to send email: %v", lastErr)
    }),
)

if err != nil {
    log.Fatalf("Failed to create job: %v", err)
}

// Submit job vào pool
if !hub.Submit(emailJob) {
    log.Println("Failed to submit job")
}
```

### 2.4 Lifecycle Management

```go
// Kiểm tra hub có đang chạy
if !hub.IsRunning() {
    log.Println("Hub is not running")
    return
}

// Dừng hub
ctx := context.Background()
if err := hub.Stop(ctx); err != nil {
    log.Printf("Failed to stop hub: %v", err)
}

// Sau khi stop, không thể tạo job mới
_, err := hub.Create("email", handler)
if err != nil {
    log.Printf("Cannot create job after stop: %v", err) // "hub is stopped"
}
```

---

## Pattern Sử Dụng Phổ Biến

### Pattern 1: Simple Fire-and-Forget

```go
handler := func(ctx context.Context) error {
    return myAsyncTask(ctx)
}

job := job.New(handler)

// Submit vào goroutine
go func() {
    ctx := context.Background()
    job.Execute(ctx)
}()
```

### Pattern 2: Critical Task with Heavy Retry

```go
handler := func(ctx context.Context) error {
    return criticalPaymentProcessing(ctx)
}

job := job.New(handler,
    job.WithTimeout(60 * time.Second),
    job.WithRetries([]time.Duration{
        1 * time.Second,
        5 * time.Second,
        10 * time.Second,
        30 * time.Second,
        1 * time.Minute,
    }),
    job.WithJitter(0.1),
    job.WithOnPermanent(func(err error) {
        // Alert ops
        alertOps("Payment processing failed", err)
    }),
)

ctx := context.Background()
if err := job.RunWithRetry(ctx); err != nil {
    log.Printf("Payment failed: %v", err)
}
```

### Pattern 3: Concurrent Job Submission

```go
jobs := []*job.Job{}

for i := 0; i < 1000; i++ {
    handler := &EmailJobHandler{
        To:      users[i].Email,
        Subject: "Newsletter",
    }

    j, _ := hub.Create("email", handler,
        job.WithTimeout(30 * time.Second),
    )
    jobs = append(jobs, j)
}

// Submit tất cả concurrently
for _, j := range jobs {
    hub.Submit(j)
}

// Worker pool sẽ xử lý theo thứ tự
```

### Pattern 4: Dynamic Retry Configuration

```go
job := job.New(handler,
    job.WithRetries([]time.Duration{1 * time.Second}),
)

// Thay đổi retry strategy sau khi tạo
if isNetworkUnstable() {
    job.SetRetries([]time.Duration{
        1 * time.Second,
        2 * time.Second,
        5 * time.Second,
        10 * time.Second,
    })
}

ctx := context.Background()
job.RunWithRetry(ctx)
```

---

## Best Practices

### 1. Timeout Management
- Luôn set timeout cho các external calls
- Timeout nên lớn hơn worst-case execution time
- Ví dụ: Database timeout (5s) + network buffer (2s) = Job timeout (10s)

### 2. Retry Strategy
- Không retry cho validation errors, chỉ retry cho transient errors
- Exponential backoff giúp tránh quá tải
- Sử dụng jitter để tránh thundering herd

### 3. Error Handling
- Log chi tiết mỗi lần retry
- Phân biệt giữa retriable vs permanent errors
- Implement fallback tại OnPermanent callback

### 4. Resource Management
- Set appropriate worker pool size
- Monitor job queue depth
- Gracefully shutdown hub trước khi thoát

### 5. Testing
- Mock handler để test retry logic
- Test timeout scenarios
- Test context cancellation

---

## Configuration Reference

| Option | Purpose | Example |
|--------|---------|---------|
| `WithName` | Job identifier | `WithName("send-email")` |
| `WithTimeout` | Execution timeout | `WithTimeout(30*time.Second)` |
| `WithRetries` | Retry schedule | `WithRetries([]time.Duration{1s, 5s})` |
| `WithJitter` | Random delay variation | `WithJitter(0.2)` |
| `WithOnRetry` | Retry callback | `WithOnRetry(func(idx, delay, err) {...})` |
| `WithOnComplete` | Success callback | `WithOnComplete(func() {...})` |
| `WithOnPermanent` | Final failure callback | `WithOnPermanent(func(err) {...})` |

---

## State Reference

| State | Value | Meaning |
|-------|-------|---------|
| `StateInit` | 0 | Job chưa được execute |
| `StateRunning` | 1 | Job đang chạy |
| `StateFailed` | 2 | Job thất bại lần đầu |
| `StateTimeout` | 3 | Job quá timeout |
| `StateCompleted` | 4 | Job thành công |
| `StateRetryFailed` | 5 | Job thất bại sau tất cả retry |

---

## Troubleshooting

### Job không được execute
- Kiểm tra pool có accept job không
- Kiểm tra hub có còn running không
- Kiểm tra handler function có nil không

### Retry không hoạt động
- Kiểm tra WithRetries có được set không
- Kiểm tra RunWithRetry() được gọi (không phải Execute())
- Kiểm tra handler có return error không

### Job luôn timeout
- Kiểm tra timeout value có quá ngắn không
- Kiểm tra external service có chậm không
- Tăng timeout hoặc optimize handler logic
