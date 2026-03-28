package postgres

import "context"

type PostgresProvider interface {
	Query(ctx context.Context, query string, args ...interface{}) ([]map[string]interface{}, error)
	QueryOne(ctx context.Context, query string, args ...interface{}) (map[string]interface{}, error)
	Execute(ctx context.Context, query string, args ...interface{}) error
	ExecuteTx(ctx context.Context, fn func(tx PostgresProvider) error) error // transaction support
}