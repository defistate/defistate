package engine

import (
	"github.com/prometheus/client_golang/prometheus"
)

// --- Prometheus Metrics Definition ---

// Metrics contains all the Prometheus metrics for the engine.
type Metrics struct {
	// --- Configuration Context (set once at startup) ---
	ActiveProtocols prometheus.Gauge

	// --- System Health & Liveness ---
	LastProcessedBlock prometheus.Gauge
	ErrorsTotal        *prometheus.CounterVec

	// --- Performance & Bottleneck Identification ---
	BlockQueueDepth    prometheus.Gauge
	ActiveSubscribers  prometheus.Gauge
	BlockProcessingDur prometheus.Histogram
	SyncDur            prometheus.Histogram
	SyncCheckDur       prometheus.Histogram
	StateBuildDur      prometheus.Histogram
}

// NewMetrics creates and explicitly registers all the Prometheus metrics for the system.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		// --- Configuration Context ---

		ActiveProtocols: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "engine_protocols",
			Help: "Total number of protocols configured and active in the engine.",
		}),

		// --- System Health & Liveness ---
		LastProcessedBlock: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "engine_last_processed_block",
			Help: "The block number of the last block successfully processed by the engine.",
		}),
		ErrorsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "engine_errors_total",
			Help: "Total number of errors encountered by the engine, labeled by error type.",
		}, []string{"type"}),

		// --- Performance & Bottleneck Identification ---
		BlockQueueDepth: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "engine_block_queue_depth",
			Help: "The current number of blocks waiting in the processing queue.",
		}),
		ActiveSubscribers: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "engine_active_subscribers",
			Help: "The current number of active subscribers receiving view events.",
		}),
		BlockProcessingDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "engine_block_processing_duration_seconds",
			Help:    "A histogram of the time it takes to fully process a single block (sync + build).",
			Buckets: prometheus.DefBuckets,
		}),
		SyncDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "engine_sync_duration_seconds",
			Help:    "A histogram of the time it takes for all block-synchronized protocols to sync for a given block.",
			Buckets: prometheus.DefBuckets,
		}),
		SyncCheckDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "engine_sync_check_duration_seconds",
			Help:    "A histogram of the time it takes to execute a single check of areProtocolsSynced.",
			Buckets: prometheus.DefBuckets,
		}),
		StateBuildDur: prometheus.NewHistogram(prometheus.HistogramOpts{
			Name:    "engine_state_build_duration_seconds",
			Help:    "A histogram of the time it takes to build the full state from all protocols after they are synced.",
			Buckets: prometheus.DefBuckets,
		}),
	}

	// Explicitly register all the metrics with the provided registry.
	// MustRegister will panic on startup if there's a duplicate metric name, which is good for catching errors early.
	reg.MustRegister(
		m.ActiveProtocols,
		m.LastProcessedBlock,
		m.ErrorsTotal,
		m.BlockQueueDepth,
		m.ActiveSubscribers,
		m.BlockProcessingDur,
		m.SyncDur,
		m.SyncCheckDur,
		m.StateBuildDur,
	)

	return m
}
