package ticks

import (
	"github.com/prometheus/client_golang/prometheus"
)

// --- Prometheus Metrics Definition ---

// indexerMetrics holds all the Prometheus collectors for the TickIndexer.
type indexerMetrics struct {
	trackedPoolsTotal         prometheus.Gauge
	lastProcessedBlock        prometheus.Gauge
	pendingUpdatesQueue       prometheus.Gauge
	errorsTotal               *prometheus.CounterVec
	updatesDroppedTotal       prometheus.Counter
	resyncPoolsCorrectedTotal prometheus.Counter
	resyncDuration            *prometheus.HistogramVec
	blockProcessingDuration   *prometheus.HistogramVec
}

// newIndexerMetrics creates and registers all the Prometheus metrics.
func newIndexerMetrics(reg prometheus.Registerer, systemName string) *indexerMetrics {
	// We append "_tick_indexer" to the system name to create a clear, unique subsystem.
	// e.g., "uniswapv3_tick_indexer" or "pancakeswapv3_tick_indexer"
	subsystem := systemName + "_tick_indexer"

	m := &indexerMetrics{
		trackedPoolsTotal: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      "tracked_pools_total",
				Help:      "The total number of pools currently being tracked by the indexer.",
			},
		),
		lastProcessedBlock: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      "last_processed_block",
				Help:      "The block number of the last block successfully processed by the indexer.",
			},
		),
		pendingUpdatesQueue: prometheus.NewGauge(
			prometheus.GaugeOpts{
				Subsystem: subsystem,
				Name:      "pending_updates_queue_depth",
				Help:      "The current number of items in the pending updates queue.",
			},
		),
		errorsTotal: prometheus.NewCounterVec(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "errors_total",
				Help:      "The total number of errors encountered, labeled by type.",
			},
			[]string{"type"}, // label: "initialization", "rpc", "update_dropped"
		),
		updatesDroppedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "updates_dropped_total",
				Help:      "The total number of real-time updates dropped due to a full queue.",
			},
		),
		resyncPoolsCorrectedTotal: prometheus.NewCounter(
			prometheus.CounterOpts{
				Subsystem: subsystem,
				Name:      "resync_ticks_corrected_total",
				Help:      "The total number of ticks corrected by the resyncer, labeled by change type.",
			}),

		blockProcessingDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "block_processing_duration_seconds",
			Help:      "A histogram of the time it takes to process a single block (the 'fast path').",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),
		resyncDuration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: subsystem,
			Name:      "resync_duration_seconds",
			Help:      "A histogram of the time it takes for the state resyncer to run a full cycle.",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),
	}

	reg.MustRegister(
		m.trackedPoolsTotal,
		m.lastProcessedBlock,
		m.pendingUpdatesQueue,
		m.errorsTotal,
		m.updatesDroppedTotal,
		m.resyncPoolsCorrectedTotal,
		m.blockProcessingDuration,
		m.resyncDuration,
	)

	return m
}
