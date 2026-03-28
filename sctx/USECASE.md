# sctx Use Cases and Examples

This document demonstrates real-world use cases and patterns for using the sctx service context management system.

## Use Case 1: Microservice with Database and Cache

### Scenario
A simple user service that needs:
- PostgreSQL database connection
- Redis cache
- RESTful API server
- Graceful shutdown handling

### Implementation

```go
package main

import (
	"context"
	"flag"
	"fmt"
	"database/sql"
	"github.com/jackdes/fcontext/sctx"
	"github.com/redis/go-redis/v9"
	_ "github.com/lib/pq"
)

// DatabaseComponent manages PostgreSQL connection
type DatabaseComponent struct {
	db *sql.DB
}

func (d *DatabaseComponent) ID() string { return "database" }

func (d *DatabaseComponent) InitFlags() {
	flag.String("db-url", "postgres://localhost/myapp", "Database URL")
	flag.Int("db-pool-size", 10, "Connection pool size")
}

func (d *DatabaseComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("Database")
	
	dbURL := flag.Lookup("db-url").Value.String()
	db, err := sql.Open("postgres", dbURL)
	if err != nil {
		logger.Error("Failed to open database: %v", err)
		return err
	}
	
	if err := db.PingContext(ctx); err != nil {
		logger.Error("Failed to ping database: %v", err)
		return err
	}
	
	d.db = db
	logger.Info("Database connected successfully")
	return nil
}

func (d *DatabaseComponent) Stop(ctx context.Context) error {
	if d.db != nil {
		return d.db.Close()
	}
	return nil
}

func (d *DatabaseComponent) Order() int {
	return 10 // Initialize first
}

// RedisComponent manages Redis cache connection
type RedisComponent struct {
	client *redis.Client
}

func (r *RedisComponent) ID() string { return "redis" }

func (r *RedisComponent) InitFlags() {
	flag.String("redis-addr", "localhost:6379", "Redis address")
	flag.String("redis-password", "", "Redis password")
}

func (r *RedisComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("Redis")
	
	addr := flag.Lookup("redis-addr").Value.String()
	password := flag.Lookup("redis-password").Value.String()
	
	client := redis.NewClient(&redis.Options{
		Addr:     addr,
		Password: password,
	})
	
	if err := client.Ping(ctx).Err(); err != nil {
		logger.Error("Failed to connect to Redis: %v", err)
		return err
	}
	
	r.client = client
	logger.Info("Redis connected successfully")
	return nil
}

func (r *RedisComponent) Stop(ctx context.Context) error {
	if r.client != nil {
		return r.client.Close()
	}
	return nil
}

func (r *RedisComponent) Order() int {
	return 20 // After database
}

// APIComponent manages REST API endpoints
type APIComponent struct {
	db    *sql.DB
	cache *redis.Client
}

func (a *APIComponent) ID() string { return "api" }

func (a *APIComponent) InitFlags() {
	flag.String("api-port", "8080", "API port")
}

func (a *APIComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("API")
	
	// Get dependencies
	db, ok := sctx.GetAs[*sql.DB](sv, "database")
	if !ok {
		return fmt.Errorf("database component required")
	}
	
	cache, ok := sctx.GetAs[*redis.Client](sv, "redis")
	if !ok {
		return fmt.Errorf("redis component required")
	}
	
	a.db = db
	a.cache = cache
	
	logger.Info("API initialized successfully")
	return nil
}

func (a *APIComponent) Stop(ctx context.Context) error {
	return nil
}

func (a *APIComponent) Order() int {
	return 100 // After database and cache
}

// Main application
func main() {
	app := sctx.New(
		sctx.WithName("user-service"),
		sctx.WithComponent(&DatabaseComponent{}),
		sctx.WithComponent(&RedisComponent{}),
		sctx.WithComponent(&APIComponent{}),
	)
	
	err := sctx.Run(app, func(ctx context.Context) error {
		logger := app.Logger("main")
		logger.Info("User service started")
		
		// Your application logic here
		// The context will be cancelled on SIGINT or SIGTERM
		<-ctx.Done()
		
		return nil
	})
	
	if err != nil {
		panic(err)
	}
}
```

## Use Case 2: Multi-Environment Configuration

### Scenario
Application that behaves differently based on environment (dev/stg/prd)

### .env File (Development)
```env
APP_ENV=dev
ENV_FILE=.env
DB_HOST=localhost
DB_PORT=5432
CACHE_ENABLED=true
LOG_LEVEL=debug
```

### .env.production File
```env
APP_ENV=prd
DB_HOST=prod-db.example.com
DB_PORT=5432
CACHE_ENABLED=true
LOG_LEVEL=info
```

