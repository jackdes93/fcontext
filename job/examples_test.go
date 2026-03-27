package job

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"net/smtp"
	"os"
	"testing"
	"time"
)

// ============================================================================
// Example 1: Simple Email Sending
// ============================================================================

type EmailHandler struct {
	To      string
	Subject string
	Body    string
}

func (h *EmailHandler) Handle(ctx context.Context) error {
	// In real scenario, call actual SMTP service
	fmt.Printf("Sending email to %s with subject: %s\n", h.To, h.Subject)

	// Simulate network delay
	select {
	case <-time.After(100 * time.Millisecond):
		fmt.Printf("Email sent successfully to %s\n", h.To)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *EmailHandler) Type() string {
	return "email"
}

func ExampleSimpleEmailSending(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(context.Background())
		}()
		return true
	})

	handler := &EmailHandler{
		To:      "user@example.com",
		Subject: "Welcome",
		Body:    "Welcome to our service!",
	}

	emailJob, _ := hub.Create("email", handler,
		WithName("welcome-email"),
		WithTimeout(5 * time.Second),
		WithOnComplete(func() {
			fmt.Println("[Callback] Email sent successfully")
		}),
		WithOnPermanent(func(err error) {
			fmt.Printf("[Callback] Failed to send email: %v\n", err)
		}),
	)

	hub.Submit(emailJob)

	time.Sleep(300 * time.Millisecond)

	if emailJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", emailJob.State())
	}

	fmt.Println("✓ Example 1 passed: Simple email sending")
}

// ============================================================================
// Example 2: Retry with Exponential Backoff
// ============================================================================

type UnreliableAPIHandler struct {
	Endpoint  string
	AttemptCount int
}

func (h *UnreliableAPIHandler) Handle(ctx context.Context) error {
	h.AttemptCount++

	// First 2 attempts fail, third succeeds
	if h.AttemptCount <= 2 {
		return errors.New("connection timeout (simulated)")
	}

	fmt.Printf("API call succeeded on attempt %d\n", h.AttemptCount)
	return nil
}

func (h *UnreliableAPIHandler) Type() string {
	return "api"
}

func ExampleRetryWithBackoff(t *testing.T) {
	retryCount := 0

	hub := NewHub(func(j Job) bool {
		go func() {
			j.RunWithRetry(context.Background())
		}()
		return true
	})

	handler := &UnreliableAPIHandler{
		Endpoint: "https://api.example.com/data",
	}

	apiJob, _ := hub.Create("api", handler,
		WithName("unreliable-api-call"),
		WithTimeout(10 * time.Second),
		WithRetries([]time.Duration{
			100 * time.Millisecond,  // Retry 100ms later
			500 * time.Millisecond,  // Then 500ms later
			1 * time.Second,         // Then 1s later
		}),
		WithJitter(0.2),  // ±20% random jitter
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			retryCount++
			fmt.Printf("[Retry %d] Will retry after %v due to: %v\n", idx, delay, err)
		}),
		WithOnComplete(func() {
			fmt.Printf("✓ API call eventually succeeded after %d retries\n", retryCount)
		}),
	)

	hub.Submit(apiJob)

	time.Sleep(3 * time.Second)

	if apiJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", apiJob.State())
	}

	if retryCount == 0 {
		t.Fatal("Expected at least one retry")
	}

	fmt.Println("✓ Example 2 passed: Retry with exponential backoff")
}

// ============================================================================
// Example 3: Timeout Handling
// ============================================================================

type SlowOperationHandler struct {
	Duration time.Duration
}

func (h *SlowOperationHandler) Handle(ctx context.Context) error {
	fmt.Printf("Starting slow operation (will take %v)...\n", h.Duration)

	select {
	case <-time.After(h.Duration):
		fmt.Println("Slow operation completed")
		return nil
	case <-ctx.Done():
		fmt.Printf("Slow operation cancelled: %v\n", ctx.Err())
		return ctx.Err()
	}
}

func (h *SlowOperationHandler) Type() string {
	return "slow-op"
}

