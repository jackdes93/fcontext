package main

import (
	"context"
	"net/http"

	httpserver "github.com/binhdp/example/plugins/http-server"
	"github.com/binhdp/example/plugins/http-server/middleware"
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

func ApiRoutes(s fcontext.ServiceContext) httpserver.RouteRegistrar {
	//log := s.Logger("api")
	return func(r *gin.Engine) {
		//log.Info("mounting API routes...")
		v1 := r.Group("/api/v1")
		v1.GET("/ping", func(c *gin.Context) {
			c.JSON(http.StatusOK, gin.H{"message": "pong"})
		})
	}
}

func newService() fcontext.ServiceContext {
	requestLogger := middleware.RequestLogger(&middleware.RequestLoggerOption{
		Enable: true,
		Prefix: "request-logger",
	})
	recoveryMW := middleware.Recovery(&middleware.RecoveryOptions{
		ForceLogStack:  false,
		RepanicInDebug: false,
		LoggerPrefix:   "recovery",
	})
	service := fcontext.New(
		fcontext.WithName("Demo-Gin-Server"),
		fcontext.WithComponent(httpserver.NewGinService("gin").
			WithMiddlewareFactory(recoveryMW).
			WithMiddlewareFactory(requestLogger).
			WithRegistrarFactory(ApiRoutes)),
	)
	return service
}

func main() {
	if err := fcontext.Run(newService(), func(ctx context.Context) error {
		<-ctx.Done()
		return nil
	}); err != nil {
		panic(err)
	}

}
