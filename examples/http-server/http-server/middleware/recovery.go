package middleware

import (
	"errors"
	"fmt"
	"net/http"
	"runtime/debug"
	"strings"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
)

type CanGetStatusCode interface {
	error
	StatusCode() int
}

type CanGetPayload interface {
	Payload() any
}

type RecoveryOptions struct {
	ForceLogStack  bool
	RepanicInDebug bool
	LoggerPrefix   string
}

func Recovery(opts *RecoveryOptions) MiddlewareFactory {
	return func(service fcontext.ServiceContext) Middleware {
		if opts == nil {
			opts = &RecoveryOptions{}
		}
		prefix := opts.LoggerPrefix
		if prefix == "" {
			prefix = "http.recovery"
		}
		log := service.Logger(prefix)
		debugMode := gin.IsDebugging()

		return func(c *gin.Context) {
			start := time.Now()
			method := c.Request.Method
			path := c.Request.URL.Path
			ua := c.Request.UserAgent()
			ip := c.ClientIP()
			reqID := c.GetHeader("X-Request-ID")

			defer func() {
				if rec := recover(); rec != nil {
					var appErr error
					switch v := rec.(type) {
					case error:
						appErr = v
					default:
						appErr = errors.New(strings.TrimSpace(toString(v)))
					}

					status := http.StatusInternalServerError
					var sc CanGetStatusCode
					if errors.As(appErr, &sc) {
						status = sc.StatusCode()
					}

					var payload any
					var cp CanGetPayload
					if errors.As(appErr, &cp) {
						payload = cp.Payload()
					} else {
						payload = gin.H{
							"code":    status,
							"status":  http.StatusText(status),
							"message": "something went wrong, please try again or contact supporters",
						}
					}
					c.AbortWithStatusJSON(status, payload)
					lat := time.Since(start)
					if debugMode || opts.ForceLogStack {
						log.Error("panic recovered: err=%v method=%s path=%s status=%d ip=%s ua=%q req_id=%s latency=%s\n%s",
							appErr, method, path, status, ip, ua, reqID, lat, debug.Stack())
					} else {
						log.Error(
							"panic recovered: err=%v method=%s path=%s status=%d ip=%s ua=%q req_id=%s latency=%s",
							appErr, method, path, status, ip, ua, reqID, lat,
						)
					}
					if debugMode && opts.RepanicInDebug {
						panic(rec)
					}
				}
			}()
			c.Next()
		}
	}
}

func toString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case []byte:
		return string(t)
	default:
		return fmt.Sprintf("%v", t)
	}
}
