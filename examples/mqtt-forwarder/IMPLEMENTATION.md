# MQTT Forwarder - Chi tiết triển khai

## Luồng xử lý dữ liệu

```
┌──────────────────────────────────────┐
│   MQTT Broker                         │
│  (Listening on localhost:1883)        │
└────────────────┬─────────────────────┘
                 │
                 │ Messages from topics:
                 │ - sensor/temperature
                 │ - sensor/humidity
                 │ - device/status
                 │
        ┌────────▼────────┐
        │ MQTT Component  │
        │  (component.go) │
        └────────┬────────┘
                 │
                 │ onMessage callback
                 │ {topic, payload}
                 │
        ┌────────▼──────────────────┐
        │ CreateForwardMessageJob   │
        │  (job_handler.go)         │
        │                            │
        │ 1. Validate JSON payload  │
        │ 2. Create job object      │
        │ 3. Retry config:          │
        │    - 1s, 2s, 5s retry    │
        │    - 10s timeout          │
        └────────┬──────────────────┘
                 │
        ┌────────▼────────────┐
        │ Worker Pool         │
        │ (10 concurrent jobs)│
        │                     │
        │ Queue: 1000 jobs   │
        └────────┬────────────┘
                 │
                 │ Execute job
                 │
        ┌────────▼────────────────┐
        │ Handler Function         │
        │ (job_handler.go)         │
        │                          │
        │ storage.SaveMessage()   │
        └────────────────────────┘
                 │
        ┌────────┴──────────────┐
        │                       │
   ┌────▼─────┐           ┌────▼──────┐
   │   Redis  │           │ PostgreSQL│
   │  (Cache) │           │  (Store)  │
   │          │           │           │
   │ Key:     │           │ Table:    │
   │ mqtt:*   │           │ mqtt_msgs │
   │ TTL:24h  │           │           │
   │          │           │ Indexing: │
   │ Memory:  │           │ - topic   │
   │ ~100KB   │           │ - time    │
   └──────────┘           └───────────┘
        │                       │
        └───────────┬───────────┘
                    │
            ┌───────▼──────────┐
            │  HTTP API        │
            │  (api.go)        │
            │                  │
            │ GET /messages/:t │
            │ GET /stats/:t    │
            │ GET /health      │
            └──────────────────┘
                    │
                    ▼
              [Client Application]
```

## Chi tiết Components

### 1. MQTT Component
**File**: `plugins/mqtt/component.go`

```go
// Subscribe đến topics
mqttComp.WithTopics(map[string]byte{
    "sensor/temperature": 1,  // QoS 1
    "sensor/humidity":    1,
    "device/status":      0,  // QoS 0
})

// Khi nhận message:
onMessage := func(topic string, payload []byte) {
    // Gọi callback
    // Payload: {"value": 25.5, "unit": "C"}
}
mqttComp.WithMessageHandler(onMessage)
```

**Callback Flow**:
1. MQTT broker gửi message
2. `SetDefaultPublishHandler` được trigger
3. Gọi `onMessage` callback
4. Callback tạo job và submit vào worker pool

### 2. Storage Component
**File**: `plugins/storage/component.go`

**Redis Part** (`redis.go`):
```
Key Structure: mqtt:messages:{topic}
Value: List of JSON messages
TTL: 24 hours (configurable)

Example:
redis> LRANGE mqtt:messages:sensor/temperature 0 -1
1) {"topic":"sensor/temperature","payload":"{...}","timestamp":"2024-01-01T12:00:00Z"}
2) {"topic":"sensor/temperature","payload":"{...}","timestamp":"2024-01-01T12:05:00Z"}
```

**PostgreSQL Part** (`postgres.go`):
```sql
CREATE TABLE mqtt_messages (
    id SERIAL PRIMARY KEY,
    topic VARCHAR(255) NOT NULL,
    payload TEXT NOT NULL,
    received_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
    created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
);

CREATE INDEX idx_mqtt_topic ON mqtt_messages(topic);
CREATE INDEX idx_mqtt_received ON mqtt_messages(received_at);
```

### 3. Job Handler
**File**: `job_handler.go`

