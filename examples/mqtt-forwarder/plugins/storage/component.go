package storage

import (
	"context"
	"flag"
	"fmt"
	"os"
	"time"

	"github.com/jackdes93/fcontext"
)

type StorageComponent struct {
	id         string
	log        fcontext.Logger
	cfg        Config
	redis      *RedisClient
	postgres   *PostgresClient
}

type Config struct {
	// Redis
	RedisAddr string
	RedisTTL  time.Duration
	
	// PostgreSQL
	PostgresDSN string
	EnableRedis bool
	EnablePostgres bool
}

func NewComponent(id string) *StorageComponent {
	return &StorageComponent{
		id: id,
		cfg: Config{
			RedisAddr:      "localhost:6379",
			RedisTTL:       24 * time.Hour,
			PostgresDSN:    "",
			EnableRedis:    true,
			EnablePostgres: true,
		},
	}
}

func (c *StorageComponent) ID() string { return c.id }

func (c *StorageComponent) InitFlags() {
	flag.StringVar(&c.cfg.RedisAddr, "redis-addr", os.Getenv("REDIS_ADDR"), "Redis address")
	flag.StringVar(&c.cfg.PostgresDSN, "postgres-dsn", os.Getenv("POSTGRES_DSN"), "PostgreSQL DSN")
	flag.BoolVar(&c.cfg.EnableRedis, "enable-redis", true, "Enable Redis")
	flag.BoolVar(&c.cfg.EnablePostgres, "enable-postgres", true, "Enable PostgreSQL")
}

func (c *StorageComponent) Order() int { return 25 } // khởi động trước MQTT

func (c *StorageComponent) Activate(ctx context.Context, sv fcontext.ServiceContext) error {
	c.log = sv.Logger(c.ID())

	// Init Redis
	if c.cfg.EnableRedis && c.cfg.RedisAddr != "" {
		c.redis = NewRedisClient(c.cfg.RedisAddr, c.cfg.RedisTTL)
		if err := c.redis.Ping(ctx); err != nil {
			c.log.Error("redis ping failed", "error", err)
			c.redis = nil
		} else {
			c.log.Info("redis connected", "addr", c.cfg.RedisAddr)
		}
	}

	// Init PostgreSQL
	if c.cfg.EnablePostgres && c.cfg.PostgresDSN != "" {
		pg, err := NewPostgresClient(c.cfg.PostgresDSN)
		if err != nil {
			c.log.Error("postgres connection failed", "error", err)
		} else {
			c.postgres = pg
			if err := c.postgres.InitSchema(ctx); err != nil {
				c.log.Error("postgres schema init failed", "error", err)
			}
			c.log.Info("postgres connected")
		}
	}

	if c.redis == nil && c.postgres == nil {
		return fmt.Errorf("no storage backend available")
	}

	c.log.Info("storage component started")
	return nil
}

func (c *StorageComponent) Stop(ctx context.Context) error {
	if c.redis != nil {
		c.redis.Close()
	}
	if c.postgres != nil {
		c.postgres.Close()
	}
	return nil
}

// SaveMessage lưu vào cả Redis và Postgres
func (c *StorageComponent) SaveMessage(ctx context.Context, topic string, payload []byte) error {
	if c.redis != nil {
		if err := c.redis.SaveMessage(ctx, topic, payload); err != nil {
			c.log.Warn("redis save failed", "error", err)
		}
	}

	if c.postgres != nil {
		if err := c.postgres.SaveMessage(ctx, topic, payload); err != nil {
			c.log.Warn("postgres save failed", "error", err)
		}
	}

	return nil
}

// GetMessages từ database
func (c *StorageComponent) GetMessages(ctx context.Context, topic string, limit int) ([]MessageRecord, error) {
	if c.postgres != nil {
		return c.postgres.GetMessages(ctx, topic, limit)
	}
	if c.redis != nil {
		return c.redis.GetMessages(ctx, topic, int64(limit))
	}
	return nil, fmt.Errorf("no storage backend available")
}

// GetRedisClient expose redis client
func (c *StorageComponent) GetRedisClient() *RedisClient {
	return c.redis
}

// GetPostgresClient expose postgres client
func (c *StorageComponent) GetPostgresClient() *PostgresClient {
	return c.postgres
}
