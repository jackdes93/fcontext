# Worker Package - Usage Guide

## Tổng Quan

Package `worker` cung cấp một **worker pool** (thread pool) để xử lý **jobs** một cách concurrent và hiệu quả. Nó tích hợp chặt chẽ với `job` package và framework `fcontext`.

**Worker Pool** bản chất là:
- Một tập hợp goroutines (workers) chạy nền
- Một hàng đợi (queue) chứa các job cần xử lý
- Tự động phân phối job cho workers
- Kiểm soát concurrency (số worker cố định)

---

## Cấu Trúc Cơ Bản

### Các Component Chính

1. **Pool** - Interface cho worker pool
   - `Submit(job) bool` - Gửi job vào queue
   - `Run(ctx)` - Chạy workers
   - `Stop(ctx)` - Dừng pool gracefully

2. **Component** - Tích hợp pool vào fcontext framework
   - Quản lý lifecycle
   - Simple job submission

3. **HubComponent** - Pool + Job Hub
   - Quản lý nhiều job types
   - Tạo job từ handlers

4. **MetricsHook** - Callback cho monitoring
   - Track job lifecycle events
   - Collect metrics

---

## Phần 1: Cơ Bản - Pool

### 1.1 Tạo Worker Pool

```go
import "github.com/jackdes93/fcontext/worker"

// Tạo pool với mặc định (4 workers, 1024 queue size)
pool := worker.NewPool(logger, metrics)

// Hoặc với custom options
pool := worker.NewPool(logger, metrics,
    worker.WithName("api-worker"),
    worker.WithSize(8),           // 8 workers
    worker.WithQueueSize(2048),   // queue size
    worker.WithStopTimeout(30 * time.Second),
)
```

### 1.2 Submit Job

```go
// Tạo job
handler := func(ctx context.Context) error {
    // Làm việc gì đó
    return nil
}

j := job.New(handler,
    job.WithTimeout(30 * time.Second),
    job.WithRetries([]time.Duration{1*time.Second, 5*time.Second}),
)

// Submit vào pool
ok := pool.Submit(j)
if !ok {
    log.Println("Queue full or pool stopped")
}
```

### 1.3 Chạy Pool

```go
ctx, cancel := context.WithCancel(context.Background())
defer cancel()

// Chạy pool (blocking)
go pool.Run(ctx)

// Pool sẽ xử lý jobs đến khi context cancelled

// Dừng pool
cancel()
pool.Stop(ctx)  // Graceful shutdown
```

### 1.4 Kiểm Tra Trạng Thái

```go
// Kiểm tra pool có chạy không
if pool.IsRunning() {
    fmt.Println("Pool is active")
}

// Lấy statistics
stats := pool.Stats()
fmt.Printf("Workers: %d, Queue: %d/%d\n",
    stats.WorkerCount, stats.QueueLen, stats.QueueSize)
```

---

## Phần 2: Integration - Component

### 2.1 Tích Hợp vào Application

```go
// Tạo component
workerComponent := worker.NewComponent("app-worker",
    metrics,  // MetricsHook
    worker.WithSize(8),
)

// Khởi động (Activate)
err := workerComponent.Activate(ctx, serviceContext)

// Shutdown (Stop)
workerComponent.Stop(ctx)
```

### 2.2 Submit Job qua Component

```go
j := job.New(handler)

ok := workerComponent.Submit(j)
if !ok {
    log.Println("Failed to submit job")
}
```

---

## Phần 3: Hub - Multiple Job Types

### 3.1 Tạo HubComponent

```go
hubComponent := worker.NewHubComponent("app-hub",
    metrics,
    worker.WithSize(16),  // 16 workers cho hub
)

hubComponent.Activate(ctx, serviceContext)
```

### 3.2 Tạo JobHandler

```go
// Email handler
type EmailHandler struct {
    To   string
    Body string
}

func (h *EmailHandler) Handle(ctx context.Context) error {
    return sendEmail(ctx, h.To, h.Body)
}

func (h *EmailHandler) Type() string {
    return "email"
}
```

### 3.3 Tạo và Submit Job

```go
hub := hubComponent.GetHub()

// Tạo job từ handler
emailJob, err := hub.Create("email", 
    &EmailHandler{
        To: "user@example.com",
        Body: "Hello",
    },
    job.WithTimeout(30 * time.Second),
)

if err != nil {
    log.Fatalf("Create job failed: %v", err)
}

// Submit
ok := hubComponent.Submit(emailJob)
if !ok {
    log.Println("Submit failed")
}
```

---

## Phần 4: Monitoring & Metrics

### 4.1 Implement MetricsHook

```go
type MyMetrics struct {
    jobsStarted   atomic.Int64
    jobsSucceeded atomic.Int64
    jobsFailed    atomic.Int64
}

func (m *MyMetrics) IncJobStarted(name string) {
    m.jobsStarted.Add(1)
}

func (m *MyMetrics) IncJobSuccess(name string, latency time.Duration) {
    m.jobsSucceeded.Add(1)
    // Track latency
}

func (m *MyMetrics) IncJobFailed(name string, err error, latency time.Duration) {
    m.jobsFailed.Add(1)
    // Log error
}

func (m *MyMetrics) IncJobPermanentFailed(name string, err error) {
    // Alert ops
}
```

### 4.2 Track Latency

```go
// MetricsHook sẽ được gọi với latency
// Bạn có thể track histogram:

func (m *MyMetrics) IncJobSuccess(name string, latency time.Duration) {
    metrics.JobLatency.Observe(latency.Seconds())
}
```

