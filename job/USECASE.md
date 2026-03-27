# Job Package - Real-world Use Cases & Concepts

## Khái Niệm Cơ Bản

### Job là gì?

**Job** là một đơn vị công việc bất đồng bộ (asynchronous work unit) có các tính chất:

1. **Thực thi độc lập**: Job chạy trong goroutine riêng, không chặn caller
2. **Có lifecycle**: Trải qua các state (Init → Running → Completed/Failed)
3. **Hỗ trợ retry tự động**: Có thể tự động thực hiện lại nếu thất bại
4. **Timeout management**: Ngăn chặn job chạy vô hạn
5. **Callback system**: Có hook cho các sự kiện trong lifecycle

### Tại sao cần Job?

Trong các hệ thống phân tán, nhiều tác vụ:
- Không cần kết quả ngay lập tức
- Có thể thất bại vì network, external service
- Cần được thử lại một số lần
- Cần tracking trạng thái

**Job package cung cấp:**
- Cách uniform để xử lý tất cả những thứ này
- Thread-safety tích hợp
- Hỗ trợ concurrency qua Hub

---

## Use Case 1: Email Notifications

### Tình Huống
- Web application cần gửi email xác nhận đăng ký
- SMTP server đôi khi bị chậm hoặc tạm thời không khả dụng
- User không nên chờ 30 giây để email gửi đi

### Giải Pháp với Job

```go
type EmailNotifier struct {
    UserEmail string
    Subject   string
    Body      string
    SMTPAddr  string
}

func (n *EmailNotifier) Handle(ctx context.Context) error {
    // Với timeout 30s
    client, err := smtp.Dial(n.SMTPAddr)
    if err != nil {
        return fmt.Errorf("smtp connection failed: %w", err)
    }
    defer client.Close()

    if err := client.SendMail(n.UserEmail, []string{n.UserEmail}, []byte(n.Body)); err != nil {
        return fmt.Errorf("send failed: %w", err)
    }
    return nil
}

func (n *EmailNotifier) Type() string { return "email" }

// Sử dụng trong registration handler
func RegisterUser(w http.ResponseWriter, r *http.Request) {
    user := parseRequest(r)
    saveUser(user)

    // Tạo email job (không chặn)
    emailJob, _ := hub.Create("email", &EmailNotifier{
        UserEmail: user.Email,
        Subject:   "Welcome!",
        Body:      "Welcome to our platform",
    },
        job.WithTimeout(30 * time.Second),
        job.WithRetries([]time.Duration{
            1 * time.Second,   // Retry 1s sau
            5 * time.Second,   // Retry 5s sau
            1 * time.Minute,   // Cuối cùng retry 1 phút sau
        }),
        job.WithJitter(0.2),  // ±20% jitter
        job.WithOnPermanent(func(err error) {
            // Log failure, maybe alert
            log.Printf("Failed to send welcome email to %s: %v", user.Email, err)
        }),
    )

    hub.Submit(emailJob)

    // Response ngay, email sẽ được gửi sau
    w.WriteJSON(map[string]string{"status": "registered"})
}
```

### Lợi Ích
✅ User nhận response trong 100ms  
✅ Email được gửi lại nếu thất bại  
✅ SMTP outage không crash application  
✅ Multiple concurrent emails được handle

---

## Use Case 2: Database Migration & Cleanup

### Tình Huống
- Cần thực hiện long-running database operations
- Có thể bị timeout nếu chạy synchronously
- Nếu fail, cần retry từ điểm thích hợp

### Giải Pháp

```go
type DatabaseCleanupJob struct {
    DB        *sql.DB
    TableName string
    OlderThan time.Time
}

func (j *DatabaseCleanupJob) Handle(ctx context.Context) error {
    // Cleanup logic
    query := fmt.Sprintf(
        "DELETE FROM %s WHERE created_at < $1",
        j.TableName,
    )
    
    result, err := j.DB.ExecContext(ctx, query, j.OlderThan)
    if err != nil {
        return fmt.Errorf("cleanup failed: %w", err)
    }
    
    affected, _ := result.RowsAffected()
    log.Printf("Deleted %d records from %s", affected, j.TableName)
    return nil
}

func (j *DatabaseCleanupJob) Type() string { return "db-cleanup" }

// Scheduled daily cleanup
func ScheduleDailyCleanup(hub job.Hub) {
    ticker := time.NewTicker(24 * time.Hour)
    defer ticker.Stop()

    for range ticker.C {
        cleanupJob, _ := hub.Create("db-cleanup", 
            &DatabaseCleanupJob{
                DB:        db,
                TableName: "logs",
                OlderThan: time.Now().AddDate(0, -3, 0), // 3 months
            },
            job.WithTimeout(5 * time.Minute), // Long timeout
            job.WithRetries([]time.Duration{
                10 * time.Second,
                1 * time.Minute,
            }),
            job.WithOnComplete(func() {
                log.Println("Cleanup completed successfully")
            }),
            job.WithOnPermanent(func(err error) {
                alertOps("Database cleanup failed", err)
            }),
        )

        hub.Submit(cleanupJob)
    }
}
```

