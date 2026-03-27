package job

import (
	"context"
	"errors"
	"fmt"
	"sync/atomic"
	"testing"
	"time"
)

// TestNewJob tests creating a new job
func TestNewJob(t *testing.T) {
	called := false
	handler := func(ctx context.Context) error {
		called = true
		return nil
	}

	j := New(handler, WithName("test-job"), WithTimeout(5*time.Second))

	if j == nil {
		t.Fatal("New should return a job")
	}

	if j.State() != StateInit {
		t.Fatalf("Initial state should be StateInit, got %v", j.State())
	}

	if j.RetryIndex() != -1 {
		t.Fatalf("Initial retry index should be -1, got %d", j.RetryIndex())
	}
}

// TestJobExecuteSuccess tests successful job execution
func TestJobExecuteSuccess(t *testing.T) {
	called := false
	handler := func(ctx context.Context) error {
		called = true
		return nil
	}

	j := New(handler, WithTimeout(5*time.Second))
	ctx := context.Background()

	err := j.Execute(ctx)

	if err != nil {
		t.Fatalf("Execute should succeed, got error: %v", err)
	}

	if !called {
		t.Fatal("Handler should have been called")
	}

	if j.State() != StateCompleted {
		t.Fatalf("State should be StateCompleted, got %v", j.State())
	}
}

// TestJobExecuteError tests job execution with error
func TestJobExecuteError(t *testing.T) {
	testErr := errors.New("test error")
	handler := func(ctx context.Context) error {
		return testErr
	}

	j := New(handler, WithTimeout(5*time.Second))
	ctx := context.Background()

	err := j.Execute(ctx)

	if err != testErr {
		t.Fatalf("Execute should return the handler error, got: %v", err)
	}

	if j.State() != StateFailed {
		t.Fatalf("State should be StateFailed, got %v", j.State())
	}

	if j.LastError() != testErr {
		t.Fatal("LastError should match the error returned")
	}
}

// TestJobExecuteTimeout tests job execution timeout
func TestJobExecuteTimeout(t *testing.T) {
	handler := func(ctx context.Context) error {
		// Simulate long-running task
		time.Sleep(1 * time.Second)
		return nil
	}

	j := New(handler, WithTimeout(100*time.Millisecond))
	ctx := context.Background()

	err := j.Execute(ctx)

	if err == nil {
		t.Fatal("Execute should timeout")
	}

	if j.State() != StateTimeout {
		t.Fatalf("State should be StateTimeout, got %v", j.State())
	}

	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("Error should be DeadlineExceeded, got: %v", err)
	}
}

// TestJobExecuteContextCancellation tests context cancellation during execution
func TestJobExecuteContextCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())

	handler := func(innerCtx context.Context) error {
		// Cancel context immediately
		cancel()
		// Try to use the context
		select {
		case <-innerCtx.Done():
			return innerCtx.Err()
		case <-time.After(1 * time.Second):
			return nil
		}
	}

	j := New(handler, WithTimeout(5*time.Second))

	err := j.Execute(ctx)

	if err == nil {
		t.Fatal("Execute should return context cancellation error")
	}

	if j.State() != StateFailed {
		t.Fatalf("State should be StateFailed, got %v", j.State())
	}
}

// TestJobOnComplete tests OnComplete callback
func TestJobOnComplete(t *testing.T) {
	completeCalled := false

	handler := func(ctx context.Context) error {
		return nil
	}

	j := New(handler,
		WithTimeout(5*time.Second),
		WithOnComplete(func() {
			completeCalled = true
		}),
	)

	ctx := context.Background()
	j.Execute(ctx)

	if !completeCalled {
		t.Fatal("OnComplete callback should have been called")
	}
}

// TestJobRetrySuccess tests retry on first failure
func TestJobRetrySuccess(t *testing.T) {
	callCount := atomic.Int32{}

	handler := func(ctx context.Context) error {
		count := callCount.Add(1)
		if count == 1 {
			return errors.New("first attempt failed")
		}
		return nil
	}

	j := New(handler,
		WithTimeout(5*time.Second),
		WithRetries([]time.Duration{50 * time.Millisecond}),
	)

	ctx := context.Background()

	// First execute fails
	err := j.Execute(ctx)
	if err == nil {
		t.Fatal("First execute should return error")
	}

	// Retry should succeed
	err = j.Retry(ctx)
	if err != nil {
		t.Fatalf("Retry should succeed, got error: %v", err)
	}

	if j.State() != StateCompleted {
		t.Fatalf("State should be StateCompleted after successful retry, got %v", j.State())
	}
}

// TestJobRetryExhausted tests retry exhaustion
func TestJobRetryExhausted(t *testing.T) {
	handler := func(ctx context.Context) error {
		return errors.New("always fails")
	}

	permanentErrorCalled := false

	j := New(handler,
		WithTimeout(5*time.Second),
		WithRetries([]time.Duration{10 * time.Millisecond}),
		WithOnPermanent(func(lastErr error) {
			permanentErrorCalled = true
		}),
	)

	ctx := context.Background()

	// Execute
	j.Execute(ctx)

	// First retry
	j.Retry(ctx)

	if j.State() != StateRetryFailed {
		t.Fatalf("State should be StateRetryFailed, got %v", j.State())
	}

	if !permanentErrorCalled {
		t.Fatal("OnPermanent callback should have been called")
	}
}

