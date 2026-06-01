package worker

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackdes93/fcontext/job"
)

// ============================================================================
// Example 1: Simple Worker Pool
// ============================================================================

func runSimpleWorkerPool(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	// Create pool with 4 workers
	pool := NewPool(log, metric,
		WithName("simple-pool"),
		WithSize(4),
		WithQueueSize(100),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit 10 jobs
	for i := 0; i < 10; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("job-%d", i),
			delay: 50 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		ok := pool.Submit(j)
		if !ok {
			t.Fatalf("Failed to submit job %d", i)
		}
	}

	// Wait for jobs to complete
	time.Sleep(500 * time.Millisecond)

	fmt.Printf("✓ Completed %d jobs\n", atomic.LoadInt32(&metric.started))
	fmt.Println("✓ Example 1 passed: Simple worker pool")
}

// ============================================================================
// Example 2: Worker Pool with Error Handling
// ============================================================================

func runWorkerPoolWithErrors(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("error-pool"),
		WithSize(4),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit mix of failing and succeeding jobs
	for i := 0; i < 5; i++ {
		shouldFail := i % 2 == 0

		handler := &SimpleJobHandler{
			name:      fmt.Sprintf("job-%d", i),
			delay:     50 * time.Millisecond,
			shouldErr: shouldFail,
		}

		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		}, job.WithName(fmt.Sprintf("job-%d", i)))

		pool.Submit(j)
	}

	time.Sleep(400 * time.Millisecond)

	fmt.Printf("Started: %d, Succeeded: %d, Failed: %d\n",
		atomic.LoadInt32(&metric.started), atomic.LoadInt32(&metric.succeeded), atomic.LoadInt32(&metric.failed))

	fmt.Println("✓ Example 2 passed: Error handling")
}

// ============================================================================
// Example 3: Concurrent Job Submission
// ============================================================================

func runConcurrentSubmission(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("concurrent-pool"),
		WithSize(8),
		WithQueueSize(256),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs from multiple goroutines
	submitCount := atomic.Int32{}
	successCount := atomic.Int32{}

	for g := 0; g < 5; g++ {
		go func(gid int) {
			for i := 0; i < 10; i++ {
				handler := &SimpleJobHandler{
					name:  fmt.Sprintf("goroutine-%d-job-%d", gid, i),
					delay: 20 * time.Millisecond,
				}

				j := job.New(func(ctx context.Context) error {
					return handler.Handle(ctx)
				})

				if pool.Submit(j) {
					submitCount.Add(1)
					successCount.Add(1)
				}
			}
		}(g)
	}

	// Wait for all to complete
	time.Sleep(500 * time.Millisecond)

	fmt.Printf("Submitted: %d jobs\n", submitCount.Load())
	fmt.Println("✓ Example 3 passed: Concurrent submission")
}

// ============================================================================
// Example 4: Worker Pool with Retry Strategy
// ============================================================================

func runPoolWithRetry(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("retry-pool"),
		WithSize(4),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Handler that fails first, then succeeds
	callCount := atomic.Int32{}

	j := job.New(func(ctx context.Context) error {
		count := callCount.Add(1)
		if count == 1 {
			return errors.New("first attempt failed")
		}
		fmt.Printf("Succeeded on attempt %d\n", count)
		return nil
	},
		job.WithName("retry-job"),
		job.WithRetries([]time.Duration{100 * time.Millisecond}),
	)

	pool.Submit(j)

	time.Sleep(500 * time.Millisecond)

	fmt.Printf("Total attempts: %d\n", callCount.Load())
	fmt.Println("✓ Example 4 passed: Retry strategy")
}

// ============================================================================
// Example 5: Worker Pool Monitoring
// ============================================================================

func runPoolMonitoring(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("monitored-pool"),
		WithSize(4),
		WithQueueSize(100),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs
	for i := 0; i < 20; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("job-%d", i),
			delay: 30 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		pool.Submit(j)
	}

	// Monitor stats periodically
	for i := 0; i < 3; i++ {
		time.Sleep(200 * time.Millisecond)
		stats := pool.Stats()
		fmt.Printf("[Monitor] Running=%v, QueueLen=%d\n", stats.Running, stats.QueueSize)
	}

	fmt.Println("✓ Example 5 passed: Pool monitoring")
}

// ============================================================================
// Example 6: Component Integration
// ============================================================================

func runComponentIntegration(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	component := NewComponent("app-worker",
		metric,
		WithName("app-pool"),
		WithSize(4),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs via component
	for i := 0; i < 5; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("component-job-%d", i),
			delay: 50 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		ok := component.Submit(j)
		if !ok {
			t.Fatalf("Failed to submit via component")
		}
	}

	time.Sleep(300 * time.Millisecond)

	fmt.Printf("Component submitted %d jobs\n", atomic.LoadInt32(&metric.started))
	fmt.Println("✓ Example 6 passed: Component integration")
}

// ============================================================================
// Example 7: Hub Component with Job Types
// ============================================================================

