package differ

import (
	"github.com/prometheus/client_golang/prometheus"
)

// --- Metrics ---

// Metrics holds all the Prometheus metrics for the differ.
type Metrics struct {
	diffDuration      *prometheus.HistogramVec
	subsystemDuration *prometheus.HistogramVec
	diffsTotal        *prometheus.CounterVec
}

// NewMetrics creates and registers the metrics for the differ.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		diffDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "differ_diff_duration_seconds",
			Help:    "Total time taken to compute the full state diff.",
			Buckets: prometheus.DefBuckets,
		}, []string{}),
		subsystemDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "differ_subsystem_duration_seconds",
			Help:    "Time taken to compute the diff for a single subsystem.",
			Buckets: prometheus.DefBuckets,
		}, []string{"subsystem"}),
		diffsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "differ_diffs_total",
			Help: "Total number of diffs computed, labeled by subsystem and result.",
		}, []string{"subsystem", "result"}),
	}
	reg.MustRegister(m.diffDuration, m.subsystemDuration, m.diffsTotal)
	return m
}
