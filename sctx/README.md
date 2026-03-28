# sctx - Service Context Management

A lightweight Go package for managing service components with built-in support for dependency injection, lifecycle management, environment configuration, and graceful shutdown.

## Features

- **Component Lifecycle Management**: Define components with initialization and cleanup hooks
- **Dependency Injection**: Store and retrieve components by ID with type-safe access
- **Environment Configuration**: Support for .env files and flag-based configuration
- **Integrated Logging**: Built-in logging with zerolog
- **Graceful Shutdown**: Handle OS signals (SIGINT, SIGTERM) and cleanup components in reverse order
- **Order-based Initialization**: Control component startup order with the `Order()` method

## Quick Start

### Basic Usage

```go
package main

import (
	"context"
	"github.com/jackdes/fcontext/sctx"
)

func main() {
	// Create a new service context
	app := sctx.New(sctx.WithName("myapp"))

	// Run with graceful shutdown handling
	err := sctx.Run(app, func(ctx context.Context) error {
		logger := app.Logger("main")
		logger.Info("Application is running")
		// Your application logic here
		return nil
	})

	if err != nil {
		panic(err)
	}
}
```

### With Components

```go
type DatabaseComponent struct {
	db *sql.DB
}

func (c *DatabaseComponent) ID() string {
	return "database"
}

func (c *DatabaseComponent) InitFlags() {
	flag.String("db-url", "localhost:5432", "Database connection URL")
}

func (c *DatabaseComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	db, err := sql.Open("postgres", "your-connection-string")
	if err != nil {
		return err
	}
	c.db = db
	return nil
}

func (c *DatabaseComponent) Stop(ctx context.Context) error {
	return c.db.Close()
}

func (c *DatabaseComponent) Order() int {
	return 10 // Initialize early
}

// Usage
func main() {
	dbComponent := &DatabaseComponent{}
	app := sctx.New(
		sctx.WithName("myapp"),
		sctx.WithComponent(dbComponent),
	)

	err := sctx.Run(app, func(ctx context.Context) error {
		db, _ := sctx.GetAs[*sql.DB](app, "database")
		// Use database
		return nil
	})
}
```

## API Overview

### ServiceContext Interface

- `Load() error` - Initialize all components
- `MustGet(id string) any` - Get component by ID (panics if not found)
- `Get(id string) (any, bool)` - Get component by ID with existence check
- `Logger(prefix string) Logger` - Get a logger with prefix
- `EnvName() string` - Get current environment (dev|stg|prd)
- `GetName() string` - Get service name
- `Stop() error` - Shutdown all components
- `OutEnv()` - Print sample environment variables

### Component Interface

- `ID() string` - Unique component identifier
- `InitFlags()` - Define command-line flags
- `Activate(ctx context.Context, service ServiceContext) error` - Initialize component
- `Stop(ctx context.Context) error` - Cleanup component
- `Order() int` - Initialization priority (lower = earlier)

## Environment Configuration

### Environment Variables

- `APP_ENV`: Application environment (dev|stg|prd), default: dev
- `ENV_FILE`: Path to .env file, default: .env

### Flag Support

The `AppFlagSet` automatically maps command-line flags to environment variables:

```
--flag-name           → FLAG_NAME or [PREFIX]_FLAG_NAME
--app.config.path     → APP_CONFIG_PATH
```

## Logger Interface

Built-in logger with methods:

- `Debug(msg string, args ...any)`
- `Info(msg string, args ...any)`
- `Warn(msg string, args ...any)`
- `Error(msg string, args ...any)`
- `WithPrefix(prefix string) Logger`

## Type-Safe Component Access

```go
db, ok := sctx.GetAs[*sql.DB](app, "database")
if !ok {
	log.Fatal("database component not found")
}
```

## See Also

- [GUIDE.md](GUIDE.md) - Detailed implementation guide
- [USECASE.md](USECASE.md) - Real-world usage examples
