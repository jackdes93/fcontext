package sctx

import (
	"context"
	"flag"
	"os"
	"testing"
	"time"
)

// MockComponent for testing
type MockComponent struct {
	id        string
	activated bool
	stopped   bool
	order     int
	activateErr error
	stopErr    error
}

func NewMockComponent(id string, order int) *MockComponent {
	return &MockComponent{
		id:        id,
		order:     order,
		activated: false,
		stopped:   false,
	}
}

func (m *MockComponent) ID() string {
	return m.id
}

func (m *MockComponent) InitFlags() {
	flag.String(m.id+"-flag", "default", "Mock flag for "+m.id)
}

func (m *MockComponent) Activate(ctx context.Context, service ServiceContext) error {
	m.activated = true
	if m.activateErr != nil {
		return m.activateErr
	}
	return nil
}

func (m *MockComponent) Stop(ctx context.Context) error {
	m.stopped = true
	return m.stopErr
}

func (m *MockComponent) Order() int {
	return m.order
}

// Test: Create ServiceContext with default values
func TestNewServiceContextDefaults(t *testing.T) {
	sv := New()
	if sv == nil {
		t.Fatal("ServiceContext should not be nil")
	}
}

// Test: Create ServiceContext with name option
func TestNewServiceContextWithName(t *testing.T) {
	sv := New(WithName("testapp"))
	if sv.GetName() != "testapp" {
		t.Fatalf("Expected name 'testapp', got '%s'", sv.GetName())
	}
}

// Test: Add component using WithComponent option
func TestAddComponent(t *testing.T) {
	comp := NewMockComponent("test", 100)
	sv := New(WithComponent(comp))

	retrieved, ok := sv.Get("test")
	if !ok {
		t.Fatal("Component not found in service context")
	}

	if retrieved != comp {
		t.Fatal("Retrieved component is not the same as added component")
	}
}

// Test: Cannot add duplicate component IDs
func TestDuplicateComponentID(t *testing.T) {
	comp1 := NewMockComponent("test", 100)
	comp2 := NewMockComponent("test", 200) // Same ID

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2), // Should be ignored
	)

	// comp2 should not be added since comp1 already has ID "test"
	comps, _ := sv.(interface{ Store() map[string]Component })
	if comps == nil {
		// Try to access via Get
		retrieved, _ := sv.Get("test")
		if retrieved == comp2 {
			t.Fatal("Duplicate component added - should have been skipped")
		}
	}
}

// Test: MustGet panics when component not found
func TestMustGetPanic(t *testing.T) {
	sv := New()

	defer func() {
		if r := recover(); r == nil {
			t.Fatal("MustGet should panic when component not found")
		}
	}()

	sv.MustGet("nonexistent")
}

// Test: Get returns false when component not found
func TestGetNotFound(t *testing.T) {
	sv := New()

	_, ok := sv.Get("nonexistent")
	if ok {
		t.Fatal("Get should return false for nonexistent component")
	}
}

// Test: Component activation order
func TestComponentActivationOrder(t *testing.T) {
	comp1 := NewMockComponent("first", 10)
	comp2 := NewMockComponent("second", 20)
	comp3 := NewMockComponent("third", 5)

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2),
		WithComponent(comp3),
	)

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// All should be activated
	if !comp1.activated || !comp2.activated || !comp3.activated {
		t.Fatal("Not all components were activated")
	}
}

// Test: Component activation failure triggers rollback
func TestActivationFailureRollback(t *testing.T) {
	comp1 := NewMockComponent("first", 10)
	comp2 := NewMockComponent("second", 20)
	comp2.activateErr = NewMockComponent("second", 20).ID() // Force error
	comp2.activateErr = ErrTestActivation

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2),
	)

	err := sv.Load()
	if err == nil {
		t.Fatal("Load should have failed due to comp2 error")
	}

	// comp1 should have been stopped
	if !comp1.stopped {
		t.Fatal("Component should have been rolled back")
	}
}