### Lợi Ích
✅ Không chặn main application thread  
✅ Timeout ngăn query chạy quá lâu  
✅ Automatic retry nếu transient failure  
✅ Observable via callbacks

---

## Use Case 3: Webhook Delivery

### Tình HuQuinn
- API muốn push events cho subscriber webhooks
- Subscriber endpoint có thể chậm, offline, hoặc lỗi
- Cần retry vài lần, nhưng không vô hạn

### Giải Pháp

```go
type WebhookDeliver struct {
    WebhookURL string
    Event      map[string]interface{}
    AttemptNum int
}

func (w *WebhookDeliver) Handle(ctx context.Context) error {
    payload, _ := json.Marshal(w.Event)
    
    req, _ := http.NewRequestWithContext(ctx, "POST", w.WebhookURL, bytes.NewReader(payload))
    req.Header.Set("Content-Type", "application/json")
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Retry nếu 5xx, không retry nếu 4xx
    if resp.StatusCode >= 500 {
        return fmt.Errorf("server error: %d", resp.StatusCode)
    }
    
    if resp.StatusCode >= 400 {
        return fmt.Errorf("client error: %d (no retry)", resp.StatusCode)
    }
    
    return nil
}

func (w *WebhookDeliver) Type() string { return "webhook" }

// Khi có event, tạo webhook job
func PublishEvent(hub job.Hub, event Event) {
    retries := []time.Duration{
        1 * time.Second,
        10 * time.Second,
        1 * time.Minute,
        5 * time.Minute,
    }
    
    for _, webhook := range getSubscribedWebhooks() {
        webhookJob, _ := hub.Create("webhook",
            &WebhookDeliver{
                WebhookURL: webhook.URL,
                Event:      event.ToMap(),
            },
            job.WithTimeout(10 * time.Second),
            job.WithRetries(retries),
            job.WithJitter(0.1),
            job.WithOnRetry(func(idx int, delay time.Duration, err error) {
                log.Printf("Webhook retry #%d for %s in %v", idx, webhook.URL, delay)
            }),
            job.WithOnPermanent(func(err error) {
                log.Printf("Webhook delivery failed for %s after retries: %v", 
                    webhook.URL, err)
            }),
        )
        
        hub.Submit(webhookJob)
    }
}
```

### Lợi Ích
✅ Asynchronous, subscriber timeout không crash event publisher  
✅ Exponential backoff không overwhelm failed webhook  
✅ Clear distinction giữa retriable vs non-retriable errors  
✅ Audit trail via callback logging

---

## Use Case 4: External API Integration (Throttle & Retry)

### Tình Huống
- Gọi external API có rate limiting
- API đôi khi bị overload (503 Unavailable)
- Cần respect rate limits và retry

### Giải Pháp

```go
type APICallJob struct {
    URL    string
    Method string
    Body   []byte
}

var (
    apiLimiter = time.NewTicker(100 * time.Millisecond) // 10 req/sec
)

func (a *APICallJob) Handle(ctx context.Context) error {
    // Wait for rate limiter
    select {
    case <-apiLimiter.C:
        // Proceed
    case <-ctx.Done():
        return ctx.Err()
    }
    
    req, _ := http.NewRequestWithContext(ctx, a.Method, a.URL, bytes.NewReader(a.Body))
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("api call failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Handle rate limit
    if resp.StatusCode == http.StatusTooManyRequests {
        if retryAfter := resp.Header.Get("Retry-After"); retryAfter != "" {
            return fmt.Errorf("rate limited, retry after: %s", retryAfter)
        }
        return errors.New("rate limited")
    }
    
    if resp.StatusCode >= 500 {
        return fmt.Errorf("server error: %d", resp.StatusCode)
    }
    
    if resp.StatusCode >= 400 {
        return fmt.Errorf("client error: %d (no retry)", resp.StatusCode)
    }
    
    return nil
}

func (a *APICallJob) Type() string { return "api" }
```

### Lợi Ích
✅ Rate limiting built-in  
✅ Retry schedule tuned cho timeout  
✅ Respects server Retry-After header  
✅ Jitter prevents thundering herd

---

## Use Case 5: Batch Processing

### Tình Huống
- Xử lý dữ liệu từ file CSV (100k records)
- Mỗi record cần external API call
- Resource-intensive, cần distribute qua workers

### Giải Pháp

