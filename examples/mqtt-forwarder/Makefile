.PHONY: help start-services stop-services build run docker-build docker-run test

help:
	@echo "MQTT Forwarder - Available commands:"
	@echo "  make start-services    - Start all Docker services (MQTT, Redis, Postgres)"
	@echo "  make stop-services     - Stop all Docker services"
	@echo "  make build             - Build the application"
	@echo "  make run               - Run the application"
	@echo "  make test              - Run tests"
	@echo "  make docker-build      - Build Docker image"
	@echo "  make docker-run        - Run in Docker"

# Docker services
start-services:
	@echo "Starting Docker services..."
	docker run -d --name mqtt-broker -p 1883:1883 -p 9001:9001 eclipse-mosquitto:2.0 || true
	docker run -d --name redis -p 6379:6379 redis:latest || true
	docker run -d --name postgres \
		-e POSTGRES_DB=mqtt_db \
		-e POSTGRES_USER=mqtt_user \
		-e POSTGRES_PASSWORD=mqtt_pass \
		-p 5432:5432 postgres:15 || true
	@echo "Services started!"
	@sleep 3

stop-services:
	@echo "Stopping Docker services..."
	docker stop mqtt-broker redis postgres || true
	docker rm mqtt-broker redis postgres || true
	@echo "Services stopped!"

# Build & Run
build:
	@echo "Building application..."
	go mod download
	go build -o mqtt-forwarder .

run:
	@echo "Running MQTT Forwarder..."
	go run . \
		--mqtt-broker localhost \
		--redis-addr localhost:6379 \
		--postgres-dsn "postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db?sslmode=disable" \
		--app-env dev

# Docker
docker-build:
	docker build -t mqtt-forwarder:latest .

docker-run: docker-build
	docker run --rm \
		--name mqtt-forwarder \
		--network host \
		-e MQTT_BROKER=localhost \
		-e REDIS_ADDR=localhost:6379 \
		-e POSTGRES_DSN="postgres://mqtt_user:mqtt_pass@localhost:5432/mqtt_db?sslmode=disable" \
		mqtt-forwarder:latest

# Testing
test:
	@echo "Running tests..."
	go test -v ./...

# Development
dev: stop-services start-services build run

# Cleanup
clean:
	rm -f mqtt-forwarder
	go clean

logs:
	@echo "Tailing application logs (docker)..."
	docker logs -f mqtt-forwarder
