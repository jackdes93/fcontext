# fcontext — Production-Grade Go Framework

A comprehensive Go framework for building scalable, maintainable service applications with built-in support for component lifecycle management, asynchronous job execution, and concurrent processing.

## 🎯 Overview

`fcontext` is a lightweight yet powerful framework designed to help developers build production-grade Go services by providing:

- **Component-Based Architecture** — Modular service composition with automatic lifecycle management
- **Asynchronous Job Execution** — Robust job framework with automatic retry, timeout, and state management
- **Worker Pool** — High-performance concurrent job processing with graceful shutdown
- **Framework Integration** — Seamless integration between components, jobs, and worker pools
- **Production-Ready** — Built-in logging (zerolog), error handling, and observability hooks

### Why fcontext?

Most Go services need similar boilerplate: managing multiple services/dependencies, handling graceful shutdown, executing async tasks, managing concurrent work. `fcontext` eliminates this repetition by providing:

1. **Single Source of Truth** — Centralized service context for all components
2. **Lifecycle Safety** — Automatic activation order, error rollback, and coordinated shutdown
3. **Built-in Patterns** — Job retry, timeout, worker pool — all battle-tested patterns, ready to use
4. **Minimal Dependencies** — Only `zerolog` for logging and `godotenv` for .env support
5. **Developer Experience** — Clear abstractions, comprehensive examples, detailed documentation

---

## 🚀 Quick Start

### Installation

```bash
go get github.com/jackdes93/fcontext@latest
```

**Requirements:** Go 1.22 or higher

### 30-Second Example

```go
package main

import (
	"context"
	"log"
	"time"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
	"github.com/jackdes93/fcontext/worker"
)

func main() {
	// Create service
	service := fcontext.New(
		fcontext.WithName("MyService"),
	)

	// Run service
	if err := fcontext.Run(service, func(ctx context.Context) error {
		// Create worker pool
		pool := worker.NewPool(nil, nil, worker.WithSize(4))
		go pool.Run(ctx)

		// Create and submit jobs
		for i := 0; i < 10; i++ {
			j := job.New(func(ctx context.Context) error {
				log.Println("Processing job...")
				return nil
			}, job.WithTimeout(5*time.Second))
			pool.Submit(j)
		}

		<-ctx.Done()
		return nil
	}); err != nil {
		panic(err)
	}
}
```

---

## 📦 Core Packages

### 1. **sctx** — Service Context & Component Management

Provides the foundation for service lifecycle management.

**Key Concepts:**
- `ServiceContext` — Central registry for all service components
- `Component` — An interface with lifecycle hooks (Activate, Stop)
- Automatic activation/deactivation in correct order
- Built-in logging and environment configuration

**Features:**
- Automatic component ordering by `Order()`
- Error rollback on activation failure
- Environment management (dev/stg/prd)
- `.env` file support
- Signal handling (SIGINT/SIGTERM)

**Example:**
```go
// Define a component
type DatabaseComponent struct{}

func (d *DatabaseComponent) ID() string           { return "db" }
func (d *DatabaseComponent) InitFlags()           {}
func (d *DatabaseComponent) Order() int           { return 10 }
func (d *DatabaseComponent) Activate(ctx context.Context, svc fcontext.ServiceContext) error {
	// Initialize database connection
	return nil
}
func (d *DatabaseComponent) Stop(ctx context.Context) error {
	// Close database connection
	return nil
}

// Use in service
service := fcontext.New(
	fcontext.WithName("MyAPI"),
	fcontext.WithComponent(&DatabaseComponent{}),
)
```

### 2. **job** — Asynchronous Job Execution Framework

Production-ready job execution with automatic retry, timeout, and state management.

**Key Features:**
- ✅ Automatic retry with exponential backoff and jitter
- ⏱️ Timeout management (prevent infinite execution)
- 🔄 6 distinct job states (Init, Running, Failed, Timeout, Completed, RetryFailed)
- 📊 Hub pattern for managing multiple job types
- 🎯 Callback lifecycle (OnRetry, OnComplete, OnPermanent)
- 🛡️ Thread-safe operations

**Job States:**
```
Init → Running → {Completed, Failed, Timeout}
                     ↓
                  {Retry Failed / Permanent Failed}
```

**Example:**
```go
// Create a job with retry and timeout
j := job.New(
	func(ctx context.Context) error {
		// Your async work here
		return nil
	},
	job.WithTimeout(30 * time.Second),
	job.WithRetries([]time.Duration{
		1 * time.Second,
		5 * time.Second,
		30 * time.Second,
	}),
	job.WithJitter(0.1), // ±10% jitter
	job.WithOnRetry(func(idx int, nextDelay time.Duration, err error) {
		log.Printf("Retry %d in %v: %v", idx, nextDelay, err)
	}),
)

// Execute with automatic retry
if err := j.RunWithRetry(context.Background()); err != nil {
	log.Printf("Job permanently failed: %v", err)
}
```