---

## Patterns Phổ Biến

### Pattern 1: Simple Async Work

```go
pool := worker.NewPool(logger, metrics, worker.WithSize(4))
go pool.Run(ctx)

// Submit work
for i := 0; i < 100; i++ {
    j := job.New(doWork)
    pool.Submit(j)
}
```

### Pattern 2: Async API Response

```go
// HTTP handler
func handleRequest(w http.ResponseWriter, r *http.Request) {
    // Tạo job trong background
    j := job.New(func(ctx context.Context) error {
        return processRequest(ctx, r)
    })
    
    pool.Submit(j)
    
    // Response ngay
    w.WriteJSON(map[string]string{"status": "processing"})
}
```

### Pattern 3: Rate Limiting

```go
// Pool tự động control concurrency via queue size
pool := worker.NewPool(logger, metrics,
    worker.WithSize(4),           // Max 4 concurrent
    worker.WithQueueSize(100),    // Max 100 queued
)

// Submit sẽ return false nếu queue full
for _, job := range jobs {
    for !pool.Submit(job) {
        // Wait and retry
        time.Sleep(100 * time.Millisecond)
    }
}
```

### Pattern 4: Hub với Multiple Types

```go
hub := hubComponent.GetHub()

// Email
emailJob, _ := hub.Create("email", emailHandler)
pool.Submit(emailJob)

// Database
dbJob, _ := hub.Create("database", dbHandler)
pool.Submit(dbJob)

// Notification
notifJob, _ := hub.Create("notification", notifHandler)
pool.Submit(notifJob)

// Tất cả được xử lý qua cùng 1 pool
```

---

## Configuration Options

### PoolOption Functions

| Option | Purpose | Example |
|--------|---------|---------|
| `WithName` | Pool identifier | `WithName("api-worker")` |
| `WithSize` | Number of workers | `WithSize(8)` |
| `WithQueueSize` | Queue buffer size | `WithQueueSize(2048)` |
| `WithStopTimeout` | Graceful shutdown timeout | `WithStopTimeout(30*time.Second)` |

### Recommended Values

```go
// Light load (background tasks)
worker.WithSize(4)
worker.WithQueueSize(256)

// Medium load (API workers)
worker.WithSize(16)
worker.WithQueueSize(1024)

// Heavy load (batch processing)
worker.WithSize(32)
worker.WithQueueSize(4096)
```

---

## Best Practices

### ✅ Do's

```go
// ✓ Configure pool size based on workload
// I/O bound: Size = num_CPUs * 2-4
// CPU bound: Size = num_CPUs
pool.WithSize(runtime.NumCPU() * 2)

// ✓ Always check Submit result
ok := pool.Submit(job)
if !ok {
    // Handle queue full
}

// ✓ Use appropriate timeout for Stop
pool.WithStopTimeout(30 * time.Second)

// ✓ Implement MetricsHook for observability
pool := worker.NewPool(logger, myMetrics)

// ✓ Graceful shutdown
cancel()
pool.Stop(ctx)
```

### ❌ Don'ts

```go
// ✗ Don't ignore Submit failure
pool.Submit(job)  // Ignoring return value!

// ✗ Don't make workers too many
worker.WithSize(1000)  // Will crash!

// ✗ Don't make queue too small
worker.WithQueueSize(1)  // Will drop all jobs!

// ✗ Don't forget to start pool
pool := worker.NewPool(...)
// Missing: go pool.Run(ctx)

// ✗ Don't submit jobs after Stop
pool.Stop(ctx)
pool.Submit(job)  // Will fail silently
```

---

## Troubleshooting

| Problem | Solution |
|---------|----------|
| Jobs not executing | Check `pool.Run()` is called; `IsRunning()` returns true |
| "queue full" warnings | Increase queue size or increase number of workers |
| High memory usage | Reduce queue size; monitor job count |
| Slow execution | Check job latency; increase workers if I/O bound |
| Jobs drop on shutdown | Increase `StopTimeout` |
| Metrics not collected | Implement all MetricsHook methods |

---

## Pool Lifecycle

```
Creation (NewPool)
       ↓
Activation (Run) ← Submit jobs here (Submit returns false before)
       ↓
Processing (workers consuming)
       ↓
Shutdown Signal (context cancel)
       ↓
Graceful Stop (close queue, drain jobs)
       ↓
Done (all workers exit)
```

---

## Integration with fcontext

### Framework Integration

```go
// In your application startup
hubComponent := worker.NewHubComponent("worker",
    metrics,
    worker.WithSize(16),
)

// Register with fcontext
app.Use(hubComponent)

// In handlers
func setupApp(app *fcontext.App) {
    hub := app.Component("worker").(*worker.HubComponent).GetHub()
    
    // Now use hub to create/submit jobs
    job, _ := hub.Create("email", handler)
    hub.Submit(job)
}
```

---

## Summary

| Aspect | Detail |
|--------|--------|
| **Purpose** | Execute jobs concurrently with controlled goroutines |
| **Use Case** | Background tasks, async processing, rate limiting |
| **Performance** | High throughput, low latency, predictable resources |
| **Integration** | Component, Hub models for framework integration |
| **Monitoring** | MetricsHook for metrics, stats for pool info |

**Worker Package enables**:
- ✅ Controlled concurrency
- ✅ Queue-based job distribution  
- ✅ Graceful shutdown
- ✅ Observable processing
- ✅ Framework integration (fcontext)
