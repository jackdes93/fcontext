# Worker Package - Real-World Use Cases

## Giới Thiệu

Worker Pool là thành phần **quan trọng** trong bất kỳ hệ thống scalable nào. Document này trình bày 6 use case thực tế với code mẫu và architectural patterns.

---

## Use Case 1: Email Queue System

### Bài Toán

```
User đăng ký → Gửi email xác nhận
Không thể gửi email ngay (chậm, không tin cậy)
→ Đưa vào queue, xử lý bất đồng bộ
```

### Architecture

```
HTTP Request             Worker Pool (8 workers)
  ↓                              ↓
Email Job ──→ Queue ──→ Worker 1 ─→ SMTP Server
                    └─→ Worker 2 ─→ SMTP Server
                    └─→ Worker 3 ─→ Retry Logic
                    └─→... (8 total)
```

### Implementation

```go
// EmailHandler - implements job.Handler
type EmailHandler struct {
    To       string
    Subject  string
    Body     string
    RetryMax int
}

func (h *EmailHandler) Handle(ctx context.Context) error {
    // Retry logic built-in to job package
    client := smtp.NewClient(smtpServer, 25)
    defer client.Close()
    
    return client.SendMail(h.To, h.Subject, h.Body)
}

func (h *EmailHandler) Type() string {
    return "email"
}

// Application setup
func SetupEmailWorker(app *fcontext.App) error {
    hub := app.Component("worker").(*worker.HubComponent).GetHub()
    hub.Register("email", &EmailHandler{})
    return nil
}

// HTTP handler
func handleSignup(hub job.Hub, w http.ResponseWriter, r *http.Request) {
    // Process signup
    user := parseSignup(r)
    
    // Queue email immediately
    emailJob, err := hub.Create("email", &EmailHandler{
        To:      user.Email,
        Subject: "Verify your email",
        Body:    generateVerificationEmail(user),
    },
        job.WithRetries([]time.Duration{
            5 * time.Second,
            30 * time.Second,
            2 * time.Minute,
        }),
        job.WithTimeout(30 * time.Second),
    )
    
    if err != nil {
        http.Error(w, "Failed to queue email", http.StatusInternalServerError)
        return
    }
    
    hub.Submit(emailJob)
    
    // Response immediately
    w.WriteJSON(map[string]string{
        "status": "pending",
        "message": "Check your email for verification link",
    })
}
```

### Benefits

| Metric | Before | After |
|--------|--------|-------|
| Response Time | 2-5 seconds (SMTP wait) | <100ms (queue put) |
| Throughput | ~10 req/sec | ~1000 req/sec |
| Reliability | Lost if process dies | Persisted in queue |
| User Experience | Slow signup | Fast, responsive UI |

### Key Patterns

- **Async Response** - Return immediately, process in background
- **Retry Strategy** - Exponential backoff for email failures
- **Timeout** - 30 second per email prevents hanging
- **Monitoring** - MetricsHook tracks send success rate

---

## Use Case 2: Database Batch Processing

### Bài Toán

```
Processing 100,000 records sequentially → 10 minutes
Processing with 16 workers → 40 seconds
```

### Architecture

```
Raw Data File
     ↓
Parser → BatchJob 1 (records 1-6250)    ┐
      → BatchJob 2 (records 6251-12500) ┤→ Worker Pool →  DB Inserts
      → BatchJob 3 (records 12501-18750) ┤
      → ... 16 jobs total              ┘
```

### Implementation

