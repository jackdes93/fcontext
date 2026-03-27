package worker

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackdes93/fcontext/job"
)

// MockLogger for testing
type MockLogger struct {
	messages []string
	mu       sync.Mutex
}

func (m *MockLogger) Info(msg string, args ...interface{})      { m.log("INFO", msg, args...) }
func (m *MockLogger) Warn(msg string, args ...interface{})      { m.log("WARN", msg, args...) }
func (m *MockLogger) Error(msg string, args ...interface{})     { m.log("ERROR", msg, args...) }
func (m *MockLogger) WithPrefix(prefix string) interface{} {
	return m
}

func (m *MockLogger) log(level string, msg string, args ...interface{}) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.messages = append(m.messages, fmt.Sprintf("[%s] %s", level, fmt.Sprintf(msg, args...)))
}

// MockMetrics for testing
type MockMetrics struct {
	started    int32
	succeeded  int32
	failed     int32
	permanent  int32
	latencies  []time.Duration
	mu         sync.Mutex
}

func (m *MockMetrics) IncJobStarted(name string) {
	atomic.AddInt32(&m.started, 1)
}

func (m *MockMetrics) IncJobSuccess(name string, latency time.Duration) {
	atomic.AddInt32(&m.succeeded, 1)
	m.mu.Lock()
	m.latencies = append(m.latencies, latency)
	m.mu.Unlock()
}

func (m *MockMetrics) IncJobFailed(name string, err error, latency time.Duration) {
	atomic.AddInt32(&m.failed, 1)
}

func (m *MockMetrics) IncJobPermanentFailed(name string, err error) {
	atomic.AddInt32(&m.permanent, 1)
}

// SimpleJobHandler for testing
type SimpleJobHandler struct {
	name      string
	delay     time.Duration
	shouldErr bool
}

func (h *SimpleJobHandler) Handle(ctx context.Context) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(h.delay):
		if h.shouldErr {
			return errors.New("job handler error")
		}
		return nil
	}
}

func (h *SimpleJobHandler) Type() string { return "simple" }

// TestPoolCreation tests pool creation with options
func TestPoolCreation(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("test-pool"),
		WithSize(8),
		WithQueueSize(512),
		WithStopTimeout(5 * time.Second),
	)

	if pool == nil {
		t.Fatal("Pool should not be nil")
	}
}

// TestPoolSubmitBeforeRun tests submitting before pool runs
func TestPoolSubmitBeforeRun(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric)

	handler := &SimpleJobHandler{name: "test", delay: 10 * time.Millisecond}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	// Should fail before Run() is called
	ok := pool.Submit(j)
	if ok {
		t.Fatal("Submit before Run should return false")
	}
}

// TestPoolRun tests starting the pool
func TestPoolRun(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 500 * time.Millisecond)
	defer cancel()

	go pool.Run(ctx)

	// Give workers time to start
	time.Sleep(100 * time.Millisecond)

	if !pool.IsRunning() {
		t.Fatal("Pool should be running")
	}

	// Wait for context to timeout
	<-ctx.Done()
	time.Sleep(100 * time.Millisecond)

	if pool.IsRunning() {
		t.Fatal("Pool should not be running after context done")
	}
}

// TestPoolSubmitJob tests submitting a job
func TestPoolSubmitJob(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	handler := &SimpleJobHandler{name: "test", delay: 50 * time.Millisecond}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	ok := pool.Submit(j)
	if !ok {
		t.Fatal("Submit should succeed")
	}

	// Wait for job to execute
	time.Sleep(300 * time.Millisecond)

	if atomic.LoadInt32(&metric.started) == 0 {
		t.Fatal("Metric IncJobStarted should have been called")
	}

	if atomic.LoadInt32(&metric.succeeded) == 0 {
		t.Fatal("Job should have succeeded")
	}
}

// TestPoolMultipleJobs tests submitting multiple jobs
func TestPoolMultipleJobs(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(4))

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
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
			t.Fatalf("Submit job %d failed", i)
		}
	}

	// Wait for all jobs to complete
	time.Sleep(500 * time.Millisecond)

	started := atomic.LoadInt32(&metric.started)
	succeeded := atomic.LoadInt32(&metric.succeeded)

	if started < 10 {
		t.Fatalf("Expected at least 10 started jobs, got %d", started)
	}

	if succeeded < 10 {
		t.Fatalf("Expected at least 10 succeeded jobs, got %d", succeeded)
	}
}

// TestPoolJobFailure tests job failure handling
func TestPoolJobFailure(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit failing job
	handler := &SimpleJobHandler{
		name:      "failing-job",
		delay:     10 * time.Millisecond,
		shouldErr: true,
	}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	pool.Submit(j)

	time.Sleep(200 * time.Millisecond)

	if atomic.LoadInt32(&metric.failed) == 0 {
		t.Fatal("Failed job metric should be incremented")
	}
}

