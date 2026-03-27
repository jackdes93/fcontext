package middleware

import (
	"net/http"
	"strings"
	"sync"

	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
	"golang.org/x/time/rate"
)

type RateLimitOptions struct {
	Enabled bool
	RPS     float64
	Burst   int
	Scope   string
	SkipCSV string
	Prefix  string
}

func RateLimitFactory(opts RateLimitOptions) MiddlewareFactory {
	return func(service fcontext.ServiceContext) Middleware {
		logger := service.Logger(opts.Prefix)
		if !opts.Enabled {
			return func(c *gin.Context) {
				c.Next()
			}
		}

		skip := make(map[string]struct{}, 8)
		for _, p := range splitCSV(opts.SkipCSV) {
			skip[p] = struct{}{}
		}
		shouldSkip := func(path string) bool { _, ok := skip[path]; return ok }
		switch strings.ToLower(opts.Scope) {
		case "global":
			lim := rate.NewLimiter(rate.Limit(opts.RPS), opts.Burst)
			return func(c *gin.Context) {
				if shouldSkip(c.FullPath()) {
					c.Next()
					return
				}
				if !lim.Allow() {
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limited"})
					return
				}
				c.Next()
			}
		default:
			var ipLimiters sync.Map
			get := func(ip string) *rate.Limiter {
				if v, ok := ipLimiters.Load(ip); ok {
					return v.(*rate.Limiter)
				}
				n := rate.NewLimiter(rate.Limit(opts.RPS), opts.Burst)
				if v, loaded := ipLimiters.LoadOrStore(ip, n); loaded {
					return v.(*rate.Limiter)
				}
				return n
			}
			return func(c *gin.Context) {
				if shouldSkip(c.FullPath()) {
					c.Next()
					return
				}
				ip := c.ClientIP()
				if !get(ip).Allow() {
					logger.Warn("ip=%s limited path=%s", ip, c.FullPath())
					c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{"error": "rate limited"})
					return
				}
				c.Next()
			}
		}
	}
}