// TestJobRunWithRetry tests full retry flow
func TestJobRunWithRetry(t *testing.T) {
	callCount := atomic.Int32{}

	handler := func(ctx context.Context) error {
		count := callCount.Add(1)
		if count <= 2 {
			return errors.New("attempt failed")
		}
		return nil
	}

	retryCount := atomic.Int32{}

	j := New(handler,
		WithTimeout(5*time.Second),
		WithRetries([]time.Duration{10 * time.Millisecond, 10 * time.Millisecond, 10 * time.Millisecond}),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			retryCount.Add(1)
		}),
	)

	ctx := context.Background()

	err := j.RunWithRetry(ctx)

	if err != nil {
		t.Fatalf("RunWithRetry should eventually succeed, got error: %v", err)
	}

	if j.State() != StateCompleted {
		t.Fatalf("State should be StateCompleted, got %v", j.State())
	}

	if retryCount.Load() == 0 {
		t.Fatal("OnRetry callback should have been called")
	}
}

// TestJobSetRetries tests SetRetries method
func TestJobSetRetries(t *testing.T) {
	handler := func(ctx context.Context) error {
		return nil
	}

	j := New(handler)

	newRetries := []time.Duration{100 * time.Millisecond, 200 * time.Millisecond}
	j.SetRetries(newRetries)

	if j.RetryIndex() != -1 {
		t.Fatal("RetryIndex should be reset to -1")
	}
}

// TestJitterApplication tests jitter application
func TestJitterApplication(t *testing.T) {
	baseDelay := 100 * time.Millisecond
	jitterPct := 0.2 // 20%

	// Apply jitter multiple times to verify it's not deterministic
	results := make([]time.Duration, 10)
	for i := 0; i < 10; i++ {
		results[i] = applyJitter(baseDelay, jitterPct)
	}

	// Check that values are within expected range
	minExpected := time.Duration(float64(baseDelay) * (1 - jitterPct))
	maxExpected := time.Duration(float64(baseDelay) * (1 + jitterPct))

	for i, d := range results {
		if d < minExpected || d > maxExpected {
			t.Fatalf("Jitter result %d out of range: %v (expected %v to %v)",
				i, d, minExpected, maxExpected)
		}
	}
}

// TestJitterNoJitter tests no jitter when pct is 0
func TestJitterNoJitter(t *testing.T) {
	baseDelay := 100 * time.Millisecond

	result := applyJitter(baseDelay, 0)

	if result != baseDelay {
		t.Fatalf("Jitter with 0%% should return original delay, got: %v", result)
	}
}

// TestJobStateConcurrency tests state safety under concurrency
func TestJobStateConcurrency(t *testing.T) {
	stateChanges := atomic.Int32{}

	handler := func(ctx context.Context) error {
		return nil
	}

	j := New(handler, WithTimeout(5*time.Second))
	ctx := context.Background()

	// Execute job in goroutine
	go func() {
		j.Execute(ctx)
	}()

	// Continuously check state from main goroutine
	for i := 0; i < 100; i++ {
		state := j.State()
		if state != StateInit && state != StateRunning && state != StateCompleted {
			t.Fatalf("Unexpected state: %v", state)
		}
		stateChanges.Add(1)
		time.Sleep(1 * time.Millisecond)
	}

	if stateChanges.Load() == 0 {
		t.Fatal("State checks should have been performed")
	}
}

// TestMultipleRetries tests multiple consecutive retries
func TestMultipleRetries(t *testing.T) {
	callCount := atomic.Int32{}
	retryCount := atomic.Int32{}

	handler := func(ctx context.Context) error {
		count := callCount.Add(1)
		if count <= 3 {
			return errors.New("failed")
		}
		return nil
	}

	j := New(handler,
		WithTimeout(5*time.Second),
		WithRetries([]time.Duration{
			10 * time.Millisecond,
			10 * time.Millisecond,
			10 * time.Millisecond,
			10 * time.Millisecond,
		}),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			retryCount.Add(1)
		}),
	)

	ctx := context.Background()

	err := j.RunWithRetry(ctx)

	if err != nil {
		t.Fatalf("RunWithRetry should eventually succeed, got: %v", err)
	}

	expectedCalls := 4 // 1 initial + 3 retries
	if callCount.Load() != int32(expectedCalls) {
		t.Fatalf("Expected %d handler calls, got %d", expectedCalls, callCount.Load())
	}

	if j.State() != StateCompleted {
		t.Fatalf("Final state should be StateCompleted, got %v", j.State())
	}
}

// TestJobLastError tests LastError tracking
func TestJobLastError(t *testing.T) {
	testErr := errors.New("specific error")

	handler := func(ctx context.Context) error {
		return testErr
	}

	j := New(handler, WithTimeout(5*time.Second))
	ctx := context.Background()

	j.Execute(ctx)

	if j.LastError() != testErr {
		t.Fatalf("LastError should be %v, got %v", testErr, j.LastError())
	}
}

// TestJobWithoutTimeout tests job without timeout
func TestJobWithoutTimeout(t *testing.T) {
	handler := func(ctx context.Context) error {
		time.Sleep(100 * time.Millisecond)
		return nil
	}

	// No timeout specified
	j := New(handler)
	ctx := context.Background()

	err := j.Execute(ctx)

	if err != nil {
		t.Fatalf("Execute without timeout should succeed, got error: %v", err)
	}

	if j.State() != StateCompleted {
		t.Fatalf("State should be StateCompleted, got %v", j.State())
	}
}
