# Worker Package

**A high-performance worker pool for concurrent job execution in Go.**

## Overview

Package `worker` provides a thread pool implementation for executing jobs concurrently and efficiently. It's designed as a companion to the `job` package and integrates seamlessly with the `fcontext` framework.

### Key Features

- 🚀 **Controlled Concurrency** - Fixed number of worker goroutines
- 📦 **Queue-Based Distribution** - Buffered channel for job distribution
- 🛑 **Graceful Shutdown** - Properly drain queue before exit
- 📊 **Observability** - MetricsHook for monitoring
- 🔗 **Framework Integration** - Component & HubComponent for fcontext
- ⚡ **High Performance** - Low latency, high throughput

## Installation

```go
import "github.com/jackdes93/fcontext/worker"
```

## Quick Start

### 1. Basic Pool Usage

```go
package main

import (
    "context"
    "fmt"
    "time"
    
    "github.com/jackdes93/fcontext/job"
    "github.com/jackdes93/fcontext/worker"
)

func main() {
    // Create pool
    pool := worker.NewPool(nil, nil,
        worker.WithSize(4),           // 4 workers
        worker.WithQueueSize(256),    // queue size
    )
    
    // Start pool
    ctx, cancel := context.WithCancel(context.Background())
    go pool.Run(ctx)
    
    // Submit jobs
    for i := 0; i < 100; i++ {
        j := job.New(func(ctx context.Context) error {
            fmt.Println("Processing job")
            time.Sleep(100 * time.Millisecond)
            return nil
        })
        
        pool.Submit(j)
    }
    
    // Wait and shutdown
    time.Sleep(15 * time.Second)
    cancel()
    pool.Stop(ctx)
}
```

### 2. With Hub (Multiple Job Types)

```go
// Define handler
type EmailHandler struct {
    To   string
    Body string
}

func (h *EmailHandler) Handle(ctx context.Context) error {
    fmt.Printf("Sending email to %s\n", h.To)
    return nil
}

func (h *EmailHandler) Type() string {
    return "email"
}

// Use with Hub
func main() {
    hub := job.NewHub()
    hub.Register("email", func() job.Handler { return &EmailHandler{} })
    
    pool := worker.NewPool(nil, nil, worker.WithSize(8))
    go pool.Run(context.Background())
    
    // Create and submit
    j, _ := hub.Create("email", &EmailHandler{
        To:   "user@example.com",
        Body: "Hello",
    })
    
    pool.Submit(j)
}
```

### 3. Framework Integration

```go
// In your fcontext app setup
func setupWorkers(app *fcontext.App) error {
    component := worker.NewComponent("app-worker",
        metricsHook,
        worker.WithSize(16),
    )
    
    // Register component
    app.Use(component)
    
    return nil
}

// In handlers
func handleRequest(w http.ResponseWriter, r *http.Request) {
    comp := app.Component("app-worker").(*worker.Component)
    
    j := job.New(func(ctx context.Context) error {
        // async work
        return nil
    })
    
    comp.Submit(j)
    w.WriteJSON(map[string]string{"status": "queued"})
}
```

## Architecture

### Components

```
┌─────────────────────────────┐
│     Application             │
│  (HTTP Handlers, etc.)      │
└────────┬────────────────────┘
         │ Submit Job
         ↓
    ┌─────────────┐
    │  Pool Queue │ (buffered channel)
    └────────┬────────────────────┐
             │                    │
             ↓ Consume            ↓ Consume
        ┌──────────┐        ┌──────────┐
        │ Worker 1 │        │ Worker N │
        └────────┬─┘        └────┬─────┘
                 │               │
                 └───────┬───────┘
                         ↓
                   Job Execution
                   (with retry,
                    timeout,
                    callbacks)
```

### Interfaces

**Pool** - Core worker pool interface
```go
type Pool interface {
    Submit(j Job) bool                    // Queue job
    Run(ctx context.Context)              // Start workers (blocking)
    Stop(ctx context.Context) error       // Graceful shutdown
    IsRunning() bool                      // Check status
    Stats() PoolStats                     // Get metrics
}
```

**Component** - Framework integration
```go
type Component struct {
    pool    Pool
    hub     *job.JobHub
    // ...
}
```

**HubComponent** - Combined pool + hub
```go
type HubComponent struct {
    pool worker.Pool
    hub  job.Hub
    // ...
}
```

## Configuration

### Pool Options

```go
type PoolOption func(*pool)

// Available options
WithName(name string)                           // Pool identifier
WithSize(workers int)                           // Number of workers
WithQueueSize(size int)                         // Queue buffer size
WithStopTimeout(duration time.Duration)         // Graceful shutdown timeout
```

### Recommended Configurations

```go
// Light background tasks
worker.NewPool(logger, metrics,
    worker.WithSize(4),
    worker.WithQueueSize(256),
)

// API request processing
worker.NewPool(logger, metrics,
    worker.WithSize(16),
    worker.WithQueueSize(1024),
)

// High-throughput event processing
worker.NewPool(logger, metrics,
    worker.WithSize(32),
    worker.WithQueueSize(4096),
)

// Batch processing
worker.NewPool(logger, metrics,
    worker.WithSize(8),
    worker.WithQueueSize(2048),
)
```

## Monitoring

### MetricsHook