```go
// BatchInsertHandler - bulk database insert
type BatchInsertHandler struct {
    Records []map[string]interface{}
    Table   string
    BatchID string
}

func (h *BatchInsertHandler) Handle(ctx context.Context) error {
    db := ctx.Value("database").(*sql.DB)
    
    tx, err := db.BeginTx(ctx, nil)
    if err != nil {
        return fmt.Errorf("begin transaction: %w", err)
    }
    defer tx.Rollback()
    
    stmt, err := tx.Prepare(fmt.Sprintf(
        "INSERT INTO %s (id, data) VALUES (?, ?)", h.Table))
    if err != nil {
        return fmt.Errorf("prepare: %w", err)
    }
    defer stmt.Close()
    
    for _, record := range h.Records {
        _, err := stmt.ExecContext(ctx,
            record["id"],
            record["data"],
        )
        if err != nil {
            return fmt.Errorf("exec: %w", err)
        }
    }
    
    return tx.Commit()
}

func (h *BatchInsertHandler) Type() string {
    return "batch-insert"
}

// Process large file
func ProcessLargeCSV(pool worker.Pool, csvPath string) error {
    const batchSize = 500
    
    file, _ := os.Open(csvPath)
    defer file.Close()
    
    reader := csv.NewReader(file)
    batch := make([]map[string]interface{}, 0, batchSize)
    batchNum := 0
    
    for {
        record, err := reader.Read()
        if err == io.EOF {
            if len(batch) > 0 {
                if !pool.Submit(createBatchJob(batch, batchNum)) {
                    return errors.New("queue full")
                }
            }
            break
        }
        
        batch = append(batch, recordToMap(record))
        
        if len(batch) >= batchSize {
            if !pool.Submit(createBatchJob(batch, batchNum)) {
                return errors.New("queue full")
            }
            batchNum++
            batch = make([]map[string]interface{}, 0, batchSize)
        }
    }
    
    return nil
}

func createBatchJob(records []map[string]interface{}, num int) job.Job {
    return job.New(
        func(ctx context.Context) error {
            handler := &BatchInsertHandler{
                Records: records,
                Table:   "events",
                BatchID: fmt.Sprintf("batch-%d", num),
            }
            return handler.Handle(ctx)
        },
        job.WithTimeout(30 * time.Second),
        job.WithRetries([]time.Duration{1 * time.Second}),
    )
}
```

### Optimization

```go
// Monitoring progress
func MonitorBatchProgress(hub job.Hub) {
    ticker := time.NewTicker(5 * time.Second)
    defer ticker.Stop()
    
    for range ticker.C {
        stats := pool.Stats()
        percentage := (stats.Processed * 100) / totalRecords
        log.Printf("Progress: %d%% (%d/%d)", 
            percentage, stats.Processed, totalRecords)
    }
}
```

### Impact

```
Sequential:  ████████████████████████████ 10 minutes
Pool (4):    ████████████ 2.5 minutes (3.7x faster)
Pool (16):   ███ 40 seconds (15x faster!)
Pool (32):   ██ 22 seconds (27x faster!)
```

---

## Use Case 3: API Call Aggregator

### Bài Toán

```
Aggregate data from 5 different APIs
Each API call: 100-500ms
Sequential: 500-2500ms per request
Concurrent: 100-500ms (fastest + parallelism)
```

### Architecture

```
Client Request
     ↓
APIAggregator (main goroutine)
     ├─→ Job: FetchWeather API    ──→ Worker 1
     ├─→ Job: FetchExchange API   ──→ Worker 2
     ├─→ Job: FetchNews API       ──→ Worker 3
     ├─→ Job: FetchPrice API      ──→ Worker 4
     └─→ Wait for all completion
           ↓
    Merge results
           ↓
    Return to client (100-500ms total!)
```

### Implementation

