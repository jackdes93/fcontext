package main

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/jackdes93/fcontext"
	"github.com/jackdes93/fcontext/job"
	"github.com/binhdp/mqtt-forwarder/plugins/storage"
)

// ForwardMessageJob định nghĩa job để xử lý MQTT message
type ForwardMessageJob struct {
	Topic   string
	Payload []byte
	Storage *storage.StorageComponent
	Logger  fcontext.Logger
}

// CreateForwardMessageHandler tạo handler cho job
func CreateForwardMessageHandler(st *storage.StorageComponent, log fcontext.Logger) func(topic string, payload []byte) job.Job {
	return func(topic string, payload []byte) job.Job {
		handler := func(ctx context.Context) error {
			// Validate payload
			var data interface{}
			if err := json.Unmarshal(payload, &data); err != nil {
				log.Warn("invalid json payload", "topic", topic, "error", err)
				// Vẫn lưu lại dù không phải JSON
			}

			// Save message to storage (Redis + Postgres)
			if err := st.SaveMessage(ctx, topic, payload); err != nil {
				return fmt.Errorf("save message failed: %w", err)
			}

			log.Debug("message forwarded", "topic", topic, "size", len(payload))
			return nil
		}

		return job.New(
			handler,
			job.WithName(fmt.Sprintf("forward-%s", topic)),
			job.WithTimeout(10 * 1000), // 10 seconds
			job.WithRetries([]time.Duration{
				1 * time.Second,
				2 * time.Second,
				5 * time.Second,
			}),
			job.WithJitter(0.1), // 10% jitter
		)
	}
}