### Implementation
```go
type ConfigComponent struct{}

func (c *ConfigComponent) ID() string { return "config" }

func (c *ConfigComponent) InitFlags() {
	flag.String("db-host", "localhost", "Database host")
	flag.String("db-port", "5432", "Database port")
	flag.Bool("cache-enabled", true, "Enable caching")
}

func (c *ConfigComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	logger := sv.Logger("Config")
	
	env := sv.EnvName()
	logger.Info("Loading configuration for environment: %s", env)
	
	switch env {
	case "prd":
		logger.Info("Production configuration loaded")
	case "stg":
		logger.Info("Staging configuration loaded")
	default:
		logger.Info("Development configuration loaded")
	}
	
	return nil
}

func (c *ConfigComponent) Stop(ctx context.Context) error { return nil }
func (c *ConfigComponent) Order() int { return 1 } // Load config first

// Run with:
// ./app -app-env prd -env-file .env.production
// or
// ENV_FILE=.env.production APP_ENV=prd ./app
```

## Use Case 3: Plugin Architecture

### Scenario
Application with optional plugin components that are conditionally loaded

```go
type PluginComponent interface {
	sctx.Component
	IsEnabled() bool
}

type AnalyticsPlugin struct {
	enabled bool
}

func (p *AnalyticsPlugin) ID() string { return "analytics" }

func (p *AnalyticsPlugin) InitFlags() {
	flag.Bool("analytics-enabled", false, "Enable analytics")
}

func (p *AnalyticsPlugin) IsEnabled() bool { return p.enabled }

func (p *AnalyticsPlugin) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	p.enabled = flag.Lookup("analytics-enabled").Value.(flag.Value).String() == "true"
	
	if !p.enabled {
		sv.Logger("Analytics").Info("Analytics plugin disabled")
		return nil
	}
	
	sv.Logger("Analytics").Info("Analytics plugin enabled")
	return nil
}

func (p *AnalyticsPlugin) Stop(ctx context.Context) error { return nil }
func (p *AnalyticsPlugin) Order() int { return 50 }
```

## Use Case 4: Health Check Endpoint

### Scenario
Application provides health check for load balancers and monitoring

```go
type HealthCheckComponent struct {
	healthy bool
}

func (h *HealthCheckComponent) ID() string { return "healthcheck" }

func (h *HealthCheckComponent) InitFlags() {}

func (h *HealthCheckComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	h.healthy = true
	return nil
}

func (h *HealthCheckComponent) Stop(ctx context.Context) error {
	h.healthy = false
	return nil
}

func (h *HealthCheckComponent) Order() int { return 200 }

// In your HTTP handler:
func handleHealth(w http.ResponseWriter, r *http.Request) {
	health, ok := sctx.GetAs[*HealthCheckComponent](app, "healthcheck")
	if !ok || !health.healthy {
		w.WriteHeader(http.StatusServiceUnavailable)
		json.NewEncoder(w).Encode(map[string]string{"status": "unhealthy"})
		return
	}
	
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(map[string]string{"status": "healthy"})
}
```

## Use Case 5: Graceful Shutdown with Timeout

### Scenario
Ensure all components shut down within a timeout period

```go
func main() {
	app := sctx.New(sctx.WithName("service"))
	
	// Add components...
	
	err := sctx.Run(app, func(ctx context.Context) error {
		// Create shutdown context with timeout
		shutdownDone := make(chan error, 1)
		
		go func() {
			<-ctx.Done()
			
			// Create timeout context for shutdown
			shutdownCtx, cancel := context.WithTimeout(
				context.Background(),
				30*time.Second,
			)
			defer cancel()
			
			// Shutdown will use this context
			shutdownDone <- app.Stop()
		}()
		
		// Your application logic
		
		return <-shutdownDone
	})
	
	if err != nil {
		log.Fatal(err)
	}
}
```

## Use Case 6: Metrics and Monitoring

### Scenario
Track component lifecycle events for observability

```go
type MetricsComponent struct {
	activations int
	failures    int
}

func (m *MetricsComponent) ID() string { return "metrics" }

func (m *MetricsComponent) InitFlags() {}

func (m *MetricsComponent) Activate(ctx context.Context, sv sctx.ServiceContext) error {
	m.activations++
	logger := sv.Logger("Metrics")
	logger.Info("Component activations: %d", m.activations)
	return nil
}

func (m *MetricsComponent) Stop(ctx context.Context) error {
	return nil
}

func (m *MetricsComponent) Order() int { return 5 }

// Export metrics:
func exposemetrics(app sctx.ServiceContext) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		metrics, _ := sctx.GetAs[*MetricsComponent](app, "metrics")
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(metrics)
	}
}
```

## Best Practices Summary

1. **Always use Order()** for explicit component startup ordering
2. **Check dependencies** with `GetAs` before using them
3. **Log component lifecycle** events for debugging
4. **Handle errors gracefully** during activation and shutdown
5. **Use environment variables** for configuration across environments
6. **Implement Stop()** even if it's a no-op
7. **Test components independently** before integration
8. **Document dependencies** between components
9. **Use type-safe GetAs** instead of casting raw interface{}
10. **Handle context cancellation** for graceful shutdown
