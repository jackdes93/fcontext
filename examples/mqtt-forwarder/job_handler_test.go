package main

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/jackdes93/fcontext/job"
)

func TestJobHandler(t *testing.T) {
	// Mock storage
	mockStorage := &mockStorageComponent{}

	// Mock logger
	mockLogger := &mockLogger{}

	// Create handler factory
	handler := CreateForwardMessageHandler(mockStorage, mockLogger)

	tests := []struct {
		name    string
		topic   string
		payload []byte
		wantErr bool
	}{
		{
			name:    "valid_json_payload",
			topic:   "sensor/temperature",
			payload: []byte(`{"value": 25.5, "unit": "C"}`),
			wantErr: false,
		},
		{
			name:    "invalid_json_payload",
			topic:   "sensor/temperature",
			payload: []byte(`invalid json`),
			wantErr: false, // Should not error, just log warning
		},
		{
			name:    "empty_payload",
			topic:   "sensor/temperature",
			payload: []byte(``),
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			j := handler(tt.topic, tt.payload)

			if j == nil {
				t.Fatalf("handler returned nil job")
			}

			// Reset mock
			mockStorage.saveCount = 0

			// Execute job
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()

			err := j.Execute(ctx)

			if (err != nil) != tt.wantErr {
				t.Errorf("Execute() error = %v, wantErr %v", err, tt.wantErr)
			}

			// Verify storage was called
			if mockStorage.saveCount == 0 {
				t.Errorf("SaveMessage was not called")
			}

			if mockStorage.lastTopic != tt.topic {
				t.Errorf("SaveMessage topic = %v, want %v", mockStorage.lastTopic, tt.topic)
			}
		})
	}
}

func TestJobRetry(t *testing.T) {
	retryCount := 0
	maxRetries := 3

	handler := func(ctx context.Context) error {
		retryCount++
		if retryCount < maxRetries {
			return fmt.Errorf("temporary error")
		}
		return nil
	}

	j := job.New(
		handler,
		job.WithRetries([]time.Duration{
			100 * time.Millisecond,
			100 * time.Millisecond,
		}),
		job.WithTimeout(5 * time.Second),
	)

	ctx := context.Background()
	err := j.RunWithRetry(ctx)

	if err != nil {
		t.Errorf("RunWithRetry() error = %v, want nil", err)
	}

	if retryCount != maxRetries {
		t.Errorf("handler called %d times, want %d", retryCount, maxRetries)
	}
}

// Mock implementations
type mockStorageComponent struct {
	saveCount int
	lastTopic string
	lastData  []byte
}

func (m *mockStorageComponent) SaveMessage(ctx context.Context, topic string, payload []byte) error {
	m.saveCount++
	m.lastTopic = topic
	m.lastData = payload
	return nil
}

func (m *mockStorageComponent) GetMessages(ctx context.Context, topic string, limit int) ([]interface{}, error) {
	return make([]interface{}, 0), nil
}

type mockLogger struct{}

func (m *mockLogger) Info(msg string, keysAndValues ...interface{})    {}
func (m *mockLogger) Debug(msg string, keysAndValues ...interface{})   {}
func (m *mockLogger) Warn(msg string, keysAndValues ...interface{})    {}
func (m *mockLogger) Error(msg string, keysAndValues ...interface{})   {}
func (m *mockLogger) InfoWithMap(msg string, data map[string]interface{}) {}
