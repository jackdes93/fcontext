package sctx

import (
	"context"
	"os"
	"os/signal"
	"syscall"
)

func Run(app ServiceContext, fn func(ctx context.Context) error) (err error) {
	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	if err = app.Load(); err != nil {
		return err
	}

	defer func() {
		_ = app.Stop()
	}()

	if e := fn(ctx); err != nil {
		err = e
	}

	signal.Stop(make(chan os.Signal, 1))
	return
}
