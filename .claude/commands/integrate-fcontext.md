# Integrate fcontext into a Go Project

Integrate the `github.com/jackdes93/fcontext` framework into the current Go project.

## What this skill does

Analyzes the current project, then wires in fcontext step by step:

1. Adds fcontext dependency via `go get`
2. Creates a `ServiceContext` with `sctx.New()`
3. Wraps `main()` with `sctx.Run()` for signal handling & graceful shutdown
4. Registers any existing infra (DB, HTTP server, etc.) as fcontext `Component`s
5. Optionally adds a `worker.HubComponent` for async job processing
6. Verifies the project compiles

---

## Instructions for Claude

When this skill is invoked:

### Step 0 — Understand the target project

Read `go.mod` to get the module name and Go version.
Scan the top-level directory for `main.go` (or `cmd/*/main.go`).
Identify what infrastructure already exists (database, HTTP server, Redis, message broker, etc.).

### Step 1 — Add fcontext dependency

Run:
```bash
go get github.com/jackdes93/fcontext@latest
```

If the project uses a `go.work`, run inside the correct module directory.

### Step 2 — Explain the three core packages to the user

Before writing any code, print a short summary:

```
fcontext có 3 package chính:

  sctx/   — ServiceContext: registry vòng đời cho toàn bộ component
  job/    — Job: async task với retry, timeout, jitter tự động
  worker/ — Pool + HubComponent: goroutine pool xử lý job

Luồng hoạt động:
  sctx.New(WithComponent(...)) → sctx.Run() → Activate → app logic → Stop
```

### Step 3 — Create or update main.go

Replace (or wrap) the existing `main()` with the fcontext pattern:

```go
package main

import (
    "context"

    "github.com/jackdes93/fcontext/sctx"
    // import your components
)

func newService() sctx.ServiceContext {
    return sctx.New(
        sctx.WithName("<project-name>"),
        // sctx.WithComponent(...),  ← register components here
    )
}

func main() {
    sv := newService()

    if err := sctx.Run(sv, func(ctx context.Context) error {
        // Application logic here.
        // ctx is cancelled on SIGINT / SIGTERM.
        <-ctx.Done()
        return nil
    }); err != nil {
        panic(err)
    }
}
```

Replace `<project-name>` with the module name from `go.mod`.

### Step 4 — Wrap existing infrastructure as Components

For **each** piece of infrastructure found in Step 0, create a Component file.
Follow this pattern exactly:

```go
package <package>

import (
    "context"

    "github.com/jackdes93/fcontext/sctx"
)

const <Name>ComponentID = "<kebab-case-id>"

type <Name>Component struct {
    // config fields (populated from flags or env vars)
    // resource fields (populated in Activate)
    log sctx.Logger
}

func New<Name>Component() *<Name>Component {
    return &<Name>Component{}
}

// ID returns the unique component identifier used by sctx.MustGet().
func (c *<Name>Component) ID() string { return <Name>ComponentID }

// Order controls activation sequence. Use these conventions:
//   10  — config / secrets loaders
//   20  — databases
//   30  — caches / message brokers
//   40  — worker pools  (worker.NewHubComponent defaults to 40)
//   50  — HTTP / gRPC servers
func (c *<Name>Component) Order() int { return <N> }

// InitFlags registers CLI flags. Flag name maps to env var automatically:
//   --db-host  →  DB_HOST
func (c *<Name>Component) InitFlags() {
    // flag.StringVar(&c.host, "db-host", "localhost", "Database host")
}

// Activate initialises the resource. Return non-nil error to abort startup
// and trigger automatic rollback of all previously activated components.
func (c *<Name>Component) Activate(ctx context.Context, sv sctx.ServiceContext) error {
    c.log = sv.Logger(c.ID())
    // TODO: open connection, run migrations, etc.
    c.log.Info().Msg("activated")
    return nil
}

// Stop releases resources. Called in reverse activation order on shutdown.
func (c *<Name>Component) Stop(_ context.Context) error {
    c.log.Info().Msg("stopped")
    // TODO: close connections
    return nil
}
```

Rules:
- One file per component, named `<name>_component.go`.
- Export a `Get<Name>(sv sctx.ServiceContext) *<Name>Component` helper for type-safe retrieval:
  ```go
  func Get<Name>(sv sctx.ServiceContext) *<Name>Component {
      return sv.MustGet(<Name>ComponentID).(*<Name>Component)
  }
  ```
- Register the component in `newService()` with `sctx.WithComponent(New<Name>Component())`.

### Step 5 — Add async job processing (optional, ask the user)

Ask: **"Dự án có cần xử lý async job không? (worker pool + job hub)"**

If yes, add a `worker.HubComponent`:

```go
import "github.com/jackdes93/fcontext/worker"

// In newService():
sctx.WithComponent(
    worker.NewHubComponent("job-hub", nil,   // nil = no metrics hook
        worker.WithSize(4),                   // goroutine count
        worker.WithQueueSize(1024),
    ),
),
```

Then show the user how to create and submit a job:

```go
import (
    "github.com/jackdes93/fcontext/job"
    "github.com/jackdes93/fcontext/worker"
)

// Inside sctx.Run callback:
hub := sv.MustGet("job-hub").(*worker.HubComponent)

myJob := job.New(
    func(ctx context.Context) error {
        // job logic
        return nil
    },
    job.WithName("my-job"),
    job.WithTimeout(30*time.Second),
    job.WithRetries([]time.Duration{1*time.Second, 5*time.Second, 30*time.Second}),
)

hub.Submit(myJob)
```

To define typed jobs via the Hub interface:

```go
// 1. Implement job.JobHandler
type MyHandler struct{}

func (h *MyHandler) Type() string { return "my-job-type" }
func (h *MyHandler) Handle(ctx context.Context) error {
    // logic
    return nil
}

// 2. Create via hub
jb, err := hub.GetHub().Create("my-job-type", &MyHandler{},
    job.WithTimeout(10*time.Second),
)
if err != nil { /* handle */ }
hub.Submit(jb)
```

### Step 6 — Environment & flags

Explain that fcontext reads `.env` automatically. Suggest creating `.env`:

```
APP_ENV=dev        # dev | stg | prd  (controls log format: colored vs JSON)
APP_NAME=<name>
```

Flag → env var mapping is automatic:
- `--db-host` → `DB_HOST`
- `--redis-addr` → `REDIS_ADDR`

### Step 7 — Verify compilation

Run:
```bash
go build ./...
```

Fix any import or type errors before reporting done.

### Step 8 — Report summary

Print a concise summary:

```
✔ fcontext tích hợp thành công

Components đã đăng ký:
  [order 10] config
  [order 20] database
  [order 50] http-server

Sử dụng:
  go run .                          # chạy service
  APP_ENV=prd go run .              # production mode (JSON logs)
  go run . --help                   # xem tất cả flags

Truy cập component từ bất kỳ đâu:
  db := sv.MustGet("database").(*DatabaseComponent)
```

---

## Key rules to follow

- **Never** import `github.com/jackdes93/fcontext` directly into component files — use `sctx`, `job`, `worker` sub-packages.
- **Always** use `sv.Logger(id)` inside `Activate` — do not create zerolog loggers manually.
- **Order values must not clash** — check existing components before assigning.
- **`sctx.Run` is the only entry point** — do not call `component.Activate` manually.
- If the project already has its own signal handling (`signal.NotifyContext`, `signal.Notify`), **remove it** — `sctx.Run` handles SIGINT/SIGTERM.
- If the project has no `go.mod`, tell the user to run `go mod init <module-name>` first.
