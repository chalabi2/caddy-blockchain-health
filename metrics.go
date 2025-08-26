package blockchain_health

import (
	"github.com/prometheus/client_golang/prometheus"
)

// NewMetrics creates a new metrics instance
func NewMetrics() *Metrics {
	return &Metrics{
		totalChecks: prometheus.NewCounter(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "checks_total",
			Help:      "Total number of health checks performed",
		}),
		healthyNodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "healthy_nodes",
			Help:      "Number of currently healthy nodes",
		}),
		unhealthyNodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "unhealthy_nodes",
			Help:      "Number of currently unhealthy nodes",
		}),
		checkDuration: prometheus.NewHistogram(prometheus.HistogramOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "check_duration_seconds",
			Help:      "Duration of health checks in seconds",
			Buckets:   prometheus.DefBuckets,
		}),
		blockHeightGauge: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "block_height",
			Help:      "Current block height of each node",
		}, []string{"node_name"}),
		errorCount: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "errors_total",
			Help:      "Total number of errors by node and type",
		}, []string{"node_name", "error_type"}),
	}
}

// Register registers all metrics with the default prometheus registry
func (m *Metrics) Register() error {
	collectors := []prometheus.Collector{
		m.totalChecks,
		m.healthyNodes,
		m.unhealthyNodes,
		m.checkDuration,
		m.blockHeightGauge,
		m.errorCount,
	}

	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			// If already registered, that's okay
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// Unregister removes all metrics from the default prometheus registry
func (m *Metrics) Unregister() {
	collectors := []prometheus.Collector{
		m.totalChecks,
		m.healthyNodes,
		m.unhealthyNodes,
		m.checkDuration,
		m.blockHeightGauge,
		m.errorCount,
	}

	for _, collector := range collectors {
		prometheus.Unregister(collector)
	}
}

// RecordCheckDuration records the duration of a health check
func (m *Metrics) RecordCheckDuration(duration float64) {
	m.checkDuration.Observe(duration)
}

// IncrementTotalChecks increments the total checks counter
func (m *Metrics) IncrementTotalChecks() {
	m.totalChecks.Inc()
}

// SetHealthyNodes sets the number of healthy nodes
func (m *Metrics) SetHealthyNodes(count float64) {
	m.healthyNodes.Set(count)
}

// SetUnhealthyNodes sets the number of unhealthy nodes
func (m *Metrics) SetUnhealthyNodes(count float64) {
	m.unhealthyNodes.Set(count)
}

// SetBlockHeight sets the block height for a specific node
func (m *Metrics) SetBlockHeight(nodeName string, height float64) {
	m.blockHeightGauge.WithLabelValues(nodeName).Set(height)
}

// IncrementError increments the error counter for a specific node and error type
func (m *Metrics) IncrementError(nodeName, errorType string) {
	m.errorCount.WithLabelValues(nodeName, errorType).Inc()
}