```go
type BatchProcessJob struct {
    RecordID int
    Data     map[string]string
}

func (b *BatchProcessJob) Handle(ctx context.Context) error {
    // Process single record
    return processRecord(ctx, b.RecordID, b.Data)
}

func (b *BatchProcessJob) Type() string { return "batch" }

func ProcessCSVFile(hub job.Hub, filePath string) {
    file, _ := os.Open(filePath)
    defer file.Close()
    
    reader := csv.NewReader(file)
    
    jobCount := 0
    for {
        record, err := reader.Read()
        if err == io.EOF {
            break
        }
        
        // Create job for each record
        recordData := map[string]string{
            "name": record[0],
            "email": record[1],
            // ...
        }
        
        job, _ := hub.Create("batch",
            &BatchProcessJob{
                RecordID: jobCount,
                Data:     recordData,
            },
            job.WithTimeout(30 * time.Second),
            job.WithRetries([]time.Duration{
                1 * time.Second,
                5 * time.Second,
            }),
        )
        
        hub.Submit(job)  // Worker pool controls concurrency
        jobCount++
    }
    
    log.Printf("Submitted %d batch jobs", jobCount)
}
```

### Lợi Ích
✅ Worker pool automatically manages concurrency  
✅ Each record processed independently  
✅ Failure of one record doesn't stop others  
✅ Progress tracking per record

---

## Real-world Architecture

### Typical Setup

```
┌─────────────────────────────────────────┐
│   Application Layer                      │
│  (HTTP handlers, services)               │
└──────────────┬──────────────────────────┘
               │ job.New() / hub.Create()
               ▼
┌─────────────────────────────────────────┐
│   Job Package                            │
│  - Config, Options, State Management    │
│  - Timeout, Retry Logic                 │
└──────────────┬──────────────────────────┘
               │ hub.Submit()
               ▼
┌─────────────────────────────────────────┐
│   Worker Pool (fcontext/worker)          │
│  - 10-100 concurrent workers             │
│  - Distributes jobs                      │
│  - Manages goroutines                    │
└──────────────┬──────────────────────────┘
               │ job.Execute()
               ▼
┌─────────────────────────────────────────┐
│   Handler Implementation                 │
│  (Email, Database, API, etc.)           │
│  - Does actual work                      │
│  - Returns error or success              │
└─────────────────────────────────────────┘
```

### Key Components Interaction

1. **Application Layer**: Creates jobs with business logic
2. **Job Package**: Manages lifecycle, retry, timeout
3. **Worker Pool**: Provides concurrency control
4. **Handlers**: Implements specific job types

---

## Performance Characteristics

### Throughput
- Ngân Job/second: Phụ thuộc vào worker pool size
- Pool size 10: ~100-1000 jobs/sec (phụ thuộc handler latency)
- Pool size 100: ~10x throughput

### Latency
- Job submission: <1ms (non-blocking enqueue)
- Job start: 0-100ms (depends on pool congestion)
- Retry start: schedule delay (configurable)

### Resource Usage
- Per job: ~1-2KB memory overhead
- Goroutine: 1 per active job + pool overhead
- No memory leak: State properly cleaned up

---

## Common Patterns & Anti-patterns

### ✅ Good Pattern: Exponential Backoff

```go
job.WithRetries([]time.Duration{
    1 * time.Second,      // 1s after first failure
    5 * time.Second,      // 5s after second failure
    25 * time.Second,     // 25s after third failure
    2 * time.Minute,      // 2m after fourth failure
})
```

### ❌ Bad Pattern: Too Many Retries

```go
// Don't do this - will retry for 20+ minutes!
retries := make([]time.Duration, 100)
for i := range retries {
    retries[i] = 100 * time.Millisecond
}
job.WithRetries(retries)
```

### ✅ Good Pattern: Distinguish Error Types

```go
func (j *Job) Handle(ctx context.Context) error {
    // Transient error - can retry
    if isNetworkError(err) {
        return err  // Will trigger retry
    }
    
    // Permanent error - don't retry
    if isValidationError(err) {
        return fmt.Errorf("validation failed (no retry): %w", err)
    }
}
```

### ❌ Bad Pattern: Retry Everything

```go
// Don't retry validation, auth, 404 errors
job.WithRetries([]time.Duration{1s, 5s, 10s})
```

---

## Monitoring & Observability

```go
type MetricsCollector struct {}

func (m *MetricsCollector) OnComplete() {
    metrics.jobsCompleted.Inc()
}

func (m *MetricsCollector) OnRetry(idx int, delay time.Duration, err error) {
    metrics.jobsRetried.Inc()
    metrics.retryDelayHistogram.Observe(delay.Seconds())
}

func (m *MetricsCollector) OnPermanent(err error) {
    metrics.jobsFailed.Inc()
    metrics.recordError(err)
}

// Use it
job := job.New(handler,
    job.WithOnComplete(collector.OnComplete),
    job.WithOnRetry(collector.OnRetry),
    job.WithOnPermanent(collector.OnPermanent),
)
```

---

## Summary

| Use Case | Key Features | Benefits |
|----------|-------------|----------|
| Email Notifications | Retry, Timeout, Non-blocking | Better UX, Resilience |
| DB Operations | Long timeout, Cleanup | Scale operations |
| Webhooks | Exponential backoff | Reliable delivery |
| API Integration | Rate limiting, Jitter | Respect downstream limits |
| Batch Processing | Pool concurrency | Handle large volumes |

**Job package is essential for any production system handling**:
- External API calls
- Long-running operations
- Unreliable networks
- High throughput scenarios
