# MQTT Forwarder Example

Ví dụ service lắng nghe MQTT topics và chuyển tiếp messages sang Redis/PostgreSQL sử dụng fcontext, job, và worker.

## Kiến trúc

```
MQTT Broker
    ↓
[MQTT Component] → (messages)
    ↓
[Message Handler] → (create jobs)
    ↓
[Worker Pool] → (process jobs concurrently)
    ↓
[Storage Component]
    ├─→ Redis (cache)
    └─→ PostgreSQL (persistence)
    ↓
[HTTP API] → (retrieve messages)
```

## Thành phần

### 1. MQTT Component (`plugins/mqtt/`)
- Kết nối tới MQTT broker
- Subscribe multiple topics
- Gọi message handler khi nhận message

### 2. Storage Component (`plugins/storage/`)
- **Redis**: Lưu cache messages (TTL configurable)
- **PostgreSQL**: Lưu persistence
- Hỗ trợ CRUD operations

### 3. Job Handler (`job_handler.go`)
- Validate JSON payload
- Save message to storage
- Có retry logic & timeout

### 4. HTTP API (`api.go`)
- `GET /api/v1/health` - Health check
- `GET /api/v1/messages/:topic?limit=100` - Lấy messages
- `GET /api/v1/stats/:topic` - Thống kê

## Setup & Run

### 1. Prerequisites

```bash
# MQTT Broker (Docker)
docker run -d -p 1883:1883 -p 9001:9001 eclipse-mosquitto:latest

# Redis (Docker)
docker run -d -p 6379:6379 redis:latest

# PostgreSQL (Docker)
docker run -d -p 5432:5432 \
  -e POSTGRES_DB=mqtt_db \
  -e POSTGRES_USER=mqtt_user \
  -e POSTGRES_PASSWORD=mqtt_pass \
  postgres:latest
```

### 2. Environment Variables

```bash
# .env file
MQTT_BROKER=localhost
MQTT_PORT=1883
REDIS_ADDR=localhost:6379
POSTGRES_DSN=postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db?sslmode=disable
```

### 3. Run Service

```bash
go run *.go \
  --mqtt-broker localhost \
  --redis-addr localhost:6379 \
  --postgres-dsn "postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db?sslmode=disable" \
  --app-env dev
```

## Publish MQTT Messages (Test)

```bash
# Subscribe to topic (terminal 1)
mosquitto_sub -t "sensor/#" -h localhost -p 1883

# Publish messages (terminal 2)
mqtt pub -t "sensor/temperature" -h localhost -p 1883 '{"value": 25.5, "unit": "C"}'
mqtt pub -t "sensor/humidity" -h localhost -p 1883 '{"value": 60, "unit": "%"}'
```

## Test HTTP Endpoints

```bash
# Health check
curl http://localhost:8080/api/v1/health

# Get messages
curl "http://localhost:8080/api/v1/messages/sensor/temperature?limit=10"

# Get stats
curl http://localhost:8080/api/v1/stats/sensor/temperature
```

## Configuration

### Worker Pool Options
- `WithPoolSize(n)` - Số worker threads (default: 10)
- `WithQueueSize(n)` - Job queue size (default: 1000)

### Job Options
- `WithTimeout(d)` - Timeout per job
- `WithRetries([]time.Duration)` - Retry schedule
- `WithJitter(p)` - Jitter percentage

### Storage Options
- `EnableRedis: true/false`
- `EnablePostgres: true/false`
- `RedisTTL: 24h` (default)

## Performance

Với cấu hình mặc định:
- **Throughput**: ~10k messages/second (tùy vào payload size & storage latency)
- **Memory**: ~50MB + Redis/Postgres buffer
- **Latency**: <100ms per message (trong conditions tốt)

## Mở rộng

Bạn có thể mở rộng ví dụ này bằng:

1. **Thêm Message Transformation**
   - Thêm plugin transform pipeline
   - Hỗ trợ multiple output formats

2. **Thêm Consumer Services**
   - Multiple Postgres tables per topic
   - WebSocket subscribers
   - Message streaming

3. **Monitoring & Metrics**
   - Prometheus metrics
   - Jaeger tracing
   - Health checks

4. **Advanced Retry & DLQ**
   - Dead letter queue
   - Exponential backoff
   - Circuit breaker

## Troubleshooting

### MQTT Connection Failed
```
Check if MQTT broker is running:
mosquitto_sub -t "#" -h localhost
```

### Redis Connection Failed
```
redis-cli -h localhost ping
```

### Postgres Connection Failed
```
psql "postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db"
```