```go
// APIFetchHandler
type APIFetchHandler struct {
    URL      string
    Method   string
    Headers  map[string]string
    ResultCh chan<- APIResult
}

func (h *APIFetchHandler) Handle(ctx context.Context) error {
    req, _ := http.NewRequestWithContext(ctx, h.Method, h.URL, nil)
    for k, v := range h.Headers {
        req.Header.Set(k, v)
    }
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return err
    }
    defer resp.Body.Close()
    
    body, _ := io.ReadAll(resp.Body)
    h.ResultCh <- APIResult{
        Source: h.URL,
        Data:   body,
        Error:  err,
    }
    
    return nil
}

// Aggregator
func AggregateAPIs(pool worker.Pool) (Response, error) {
    results := make(chan APIResult, 5)
    ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
    defer cancel()
    
    // Submit all jobs concurrently
    ids := []string{"weather", "exchange", "news", "price", "sports"}
    apis := []string{
        "https://api.weather.com/...",
        "https://api.exchange.com/...",
        "https://api.news.com/...",
        "https://api.price.com/...",
        "https://api.sports.com/...",
    }
    
    for i, url := range apis {
        j := job.New(func() error {
            handler := &APIFetchHandler{
                URL:      url,
                Method:   "GET",
                ResultCh: results,
            }
            return handler.Handle(ctx)
        },
            job.WithTimeout(1 * time.Second),
            job.WithRetries([]time.Duration{100 * time.Millisecond}),
        )
        
        if !pool.Submit(j) {
            return Response{}, errors.New("queue full")
        }
    }
    
    // Collect results (with timeout)
    aggregated := make(map[string]interface{})
    timer := time.NewTimer(1500 * time.Millisecond)
    defer timer.Stop()
    
    for i := 0; i < 5; i++ {
        select {
        case result := <-results:
            aggregated[result.Source] = result.Data
        case <-timer.C:
            return Response{}, errors.New("timeout collecting results")
        }
    }
    
    return Response{Data: aggregated}, nil
}
```

### Comparison

```
Sequential Call:
  Weather (200ms) ──→ Exchange (150ms) ──→ News (300ms) 
  = 650ms total

Parallel Pool:
  Weather (200ms)  ─┐
  Exchange (150ms) ─┼─ 300ms max(all)
  News (300ms)     ─┘

Speedup: 650ms → 300ms = 2.17x faster
```

---

## Use Case 4: Webhook Delivery System

### Bài Toán

```
Send webhooks to N external services
Each webhook call: 50-200ms
Timeout requirement: delivery within 5 minutes
```

### Architecture

```
Event Triggered
     ↓
WebhookJob created for each endpoint
     ↓
Worker Pool (8 workers)
     ├─→ /webhook/endpoint1  (retry max 3 times)
     ├─→ /webhook/endpoint2  (retry exponential backoff)
     ├─→ /webhook/endpoint3
     └─→ ...
           ↓
    Delivery confirmed / failed logged
```

### Implementation

```go
// WebhookHandler
type WebhookHandler struct {
    Endpoint string
    Payload  interface{}
    EventID  string
    Attempt  int
}

func (h *WebhookHandler) Handle(ctx context.Context) error {
    body, _ := json.Marshal(h.Payload)
    
    req, _ := http.NewRequestWithContext(ctx,
        http.MethodPost,
        h.Endpoint,
        bytes.NewReader(body),
    )
    req.Header.Set("Content-Type", "application/json")
    req.Header.Set("X-Event-ID", h.EventID)
    req.Header.Set("X-Attempt", fmt.Sprintf("%d", h.Attempt))
    
    resp, err := http.DefaultClient.Do(req)
    if err != nil {
        return fmt.Errorf("request failed: %w", err)
    }
    defer resp.Body.Close()
    
    // Only 2xx is success
    if resp.StatusCode < 200 || resp.StatusCode >= 300 {
        return fmt.Errorf("status %d", resp.StatusCode)
    }
    
    // Log success
    log.Printf("Webhook delivered: %s (event: %s)", h.Endpoint, h.EventID)
    return nil
}

func (h *WebhookHandler) Type() string {
    return "webhook"
}

// Broadcast event
func BroadcastEvent(hub job.Hub, event *Event) error {
    endpoints := []string{
        "https://partner1.com/webhook",
        "https://partner2.com/webhook",
        "https://partner3.com/webhook",
    }
    
    for _, endpoint := range endpoints {
        j, _ := hub.Create("webhook", &WebhookHandler{
            Endpoint: endpoint,
            Payload:  event,
            EventID:  event.ID,
            Attempt:  1,
        },
            job.WithTimeout(10 * time.Second),
            job.WithRetries([]time.Duration{
                1 * time.Second,    // 1s
                3 * time.Second,    // 3s
                10 * time.Second,   // 10s
            }),
        )
        
        hub.Submit(j)
    }
    
    return nil
}

// Callback-based retry tracking
type EventMetrics struct {
    mu                atomic.Mutex
    webhooksSent      int64
    webhooksSucceeded int64
    webhooksFailed    int64
}

func (em *EventMetrics) IncJobSuccess(name string, latency time.Duration) {
    if name == "webhook" {
        em.mu.Lock()
        em.webhooksSucceeded++
        em.mu.Unlock()
    }
}

func (em *EventMetrics) IncJobFailed(name string, err error, latency time.Duration) {
    if name == "webhook" {
        em.mu.Lock()
        em.webhooksFailed++
        em.mu.Unlock()
        log.Printf("Webhook delivery failed: %v", err)
    }
}
```

