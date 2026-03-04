package clientmanager

import "github.com/prometheus/client_golang/prometheus"

// -----------------------
// Prometheus Metrics
// -----------------------
// @todo add more fine grained metrics
var (
	// clientHealthCheckCounter tracks the total number of health checks,
	// labeled by their result ("success" or "failure").
	clientHealthCheckCounter = prometheus.NewCounterVec(
		prometheus.CounterOpts{
			Name: "client_manager_client_health_check_total",
			Help: "Total number of health checks performed by clients, labeled by result.",
		},
		[]string{"status"},
	)

	// clientHealthCheckDuration measures the duration (in seconds) of each health check.
	clientHealthCheckDuration = prometheus.NewHistogram(
		prometheus.HistogramOpts{
			Name:    "client_manager_client_health_check_duration_seconds",
			Help:    "Duration of client health checks in seconds.",
			Buckets: prometheus.DefBuckets,
		},
	)

	// clientManagerNewBlockProcessedCounter counts how many new blocks have been processed by the ClientManager.
	clientManagerNewBlockProcessedCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "client_manager_new_block_processed_total",
			Help: "Total number of new blocks processed by the ClientManager.",
		},
	)

	// clientManagerSubscriptionErrorCounter counts errors occurring in subscribeNewBlocks.
	clientManagerSubscriptionErrorCounter = prometheus.NewCounter(
		prometheus.CounterOpts{
			Name: "client_manager_subscription_error_total",
			Help: "Total number of errors in block subscription in subscribeNewBlocks.",
		},
	)
)

func init() {
	// Register all Prometheus metrics in one call for clarity.
	prometheus.MustRegister(
		clientHealthCheckCounter,
		clientHealthCheckDuration,
		clientManagerNewBlockProcessedCounter,
		clientManagerSubscriptionErrorCounter,
	)
}