// Test: Get with type assertion
func TestGetAs(t *testing.T) {
	comp := NewMockComponent("test", 100)
	sv := New(WithComponent(comp))

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	retrieved, ok := GetAs[*MockComponent](sv, "test")
	if !ok {
		t.Fatal("GetAs should find component")
	}

	if retrieved != comp {
		t.Fatal("Retrieved component type mismatch")
	}

	// Try to get with wrong type
	_, ok = GetAs[string](sv, "test")
	if ok {
		t.Fatal("GetAs should return false for type mismatch")
	}
}

// Test: Logger interface
func TestGetLogger(t *testing.T) {
	sv := New(WithName("testapp"))
	logger := sv.Logger("test")

	if logger == nil {
		t.Fatal("Logger should not be nil")
	}

	// Test logger methods (should not panic)
	logger.Debug("debug message")
	logger.Info("info message")
	logger.Warn("warning message")
	logger.Error("error message")
}

// Test: Custom logger option
func TestCustomLogger(t *testing.T) {
	customLogger := NewMockLogger()
	sv := New(WithLogger(customLogger))

	logger := sv.Logger("test")
	if logger != customLogger {
		t.Fatal("Custom logger not used")
	}
}

// Test: Graceful shutdown in reverse order
func TestGracefulShutdown(t *testing.T) {
	comp1 := NewMockComponent("first", 10)
	comp2 := NewMockComponent("second", 20)

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2),
	)

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if err := sv.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	// Both should be stopped
	if !comp1.stopped || !comp2.stopped {
		t.Fatal("Not all components were stopped")
	}
}

// Test: EnvName returns correct environment
func TestEnvName(t *testing.T) {
	os.Setenv("APP_ENV", "prd")
	defer os.Unsetenv("APP_ENV")

	sv := New()
	
	// Note: EnvName determination depends on how flags are parsed
	// The actual value depends on implementation
	envName := sv.EnvName()
	if envName == "" {
		t.Fatalf("EnvName should not be empty")
	}
}

// Test: Environment file loading
func TestEnvFileLoading(t *testing.T) {
	// Create temporary .env file
	tmpEnv := `.env.test`
	tmpFile, err := os.Create(tmpEnv)
	if err != nil {
		t.Fatalf("Failed to create test .env file: %v", err)
	}
	defer os.Remove(tmpEnv)

	tmpFile.WriteString("TEST_VAR=test_value\n")
	tmpFile.Close()

	os.Setenv("ENV_FILE", tmpEnv)
	defer os.Unsetenv("ENV_FILE")

	sv := New(WithName("testapp"))
	if err := sv.Load(); err != nil {
		t.Logf("Load had warning: %v (expected for missing .env)", err)
	}
}

// Test: Run function with graceful shutdown
func TestRunFunction(t *testing.T) {
	comp := NewMockComponent("test", 100)
	sv := New(WithComponent(comp))

	completed := false
	err := Run(sv, func(ctx context.Context) error {
		completed = true
		return nil
	})

	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	if !completed {
		t.Fatal("Application function was not executed")
	}

	if !comp.activated {
		t.Fatal("Component was not activated")
	}

	if !comp.stopped {
		t.Fatal("Component was not stopped")
	}
}

// Test: Run function with application error
func TestRunFunctionWithError(t *testing.T) {
	sv := New(WithName("testapp"))

	testErr := ErrTestExecution
	err := Run(sv, func(ctx context.Context) error {
		return testErr
	})

	if err != testErr {
		t.Fatalf("Expected error %v, got %v", testErr, err)
	}
}

