package storage

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	_ "github.com/lib/pq"
)

type PostgresClient struct {
	db *sql.DB
}

func NewPostgresClient(dsn string) (*PostgresClient, error) {
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		return nil, err
	}
	
	if err := db.Ping(); err != nil {
		return nil, err
	}
	
	return &PostgresClient{db: db}, nil
}

// InitSchema tạo bảng nếu chưa tồn tại
func (p *PostgresClient) InitSchema(ctx context.Context) error {
	query := `
	CREATE TABLE IF NOT EXISTS mqtt_messages (
		id SERIAL PRIMARY KEY,
		topic VARCHAR(255) NOT NULL,
		payload TEXT NOT NULL,
		received_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP,
		created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
	);
	
	CREATE INDEX IF NOT EXISTS idx_mqtt_topic ON mqtt_messages(topic);
	CREATE INDEX IF NOT EXISTS idx_mqtt_received ON mqtt_messages(received_at);
	`
	
	_, err := p.db.ExecContext(ctx, query)
	return err
}

// SaveMessage lưu message vào database
func (p *PostgresClient) SaveMessage(ctx context.Context, topic string, payload []byte) error {
	query := `
	INSERT INTO mqtt_messages (topic, payload, received_at)
	VALUES ($1, $2, $3)
	`
	
	_, err := p.db.ExecContext(ctx, query, topic, string(payload), time.Now())
	return err
}

// GetMessages lấy messages từ database
func (p *PostgresClient) GetMessages(ctx context.Context, topic string, limit int) ([]MessageRecord, error) {
	query := `
	SELECT topic, payload, received_at
	FROM mqtt_messages
	WHERE topic = $1
	ORDER BY received_at DESC
	LIMIT $2
	`
	
	rows, err := p.db.QueryContext(ctx, query, topic, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	
	messages := make([]MessageRecord, 0, limit)
	for rows.Next() {
		var msg MessageRecord
		if err := rows.Scan(&msg.Topic, &msg.Payload, &msg.Timestamp); err != nil {
			continue
		}
		messages = append(messages, msg)
	}
	
	return messages, rows.Err()
}

// GetMessageCountByTopic đếm messages theo topic
func (p *PostgresClient) GetMessageCountByTopic(ctx context.Context, topic string) (int, error) {
	query := `SELECT COUNT(*) FROM mqtt_messages WHERE topic = $1`
	
	var count int
	err := p.db.QueryRowContext(ctx, query, topic).Scan(&count)
	return count, err
}

func (p *PostgresClient) Close() error {
	if p.db != nil {
		return p.db.Close()
	}
	return nil
}