### Reliability Guarantees

```
Endpoint Status                    Job Behavior
─────────────────────────────────────────────────
Success (2xx)                  → Complete immediately
Transient Error (5xx, timeout) → Retry with backoff
Permanent Error (4xx, invalid) → Mark failed permanently
Network Error                  → Retry with backoff
```

---

## Use Case 5: Data Pipeline / ETL

### Bài Toán

```
ETL Pipeline:
  Extract → Transform → Load

Each stage involves:
  - Parse CSV (500 files)
  - Transform data
  - Insert to database (batches)
  
Sequential: 30 minutes
With worker pool: 2-3 minutes
```

### Architecture

```
CSV Files (500)
     ↓
Extraction Workers (4)              Transformation Workers (8)          Load Workers (4)
├─ File 1-125 ──→ Json ──────────→ Extract fields ────────────────→ Batch 1 → DB
├─ File 126-250 ──→ Json ──────→ Extract fields ────────────────→ Batch 2 → DB
├─ File 251-375 ──→ Json ──────→ Extract fields ────────────────→ Batch 3 → DB
└─ File 376-500 ──→ Json ──────→ Extract fields ────────────────→ Batch 4 → DB

Can use same pool or separate pools for each stage
```

### Implementation

```go
// Stage 1: Extract
type ExtractCSVHandler struct {
    FilePath  string
    OutputCh  chan<- []map[string]interface{}
}

func (h *ExtractCSVHandler) Handle(ctx context.Context) error {
    file, _ := os.Open(h.FilePath)
    defer file.Close()
    
    reader := csv.NewReader(file)
    records := make([]map[string]interface{}, 0)
    
    headers, _ := reader.Read()
    for {
        row, err := reader.Read()
        if err == io.EOF {
            break
        }
        
        record := make(map[string]interface{})
        for i, val := range row {
            record[headers[i]] = val
        }
        records = append(records, record)
    }
    
    h.OutputCh <- records
    return nil
}

// Stage 2: Transform
type TransformHandler struct {
    Records  []map[string]interface{}
    OutputCh chan<- []map[string]interface{}
}

func (h *TransformHandler) Handle(ctx context.Context) error {
    transformed := make([]map[string]interface{}, 0)
    
    for _, record := range h.Records {
        // Business logic: validation, enrichment, etc
        if record["email"].(string) != "" {
            record["email_validated"] = validateEmail(record["email"].(string))
            record["processed_at"] = time.Now()
            transformed = append(transformed, record)
        }
    }
    
    h.OutputCh <- transformed
    return nil
}

// Stage 3: Load (Batch Insert)
type LoadHandler struct {
    Records []map[string]interface{}
    DBConn  *sql.DB
}

func (h *LoadHandler) Handle(ctx context.Context) error {
    tx, _ := h.DBConn.BeginTx(ctx, nil)
    defer tx.Rollback()
    
    stmt, _ := tx.Prepare("INSERT INTO users (...) VALUES (...)")
    defer stmt.Close()
    
    for _, record := range h.Records {
        stmt.ExecContext(ctx, record["id"], record["email"], ...)
    }
    
    return tx.Commit()
}

// Orchestrate
func RunETLPipeline(extractPool, transformPool, loadPool worker.Pool, csvDir string) error {
    files, _ := os.ReadDir(csvDir)
    
    // Extract jobs
    for _, file := range files {
        extractCh := make(chan []map[string]interface{}, 1)
        
        extractJ := job.New(func() error {
            handler := &ExtractCSVHandler{
                FilePath:  filepath.Join(csvDir, file.Name()),
                OutputCh:  extractCh,
            }
            return handler.Handle(context.Background())
        })
        
        extractPool.Submit(extractJ)
        
        // Wait for extraction result
        records := <-extractCh
        
        // Transform jobs (can batch)
        transformJ := job.New(func() error {
            handler := &TransformHandler{
                Records:  records,
                OutputCh: transformCh,
            }
            return handler.Handle(context.Background())
        })
        
        transformPool.Submit(transformJ)
        
        // Load jobs
        loadRecords := <-transformCh
        loadJ := job.New(func() error {
            handler := &LoadHandler{
                Records: loadRecords,
                DBConn:  myDB,
            }
            return handler.Handle(context.Background())
        })
        
        loadPool.Submit(loadJ)
    }
    
    return nil
}
```

