// Package jobdb
package jobdb

import (
	"context"

	"github.com/binhdp/storage-service/plugins/postgres"
)

type CheckSchemaExistsHandler struct {
	Query string
	DB    postgres.PostgresProvider
}

func (h *CheckSchemaExistsHandler) Handle(ctx context.Context) error {
	return h.DB.Execute(ctx, h.Query)
}

func (h *CheckSchemaExistsHandler) Type() string {
	return "check-schema-exists"
}
