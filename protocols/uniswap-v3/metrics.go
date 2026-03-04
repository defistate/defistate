package uniswapv3

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// --- Prometheus Metrics Definition ---

// Metrics contains all the Prometheus metrics for the UniswapV3System.
type Metrics struct {
	// --- Tier 1: Critical System Health & Liveness ---
	LastProcessedBlock *prometheus.GaugeVec
	ErrorsTotal        *prometheus.CounterVec

	// --- Tier 2: Performance & Bottleneck Identification ---
	PendingInitQueueSize   *prometheus.GaugeVec
	BlockProcessingDur     *prometheus.HistogramVec
	PoolInitDur            *prometheus.HistogramVec
	ReconciliationDuration *prometheus.HistogramVec
	PruningDuration        *prometheus.HistogramVec

	// --- Tier 3: Data & State Integrity ---
	PoolsInRegistry  *prometheus.GaugeVec
	PoolsInitialized *prometheus.CounterVec
	PoolsPruned      *prometheus.CounterVec

	UpdateCachedVeiwDur *prometheus.HistogramVec
}

// NewMetrics creates and registers all the Prometheus metrics for the system.
func NewMetrics(reg prometheus.Registerer, systemName string) *Metrics {
	return &Metrics{
		// --- Tier 1 Metrics ---
		LastProcessedBlock: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: systemName,
			Name:      "last_processed_block",
			Help:      "The block number of the last block successfully processed or skipped by the system.",
		}, []string{}),

		ErrorsTotal: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: systemName,
			Name:      "errors_total",
			Help:      "Total number of errors encountered by the system, labeled by error type.",
		}, []string{"type"}),

		// --- Tier 2 Metrics ---
		PendingInitQueueSize: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: systemName,
			Name:      "pending_initialization_queue_size",
			Help:      "The current number of pools waiting in the queue for asynchronous initialization.",
		}, []string{}),

		BlockProcessingDur: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: systemName,
			Name:      "block_processing_duration_seconds",
			Help:      "A histogram of the time it takes to process a single block (the 'fast path').",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),

		PoolInitDur: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: systemName,
			Name:      "pool_initialization_duration_seconds",
			Help:      "A histogram of the time it takes for a batch of pending pools to be initialized.",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),

		ReconciliationDuration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: systemName,
			Name:      "reconciliation_duration_seconds",
			Help:      "A histogram of the time it takes for the state reconciler to run a full cycle.",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),

		PruningDuration: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: systemName,
			Name:      "pruning_duration_seconds",
			Help:      "A histogram of the time it takes for the pruner to run a full cycle.",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),

		// --- Tier 3 Metrics ---
		PoolsInRegistry: promauto.With(reg).NewGaugeVec(prometheus.GaugeOpts{
			Subsystem: systemName,
			Name:      "pools_in_registry_total",
			Help:      "The total number of pools currently being tracked in the system's registry.",
		}, []string{}),

		PoolsInitialized: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: systemName,
			Name:      "pools_initialized_total",
			Help:      "A counter of pools successfully initialized and added to the registry.",
		}, []string{}),

		PoolsPruned: promauto.With(reg).NewCounterVec(prometheus.CounterOpts{
			Subsystem: systemName,
			Name:      "pools_pruned_total",
			Help:      "A counter of pools removed from the registry by the pruner.",
		}, []string{"reason"}), // reason can be 'blocked' or 'orphaned'

		UpdateCachedVeiwDur: promauto.With(reg).NewHistogramVec(prometheus.HistogramOpts{
			Subsystem: systemName,
			Name:      "update_cached_view_duration_seconds",
			Help:      "A histogram of the time it takes for a cached view update",
			Buckets:   prometheus.DefBuckets,
		}, []string{}),
	}
}