Implement `MetricsHook` to track job lifecycle:

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
    // collect histogram
}

func (m *MyMetrics) IncJobFailed(name string, err error, latency time.Duration) {
    m.jobsFailed.Add(1)
    // alert
}

func (m *MyMetrics) IncJobPermanentFailed(name string, err error) {
    // log for analysis
}

// Use it
metrics := &MyMetrics{}
pool := worker.NewPool(logger, metrics)
```

### Pool Stats

```go
stats := pool.Stats()

fmt.Printf("Workers: %d\n", stats.WorkerCount)      // Worker count
fmt.Printf("Queue: %d/%d\n", stats.QueueLen, stats.QueueSize)  // Current/max queue
fmt.Printf("Running: %v\n", stats.Running)          // Pool is active
fmt.Printf("Stopped: %v\n", stats.Stopped)          // Pool is stopped
```

## Patterns

### Pattern 1: Fire-and-Forget

```go
j := job.New(doWork)
pool.Submit(j)  // Don't wait for result
```

### Pattern 2: Rate Limiting

```go
// Pool automatically limits concurrency
pool := worker.NewPool(nil, nil,
    worker.WithSize(4),          // Max 4 concurrent
    worker.WithQueueSize(100),   // Max 100 queued
)
```

### Pattern 3: Batch Processing

```go
for _, record := range records {
    j := job.New(func(ctx context.Context) error {
        return processRecord(ctx, record)
    })
    pool.Submit(j)
}

// Wait for completion
for pool.Stats().QueueLen > 0 || pool.Stats().ActiveWorkers > 0 {
    time.Sleep(100 * time.Millisecond)
}
```

### Pattern 4: Timeout + Retry

```go
j := job.New(apiCall,
    job.WithTimeout(30 * time.Second),
    job.WithRetries([]time.Duration{
        1 * time.Second,
        5 * time.Second,
        10 * time.Second,
    }),
)
pool.Submit(j)
```

## Testing

### Testing with Pool

```go
func TestWithWorkerPool(t *testing.T) {
    pool := worker.NewPool(nil, nil, worker.WithSize(2))
    go pool.Run(context.Background())
    
    executed := make(chan struct{})
    j := job.New(func(ctx context.Context) error {
        executed <- struct{}{}
        return nil
    })
    
    pool.Submit(j)
    
    select {
    case <-executed:
        // Success
    case <-time.After(time.Second):
        t.Fatal("Job not executed")
    }
}
```

## Best Practices

### ✅ Do's

- ✓ Check `Submit()` return value
- ✓ Configure appropriate pool size
- ✓ Implement MetricsHook for observability
- ✓ Use graceful shutdown with timeout
- ✓ Monitor queue depth

### ❌ Don'ts

- ✗ Ignore Submit failures
- ✗ Create too many workers
- ✗ Make queue too small
- ✗ Submit jobs after Stop
- ✗ Forget to start pool with Run()

## Documentation

See complete guides:

- **[GUIDE.md](GUIDE.md)** - Detailed usage guide with patterns
- **[USECASE.md](USECASE.md)** - Real-world use cases with implementations
- **[examples_test.go](examples_test.go)** - 10+ runnable examples

## Performance Characteristics

### Throughput by Configuration

```
Light Load (4 workers):      ~500 jobs/sec
Medium Load (16 workers):    ~2000 jobs/sec
Heavy Load (32 workers):     ~5000 jobs/sec
```

### Latency Impact

```
Sequential Processing:  100ms × 100 jobs = 10 seconds
Pool (4 workers):       100ms / 4 ≈ 25ms average
Pool (16 workers):      100ms / 16 ≈ 6ms average
```

## Integration

### With fcontext Framework

```go
// Standalone
pool := worker.NewPool(logger, metrics)
go pool.Run(ctx)
pool.Submit(job)

// Via Component
component := worker.NewComponent("worker", metrics)
app.Use(component)

// Via HubComponent (multiple job types)
hubComponent := worker.NewHubComponent("hub", metrics)
app.Use(hubComponent)
hub := hubComponent.GetHub()
```

### With job Package

```go
// Jobs are executed by workers
j := job.New(handler,
    job.WithTimeout(30 * time.Second),
    job.WithRetries(retries),
)
pool.Submit(j)

// Job's retry, timeout, callbacks all work
// Worker pool just distributes across goroutines
```

## Related Packages

- **[job](../job/)** - Job scheduling with retry & timeout
- **[fcontext](../)** - Application framework

## FAQ

**Q: How many workers should I use?**
A: For I/O-bound: CPU_count × 2-4. For CPU-bound: CPU_count.

**Q: What if queue is full?**
A: `Submit()` returns `false`. Handle by retrying or discarding.

**Q: Can I resize pool after creation?**
A: Currently no. Create new pool if needed.

**Q: How long does graceful shutdown take?**
A: Configured timeout, after which workers are forcefully stopped.

**Q: Can I use without MetricsHook?**
A: Yes, pass `nil`. But you won't have observability.

**Q: What's the difference between Component and HubComponent?**
A: Component = simple pool. HubComponent = pool + hub for multiple job types.

## License

MIT

## Contributing

Contributions welcome! Please ensure tests pass and documentation is updated.

---

**Get started:** See [GUIDE.md](GUIDE.md) for detailed usage patterns!
