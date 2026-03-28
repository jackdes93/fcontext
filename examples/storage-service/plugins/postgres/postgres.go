// Package postgres
package postgres

import (
	"context"
	"flag"
	"time"

	"github.com/jackc/pgx/v4/pgxpool"
	"github.com/jackdes93/fcontext/sctx"
)

type Config struct {
	uri                string
	maxConns           int
	minConns           int
	maxConnLifeTime    int
	maxConnIdleTime    int
	healthyCheckPeriod int
}

type postgresDB struct {
	id     string
	logger sctx.Logger
	client *pgxpool.Pool
	cfg    *Config
}

func NewPostgresDB(id string) *postgresDB {
	return &postgresDB{
		id:  id,
		cfg: &Config{},
	}
}

func (p *postgresDB) Order() int {
	return 20
}

func (p *postgresDB) ID() string { return p.id }

func (p *postgresDB) InitFlags() {
	flag.StringVar(&p.cfg.uri, "postgres-uri", "postgres://localhost:5432/mqtt_db?sslmode=disable", "uri connect string of postgresDB")
	flag.IntVar(&p.cfg.maxConns, "postgres-max-conn", 4, "postgres max connections")
	flag.IntVar(&p.cfg.minConns, "postgres-min-conn", 2, "postgres min connections")
	flag.IntVar(&p.cfg.maxConnLifeTime, "postgres-max-life-time", 1, "postgres max connection life time")
	flag.IntVar(&p.cfg.maxConnIdleTime, "postgres-max-conn-idle-time", 10, "postgres max connection idle time")
	flag.IntVar(&p.cfg.healthyCheckPeriod, "postgres-healthy-check-period", 1, "postgres healthy check period")
}

func (p *postgresDB) LoadConfig() (*pgxpool.Config, error) {
	config, err := pgxpool.ParseConfig(p.cfg.uri)
	if err != nil {
		return nil, err
	}
	config.MaxConns = int32(p.cfg.maxConns)                                          // default: 4 — thường quá thấp
	config.MinConns = int32(p.cfg.minConns)                                          // giữ sẵn conn, tránh latency spike
	config.MaxConnLifetime = time.Duration(p.cfg.maxConnLifeTime) * time.Hour        // tránh conn bị Postgres/firewall kill ngầm
	config.MaxConnIdleTime = time.Duration(p.cfg.maxConnIdleTime) * time.Minute      // đóng conn không dùng
	config.HealthCheckPeriod = time.Duration(p.cfg.healthyCheckPeriod) * time.Minute // tự phát hiện conn chết
	return config, nil
}

func (p *postgresDB) Activate(ctx context.Context, service sctx.ServiceContext) error {
	p.logger = service.Logger(p.ID())
	p.logger.Info("postgresDB is starting....")

	config, err := p.LoadConfig()
	if err != nil {
		p.logger.Info("error load config ", err.Error())
		return err
	}

	conn, err := pgxpool.ConnectConfig(ctx, config)
	if err != nil {
		p.logger.Error("cannot connect to postgresDB at %s with error %s", p.cfg.uri, err.Error())
		return err
	}
	p.logger.Info("connect success to postgresdb")
	p.client = conn
	return nil
}

func (p *postgresDB) Stop(ctx context.Context) error {
	if p.client == nil {
		return nil
	}
	p.logger.Info("stopping postgres service....")
	_, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	p.client.Close()
	p.client = nil
	p.logger.Info("postgresDB service stopped")
	return nil
}

func (p *postgresDB) Query(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := p.client.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	defer rows.Close()

	return scanRows(rows)
}

func (p *postgresDB) QueryOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := p.client.Query(ctx, query)
	if err != nil {
		return nil, err
	}

	defer rows.Close()
	fields := rows.FieldDescriptions()

	if !rows.Next() {
		if err := rows.Err(); err != nil {
			return nil, err
		}
		return nil, nil
	}

	return scanRow(rows, fields)
}

func (p *postgresDB) Execute(ctx context.Context, query string, args ...any) error {
	_, err := p.client.Exec(ctx, query, args...)
	return err
}

func (p *postgresDB) ExecuteTx(ctx context.Context, fn func(tx PostgresProvider) error) error {
	tx, err := p.client.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)

	if err := fn(&postgresTx{tx: tx}); err != nil {
		return err
	}

	return tx.Commit(ctx)
}
