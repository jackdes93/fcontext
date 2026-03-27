package storage

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/redis/go-redis/v9"
)

type RedisClient struct {
	client *redis.Client
	ttl    time.Duration
}

type MessageRecord struct {
	Topic     string    `json:"topic"`
	Payload   string    `json:"payload"`
	Timestamp time.Time `json:"timestamp"`
}

func NewRedisClient(addr string, ttl time.Duration) *RedisClient {
	return &RedisClient{
		client: redis.NewClient(&redis.Options{
			Addr: addr,
		}),
		ttl: ttl,
	}
}

func (r *RedisClient) Ping(ctx context.Context) error {
	return r.client.Ping(ctx).Err()
}

// SaveMessage lưu message vào Redis với key = "mqtt:messages:{topic}"
func (r *RedisClient) SaveMessage(ctx context.Context, topic string, payload []byte) error {
	msg := MessageRecord{
		Topic:     topic,
		Payload:   string(payload),
		Timestamp: time.Now(),
	}
	
	data, _ := json.Marshal(msg)
	key := fmt.Sprintf("mqtt:messages:%s", topic)
	
	// Lưu vào Redis List
	if err := r.client.RPush(ctx, key, string(data)).Err(); err != nil {
		return err
	}
	
	// Set expiration
	if r.ttl > 0 {
		r.client.Expire(ctx, key, r.ttl)
	}
	
	return nil
}

// GetMessages lấy messages từ Redis
func (r *RedisClient) GetMessages(ctx context.Context, topic string, limit int64) ([]MessageRecord, error) {
	key := fmt.Sprintf("mqtt:messages:%s", topic)
	
	vals, err := r.client.LRange(ctx, key, 0, limit-1).Result()
	if err != nil {
		return nil, err
	}
	
	messages := make([]MessageRecord, 0, len(vals))
	for _, val := range vals {
		var msg MessageRecord
		if err := json.Unmarshal([]byte(val), &msg); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	
	return messages, nil
}

func (r *RedisClient) Close() error {
	return r.client.Close()
}