### Pipeline Diagram

```
Time →

Extract [File1] [File2] [File3] ...
          ↓
Transform      [T1] [T2] [T3] ...
                ↓
Load                 [L1] [L2] [L3] ...

Total time = max(extract) + max(transform) + max(load)
Not = sum of all stages!
```

---

## Use Case 6: High-Throughput Event Processing

### Bài Toán

```
Processing server events:
  - 10,000 events/second
  - Average processing: 5ms per event
  - Need to handle traffic spikes

Sequential: IMPOSSIBLE (can only do 200 events/sec)
Pool: SCALABLE (tune workers based on load)
```

### Architecture

```
Event Stream (Kafka, Queue, etc)
     ↓
Event Demux → Worker Pool (32 workers)
     │         ├─→ Validate event
     │         ├─→ Enrich data
     │         ├─→ Store to cache/DB
     │         └─→ Send metrics
     ↓
Metrics Aggregator
```

### Implementation

```go
// EventHandler - high-volume processing
type EventHandler struct {
    EventID   string
    EventType string
    Data      map[string]interface{}
    Timestamp time.Time
}

func (h *EventHandler) Handle(ctx context.Context) error {
    // Validate
    if err := validateEvent(h); err != nil {
        return fmt.Errorf("validation failed: %w", err)
    }
    
    // Enrich
    h.Data["processed_at"] = time.Now()
    h.Data["is_valid"] = true
    
    // Store (async to cache/DB)
    cache := ctx.Value("cache").(redis.Cmdable)
    key := fmt.Sprintf("event:%s", h.EventID)
    cache.Set(ctx, key, h.Data, 15*time.Minute)
    
    // Emit metrics
    metrics := ctx.Value("metrics").(MetricsHook)
    metrics.IncEventProcessed(h.EventType)
    
    return nil
}

// High-throughput configuration
func setupHighThroughputPool(metrics MetricsHook) worker.Pool {
    // Calculate based on system resources
    numWorkers := runtime.NumCPU() * 4  // For I/O bound
    queueSize := numWorkers * 100        // Enough buffer
    
    return worker.NewPool(logger, metrics,
        worker.WithName("event-processor"),
        worker.WithSize(numWorkers),
        worker.WithQueueSize(queueSize),
        worker.WithStopTimeout(1 * time.Minute),
    )
}

// Consumer loop
func ConsumeEvents(pool worker.Pool, eventSource <-chan *Event) {
    for event := range eventSource {
        j := job.New(func(ctx context.Context) error {
            handler := &EventHandler{
                EventID:   event.ID,
                EventType: event.Type,
                Data:      event.Data,
                Timestamp: event.Timestamp,
            }
            return handler.Handle(ctx)
        },
            job.WithTimeout(500 * time.Millisecond),  // Fail fast
            job.WithRetries([]time.Duration{          // Quick retry
                10 * time.Millisecond,
                50 * time.Millisecond,
            }),
        )
        
        // Non-blocking submit
        if !pool.Submit(j) {
            log.Printf("WARNING: Queue overflow, dropping event %s", event.ID)
        }
    }
}

// Metrics implementation
type HighThroughputMetrics struct {
    eventsProcessed atomic.Int64
    eventsFailed    atomic.Int64
    latencyP50      atomic.Int64
    latencyP99      atomic.Int64
}

func (m *HighThroughputMetrics) IncJobSuccess(name string, latency time.Duration) {
    m.eventsProcessed.Add(1)
    
    // Track percentiles
    ms := latency.Milliseconds()
    m.latencyP50.Store(ms)  // simplified; use histogram in prod
}

func (m *HighThroughputMetrics) String() string {
    return fmt.Sprintf(
        "Events: %d total, %d failed | P50: %dms, P99: %dms",
        m.eventsProcessed.Load(),
        m.eventsFailed.Load(),
        m.latencyP50.Load(),
        m.latencyP99.Load(),
    )
}
```

