package middleware

import (
	"strings"
	"time"

	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

type CORSOptions struct {
	Enabled       bool
	Origins       string
	Methods       string
	Headers       string
	Credentials   bool
	MaxAgeSeconds int
}

func CorsMW(opts CORSOptions) MiddlewareFactory {
	return func(service fcontext.ServiceContext) Middleware {
		if !opts.Enabled {
			return func(c *gin.Context) { c.Next() }
		}
		cfg := cors.Config{
			AllowMethods:     splitCSV(opts.Methods),
			AllowHeaders:     splitCSV(opts.Headers),
			ExposeHeaders:    []string{},
			AllowCredentials: opts.Credentials,
			MaxAge:           time.Duration(opts.MaxAgeSeconds) * time.Second,
		}
		origins := strings.TrimSpace(opts.Origins)
		if origins == "*" {
			cfg.AllowAllOrigins = true
		} else {
			cfg.AllowOrigins = splitCSV(origins)
		}
		return cors.New(cfg)
	}
}

func splitCSV(s string) []string {
	if strings.TrimSpace(s) == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if v := strings.TrimSpace(p); v != "" {
			out = append(out, v)
		}
	}
	return out
}
