package worker

import (
	"context"
	"testing"
	"time"

	"github.com/jackdes93/fcontext/sctx"
	"github.com/jackdes93/fcontext/job"
)

// MockServiceContext for testing
type MockServiceContext struct {
	logger sctx.Logger
}

func (m *MockServiceContext) Logger(name string) sctx.Logger {
	return m.logger
}

// TestComponentActivate tests component activation
func TestComponentActivate(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewComponent("test-worker", metric, WithSize(4))

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	err := component.Activate(ctx, sc)
	if err != nil {
		t.Fatalf("Activate failed: %v", err)
	}

	if component.ID() != "test-worker" {
		t.Fatalf("Expected ID 'test-worker', got %s", component.ID())
	}
}

// TestComponentStop tests component shutdown
func TestComponentStop(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewComponent("test-worker", metric)

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	component.Activate(ctx, sc)

	err := component.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestComponentSubmit tests job submission through component
func TestComponentSubmit(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewComponent("test-worker", metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	// Create and submit job
	handler := &SimpleJobHandler{name: "test", delay: 50 * time.Millisecond}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	ok := component.Submit(j)
	if !ok {
		t.Fatal("Submit through component should succeed")
	}

	time.Sleep(200 * time.Millisecond)

	if metric.started <= 0 {
		t.Fatal("Job should have been started")
	}
}

// TestHubComponentActivate tests hub component activation
func TestHubComponentActivate(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewHubComponent("test-hub", metric, WithSize(4))

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	err := component.Activate(ctx, sc)
	if err != nil {
		t.Fatalf("Activate failed: %v", err)
	}

	if component.ID() != "test-hub" {
		t.Fatalf("Expected ID 'test-hub', got %s", component.ID())
	}
}

// TestHubComponentGetHub tests hub retrieval
func TestHubComponentGetHub(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewHubComponent("test-hub", metric)

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	component.Activate(ctx, sc)

	hub := component.GetHub()
	if hub == nil {
		t.Fatal("GetHub should return non-nil hub")
	}
}

// TestHubComponentSubmit tests job submission through hub component
func TestHubComponentSubmit(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewHubComponent("test-hub", metric, WithSize(2))

	ctx, cancel := context.WithTimeout(context.Background(), 2 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	// Create and submit job via component
	handler := &SimpleJobHandler{name: "test", delay: 50 * time.Millisecond}
	j := job.New(func(ctx context.Context) error {
		return handler.Handle(ctx)
	})

	ok := component.Submit(j)
	if !ok {
		t.Fatal("Submit through hub component should succeed")
	}

	time.Sleep(200 * time.Millisecond)

	if metric.started <= 0 {
		t.Fatal("Job should have been started")
	}
}

// TestHubComponentStop tests hub component shutdown
func TestHubComponentStop(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewHubComponent("test-hub", metric)

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	component.Activate(ctx, sc)

	err := component.Stop(ctx)
	if err != nil {
		t.Fatalf("Stop failed: %v", err)
	}
}

// TestComponentMultipleTimes tests activating component multiple times
func TestComponentMultipleTimes(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewComponent("test-worker", metric)

	ctx := context.Background()
	sc := &MockServiceContext{logger: log}

	// First activation
	err1 := component.Activate(ctx, sc)
	if err1 != nil {
		t.Fatalf("First activate failed: %v", err1)
	}

	// Second activation (should not error, but reuses same pool)
	err2 := component.Activate(ctx, sc)
	if err2 != nil {
		t.Fatalf("Second activate failed: %v", err2)
	}
}

// TestHubComponentMultipleJobs tests submitting multiple jobs via hub component
func TestHubComponentMultipleJobs(t *testing.T) {
	log := &MockLogger{}
	metric := &MockMetrics{}
	component := NewHubComponent("test-hub", metric, WithSize(4))

	ctx, cancel := context.WithTimeout(context.Background(), 3 * time.Second)
	defer cancel()

	sc := &MockServiceContext{logger: log}
	component.Activate(ctx, sc)

	time.Sleep(100 * time.Millisecond)

	hub := component.GetHub()

	// Create multiple jobs via hub
	for i := 0; i < 5; i++ {
		handler := &SimpleJobHandler{
			name:  "job",
			delay: 50 * time.Millisecond,
		}
		j, err := hub.Create("test", handler)
		if err != nil {
			t.Fatalf("Create job failed: %v", err)
		}

		ok := component.Submit(j)
		if !ok {
			t.Fatalf("Submit job %d failed", i)
		}
	}

	time.Sleep(400 * time.Millisecond)

	if metric.started < 5 {
		t.Fatalf("Expected at least 5 jobs started, got %d", metric.started)
	}
}

// TestComponentOrder tests component order
func TestComponentOrder(t *testing.T) {
	component := NewComponent("test", nil)
	order := component.Order()

	if order != 40 {
		t.Fatalf("Expected order 40, got %d", order)
	}
}

// TestHubComponentOrder tests hub component order
func TestHubComponentOrder(t *testing.T) {
	component := NewHubComponent("test", nil)
	order := component.Order()

	if order != 40 {
		t.Fatalf("Expected order 40, got %d", order)
	}
}

// TestComponentInitFlags tests InitFlags
func TestComponentInitFlags(t *testing.T) {
	component := NewComponent("test", nil)
	// Should not panic
	component.InitFlags()
}

// TestHubComponentInitFlags tests hub component InitFlags
func TestHubComponentInitFlags(t *testing.T) {
	component := NewHubComponent("test", nil)
	// Should not panic
	component.InitFlags()
}