### 3. **worker** — Concurrent Job Processing Pool

High-performance worker pool for distributed job execution.

**Key Features:**
- 🚀 Controlled concurrency (fixed worker count)
- 📦 Queue-based job distribution
- 🛑 Graceful shutdown with queue draining
- 📊 MetricsHook for observability
- 🔗 Component & HubComponent for framework integration
- ⚡ Low latency, high throughput

**Example:**
```go
// Create worker pool
pool := worker.NewPool(logger, metricsHook,
	worker.WithSize(8),           // 8 concurrent workers
	worker.WithQueueSize(1024),   // queue capacity
	worker.WithStopTimeout(30*time.Second),
)

// Start pool
ctx := context.Background()
go pool.Run(ctx)

// Submit jobs
for i := 0; i < 100; i++ {
	j := job.New(handler)
	if !pool.Submit(j) {
		log.Println("Queue full, job rejected")
	}
}

// Graceful shutdown
pool.Stop(context.Background())
```

---

## 🏗️ Architecture & Design Philosophy

### Component-Driven Design

```
Service Context
├── Logger Component
├── Database Component
├── HTTP Server Component
├── Cache Component
└── Worker Pool Component (with Job Hub)
    ├── Job 1 (Executing)
    ├── Job 2 (Queued)
    └── Job 3 (Queued)
```

### Design Principles

1. **Composition Over Inheritance** — Build services by composing components
2. **Separation of Concerns** — Each component handles one responsibility
3. **Ordered Activation** — Dependencies activate in the correct order
4. **Error Safety** — Automatic rollback on activation failure
5. **Graceful Degradation** — Coordinated shutdown with error collection
6. **Production-Ready** — Battle-tested patterns for real-world scenarios

### Lifecycle Flow

```
┌─────────────────────────────────────┐
│  Service Creation                   │
│  (fcontext.New())                   │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│  Parse Flags & Load Config          │
│  (.env file support)                │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│  Component Activation (by Order)    │
│  Component.Activate(ctx)            │
│  On error: Rollback all             │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│  Application Main Logic             │
│  (fcontext.Run callback)            │
│  Listen for SIGINT/SIGTERM          │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│  Component Shutdown (Reverse Order) │
│  Component.Stop(ctx)                │
│  Collect errors                     │
└────────────┬────────────────────────┘
             │
             ▼
┌─────────────────────────────────────┐
│  Service Termination                │
└─────────────────────────────────────┘
```

---

## 📋 Project Structure

```
fcontext/
├── go.mod, go.sum              # Module definition
├── README.md                   # This file
│
├── sctx/                       # Service Context & Components
│   ├── context.go              # ServiceContext interface
│   ├── component.go            # Component interface
│   ├── logger.go               # Logger abstraction
│   ├── run.go                  # Application runner
│   ├── flags.go                # Flag management
│   ├── options.go              # Service options
│   └── ...
│
├── job/                        # Async Job Execution
│   ├── types.go                # Job interface & implementation
│   ├── hub.go                  # Job hub for multiple types
│   ├── types_test.go           # Job tests
│   ├── hub_test.go             # Hub tests
│   ├── examples_test.go        # Usage examples
│   ├── GUIDE.md                # Detailed usage guide
│   ├── USECASE.md              # Real-world use cases
│   └── README.md               # Job package documentation
│
├── worker/                     # Worker Pool
│   ├── pool.go                 # Pool interface & implementation
│   ├── component.go            # Pool component for fcontext
│   ├── hub_component.go        # Pool + Job hub integration
│   ├── pool_test.go            # Pool tests
│   ├── component_test.go       # Component tests
│   ├── examples_test.go        # Usage examples
│   ├── GUIDE.md                # Detailed usage guide
│   ├── USECASE.md              # Real-world use cases
│   └── README.md               # Worker package documentation
│
└── examples/                   # Production Examples
    ├── http-server/            # Gin HTTP API with worker pool
    │   ├── main.go
    │   ├── hub_example.go
    │   └── http-server/        # Gin integration
    │       └── middleware/     # CORS, logger, recovery, etc.
    │
    └── mqtt-forwarder/         # MQTT → Redis/PostgreSQL
        ├── main.go
        ├── api.go              # REST API
        ├── job_handler.go      # Job handlers
        ├── plugins/
        │   ├── mqtt/          # MQTT component
        │   └── storage/       # Redis & PostgreSQL components
        ├── Dockerfile
        └── docker-compose.yml  # Complete setup
```

---

## 📚 Documentation

### Getting Started
- **[sctx Guide](sctx/)** — Service context and component management
- **[job Guide](job/GUIDE.md)** — Job creation and retry patterns
- **[worker Guide](worker/GUIDE.md)** — Worker pool and concurrency