func ExampleTimeoutHandling(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(context.Background())
		}()
		return true
	})

	// Handler will take 2 seconds, but timeout is 500ms
	handler := &SlowOperationHandler{
		Duration: 2 * time.Second,
	}

	slowJob, _ := hub.Create("slow-op", handler,
		WithName("timeout-test"),
		WithTimeout(500 * time.Millisecond),  // Timeout!
		WithOnPermanent(func(err error) {
			fmt.Printf("[Timeout] Operation failed: %v\n", err)
		}),
	)

	hub.Submit(slowJob)

	time.Sleep(1 * time.Second)

	if slowJob.State() != StateTimeout {
		t.Fatalf("Expected StateTimeout, got %v", slowJob.State())
	}

	fmt.Println("✓ Example 3 passed: Timeout handling")
}

// ============================================================================
// Example 4: Multiple Job Types with Hub
// ============================================================================

type DatabaseQueryHandler struct {
	Query string
}

func (h *DatabaseQueryHandler) Handle(ctx context.Context) error {
	fmt.Printf("Executing query: %s\n", h.Query)

	select {
	case <-time.After(50 * time.Millisecond):
		fmt.Printf("Query executed successfully\n")
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *DatabaseQueryHandler) Type() string {
	return "database"
}

func ExampleMultipleJobTypes(t *testing.T) {
	completedJobs := 0

	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(context.Background())
			if j.State() == StateCompleted {
				completedJobs++
			}
		}()
		return true
	})

	// Create email job
	emailJob, _ := hub.Create("email", &EmailHandler{
		To:      "admin@example.com",
		Subject: "Report",
		Body:    "Daily report",
	},
		WithName("daily-report-email"),
		WithTimeout(5 * time.Second),
	)

	// Create database job
	dbJob, _ := hub.Create("database", &DatabaseQueryHandler{
		Query: "SELECT * FROM users WHERE active=true",
	},
		WithName("fetch-active-users"),
		WithTimeout(5 * time.Second),
	)

	// Submit both
	hub.Submit(emailJob)
	hub.Submit(dbJob)

	time.Sleep(500 * time.Millisecond)

	if emailJob.State() != StateCompleted || dbJob.State() != StateCompleted {
		t.Fatal("Both jobs should be completed")
	}

	fmt.Printf("✓ Completed %d jobs of different types\n", completedJobs)
	fmt.Println("✓ Example 4 passed: Multiple job types")
}

// ============================================================================
// Example 5: Error Classification (Retriable vs Permanent)
// ============================================================================

type PaymentProcessHandler struct {
	OrderID string
	Amount  float64
	Retry   int
}

func (h *PaymentProcessHandler) Handle(ctx context.Context) error {
	fmt.Printf("Processing payment for order %s (attempt %d)\n", h.OrderID, h.Retry+1)

	// Simulate different error scenarios
	switch h.Retry {
	case 0:
		// Network timeout - retriable
		return errors.New("network timeout")
	case 1:
		// Still failing - retriable
		return errors.New("payment gateway temporarily unavailable")
	case 2:
		// Success on third attempt
		fmt.Printf("✓ Payment successfully processed\n")
		return nil
	}

	return nil
}

func (h *PaymentProcessHandler) Type() string {
	return "payment"
}

func ExampleErrorClassification(t *testing.T) {
	retries := 0

	hub := NewHub(func(j Job) bool {
		go func() {
			j.RunWithRetry(context.Background())
		}()
		return true
	})

	handler := &PaymentProcessHandler{
		OrderID: "ORDER-12345",
		Amount:  99.99,
	}

	paymentJob, _ := hub.Create("payment", handler,
		WithName("process-payment"),
		WithTimeout(30 * time.Second),
		WithRetries([]time.Duration{
			100 * time.Millisecond,
			500 * time.Millisecond,
			1 * time.Second,
		}),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			retries++
			handler.Retry = idx + 1
			fmt.Printf("[Retry] Attempt #%d failed: %v (retry in %v)\n",
				idx, err, delay)
		}),
		WithOnComplete(func() {
			fmt.Printf("✓ Payment completed after %d retries\n", retries)
		}),
		WithOnPermanent(func(err error) {
			fmt.Printf("✗ Payment failed permanently: %v\n", err)
		}),
	)

	hub.Submit(paymentJob)

	time.Sleep(3 * time.Second)

	if paymentJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", paymentJob.State())
	}

	fmt.Println("✓ Example 5 passed: Error classification")
}

