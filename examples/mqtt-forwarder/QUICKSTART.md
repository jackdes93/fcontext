# Hướng dẫn Bắt đầu nhanh (Quickstart)

## 1️⃣ Yêu cầu

- Go 1.21+
- Docker & Docker Compose
- Git

## 2️⃣ Clone & Setup

```bash
cd /path/to/fcontext/example

# Copy env template
cp mqtt-forwarder/.env.example mqtt-forwarder/.env
```

## 3️⃣ Khởi động Services

### Option A: Dùng Docker Compose (Recommended)
```bash
cd mqtt-forwarder

# Start all services (MQTT + Redis + Postgres + App)
docker-compose up -d

# Check logs
docker-compose logs -f

# Stop
docker-compose down
```

### Option B: Khởi động thủ công

**Terminal 1: Start Docker services**
```bash
# MQTT Broker
docker run -d --name mqtt -p 1883:1883 -p 9001:9001 eclipse-mosquitto:2.0

# Redis
docker run -d --name redis -p 6379:6379 redis:latest

# PostgreSQL
docker run -d --name postgres \
  -e POSTGRES_DB=mqtt_db \
  -e POSTGRES_USER=mqtt_user \
  -e POSTGRES_PASSWORD=mqtt_pass \
  -p 5432:5432 postgres:15

# Verify
docker ps
```

**Terminal 2: Build & Run App**
```bash
cd mqtt-forwarder

go mod tidy
go run . \
  --mqtt-broker localhost \
  --redis-addr localhost:6379 \
  --postgres-dsn "postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db?sslmode=disable" \
  --app-env dev
```

## 4️⃣ Publish Test Messages

**Terminal 3: Publish MQTT messages**
```bash
# Install mosquitto client (Mac)
brew install mosquitto

# Pub temperature
mqtt pub -t "sensor/temperature" -h localhost -p 1883 '{"value": 25.5, "unit": "C"}'

# Pub humidity
mqtt pub -t "sensor/humidity" -h localhost -p 1883 '{"value": 60, "unit": "%"}'

# Pub device status
mqtt pub -t "device/status" -h localhost -p 1883 '{"status": "online", "uptime": 3600}'

# Subscribe to all topics
mqtt sub -t "#" -h localhost -p 1883
```

## 5️⃣ Test HTTP Endpoints

**Terminal 4: Query API**
```bash
# Health check
curl http://localhost:8080/api/v1/health

# Get messages by topic
curl "http://localhost:8080/api/v1/messages/sensor/temperature?limit=10"

# Get stats
curl http://localhost:8080/api/v1/stats/sensor/temperature

# Pretty print
curl -s http://localhost:8080/api/v1/messages/sensor/temperature | jq
```

## 6️⃣ Verify Data in Storage

### Redis
```bash
redis-cli

# List all keys
KEYS mqtt:*

# Get messages
LRANGE mqtt:messages:sensor/temperature 0 -1

# Count
LLEN mqtt:messages:sensor/temperature
```

### PostgreSQL
```bash
psql -U mqtt_user -d mqtt_db -h localhost

# Query messages
SELECT * FROM mqtt_messages ORDER BY received_at DESC LIMIT 5;

# Count by topic
SELECT topic, COUNT(*) as count FROM mqtt_messages GROUP BY topic;
```

## 7️⃣ Cleanup

```bash
# Stop Docker Compose stack
docker-compose down

# Or stop individual services
docker stop mqtt redis postgres
docker rm mqtt redis postgres

# Or kill Go process
pkill -f "go run"
```

---

## Troubleshooting

### MQTT Connection Failed
```bash
# Check if broker is running
docker ps | grep mqtt

# Test connection
mqtt sub -t test -h localhost -p 1883

# Logs
docker logs mqtt
```

### PostgreSQL Connection Failed
```bash
# Check if running
docker ps | grep postgres

# Connect directly
psql -h localhost -U mqtt_user -d mqtt_db
# Password: mqtt_pass

# Logs
docker logs postgres
```

### Redis Connection Failed
```bash
# Check if running
docker ps | grep redis

# Test connection
redis-cli -h localhost ping

# Should return: PONG

# Logs
docker logs redis
```

### No Messages in Storage
1. Verify MQTT subscription:
   ```bash
   mqtt sub -t "sensor/temperature" -h localhost
   ```

2. Check app logs for errors:
   ```bash
   # App should show:
   # INFO MQTT message received topic=sensor/temperature
   # INFO message forwarded topic=sensor/temperature
   ```

3. Verify API returns data:
   ```bash
   curl http://localhost:8080/api/v1/messages/sensor/temperature
   ```

---

## Next Steps

1. **Modify job handler** (`job_handler.go`):
   - Add custom validation
   - Add data transformation
   - Add error handling

2. **Add new topics** (main.go):
   ```go
   WithTopics(map[string]byte{
       "sensor/new/topic": 1,
       "another/topic":    0,
   })
   ```

3. **Adjust pool settings** (main.go):
   ```go
   worker.NewComponent("worker", nil,
       worker.WithPoolSize(50),        // Increase workers
       worker.WithQueueSize(5000),     // Bigger queue
   )
   ```

4. **Add more HTTP endpoints** (api.go):
   - Implement search by timestamp
   - Add export to CSV
   - Add webhook notifications

5. **Deploy to production**:
   - Use Kubernetes manifests
   - Setup health checks
   - Add monitoring (Prometheus)
   - Add tracing (Jaeger)

---

## File Structure

```
mqtt-forwarder/
├── go.mod                    # Dependencies
├── go.sum
├── main.go                   # Entry point
├── job_handler.go            # Job creation
├── api.go                    # HTTP endpoints
├── .env.example              # Environment template
├── Makefile                  # Build commands
├── Dockerfile                # Docker image
├── docker-compose.yml        # Stack definition
├── README.md                 # Overview
├── IMPLEMENTATION.md         # Architecture details
├── QUICKSTART.md             # This file
└── plugins/
    ├── mqtt/
    │   └── component.go      # MQTT component
    └── storage/
        ├── component.go      # Storage component
        ├── redis.go          # Redis client
        └── postgres.go       # Postgres client
```

---

Happy coding! 🚀