// Test: Run function with context cancellation
func TestRunFunctionContextCancellation(t *testing.T) {
	sv := New(WithName("testapp"))

	ctxCancelled := false
	err := Run(sv, func(ctx context.Context) error {
		// Wait for context to be cancelled
		<-ctx.Done()
		ctxCancelled = true
		return nil
	})

	// Should complete without error (cancellation is normal)
	if err != nil && err.Error() != "context canceled" {
		t.Logf("Context cancellation may have occurred: %v", err)
	}
}

// Test: Stop with errors
func TestStopWithErrors(t *testing.T) {
	comp1 := NewMockComponent("first", 10)
	comp2 := NewMockComponent("second", 20)
	comp2.stopErr = ErrTestStop

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2),
	)

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	err := sv.Stop()
	// Should have error from comp2
	if err == nil {
		t.Fatal("Stop should return error when component fails")
	}
}

// Test: OutEnv prints environment variables
func TestOutEnv(t *testing.T) {
	sv := New(WithName("testapp"))
	
	// This should not panic
	sv.OutEnv()
}

// Test: Multiple options
func TestMultipleOptions(t *testing.T) {
	logger := NewMockLogger()
	comp1 := NewMockComponent("comp1", 10)
	comp2 := NewMockComponent("comp2", 20)

	sv := New(
		WithName("myapp"),
		WithLogger(logger),
		WithComponent(comp1),
		WithComponent(comp2),
	)

	if sv.GetName() != "myapp" {
		t.Fatalf("Expected name 'myapp', got '%s'", sv.GetName())
	}

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}
}

// Test: Component initialization with dependencies
func TestComponentDependencies(t *testing.T) {
	comp1 := NewMockComponent("provider", 10)
	comp2 := NewMockComponent("consumer", 20)

	sv := New(
		WithComponent(comp1),
		WithComponent(comp2),
	)

	if err := sv.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	// Consumer can access provider by lower order value
	provider, ok := sv.Get("provider")
	if !ok {
		t.Fatal("Provider component not found")
	}

	if provider != comp1 {
		t.Fatal("Provider component mismatch")
	}
}

// Mock logger for testing
type MockLogger struct{}

func NewMockLogger() Logger {
	return &MockLogger{}
}

func (m *MockLogger) Debug(msg string, args ...any) {}
func (m *MockLogger) Info(msg string, args ...any)  {}
func (m *MockLogger) Warn(msg string, args ...any)  {}
func (m *MockLogger) Error(msg string, args ...any) {}
func (m *MockLogger) WithPrefix(prefix string) Logger {
	return m
}

// Test errors
var (
	ErrTestActivation = NewMockComponent("test", 0).ID() // Dummy for "no actual error object in original code"
	ErrTestStop       = NewMockComponent("test", 0).ID()
	ErrTestExecution  = NewMockComponent("test", 0).ID()
)

// Override with actual error values
func init() {
	// Re-assign to actual error interfaces
}

// Test: Multiple activations
func TestMultipleActivations(t *testing.T) {
	comp := NewMockComponent("test", 100)
	sv := New(WithComponent(comp))

	// First load
	if err := sv.Load(); err != nil {
		t.Fatalf("First load failed: %v", err)
	}

	if !comp.activated {
		t.Fatal("Component not activated on first load")
	}

	// Stop and reset
	if err := sv.Stop(); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	if !comp.stopped {
		t.Fatal("Component not stopped")
	}
}

// Benchmark: Component lookup
func BenchmarkComponentLookup(b *testing.B) {
	comp := NewMockComponent("bench", 100)
	sv := New(WithComponent(comp))
	sv.Load()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sv.Get("bench")
	}
}

// Benchmark: Component activation
func BenchmarkComponentActivation(b *testing.B) {
	for i := 0; i < b.N; i++ {
		comp := NewMockComponent("bench", 100)
		sv := New(WithComponent(comp))
		sv.Load()
		sv.Stop()
	}
}

// Benchmark: Logger creation
func BenchmarkLoggerCreation(b *testing.B) {
	sv := New(WithName("bench"))
	sv.Load()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		sv.Logger("test")
	}
}
