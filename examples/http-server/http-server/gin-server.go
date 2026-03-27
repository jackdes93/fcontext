package httpserver

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"time"

	"github.com/binhdp/example/plugins/http-server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

const (
	defaultPort = 3000
	defaultMode = "debug"
)

type (
	RouteRegistrar        func(r *gin.Engine)
	RouteRegistrarFactory func(s fcontext.ServiceContext) RouteRegistrar
	Config                struct {
		Port int
		Mode string
	}
)

type ginEngineer struct {
	*Config
	id                  string
	logger              fcontext.Logger
	router              *gin.Engine
	srv                 *http.Server
	registrarFactories  []RouteRegistrarFactory
	middlewareFactories []middleware.MiddlewareFactory
}

func (g *ginEngineer) Order() int {
	// TODO implement me
	panic("implement me")
}

func NewGinService(id string) *ginEngineer {
	return &ginEngineer{
		Config: &Config{
			Port: defaultPort,
			Mode: defaultMode,
		},
		id: id,
	}
}

func (g *ginEngineer) ID() string { return g.id }
func (g *ginEngineer) InitFlags() {
	flag.IntVar(&g.Config.Port, "gin-port", defaultPort, "gin server port.")
	flag.StringVar(&g.Config.Mode, "gin-mode", defaultMode, "gin server mode running")
}

func (g *ginEngineer) WithRegistrarFactory(f RouteRegistrarFactory) *ginEngineer {
	g.registrarFactories = append(g.registrarFactories, f)
	return g
}

func (g *ginEngineer) WithMiddlewareFactory(mw middleware.MiddlewareFactory) *ginEngineer {
	g.middlewareFactories = append(g.middlewareFactories, mw)
	return g
}

func (g *ginEngineer) Activate(ctx context.Context, service fcontext.ServiceContext) error {
	g.logger = service.Logger(g.ID())

	switch g.Config.Mode {
	case gin.ReleaseMode:
		gin.SetMode(gin.ReleaseMode)
	case gin.DebugMode, gin.TestMode:
		gin.SetMode(gin.DebugMode)
	default:
		gin.SetMode(gin.DebugMode)
	}

	g.logger.Info("Gin service init....")
	g.router = gin.New()

	for _, mw := range g.middlewareFactories {
		g.router.Use(mw(service))
	}

	g.router.GET("/health", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"status": "OK"})
	})

	for _, f := range g.registrarFactories {
		rr := f(service)
		rr(g.router)
	}

	g.srv = &http.Server{
		Addr:              fmt.Sprintf(":%d", g.Config.Port),
		Handler:           g.router,
		ReadHeaderTimeout: 10 * time.Second,
	}

	go func() {
		g.logger.Info("gin service listening on %s", g.srv.Addr)
		if err := g.srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			g.logger.Error("gin server error: %v", err)
		}
	}()

	return nil
}

func (g *ginEngineer) Stop(ctx context.Context) error {
	if g.srv == nil {
		return nil
	}
	g.logger.Info("stopping gin service....")
	shutdownCtx, cancel := context.WithTimeout(ctx, time.Second*5)
	defer cancel()
	if err := g.srv.Shutdown(shutdownCtx); err != nil {
		g.logger.Error("gin shutdown error: %v", err)
		return err
	}
	g.logger.Info("gin service stopped")
	return nil
}
