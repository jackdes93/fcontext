package main

import (
	"context"

	httpserver "github.com/binhdp/example/plugins/http-server"
	"github.com/binhdp/example/plugins/http-server/middleware"
	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/worker"
	"github.com/binhdp/mqtt-forwarder/plugins/mqtt"
	"github.com/binhdp/mqtt-forwarder/plugins/storage"
)

// newService tạo service context với tất cả components
func newService() fcontext.ServiceContext {
	// Tạo storage component
	storageComp := storage.NewComponent("storage")

	// Tạo MQTT component với topics
	mqttComp := mqtt.NewComponent("mqtt").
		WithTopics(map[string]byte{
			"sensor/temperature": 1,
			"sensor/humidity":    1,
			"device/status":      0,
		})

	// Tạo worker component
	workerComp := worker.NewComponent(
		"worker",
		nil, // metric hook
		worker.WithPoolSize(10),
		worker.WithQueueSize(1000),
	)

	// Middleware
	requestLogger := middleware.RequestLogger(&middleware.RequestLoggerOption{
		Enable: true,
		Prefix: "request-logger",
	})
	
	recoveryMW := middleware.Recovery(&middleware.RecoveryOptions{
		ForceLogStack:  false,
		RepanicInDebug: false,
		LoggerPrefix:   "recovery",
	})

	// HTTP Server component
	httpComp := httpserver.NewGinService("http").
		WithMiddlewareFactory(recoveryMW).
		WithMiddlewareFactory(requestLogger).
		WithRegistrarFactory(RegisterAPIRoutes)

	// Tạo service context
	service := fcontext.New(
		fcontext.WithName("MQTT-Forwarder"),
		fcontext.WithComponent(storageComp),
		fcontext.WithComponent(mqttComp),
		fcontext.WithComponent(workerComp),
		fcontext.WithComponent(httpComp),
	)

	return service
}

// setupMessageForwarding set up MQTT message handler
func setupMessageForwarding(ctx context.Context, sv fcontext.ServiceContext) error {
	st := sv.MustGet("storage").(*storage.StorageComponent)
	mqttComp := sv.MustGet("mqtt").(*mqtt.MQTTComponent)
	workerComp := sv.MustGet("worker").(*worker.Component)
	log := sv.Logger("forwarder")

	// Create message handler
	jobFactory := CreateForwardMessageHandler(st, log)

	// Set MQTT message handler
	mqttComp.WithMessageHandler(func(topic string, payload []byte) {
		// Create job for each message
		j := jobFactory(topic, payload)

		// Submit to worker pool
		if !workerComp.Submit(j) {
			log.Error("failed to submit job to worker pool", "topic", topic)
		}
	})

	log.Info("message forwarding set up successfully")
	return nil
}

func main() {
	sv := newService()

	if err := fcontext.Run(sv, func(ctx context.Context) error {
		// Setup message forwarding
		if err := setupMessageForwarding(ctx, sv); err != nil {
			return err
		}

		log := sv.Logger("main")
		log.Info("MQTT Forwarder service started successfully")
		log.Info("listening for MQTT messages...", "endpoints", []string{
			"POST   /api/v1/health",
			"GET    /api/v1/messages/:topic",
			"GET    /api/v1/stats/:topic",
		})

		// Keep running
		<-ctx.Done()
		return nil
	}); err != nil {
		panic(err)
	}
}
