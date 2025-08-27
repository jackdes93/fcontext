package middleware

import (
	"strconv"
	"strings"
	"time"

	httpserver "github.com/binhdp/example/plugins/http-server"
	"github.com/gin-gonic/gin"
	"github.com/jackdes93/fcontext"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type PrometheusOption struct {
	Enabled   bool
	NameSpace string
	SubSystem string
	App       string
	Buckets   []float64
	Path      string
}

func PrometheusMiddlewareFactory(opts PrometheusOption) MiddlewareFactory {
	return func(service fcontext.ServiceContext) Middleware {
		if !opts.Enabled {
			return func(c *gin.Context) {
				c.Next()
			}
		}

		nameSpace := strings.TrimSpace(opts.NameSpace)
		subsystem := strings.TrimSpace(opts.SubSystem)
		app := strings.TrimSpace(opts.App)

		bk := opts.Buckets
		if len(bk) == 0 {
			bk = []float64{0.005, 0.01, 0.025, 0.05, 0.075, 0.1, 0.25, 0.5, 0.75, 1, 2, 4, 8}
		}
		labels := []string{"method", "route", "status"}
		if app != "" {
			labels = append(labels, "app")
		}
		inFlight := prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: nameSpace,
			Subsystem: subsystem,
			Name:      "request_in_flight",
			Help:      "Current number of in-flight HTTP requests.",
		})
		reqTotal := prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: nameSpace,
			Subsystem: subsystem,
			Name:      "request_total",
			Help:      "Total number of HTTP requests partitioned by status, method and route",
		}, labels)
		reqDuration := prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: nameSpace,
			Subsystem: subsystem,
			Name:      "request_duration_seconds",
			Help:      "HTTP request duration in seconds partitioned by status, method and route",
			Buckets:   bk,
		}, labels)
		prometheus.MustRegister(inFlight, reqDuration, reqTotal)
		return func(c *gin.Context) {
			start := time.Now()
			inFlight.Inc()
			defer inFlight.Dec()
			c.Next()
			route := c.FullPath()
			if route == "" {
				route = c.Request.URL.Path
			}
			method := c.Request.Method
			status := strconv.Itoa(c.Writer.Status())
			vals := []string{method, route, status}
			if app != "" {
				vals = append(vals, app)
			}
			reqTotal.WithLabelValues(vals...).Inc()
			reqDuration.WithLabelValues(vals...).Observe(float64(time.Since(start).Seconds()))
		}
	}
}

func PrometheusHandlerRegistrar(opts PrometheusOption) httpserver.RouteRegistrar {
	path := opts.Path
	if strings.TrimSpace(path) == "" {
		path = "/metrics"
	}
	h := promhttp.Handler()
	return func(r *gin.Engine) {
		r.GET(path, gin.WrapH(h))
	}
}