### Performance Characteristics

```
Configuration            Throughput    Latency (P99)
──────────────────────────────────────────────────
4 workers                ~2K events/s  50ms
8 workers                ~4K events/s  40ms
16 workers               ~8K events/s  30ms
32 workers               ~12K events/s 20ms
64 workers               ~15K events/s 15ms (diminishing returns)
```

---

## Comparison Matrix

| Use Case | Workers | Queue Size | Timeout | Retry Strategy | Key Challenge |
|----------|---------|-----------|---------|----------------|---------------|
| Email Queue | 8 | 1024 | 30s | Exponential backoff | SMTP reliability |
| Batch Processing | 16 | 2048 | 30s | Linear | Data consistency |
| API Aggregator | 4 | 256 | 1s | Minimal (1-2x) | Timeout management |
| Webhooks | 8 | 512 | 10s | Exponential backoff | Delivery guarantee |
| Data Pipeline | Variable | Variable | Stage-dependent | Retry + retry | Data validity |
| Event Processing | 32+ | 4096+ | 500ms | Quick fail | Throughput & latency |

---

## Key Takeaways

### When to Use Worker Pool

✅ **Ideal For:**
- I/O operations (network, database, file)
- Background/async work
- Load optimization
- Request throttling
- Concurrent processing

❌ **Not Ideal For:**
- CPU-intensive computation (use goroutines directly)
- Real-time critical (<1ms latency)
- Single simple operations

### Design Principles

1. **Size = CPU Count for CPU-bound, CPU×2-4 for I/O-bound**
2. **Queue Size = Workers × 10-100 (memory vs. throughput tradeoff)**
3. **Timeout = Expected duration × 2-3 (accounting for retries)**
4. **Retry = Exponential backoff for transient errors only**
5. **Monitor = Track all metrics via MetricsHook**

### Common Mistakes to Avoid

```go
// ❌ Queue too small
WithQueueSize(1)  // Will lose data!

// ❌ Timeout too short
WithTimeout(1 * time.Millisecond)  // Always fails

// ❌ Too many workers
WithSize(1000)  // Resource exhaustion

// ❌ No monitoring
pool := NewPool(logger, nil)  // Can't track issues

// ❌ Ignoring Submit failures
pool.Submit(job)  // Silent drop!
```

---

## Summary

Worker pool is essential for building **scalable, reliable** systems. The 6 use cases demonstrate real-world patterns applicable to most applications. Start with appropriate sizing, add monitoring, and iterate based on production metrics.

For each use case:
1. Identify bottleneck (sequential processing)
2. Design jobs with clear boundaries
3. Configure pool appropriately
4. Add comprehensive monitoring
5. Test under load
6. Deploy with graceful shutdown