// ============================================================================
// Example 6: Concurrent Job Execution
// ============================================================================

type NotificationHandler struct {
	UserID   int
	Message  string
	Delay    time.Duration
}

func (h *NotificationHandler) Handle(ctx context.Context) error {
	fmt.Printf("[User %d] Sending notification: %s\n", h.UserID, h.Message)

	select {
	case <-time.After(h.Delay):
		fmt.Printf("[User %d] Notification sent\n", h.UserID)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *NotificationHandler) Type() string {
	return "notification"
}

func ExampleConcurrentExecution(t *testing.T) {
	completedCount := 0

	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(context.Background())
			if j.State() == StateCompleted {
				completedCount++
			}
		}()
		return true
	})

	// Create 5 concurrent notification jobs
	for i := 1; i <= 5; i++ {
		handler := &NotificationHandler{
			UserID:  i,
			Message: fmt.Sprintf("Welcome user %d!", i),
			Delay:   time.Duration(100*i) * time.Millisecond,
		}

		job, _ := hub.Create("notification", handler,
			WithName(fmt.Sprintf("notify-user-%d", i)),
			WithTimeout(10 * time.Second),
		)

		hub.Submit(job)
	}

	time.Sleep(2 * time.Second)

	if completedCount != 5 {
		t.Fatalf("Expected 5 completed jobs, got %d", completedCount)
	}

	fmt.Printf("✓ Successfully executed %d concurrent jobs\n", completedCount)
	fmt.Println("✓ Example 6 passed: Concurrent execution")
}

// ============================================================================
// Example 7: Webhook Delivery with Retry
// ============================================================================

type WebhookDeliveryHandler struct {
	URL     string
	Payload interface{}
}

func (h *WebhookDeliveryHandler) Handle(ctx context.Context) error {
	fmt.Printf("Delivering webhook to %s\n", h.URL)

	// Prepare JSON payload
	data, _ := json.Marshal(h.Payload)

	// Create request
	req, _ := http.NewRequestWithContext(ctx, "POST", h.URL, nil)
	req.Header.Set("Content-Type", "application/json")

	// Note: In real scenario, would make actual HTTP call
	// For example: resp, err := http.DefaultClient.Do(req)

	// Simulate delivery
	select {
	case <-time.After(200 * time.Millisecond):
		fmt.Printf("✓ Webhook delivered: %d bytes\n", len(data))
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *WebhookDeliveryHandler) Type() string {
	return "webhook"
}

func ExampleWebhookDelivery(t *testing.T) {
	deliveryAttempts := 0

	hub := NewHub(func(j Job) bool {
		go func() {
			j.RunWithRetry(context.Background())
		}()
		return true
	})

	payload := map[string]interface{}{
		"event":      "user.created",
		"user_id":    12345,
		"timestamp":  time.Now().Unix(),
	}

	handler := &WebhookDeliveryHandler{
		URL:     "https://subscriber.example.com/webhook",
		Payload: payload,
	}

	webhookJob, _ := hub.Create("webhook", handler,
		WithName("send-webhook"),
		WithTimeout(10 * time.Second),
		WithRetries([]time.Duration{
			1 * time.Second,
			5 * time.Second,
			1 * time.Minute,
		}),
		WithJitter(0.1),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			deliveryAttempts++
			fmt.Printf("[Webhook Retry] Attempt #%d. Next retry in %v\n",
				idx, delay)
		}),
	)

	hub.Submit(webhookJob)

	time.Sleep(1 * time.Second)

	if webhookJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", webhookJob.State())
	}

	fmt.Printf("✓ Webhook delivered after %d attempts\n", deliveryAttempts)
	fmt.Println("✓ Example 7 passed: Webhook delivery")
}

// ============================================================================
// Example 8: Long-running Batch Job
// ============================================================================

type FilesyncHandler struct {
	FileCount int
	BatchSize int
}

func (h *FilesyncHandler) Handle(ctx context.Context) error {
	fmt.Printf("Starting file sync for %d files (batch size: %d)\n",
		h.FileCount, h.BatchSize)

	processed := 0
	for processed < h.FileCount {
		select {
		case <-ctx.Done():
			fmt.Printf("Sync cancelled after %d files\n", processed)
			return ctx.Err()
		case <-time.After(100 * time.Millisecond):
			processed += h.BatchSize
			if processed > h.FileCount {
				processed = h.FileCount
			}
			fmt.Printf("  Processed: %d/%d files\n", processed, h.FileCount)
		}
	}

	fmt.Printf("✓ File sync completed\n")
	return nil
}