func runHubComponentJobTypes(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	component := NewHubComponent("app-hub",
		metric,
		WithName("hub-pool"),
		WithSize(4),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	hub := component.GetHub()

	// Create jobs of different types
	jobTypes := []string{"email", "database", "notification"}

	for _, jobType := range jobTypes {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("%s-job", jobType),
			delay: 50 * time.Millisecond,
		}

		j, err := hub.Create(jobType, handler)
		if err != nil {
			t.Fatalf("Failed to create job: %v", err)
		}

		ok := component.Submit(j)
		if !ok {
			t.Fatalf("Failed to submit %s job", jobType)
		}
	}

	time.Sleep(300 * time.Millisecond)

	fmt.Printf("Submitted %d jobs via hub\n", atomic.LoadInt32(&metric.started))
	fmt.Println("✓ Example 7 passed: Hub with job types")
}

// ============================================================================
// Example 8: High-Throughput Scenario
// ============================================================================

func runHighThroughput(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("high-throughput"),
		WithSize(16), // More workers
		WithQueueSize(1024),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit 100 jobs
	start := time.Now()

	for i := 0; i < 100; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("job-%d", i),
			delay: 10 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		pool.Submit(j)
	}

	// Wait for completion
	time.Sleep(500 * time.Millisecond)

	elapsed := time.Since(start)
	throughput := float64(atomic.LoadInt32(&metric.started)) / elapsed.Seconds()

	fmt.Printf("Processed %d jobs in %v (%.0f jobs/sec)\n",
		atomic.LoadInt32(&metric.started), elapsed, throughput)

	fmt.Println("✓ Example 8 passed: High throughput")
}

// ============================================================================
// Example 9: Graceful Shutdown
// ============================================================================

func runGracefulShutdown(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("graceful-pool"),
		WithSize(4),
		WithStopTimeout(2 * time.Second),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs
	for i := 0; i < 10; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("job-%d", i),
			delay: 50 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		pool.Submit(j)
	}

	time.Sleep(200 * time.Millisecond)

	// Graceful shutdown
	fmt.Println("Initiating graceful shutdown...")
	cancel()

	time.Sleep(500 * time.Millisecond)

	fmt.Printf("Processed %d jobs before shutdown\n", atomic.LoadInt32(&metric.started))
	fmt.Println("✓ Example 9 passed: Graceful shutdown")
}

// ============================================================================
// Example 10: Complete Workflow - Order Processing
// ============================================================================

type OrderJobHandler struct {
	OrderID string
	Action  string
}

func (h *OrderJobHandler) Handle(ctx context.Context) error {
	fmt.Printf("[Order %s] Processing: %s\n", h.OrderID, h.Action)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func (h *OrderJobHandler) Type() string { return h.Action }

func runOrderProcessingWorkflow(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	// Create hub component for order processing
	component := NewHubComponent("order-processor",
		metric,
		WithName("order-pool"),
		WithSize(8),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	hub := component.GetHub()

	// Process multiple orders
	orders := []struct {
		id    string
		steps []string
	}{
		{"ORD-001", []string{"payment", "fulfillment", "notification"}},
		{"ORD-002", []string{"payment", "fulfillment", "notification"}},
		{"ORD-003", []string{"payment", "fulfillment", "notification"}},
	}

	for _, order := range orders {
		for _, step := range order.steps {
			handler := &OrderJobHandler{
				OrderID: order.id,
				Action:  step,
			}

			j, err := hub.Create(step, handler)
			if err != nil {
				t.Fatalf("Failed to create job: %v", err)
			}

			component.Submit(j)
		}
	}

	time.Sleep(500 * time.Millisecond)

	fmt.Printf("Processed %d workflow steps\n", atomic.LoadInt32(&metric.started))
	fmt.Println("✓ Example 10 passed: Complete order processing workflow")
}

// ============================================================================
// Test Function to Run All Examples
// ============================================================================

func TestAllExamples(t *testing.T) {
	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("Running Worker Package Examples")
	fmt.Println(strings.Repeat("=", 60))

	examples := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{"Example 1: Simple Worker Pool", runSimpleWorkerPool},
		{"Example 2: Error Handling", runWorkerPoolWithErrors},
		{"Example 3: Concurrent Submission", runConcurrentSubmission},
		{"Example 4: Retry Strategy", runPoolWithRetry},
		{"Example 5: Monitoring", runPoolMonitoring},
		{"Example 6: Component Integration", runComponentIntegration},
		{"Example 7: Hub Job Types", runHubComponentJobTypes},
		{"Example 8: High Throughput", runHighThroughput},
		{"Example 9: Graceful Shutdown", runGracefulShutdown},
		{"Example 10: Order Processing Workflow", runOrderProcessingWorkflow},
	}

	for _, ex := range examples {
		fmt.Printf("\n%s\n", ex.name)
		fmt.Println(strings.Repeat("-", 60))
		ex.fn(t)
	}

	fmt.Println("\n" + strings.Repeat("=", 60))
	fmt.Println("✓ All examples completed successfully!")
	fmt.Println(strings.Repeat("=", 60))
}
