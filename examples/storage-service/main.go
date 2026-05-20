package main

import (
	"context"
	"time"

	jobdb "github.com/binhdp/storage-service/modules/jobDB"
	"github.com/binhdp/storage-service/plugins/postgres"
	"github.com/jackdes93/fcontext/job"
	"github.com/jackdes93/fcontext/sctx"
)

func newService() sctx.ServiceContext {
	service := sctx.New(
		sctx.WithName("Storage Service"),
		sctx.WithComponent(postgres.NewPostgresDB("postgres")),
	)
	return service
}

func CheckSchemaTableExists(ctx context.Context, service sctx.ServiceContext) {
	db := service.MustGet("postgres").(postgres.PostgresProvider)
	logger := service.Logger("hub-job")
	hub := job.NewHub(func(j job.Job) bool {
		go func() {
			_ = j.RunWithRetry(ctx)
		}()
		return true
	})

	procedureHandler := &jobdb.CheckSchemaExistsHandler{
		Query: `
			CREATE TABLE IF NOT EXISTS group_services(
				id VARCHAR(10) PRIMARY KEY,
				name VARCHAR(255) NOT NULL,
				kind VARCHAR(20) NOT NULL,
				created_at TIMESTAMP DEFAULT CURRENT_TIMESTAMP
			);

			CREATE INDEX IF NOT EXISTS idx_group_service_name ON group_services(name);
			CREATE INDEX IF NOT EXISTS idx_group_service_kind ON group_services(kind);
		`,
		DB: db,
	}

	procedureJob, _ := hub.Create("check-schema",
		procedureHandler,
		job.WithName("Procedure"),
		job.WithTimeout(5*time.Second),
		job.WithRetries([]time.Duration{
			100 * time.Millisecond, // Retry 100ms later
			500 * time.Millisecond, // Then 500ms later
			1 * time.Second,        // Then 1s later
		}),
		job.WithJitter(0.2),
		job.WithOnRetry(func(idx int, delay time.Duration, err error) {
			logger.Info("[Retry %d] Will retry after %v due to: %v\n", idx, delay, err)
		}),
	)

	hub.Submit(procedureJob)
	time.Sleep(3 * time.Second)
	if procedureJob.State() != job.StateCompleted {
		logger.Error("Job Procedure", procedureJob.State().String())
		return
	}
	logger.Info("Job Procedure completed")
}

func main() {
	service := newService()

	if err := sctx.Run(service, func(ctx context.Context) error {
		CheckSchemaTableExists(ctx, service)
		<-ctx.Done()
		return nil
	}); err != nil {
		panic(err)
	}
}
