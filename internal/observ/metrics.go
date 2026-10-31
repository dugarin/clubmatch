// Package observ exposes Prometheus metrics (05-risks-observability, NFR-1…3, NFR-12).
package observ

import (
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	HTTPLatency = prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    "http_latency_seconds",
		Help:    "HTTP request latency by endpoint",
		Buckets: []float64{0.05, 0.1, 0.25, 0.5, 1, 3, 5},
	}, []string{"endpoint", "status"})

	NotificationLag = prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:    "notification_lag_seconds",
		Help:    "Seconds from notification creation to delivery",
		Buckets: []float64{1, 5, 15, 30, 60, 300, 900},
	})
	NotificationSent = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notification_sent_total",
	})
	NotificationBlocked = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notification_blocked_total",
	})
	NotificationRetried = prometheus.NewCounter(prometheus.CounterOpts{
		Name: "notification_retried_total",
	})
	OutboxDepth = prometheus.NewGauge(prometheus.GaugeOpts{
		Name: "outbox_depth",
		Help: "Pending notifications in outbox",
	})
)

func init() {
	prometheus.MustRegister(HTTPLatency, NotificationLag, NotificationSent, NotificationBlocked, NotificationRetried, OutboxDepth)
}

func MetricsHandler() http.Handler {
	return promhttp.Handler()
}

func LatencyMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := &statusWriter{ResponseWriter: w, status: 200}
		next.ServeHTTP(ww, r)
		HTTPLatency.WithLabelValues(endpointLabel(r), strconv.Itoa(ww.status)).Observe(time.Since(start).Seconds())
	})
}

func endpointLabel(r *http.Request) string {
	if rctx := chi.RouteContext(r.Context()); rctx != nil {
		if p := rctx.RoutePattern(); p != "" {
			return p
		}
	}
	return "unknown"
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}