```go
// Mỗi message -> 1 job
job := job.New(
    handler func(ctx context.Context) error {
        // 1. Validate JSON
        var data interface{}
        json.Unmarshal(payload, &data)
        
        // 2. Save to Redis
        redis.SaveMessage(topic, payload)
        
        // 3. Save to Postgres
        postgres.SaveMessage(topic, payload)
    },
    job.WithTimeout(10 * time.Second),
    job.WithRetries([]time.Duration{
        1 * time.Second,   // 1st retry
        2 * time.Second,   // 2nd retry
        5 * time.Second,   // 3rd retry
    }),
    job.WithJitter(0.1),  // ±10% delay
)

// Retry logic khi failure:
// 1. Execute fails
// 2. Wait 1s
// 3. Retry
// 4. Still fails, wait 2s
// 5. Retry again...
// 6. After 3 retries, mark as failed
```

### 4. Worker Pool
**Configuration**:
- Pool size: 10 workers
- Queue size: 1000 jobs
- Processing: Concurrent, FIFO

**Flow**:
```
Job submitted → Queue (if available)
             ↓
        Worker thread picks job
             ↓
        Execute handler
             ↓
        Success → Remove from queue
        Failure → Retry based on config
```

## Ví dụ Thực tế

### Scenario: Temperature Sensor
1. **MQTT Message**:
   ```json
   Topic: sensor/temperature
   Payload: {"sensorId": "temp-01", "value": 25.5, "unit": "C"}
   ```

2. **Job Created**:
   ```
   Name: forward-sensor/temperature
   Timeout: 10s
   Max Retries: 3
   ```

3. **Storage**:
   - **Redis**: Lưu 100 messages gần nhất (cache)
   - **Postgres**: Lưu tất cả messages (persistence)

4. **Retrieval via API**:
   ```bash
   GET /api/v1/messages/sensor/temperature?limit=10
   
   Response:
   {
     "topic": "sensor/temperature",
     "messages": [
       {
         "topic": "sensor/temperature",
         "payload": "{...}",
         "timestamp": "2024-01-01T12:00:00Z"
       }
     ],
     "count": 1
   }
   ```

## Performance Characteristics

### Throughput
- **Single worker**: ~100 messages/sec
- **10 workers**: ~1000 messages/sec (tùy vào storage latency)
- **With Redis cache**: +500 msg/sec
- **Bottleneck**: Database I/O

### Latency
- MQTT receive → Job creation: <1ms
- Job execution (Redis): ~5-10ms
- Job execution (Postgres): ~20-50ms
- Queue wait (if saturated): <100ms

### Memory Usage
- Base application: ~20MB
- Per cached message (Redis): ~500 bytes
- 1000 messages in cache: ~500MB

## Scaling Options

### 1. Horizontal Scaling
```yaml
# docker-compose with multiple instances
services:
  forwarder-1:
    build: .
    environment:
      INSTANCE_ID: 1
      
  forwarder-2:
    build: .
    environment:
      INSTANCE_ID: 2
      
  # Shared Redis & Postgres
  redis:
    ...
  postgres:
    ...
```

### 2. Vertical Scaling
```bash
# Increase pool size
go run . \
  --worker-pool-size 50 \
  --worker-queue-size 5000
```

### 3. Topic Sharding
```go
// Xử lý khác nhau cho topics khác nhau
topics := map[string]string{
    "sensor/temperature": "postgres", // Lưu DB
    "sensor/debug":       "redis",    // Chỉ cache
}
```

## Error Handling

### Job Failures
1. **Timeout**: Exceed 10s → Mark failed, but don't retry
2. **Storage error**: Retry with exponential backoff
3. **Validation error**: Log & skip (no retry)
4. **Permanent failure**: After 3 retries → Log & abandon

### Connection Failures
1. **MQTT disconnected**: Auto-reconnect bằng built-in handler
2. **Redis unavailable**: Log warning, skip Redis, continue with Postgres
3. **Postgres unavailable**: Log warning, skip Postgres, continue with Redis

## Monitoring & Debugging

### Logs
```bash
# All debug info
go run . --app-env dev

# Watch Redis
redis-cli MONITOR

# Watch Postgres
psql -c "SELECT * FROM mqtt_messages ORDER BY received_at DESC LIMIT 10;"

# Watch MQTT
mosquitto_sub -t "#" -h localhost
```

### Metrics to Track
- Messages/sec
- Job success rate
- Average job duration
- Queue depth
- Storage latency