### Real-World Scenarios
- **[job Use Cases](job/USECASE.md)** — Email queues, database ops, batch processing
- **[worker Use Cases](worker/USECASE.md)** — Distributed processing, rate limiting, monitoring

### Examples
- **[HTTP Server Example](examples/http-server/)** — Gin API with middleware and worker pool
- **[MQTT Forwarder Example](examples/mqtt-forwarder/)** — Complete microservice with storage backends

---

## 💡 Common Patterns

### Pattern 1: Simple Async Task Queue
```go
pool := worker.NewPool(log, nil, worker.WithSize(4))
go pool.Run(ctx)

// Submit tasks
pool.Submit(job.New(handleEmail))
pool.Submit(job.New(processPayment))
```

### Pattern 2: Multiple Job Types with Hub
```go
hub := job.NewHub(pool.Submit)

emailJob, _ := hub.Create("email", emailHandler, job.WithTimeout(30*time.Second))
webhookJob, _ := hub.Create("webhook", webhookHandler, job.WithRetries(retrySchedule))

hub.Submit(emailJob)
hub.Submit(webhookJob)
```

### Pattern 3: Service with Component Stack
```go
service := fcontext.New(
	fcontext.WithName("MyService"),
	fcontext.WithComponent(&db.PostgresComponent{}),
	fcontext.WithComponent(&cache.RedisComponent{}),
	fcontext.WithComponent(&api.GinServerComponent{}),
	fcontext.WithComponent(worker.NewHubComponent(pool)),
)
```

---

## ⚙️ Configuration

### Environment Variables
```bash
# Application environment
APP_ENV=dev|stg|prd

# Environment file (auto-loaded)
ENV_FILE=.env.local

# Custom .env file
ENV_FILE=/etc/app/.env
```

### Example .env
```env
APP_ENV=dev
DB_HOST=localhost
DB_PORT=5432
LOG_LEVEL=debug
WORKER_SIZE=8
QUEUE_SIZE=1024
```

---

## 🔌 Integration Examples

### With HTTP Server (Gin)
```go
service := fcontext.New(
	fcontext.WithComponent(worker.NewHubComponent(pool)),
	fcontext.WithComponent(ginServerComponent),
)

// In Gin handler:
handler := func(c *gin.Context) {
	job := job.New(processRequest)
	pool.Submit(job)
	c.JSON(200, "Processing...")
}
```

### With Message Queue (MQTT)
```go
// Message → Job → Worker Pool → Storage
mqttComponent.OnMessage = func(topic string, payload []byte) {
	j := job.New(func(ctx context.Context) error {
		return storage.Save(ctx, topic, payload)
	})
	pool.Submit(j)
}
```

---

## 🎯 Use Cases

- **Email/Notification Queue** — Send emails/SMS asynchronously with retry
- **Payment Processing** — Async payment handling with timeout and retry
- **Data Pipeline** — ETL jobs with worker pool for parallel processing
- **Webhook Delivery** — Reliable webhook callbacks with exponential backoff
- **Batch Operations** — Large-scale data processing without blocking
- **Real-time Aggregation** — MQTT/Kafka consumers → job queue → storage
- **Image Processing** — Heavy compute tasks with worker pool
- **API Rate Limiting** — Queue and process requests at controlled rate

---

## 📊 Observability

### Logging
```go
logger := service.Logger("mycomponent")
logger.Info("message", "key", "value")
logger.Error("error occurred", "err", err)
```

### Metrics
```go
type MyMetrics struct{}

func (m *MyMetrics) IncJobStarted(name string) {
	metrics.JobCounter.WithLabelValues(name, "started").Inc()
}

func (m *MyMetrics) IncJobSuccess(name string, latency time.Duration) {
	metrics.JobLatency.WithLabelValues(name).Observe(latency.Seconds())
}

pool := worker.NewPool(logger, myMetrics)
```

---

## 🔧 Development

### Running Tests
```bash
go test ./... -v
```

### Running Examples
```bash
cd examples/http-server
go run main.go

cd examples/mqtt-forwarder
docker-compose up
go run main.go
```

---

## 📋 Requirements

- **Go:** 1.22 or higher
- **Dependencies:**
  - `github.com/rs/zerolog` — Structured logging
  - `github.com/joho/godotenv` — .env file support

---

## 📝 License

MIT License — See LICENSE file for details

---

## 🤝 Contributing

Contributions are welcome! Please feel free to submit a Pull Request.

### Guidelines
1. Fork the repository
2. Create your feature branch
3. Add tests for new functionality
4. Update documentation
5. Submit a pull request

---

## 📞 Support

For questions and discussion:
- Open an issue on GitHub
- Check existing documentation in `GUIDE.md` and `USECASE.md` files
- Review examples in `/examples` directory

---

**Built with ❤️ for production-grade Go applications**