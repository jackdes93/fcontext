package postgres

import (
	"context"

	"github.com/jackc/pgx/v4"
)

// db/postgres_tx.go
type postgresTx struct {
	tx pgx.Tx
}

func (t *postgresTx) Query(ctx context.Context, query string, args ...any) ([]map[string]any, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	return scanRows(rows)
}

func (t *postgresTx) QueryOne(ctx context.Context, query string, args ...any) (map[string]any, error) {
	rows, err := t.tx.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	fields := rows.FieldDescriptions()
	if !rows.Next() {
		return nil, rows.Err()
	}
	return scanRow(rows, fields)
}

func (t *postgresTx) Execute(ctx context.Context, query string, args ...any) error {
	_, err := t.tx.Exec(ctx, query, args...)
	return err
}

func (t *postgresTx) ExecuteTx(ctx context.Context, fn func(tx PostgresProvider) error) error {
	// Đã trong transaction rồi, chạy thẳng fn với chính nó
	return fn(t)
}
