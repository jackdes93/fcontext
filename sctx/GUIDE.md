# sctx Implementation Guide

This guide provides detailed instructions for implementing and using the sctx service context management system.

## Table of Contents

1. [Creating Your First Component](#creating-your-first-component)
2. [Lifecycle Management](#lifecycle-management)
3. [Component Ordering](#component-ordering)
4. [Configuration Management](#configuration-management)
5. [Logger Usage](#logger-usage)
6. [Error Handling](#error-handling)
7. [Advanced Patterns](#advanced-patterns)

## Creating Your First Component

### Step 1: Define Your Component Structure

```go
type MyComponent struct {
	config string
	logger sctx.Logger
}
```

### Step 2: Implement the Component Interface

All components must implement the `Component` interface:

```go
type Component interface {
	ID() string
	InitFlags()
	Activate(ctx context.Context, service ServiceContext) error
	Stop(ctx context.Context) error
	Order() int
}
```

### Step 3: Implement Each Method

```go
func (m *MyComponent) ID() string {
	return "mycomponent"
}

func (m *MyComponent) InitFlags() {
	flag.String("my-config", "default", "My component configuration")
}

func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("MyComponent")
	logger.Info("Activating component")

	// Initialize your component
	m.logger = logger
	m.config = flag.Lookup("my-config").Value.String()

	return nil
}

func (m *MyComponent) Stop(ctx context.Context) error {
	m.logger.Info("Stopping component")
	// Cleanup resources
	return nil
}

func (m *MyComponent) Order() int {
	return 100 // Default order
}
```

## Lifecycle Management

### Component Initialization Order

Components are initialized in order based on their `Order()` return value:

```
Order 10  → Database (initializes first)
Order 50  → Cache
Order 100 → API Server (initializes last)
```

Lower values initialize first. This is crucial when components depend on others.

### Activation vs Initialization

1. **Activation** (`Activate`): Called during `app.Load()` to set up the component
2. **Deactivation** (`Stop`): Called during `app.Stop()` during shutdown

### Rollback on Failure

If a component fails to activate, all previously activated components are stopped in reverse order:

```go
// Component A (Order 10) ✓ Activated
// Component B (Order 50) ✗ Failed
// → Component A is stopped automatically
// → Error returned to caller
```

## Component Ordering

### Best Practices

1. **Dependencies First**: Components that others depend on should have lower Order values
2. **Explicit Values**: Use specific values (10, 20, 30) instead of defaults for clarity
3. **Document Dependencies**: Add comments explaining why components need specific order

### Example Order Structure

```go
const (
	OrderDatabase = 10    // Must be first
	OrderCache    = 20    // Depends on nothing
	OrderRedis    = 30    // Optional cache backend
	OrderAPI      = 100   // Depends on database
	OrderWebServer= 110   // Depends on API
)
```

## Configuration Management

### Flag-Based Configuration

```go
func (m *MyComponent) InitFlags() {
	flag.String("my-host", "localhost", "Component host")
	flag.Int("my-port", 8080, "Component port")
	flag.Duration("my-timeout", 30*time.Second, "Component timeout")
	flag.Bool("my-enabled", true, "Enable component")
}

func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	// Access flags
	host := flag.Lookup("my-host").Value.String()
	port := flag.Lookup("my-port").Value.String()
	
	return nil
}
```

### Environment Variable Mapping

Flag names are automatically mapped to environment variables:

```
Flag: --my-host          → Env: MY_HOST
Flag: --app.db.url       → Env: APP_DB_URL
Flag: --cache.redis.addr → Env: CACHE_REDIS_ADDR
```

### Using .env Files

Create a `.env` file in your project root:

```env
APP_ENV=dev
ENV_FILE=.env
MY_HOST=localhost
MY_PORT=8080
```

Or specify custom path:

```bash
./myapp -env-file /path/to/.env.production
```

Environment variables: `ENV_FILE=/path/to/.env.production`

## Logger Usage

### Getting a Logger

```go
// In component activation
logger := sv.Logger("ComponentName")

// Use logger
logger.Info("Starting component")
logger.Warn("Warning message")
logger.Error("Error occurred: %v", err)
logger.Debug("Debug information")
```

### Logger Behavior by Environment

- **Development** (dev, default): Colorized console output to stderr
- **Staging** (stg): Console output with timestamps
- **Production** (prd): JSON formatted output to stdout for log aggregation

### Log Levels

Go from least to most verbose:
1. `Error()` - Errors only
2. `Warn()` - Warnings and errors
3. `Info()` - General information
4. `Debug()` - Detailed debug info

## Error Handling

### Component Activation Errors

```go
func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	if err := m.validate(); err != nil {
		return fmt.Errorf("component validation failed: %w", err)
	}
	
	if err := m.connect(); err != nil {
		return fmt.Errorf("failed to connect: %w", err)
	}
	
	return nil
}
```

### Graceful Shutdown Error Handling

```go
func main() {
	app := sctx.New(/* ... */)
	
	err := sctx.Run(app, func(ctx context.Context) error {
		// Your application logic
		return nil
	})
	
	if err != nil {
		log.Printf("Application error: %v", err)
		os.Exit(1)
	}
}
```

## Advanced Patterns

### Pattern 1: Component Dependencies

```go
type APIComponent struct {
	db *sql.DB
}

func (a *APIComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	// Get dependency
	db, ok := sctx.GetAs[*sql.DB](sv, "database")
	if !ok {
		return fmt.Errorf("database component required")
	}
	a.db = db
	return nil
}

func (a *APIComponent) Order() int {
	return 100 // After database (Order 10)
}
```

### Pattern 2: Lazy Initialization

```go
type CacheComponent struct {
	pool *redis.Client
	err  error
	once sync.Once
}

func (c *CacheComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	c.once.Do(func() {
		c.pool, c.err = redis.NewClient().Connect(ctx)
	})
	return c.err
}
```

### Pattern 3: Feature Toggles

```go
func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	enabled := flag.Lookup("my-enabled").Value.String() == "true"
	if !enabled {
		sv.Logger("MyComponent").Info("Component disabled")
		return nil
	}
	// Proceed with initialization
	return nil
}
```

### Pattern 4: Configuration Validation

```go
func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("MyComponent")
	
	config := Config{
		Host:    flag.Lookup("my-host").Value.String(),
		Port:    flag.Lookup("my-port").Value.String(),
	}
	
	if err := config.Validate(); err != nil {
		logger.Error("Invalid configuration: %v", err)
		return fmt.Errorf("configuration validation failed: %w", err)
	}
	
	return nil
}
```

### Pattern 5: Signal Handling in Components

```go
func (m *MyComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	// ctx already has signal handling from sctx.Run()
	go func() {
		<-ctx.Done()
		m.logger.Info("Shutdown signal received")
	}()
	return nil
}
```

### Pattern 6: Component Chaining

```go
app := sctx.New(
	sctx.WithName("service"),
	sctx.WithComponent(&DatabaseComponent{}),
	sctx.WithComponent(&CacheComponent{}),
	sctx.WithComponent(&APIComponent{}),
	sctx.WithComponent(&ServerComponent{}),
)
```

Components will initialize in order of their `Order()` values.

## Testing Considerations

1. **Mock Components**: Create mock implementations for testing
2. **Dependency Injection**: Use options to inject test versions
3. **Context Cancellation**: Use context for timeouts during tests
4. **Isolation**: Create separate test services for each test

Example:

```go
func TestMyComponent(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	
	comp := &MyComponent{}
	sv := sctx.New(
		sctx.WithName("test"),
		sctx.WithComponent(comp),
	)
	
	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
	defer sv.Stop()
	
	// Test component
}
```
