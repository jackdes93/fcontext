package job

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestJobHandler implement JobHandler interface
type TestJobHandler struct {
	Name       string
	SleepTime  time.Duration
	ShouldErr  bool
	ExecutedAt time.Time
	Ctx        context.Context
}

func (t *TestJobHandler) Handle(ctx context.Context) error {
	t.ExecutedAt = time.Now()
	t.Ctx = ctx
	fmt.Printf("[%s] Executing...\n", t.Name)
	
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(t.SleepTime):
		if t.ShouldErr {
			return fmt.Errorf("test error from %s", t.Name)
		}
		fmt.Printf("[%s] Completed successfully\n", t.Name)
		return nil
	}
}

func (t *TestJobHandler) Type() string {
	return "test"
}

// TestHub tests Hub functionality
func TestHubCreate(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "test-job-1",
		SleepTime: 100 * time.Millisecond,
	}
	
	j, err := hub.Create("test", handler,
		WithName("my-test-job"),
		WithTimeout(5 * time.Second),
	)
	
	if err != nil {
		t.Fatalf("Create failed: %v", err)
	}
	
	if j.State() != StateInit {
		t.Fatalf("Expected StateInit, got %v", j.State())
	}
}

// TestHubCreateWithEmptyType tests invalid job type
func TestHubCreateWithEmptyType(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "test",
		SleepTime: 50 * time.Millisecond,
	}
	
	_, err := hub.Create("", handler)
	if err == nil {
		t.Fatal("Create with empty jobType should fail")
	}
}

// TestHubCreateWithNilHandler tests nil handler
func TestHubCreateWithNilHandler(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	_, err := hub.Create("test", nil)
	if err == nil {
		t.Fatal("Create with nil handler should fail")
	}
}

// TestHubSubmit tests job submission
func TestHubSubmit(t *testing.T) {
	submitted := false
	
	hub := NewHub(func(j Job) bool {
		submitted = true
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "submit-test",
		SleepTime: 50 * time.Millisecond,
	}
	
	j, _ := hub.Create("test", handler, WithName("submit-job"))
	
	ok := hub.Submit(j)
	if !ok {
		t.Fatal("Submit should return true")
	}
	
	if !submitted {
		t.Fatal("Submit function should have been called")
	}
}

// TestHubSubmitAfterStop tests submission after stop
func TestHubSubmitAfterStop(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	ctx := context.Background()
	hub.Stop(ctx)
	
	if hub.IsRunning() {
		t.Fatal("Hub should not be running after Stop")
	}
	
	handler := &TestJobHandler{
		Name:      "test",
		SleepTime: 50 * time.Millisecond,
	}
	
	j, err := hub.Create("test", handler)
	if err == nil {
		t.Fatal("Create after Stop should fail")
	}
}

// TestHubMultipleJobTypes tests creating multiple job types
func TestHubMultipleJobTypes(t *testing.T) {
	jobs := make(map[string]bool)
	
	hub := NewHub(func(j Job) bool {
		jobs[j.State().String()] = true
		return true
	})
	
	// Create email job
	emailHandler := &TestJobHandler{
		Name:      "email-job",
		SleepTime: 50 * time.Millisecond,
	}
	
	emailJob, err := hub.Create("email", emailHandler,
		WithName("send-email"),
		WithTimeout(5 * time.Second),
	)
	
	if err != nil {
		t.Fatalf("Create email job failed: %v", err)
	}
	
	// Create notification job
	notifHandler := &TestJobHandler{
		Name:      "notification-job",
		SleepTime: 50 * time.Millisecond,
	}
	
	notifJob, err := hub.Create("notification", notifHandler,
		WithName("send-notification"),
		WithTimeout(5 * time.Second),
	)
	
	if err != nil {
		t.Fatalf("Create notification job failed: %v", err)
	}
	
	hub.Submit(emailJob)
	hub.Submit(notifJob)
	
	if len(jobs) == 0 {
		t.Fatal("Jobs should have been submitted")
	}
}

// TestHubWithRetry tests job with retry
func TestHubWithRetry(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		go func() {
			ctx := context.Background()
			j.RunWithRetry(ctx)
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "retry-job",
		SleepTime: 10 * time.Millisecond,
		ShouldErr: false,
	}
	
	j, _ := hub.Create("test", handler,
		WithName("job-with-retry"),
		WithTimeout(5 * time.Second),
		WithRetries([]time.Duration{10 * time.Millisecond}),
	)
	
	hub.Submit(j)
	
	time.Sleep(200 * time.Millisecond)
	
	if j.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", j.State())
	}
}

// TestJobTimeout tests timeout handling
func TestJobTimeout(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		go func() {
			ctx := context.Background()
			j.Execute(ctx)
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "slow-job",
		SleepTime: 2 * time.Second, // Will exceed timeout
	}
	
	j, _ := hub.Create("test", handler,
		WithName("timeout-test"),
		WithTimeout(100 * time.Millisecond),
	)
	
	hub.Submit(j)
	
	time.Sleep(500 * time.Millisecond)
	
	if j.State() != StateTimeout {
		t.Fatalf("Expected StateTimeout, got %v", j.State())
	}
	
	if j.LastError() == nil {
		t.Fatal("LastError should not be nil for timeout")
	}
}

