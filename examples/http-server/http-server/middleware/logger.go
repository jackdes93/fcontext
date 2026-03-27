package middleware

import (
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

type RequestLoggerOption struct {
	Enable bool
	Prefix string
}

func RequestLogger(opts *RequestLoggerOption) MiddlewareFactory {
	return func(service fcontext.ServiceContext) Middleware {
		logger := service.Logger(opts.Prefix)
		if !opts.Enable {
			return func(c *gin.Context) {
				c.Next()
			}
		}
		return func(c *gin.Context) {
			start := time.Now()
			path := c.Request.URL.Path
			method := c.Request.Method
			c.Next()
			lat := time.Since(start)
			status := c.Writer.Status()
			logger.Info("%s %s -> %d (%s)", method, path, status, lat)
		}
	}
}