func (h *FilesyncHandler) Type() string {
	return "filesync"
}

func ExampleLongRunningBatch(t *testing.T) {
	hub := NewHub(func(j Job) bool {
		go func() {
			j.Execute(context.Background())
		}()
		return true
	})

	handler := &FilesyncHandler{
		FileCount: 1000,
		BatchSize: 100,
	}

	syncJob, _ := hub.Create("filesync", handler,
		WithName("bulk-file-sync"),
		WithTimeout(30 * time.Second),  // Long timeout for batch
		WithRetries([]time.Duration{
			10 * time.Second,
			30 * time.Second,
		}),
		WithOnComplete(func() {
			fmt.Println("✓ Batch job completed successfully")
		}),
	)

	hub.Submit(syncJob)

	time.Sleep(3 * time.Second)

	if syncJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", syncJob.State())
	}

	fmt.Println("✓ Example 8 passed: Long-running batch job")
}

// ============================================================================
// Example 9: Circuit Breaker Pattern with Jobs
// ============================================================================

type CircuitState int

const (
	Closed   CircuitState = iota
	Open
	HalfOpen
)

type ExternalServiceJobHandler struct {
	ServiceName string
	State       CircuitState
}

func (h *ExternalServiceJobHandler) Handle(ctx context.Context) error {
	switch h.State {
	case Open:
		return errors.New("circuit breaker is OPEN")
	case HalfOpen:
		fmt.Printf("Circuit half-open for %s, attempting recovery\n", h.ServiceName)
	case Closed:
		fmt.Printf("Circuit closed for %s, service should be healthy\n", h.ServiceName)
	}

	// Simulate service call
	select {
	case <-time.After(50 * time.Millisecond):
		fmt.Printf("✓ %s responded successfully\n", h.ServiceName)
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (h *ExternalServiceJobHandler) Type() string {
	return "service-check"
}

func ExampleCircuitBreaker(t *testing.T) {
	// This example shows how to implement circuit breaker on top of job system
	
	hub := NewHub(func(j Job) bool {
		go func() {
			j.RunWithRetry(context.Background())
		}()
		return true
	})

	handler := &ExternalServiceJobHandler{
		ServiceName: "payment-gateway",
		State:       Closed,
	}

	checkJob, _ := hub.Create("service-check", handler,
		WithName("payment-gateway-health-check"),
		WithTimeout(5 * time.Second),
		WithRetries([]time.Duration{
			1 * time.Second,
			5 * time.Second,
		}),
		WithOnRetry(func(idx int, delay time.Duration, err error) {
			fmt.Printf("Service health check failed: %v\n", err)
			// In real scenario, update circuit state here
			handler.State = HalfOpen
		}),
		WithOnPermanent(func(err error) {
			fmt.Printf("Service permanently failed: %v\n", err)
			// Switch to open state
			handler.State = Open
		}),
	)

	hub.Submit(checkJob)

	time.Sleep(1 * time.Second)

	if checkJob.State() != StateCompleted {
		t.Fatalf("Expected StateCompleted, got %v", checkJob.State())
	}

	fmt.Println("✓ Example 9 passed: Circuit breaker pattern")
}

// ============================================================================
// Example 10: Complete Workflow - E-commerce Order Processing
// ============================================================================

type OrderPaymentHandler struct {
	OrderID string
	Amount  float64
}

func (h *OrderPaymentHandler) Handle(ctx context.Context) error {
	fmt.Printf("[Order %s] Processing payment of $%.2f\n", h.OrderID, h.Amount)
	time.Sleep(50 * time.Millisecond)
	return nil
}

func (h *OrderPaymentHandler) Type() string {
	return "payment"
}

type OrderShippingHandler struct {
	OrderID string
	Items   int
}

func (h *OrderShippingHandler) Handle(ctx context.Context) error {
	fmt.Printf("[Order %s] Arranging shipment for %d items\n", h.OrderID, h.Items)
	time.Sleep(100 * time.Millisecond)
	return nil
}

func (h *OrderShippingHandler) Type() string {
	return "shipping"
}

type OrderNotificationHandler struct {
	OrderID string
	Message string
}

func (h *OrderNotificationHandler) Handle(ctx context.Context) error {
	fmt.Printf("[Order %s] Sending notification: %s\n", h.OrderID, h.Message)
	return nil
}

func (h *OrderNotificationHandler) Type() string {
	return "notification"
}

func ExampleOrderProcessing(t *testing.T) {
	completedJobs := map[string]bool{}

	hub := NewHub(func(j Job) bool {
		go func() {
			j.RunWithRetry(context.Background())
			if j.State() == StateCompleted {
				// Track completion
			}
		}()
		return true
	})

	orderID := "ORDER-2024-001"

	// 1. Process payment
	paymentJob, _ := hub.Create("payment",
		&OrderPaymentHandler{OrderID: orderID, Amount: 199.99},
		WithName("process-payment"),
		WithTimeout(30 * time.Second),
		WithRetries([]time.Duration{5 * time.Second, 10 * time.Second}),
		WithOnComplete(func() {
			fmt.Printf("[%s] ✓ Payment confirmed\n", orderID)
			completedJobs["payment"] = true
		}),
	)

	// 2. Arrange shipping
	shippingJob, _ := hub.Create("shipping",
		&OrderShippingHandler{OrderID: orderID, Items: 3},
		WithName("arrange-shipping"),
		WithTimeout(30 * time.Second),
		WithRetries([]time.Duration{5 * time.Second, 10 * time.Second}),
		WithOnComplete(func() {
			fmt.Printf("[%s] ✓ Shipping arranged\n", orderID)
			completedJobs["shipping"] = true
		}),
	)

	// 3. Send notification to customer
	notificationJob, _ := hub.Create("notification",
		&OrderNotificationHandler{OrderID: orderID, Message: "Your order is confirmed"},
		WithName("order-confirmation-email"),
		WithTimeout(10 * time.Second),
		WithRetries([]time.Duration{5 * time.Second, 10 * time.Second}),
		WithOnComplete(func() {
			fmt.Printf("[%s] ✓ Notification sent\n", orderID)
			completedJobs["notification"] = true
		}),
	)

	// Submit all jobs concurrently (they may run in parallel)
	fmt.Printf("Processing order %s...\n", orderID)
	hub.Submit(paymentJob)
	hub.Submit(shippingJob)
	hub.Submit(notificationJob)

	// Wait for all to complete
	time.Sleep(1 * time.Second)

	if len(completedJobs) != 3 {
		t.Fatalf("Expected 3 completed jobs, got %d", len(completedJobs))
	}

	fmt.Printf("✓ Order %s fully processed\n", orderID)
	fmt.Println("✓ Example 10 passed: Complete order workflow")
}

// ============================================================================
// Test Function to Run All Examples
// ============================================================================

func TestAllExamples(t *testing.T) {
	fmt.Println("\n" + "="*60)
	fmt.Println("Running Job Package Examples")
	fmt.Println("="*60 + "\n")

	examples := []struct {
		name string
		fn   func(t *testing.T)
	}{
		{"Example 1: Simple Email Sending", ExampleSimpleEmailSending},
		{"Example 2: Retry with Backoff", ExampleRetryWithBackoff},
		{"Example 3: Timeout Handling", ExampleTimeoutHandling},
		{"Example 4: Multiple Job Types", ExampleMultipleJobTypes},
		{"Example 5: Error Classification", ExampleErrorClassification},
		{"Example 6: Concurrent Execution", ExampleConcurrentExecution},
		{"Example 7: Webhook Delivery", ExampleWebhookDelivery},
		{"Example 8: Long-running Batch", ExampleLongRunningBatch},
		{"Example 9: Circuit Breaker", ExampleCircuitBreaker},
		{"Example 10: Order Processing", ExampleOrderProcessing},
	}

	for _, ex := range examples {
		fmt.Printf("\n%s\n", ex.name)
		fmt.Println("-" * 60)
		ex.fn(t)
	}

	fmt.Println("\n" + "="*60)
	fmt.Println("✓ All examples completed successfully!")
	fmt.Println("="*60 + "\n")
}
