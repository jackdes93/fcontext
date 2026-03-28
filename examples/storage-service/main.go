package main

import (
	"context"

	"github.com/binhdp/storage-service/plugins/postgres"
	"github.com/jackdes93/fcontext/sctx"
)


func newService() sctx.ServiceContext {
	service := sctx.New(
		sctx.WithName("Storage Service"),
		sctx.WithComponent(postgres.NewPostgresDB("postgres"))
	)
	return service
}

func main() {
	service := newService()
	
	if err := sctx.Run(service, func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}); err != nil {
		panic(err)
	}
}