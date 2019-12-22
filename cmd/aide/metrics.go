package main

import (
	"context"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"net/http"
)

var (
	metricsHandler = promhttp.Handler()
	metricsKey     = "metrics"

	failuresTotal = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "aide_requests_failed_total",
			Help: "Total number of failures.",
		})
	successesTotal = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "aide_requests_completed_total",
			Help: "Total number of completed requests.",
		},
		[]string{
			"type", // create | update | remove | get
		})
	requestsActive = prometheus.NewGauge(
		prometheus.GaugeOpts{
			Name: "aide_requests_active",
			Help: "Number of active requests.",
		})
)

func init() {
	prometheus.MustRegister(failuresTotal)
	prometheus.MustRegister(successesTotal)
	prometheus.MustRegister(requestsActive)

	// Flush metrics to Prometheus.
	failuresTotal.Add(0)
	for _, s := range []string{"create", "update", "remove", "get"} {
		successesTotal.WithLabelValues(s).Add(0)
	}
	requestsActive.Add(0)
}

type Metrics struct {
	Success  bool
	TaskType string
}

func measurer(h http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestsActive.Inc()
		defer requestsActive.Dec()

		var metrics Metrics
		ctx := context.WithValue(r.Context(), metricsKey, &metrics)
		h.ServeHTTP(w, r.WithContext(ctx))

		if metrics.Success {
			successesTotal.WithLabelValues(metrics.TaskType).Inc()
		} else {
			failuresTotal.Inc()
		}
	})
}