// TestJobWithRetryOnFailure tests retry on failure
func TestJobWithRetryOnFailure(t *testing.T) {
	retryCount := atomic.Int32{}
	
	hub := NewHub(func(j Job) bool {
		go func() {
			ctx := context.Background()
			j.RunWithRetry(ctx)
		}()
		return true
	})
	
	// Handler that fails first time, succeeds on retry
	callCount := atomic.Int32{}
	handler := &TestJobHandler{
		Name:      "failing-job",
		SleepTime: 10 * time.Millisecond,
	}
	
	originalHandle := handler.Handle
	handler.Handle = func(ctx context.Context) error {
		count := callCount.Add(1)
		if count == 1 {
			return errors.New("first attempt failed")
		}
		return originalHandle(ctx)
	}
	
	j, _ := hub.Create("test", handler,
		WithName("retry-on-failure"),
		WithTimeout(5 * time.Second),
		WithRetries([]time.Duration{20 * time.Millisecond, 20 * time.Millisecond}),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			retryCount.Add(1)
		}),
	)
	
	hub.Submit(j)
	
	time.Sleep(300 * time.Millisecond)
	
	if j.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted after retry, got %v", j.State())
	}
	
	if retryCount.Load() == 0 {
		t.Fatal("Retry callback should have been called")
	}
}

// TestJobStateTransitions tests state transitions
func TestJobStateTransitions(t *testing.T) {
	stateChanges := []State{}
	
	hub := NewHub(func(j Job) bool {
		go func() {
			stateChanges = append(stateChanges, j.State())
			ctx := context.Background()
			j.Execute(ctx)
			stateChanges = append(stateChanges, j.State())
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "state-test",
		SleepTime: 20 * time.Millisecond,
	}
	
	j, _ := hub.Create("test", handler, WithName("state-transition-test"))
	
	if j.State() != StateInit {
		t.Fatalf("Initial state should be StateInit, got %v", j.State())
	}
	
	hub.Submit(j)
	
	time.Sleep(200 * time.Millisecond)
	
	if j.State() != StateCompleted {
		t.Fatalf("Final state should be StateCompleted, got %v", j.State())
	}
}

// TestHubStop tests hub stopping
func TestHubStop(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "test-job",
		SleepTime: 50 * time.Millisecond,
	}
	
	j, _ := hub.Create("test", handler)
	hub.Submit(j)
	
	ctx := context.Background()
	err := hub.Stop(ctx)
	
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
	
	// After stop, new creates should fail
	_, err = hub.Create("test", handler)
	if err == nil {
		t.Fatal("Create after Stop should fail")
	}
}

// TestHubJobExecution tests job execution
func TestHubJobExecution(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	
	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(ctx)
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "exec-test",
		SleepTime: 50 * time.Millisecond,
	}
	
	completeCalled := false
	j, _ := hub.Create("test", handler,
		WithName("execution-test"),
		WithOnComplete(func() {
			completeCalled = true
		}),
	)
	
	hub.Submit(j)
	
	time.Sleep(200 * time.Millisecond)
	
	if j.State() != StateCompleted {
		t.Fatalf("Job should be completed, got state %v", j.State())
	}
	
	if !completeCalled {
		t.Fatal("OnComplete callback should have been called")
	}
}

// TestJobContextCancellation tests context cancellation
func TestJobContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	
	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(ctx)
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "cancel-test",
		SleepTime: 2 * time.Second,
	}
	
	j, _ := hub.Create("test", handler,
		WithName("cancellation-test"),
		WithTimeout(10 * time.Second),
	)
	
	hub.Submit(j)
	
	time.Sleep(50 * time.Millisecond)
	cancel()
	
	time.Sleep(200 * time.Millisecond)
	
	if j.State() != StateFailed {
		t.Fatalf("Expected StateFailed after context cancellation, got %v", j.State())
	}
}

// TestJobPermanentError tests permanent error callback
func TestJobPermanentError(t *testing.T) {
	permanentErrorCalled := false
	
	hub := NewHub(func(j Job) bool {
		go func() {
			ctx := context.Background()
			j.RunWithRetry(ctx)
		}()
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "error-job",
		SleepTime: 10 * time.Millisecond,
		ShouldErr: true,
	}
	
	j, _ := hub.Create("test", handler,
		WithName("permanent-error-test"),
		WithTimeout(5 * time.Second),
		WithRetries([]time.Duration{10 * time.Millisecond}),
		WithOnPermanent(func(lastErr error) {
			permanentErrorCalled = true
		}),
	)
	
	hub.Submit(j)
	
	time.Sleep(300 * time.Millisecond)
	
	if !permanentErrorCalled {
		t.Fatal("OnPermanent callback should have been called")
	}
	
	if j.State() != StateRetryFailed {
		t.Fatalf("Expected StateRetryFailed, got %v", j.State())
	}
}

// TestConcurrentJobExecution tests concurrent job execution
func TestConcurrentJobExecution(t *testing.T) {
	completedCount := atomic.Int32{}
	
	hub := NewHub(func(j Job) bool {
		go func() {
			ctx := context.Background()
			j.Execute(ctx)
			if j.State() == StateCompleted {
				completedCount.Add(1)
			}
		}()
		return true
	})
	
	// Submit 10 jobs concurrently
	for i := 0; i < 10; i++ {
		handler := &TestJobHandler{
			Name:      fmt.Sprintf("job-%d", i),
			SleepTime: 20 * time.Millisecond,
		}
		
		j, _ := hub.Create("test", handler,
			WithName(fmt.Sprintf("concurrent-job-%d", i)),
		)
		
		hub.Submit(j)
	}
	
	time.Sleep(500 * time.Millisecond)
	
	if completedCount.Load() != 10 {
		t.Fatalf("Expected 10 completed jobs, got %d", completedCount.Load())
	}
}

// TestSetRetries tests SetRetries method
func TestSetRetries(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		return true
	})
	
	handler := &TestJobHandler{
		Name:      "test",
		SleepTime: 10 * time.Millisecond,
	}
	
	j, _ := hub.Create("test", handler)
	
	newRetries := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	j.SetRetries(newRetries)
	
	if j.RetryIndex() != -1 {
		t.Fatal("RetryIndex should be reset to -1 after SetRetries")
	}
}
