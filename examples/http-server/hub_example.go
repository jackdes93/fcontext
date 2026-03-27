package main

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
	"github.com/jackdes93/fcontext/worker"
)

// ========== Job Types ==========

// EmailJob định nghĩa job để gửi email
type EmailJob struct {
	To      string
	Subject string
	Body    string
}

func (ej *EmailJob) Handle(ctx context.Context) error {
	log.Printf("Sending email to %s: %s", ej.To, ej.Subject)
	// Giả lập gửi email
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(500 * time.Millisecond):
		log.Printf("Email sent successfully to %s", ej.To)
		return nil
	}
}

func (ej *EmailJob) Type() string {
	return "email"
}

// DataProcessingJob định nghĩa job xử lý dữ liệu
type DataProcessingJob struct {
	Data map[string]interface{}
}

func (dj *DataProcessingJob) Handle(ctx context.Context) error {
	log.Printf("Processing data: %v", dj.Data)
	// Giả lập xử lý dữ liệu
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(1 * time.Second):
		log.Printf("Data processed successfully")
		return nil
	}
}

func (dj *DataProcessingJob) Type() string {
	return "data-processing"
}

// NotificationJob định nghĩa job gửi notification
type NotificationJob struct {
	Channel string
	Message string
}

func (nj *NotificationJob) Handle(ctx context.Context) error {
	log.Printf("Sending notification to %s: %s", nj.Channel, nj.Message)
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-time.After(200 * time.Millisecond):
		log.Printf("Notification sent to %s", nj.Channel)
		return nil
	}
}

func (nj *NotificationJob) Type() string {
	return "notification"
}

// ========== Example Usage ==========

// MockLogger đơn giản để test
type MockLogger struct{}

func (ml *MockLogger) WithPrefix(prefix string) fcontext.Logger { return ml }
func (ml *MockLogger) Info(msg string, args ...interface{})    { log.Printf("[INFO] "+msg, args...) }
func (ml *MockLogger) Warn(msg string, args ...interface{})    { log.Printf("[WARN] "+msg, args...) }
func (ml *MockLogger) Debug(msg string, args ...interface{})   { log.Printf("[DEBUG] "+msg, args...) }
func (ml *MockLogger) Error(msg string, args ...interface{})   { log.Printf("[ERROR] "+msg, args...) }

// MockMetrics đơn giản để test
type MockMetrics struct{}

func (mm *MockMetrics) IncJobStarted(name string)                                   {}
func (mm *MockMetrics) IncJobSuccess(name string, latency time.Duration)            {}
func (mm *MockMetrics) IncJobFailed(name string, err error, latency time.Duration) {}
func (mm *MockMetrics) IncJobPermanentFailed(name string, err error)                {}

func main() {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Tạo pool với 4 workers
	logger := &MockLogger{}
	metrics := &MockMetrics{}
	
	pool := worker.NewPool(logger, metrics,
		worker.WithName("example-hub"),
		worker.WithSize(4),
		worker.WithQueueSize(100),
	)

	// Tạo hub
	hub := job.NewHub(func(j job.Job) bool {
		return pool.Submit(j)
	})

	// Chạy pool trong background
	go pool.Run(ctx)

	// ========== Submit Jobs ==========

	// 1. Submit Email Job
	emailHandler := &EmailJob{
		To:      "user@example.com",
		Subject: "Hello",
		Body:    "This is a test email",
	}
	emailJob, err := hub.Create("email", emailHandler,
		job.WithName("send-email"),
		job.WithTimeout(5*time.Second),
		job.WithRetries([]time.Duration{1 * time.Second}),
	)
	if err == nil {
		hub.Submit(emailJob)
	}

	// 2. Submit Data Processing Job
	dataHandler := &DataProcessingJob{
		Data: map[string]interface{}{
			"id":   123,
			"name": "John Doe",
		},
	}
	dataJob, err := hub.Create("data-processing", dataHandler,
		job.WithName("process-data"),
		job.WithTimeout(10*time.Second),
		job.WithRetries([]time.Duration{
			1 * time.Second,
			2 * time.Second,
		}),
	)
	if err == nil {
		hub.Submit(dataJob)
	}

	// 3. Submit Notification Job
	notifHandler := &NotificationJob{
		Channel: "slack",
		Message: "Job completed successfully",
	}
	notifJob, err := hub.Create("notification", notifHandler,
		job.WithName("send-notification"),
		job.WithTimeout(5*time.Second),
	)
	if err == nil {
		hub.Submit(notifJob)
	}

	// Chờ jobs hoàn thành
	time.Sleep(5 * time.Second)

	// Stop pool
	stopCtx, stopCancel := context.WithTimeout(ctx, 5*time.Second)
	pool.Stop(stopCtx)
	stopCancel()
}