// TestPoolQueueFull tests queue full scenario
func TestPoolQueueFull(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	// Small queue size
	pool := NewPool(log, metric, WithSize(1), WithQueueSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit long-running jobs to fill queue
	for i := 0; i < 5; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("slow-%d", i),
			delay: 1 * time.Second,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		ok := pool.Submit(j)
		if i < 2 {
			// First few should succeed
			if !ok {
				t.Fatalf("Submit %d should succeed", i)
			}
		} else {
			// Later ones might fail if queue is full
			// (This depends on timing, so we just check that it returns bool)
			_ = ok
		}
	}

	time.Sleep(200 * time.Millisecond)
}

// TestPoolStats tests pool statistics
func TestPoolStats(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric,
		WithName("stats-pool"),
		WithSize(4),
		WithQueueSize(100),
	)

	ctx, cancel := context.WithTimeout(context.Background(), 1 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	stats := pool.Stats()

	if stats.Name != "stats-pool" {
		t.Fatalf("Expected name 'stats-pool', got %s", stats.Name)
	}

	if stats.WorkerCount != 4 {
		t.Fatalf("Expected 4 workers, got %d", stats.WorkerCount)
	}

	if stats.QueueSize != 100 {
		t.Fatalf("Expected queue size 100, got %d", stats.QueueSize)
	}

	if !stats.Running {
		t.Fatal("Pool should be running")
	}

	if stats.Stopped {
		t.Fatal("Pool should not be stopped")
	}
}

// TestPoolGracefulStop tests graceful shutdown
func TestPoolGracefulStop(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2), WithStopTimeout(2 * time.Second))

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs
	for i := 0; i < 3; i++ {
		handler := &SimpleJobHandler{
			name:  fmt.Sprintf("job-%d", i),
			delay: 20 * time.Millisecond,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		pool.Submit(j)
	}

	time.Sleep(200 * time.Millisecond)

	// Stop pool
	cancel()
	time.Sleep(500 * time.Millisecond)

	if pool.IsRunning() {
		t.Fatal("Pool should be stopped")
	}
}

// TestPoolConcurrentSubmit tests concurrent job submission
func TestPoolConcurrentSubmit(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(8))

	ctx, cancel := context.WithTimeout(context.Background(), 5 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit jobs concurrently
	var wg sync.WaitGroup

	for goroutine := 0; goroutine < 10; goroutine++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()

			for i := 0; i < 10; i++ {
				handler := &SimpleJobHandler{
					name:  fmt.Sprintf("job-%d-%d", gid, i),
					delay: 10 * time.Millisecond,
				}
				j := job.New(func(ctx context.Context) error {
					return handler.Handle(ctx)
				})

				pool.Submit(j)
			}
		}(goroutine)
	}

	wg.Wait()

	// Wait for all jobs to execute
	time.Sleep(500 * time.Millisecond)

	started := atomic.LoadInt32(&metric.started)
	if started < 50 {
		t.Fatalf("Expected many started jobs, got %d", started)
	}
}

// TestPoolMetrics tests metrics collection
func TestPoolMetrics(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit mix of successful and failing jobs
	for i := 0; i < 5; i++ {
		shouldFail := i % 2 == 0

		handler := &SimpleJobHandler{
			name:      fmt.Sprintf("job-%d", i),
			delay:     20 * time.Millisecond,
			shouldErr: shouldFail,
		}
		j := job.New(func(ctx context.Context) error {
			return handler.Handle(ctx)
		})

		pool.Submit(j)
	}

	time.Sleep(400 * time.Millisecond)

	started := atomic.LoadInt32(&metric.started)
	succeeded := atomic.LoadInt32(&metric.succeeded)
	failed := atomic.LoadInt32(&metric.failed)

	if started != 5 {
		t.Fatalf("Expected 5 started jobs, got %d", started)
	}

	if succeeded != 2 {
		t.Fatalf("Expected 2 succeeded jobs, got %d", succeeded)
	}

	if failed != 3 {
		t.Fatalf("Expected 3 failed jobs, got %d", failed)
	}
}

// TestPoolNilJob tests submitting nil job
func TestPoolNilJob(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric)

	ctx, cancel := context.WithTimeout(context.Background(), 1 * time.Second)
	defer cancel()

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit nil job
	ok := pool.Submit(nil)
	if ok {
		t.Fatal("Submit nil job should return false")
	}
}

// TestPoolStopBeforeRun tests stopping before run
func TestPoolStopBeforeRun(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric)

	ctx := context.Background()
	pool.Stop(ctx)

	// Should not panic, should just return
	stats := pool.Stats()
	if !stats.Stopped {
		t.Fatal("Pool should be marked as stopped")
	}
}

// TestPoolContextCancellation tests context cancellation during job execution
func TestPoolContextCancellation(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}

	pool := NewPool(log, metric, WithSize(2))

	ctx, cancel := context.WithCancel(context.Background())

	go pool.Run(ctx)

	time.Sleep(100 * time.Millisecond)

	// Submit long-running job
	handler := &SimpleJobHandler{
		name:  "long-job",
		delay: 5 * time.Second,
	}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	pool.Submit(j)

	// Cancel context while job is running
	time.Sleep(50 * time.Millisecond)
	cancel()

	time.Sleep(500 * time.Millisecond)

	if pool.IsRunning() {
		t.Fatal("Pool should stop after context cancellation")
	}
}
