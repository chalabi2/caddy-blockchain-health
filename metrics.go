package blockchain_health

import (
	"fmt"
	"sync"

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
		configuredNodes: prometheus.NewGauge(prometheus.GaugeOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "configured_nodes",
			Help:      "Number of nodes configured in the module",
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
		upstreamsIncluded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "upstreams_included_total",
			Help:      "Total number of times a node was included as an upstream",
		}, []string{"node_name", "service_type", "reason"}),
		upstreamsExcluded: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "blockchain_health",
			Name:      "upstreams_excluded_total",
			Help:      "Total number of times a node was excluded from upstreams and why",
		}, []string{"node_name", "service_type", "reason"}),
	}
}

var (
	globalMetrics           *Metrics
	globalMetricsMu         sync.Mutex
	globalMetricsRefs       int
	globalMetricsRegisterer prometheus.Registerer
)

// acquireGlobalMetrics returns a process-wide Metrics instance registered with
// the default Prometheus registry. Each caller must pair it with
// releaseGlobalMetrics when the upstream is cleaned up.
func acquireGlobalMetrics(reg prometheus.Registerer) (*Metrics, error) {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()

	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	if globalMetrics == nil || globalMetricsRegisterer != reg {
		metrics := NewMetrics()
		if err := metrics.registerWith(reg); err != nil {
			return nil, err
		}
		globalMetrics = metrics
		globalMetricsRegisterer = reg
	}

	globalMetricsRefs++
	return globalMetrics, nil
}

// releaseGlobalMetrics decrements the reference count and unregisters the
// collectors when no upstreams remain.
func releaseGlobalMetrics() {
	globalMetricsMu.Lock()
	defer globalMetricsMu.Unlock()

	if globalMetricsRefs > 0 {
		globalMetricsRefs--
	}
	if globalMetricsRefs == 0 {
		globalMetrics = nil
		globalMetricsRegisterer = nil
	}
}

// Register registers all metrics with the default prometheus registry
func (m *Metrics) Register() error {
	collectors := []prometheus.Collector{
		m.totalChecks,
		m.healthyNodes,
		m.unhealthyNodes,
		m.configuredNodes,
		m.checkDuration,
		m.blockHeightGauge,
		m.errorCount,
		m.upstreamsIncluded,
		m.upstreamsExcluded,
	}

	for _, collector := range collectors {
		if err := prometheus.Register(collector); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}

	return nil
}

// registerWith registers metrics with a specific registry.
func (m *Metrics) registerWith(reg prometheus.Registerer) error {
	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	var err error
	if m.totalChecks, err = registerCounter(reg, m.totalChecks); err != nil {
		return err
	}
	if m.healthyNodes, err = registerGauge(reg, m.healthyNodes); err != nil {
		return err
	}
	if m.unhealthyNodes, err = registerGauge(reg, m.unhealthyNodes); err != nil {
		return err
	}
	if m.configuredNodes, err = registerGauge(reg, m.configuredNodes); err != nil {
		return err
	}
	if m.checkDuration, err = registerHistogram(reg, m.checkDuration); err != nil {
		return err
	}
	if m.blockHeightGauge, err = registerGaugeVec(reg, m.blockHeightGauge); err != nil {
		return err
	}
	if m.errorCount, err = registerCounterVec(reg, m.errorCount); err != nil {
		return err
	}
	if m.upstreamsIncluded, err = registerCounterVec(reg, m.upstreamsIncluded); err != nil {
		return err
	}
	if m.upstreamsExcluded, err = registerCounterVec(reg, m.upstreamsExcluded); err != nil {
		return err
	}

	return nil
}

// Unregister removes all metrics from the default prometheus registry
func (m *Metrics) Unregister() {
	collectors := []prometheus.Collector{
		m.totalChecks,
		m.healthyNodes,
		m.unhealthyNodes,
		m.configuredNodes,
		m.checkDuration,
		m.blockHeightGauge,
		m.errorCount,
		m.upstreamsIncluded,
		m.upstreamsExcluded,
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

// RequestDeadlineMetrics tracks per-request deadline middleware metrics
type RequestDeadlineMetrics struct {
	appliedTotal    *prometheus.CounterVec
	appliedSeconds  *prometheus.HistogramVec
	timeoutsTotal   *prometheus.CounterVec
	durationSeconds *prometheus.HistogramVec
}

// NewRequestDeadlineMetrics creates request deadline metrics
func NewRequestDeadlineMetrics() *RequestDeadlineMetrics {
	return &RequestDeadlineMetrics{
		appliedTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "request_deadline",
			Name:      "applied_total",
			Help:      "Total number of requests where a deadline was applied",
		}, []string{"tier"}),
		appliedSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "caddy",
			Subsystem: "request_deadline",
			Name:      "applied_seconds",
			Help:      "Configured per-request timeout applied in seconds",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tier"}),
		timeoutsTotal: prometheus.NewCounterVec(prometheus.CounterOpts{
			Namespace: "caddy",
			Subsystem: "request_deadline",
			Name:      "timeouts_total",
			Help:      "Total number of requests that exceeded their deadline",
		}, []string{"tier", "method", "host"}),
		durationSeconds: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "caddy",
			Subsystem: "request_deadline",
			Name:      "duration_seconds",
			Help:      "Observed request duration by outcome relative to deadline middleware",
			Buckets:   prometheus.DefBuckets,
		}, []string{"tier", "outcome"}),
	}
}

// Register registers request deadline metrics
func (m *RequestDeadlineMetrics) Register() error {
	collectors := []prometheus.Collector{
		m.appliedTotal,
		m.appliedSeconds,
		m.timeoutsTotal,
		m.durationSeconds,
	}
	for _, c := range collectors {
		if err := prometheus.Register(c); err != nil {
			if _, ok := err.(prometheus.AlreadyRegisteredError); !ok {
				return err
			}
		}
	}
	return nil
}

// Unregister unregisters request deadline metrics
func (m *RequestDeadlineMetrics) Unregister() {
	prometheus.Unregister(m.appliedTotal)
	prometheus.Unregister(m.appliedSeconds)
	prometheus.Unregister(m.timeoutsTotal)
	prometheus.Unregister(m.durationSeconds)
}

var (
	requestDeadlineMetricsMu         sync.Mutex
	requestDeadlineMetricsRegisterer prometheus.Registerer
)

func acquireRequestDeadlineMetrics(reg prometheus.Registerer) (*RequestDeadlineMetrics, error) {
	requestDeadlineMetricsMu.Lock()
	defer requestDeadlineMetricsMu.Unlock()

	if reg == nil {
		reg = prometheus.DefaultRegisterer
	}

	if rdMetrics == nil || requestDeadlineMetricsRegisterer != reg {
		metrics := NewRequestDeadlineMetrics()
		if err := metrics.registerWith(reg); err != nil {
			return nil, err
		}
		rdMetrics = metrics
		requestDeadlineMetricsRegisterer = reg
	}

	return rdMetrics, nil
}

func (m *RequestDeadlineMetrics) registerWith(reg prometheus.Registerer) error {
	var err error
	if m.appliedTotal, err = registerCounterVec(reg, m.appliedTotal); err != nil {
		return err
	}
	if m.appliedSeconds, err = registerHistogramVec(reg, m.appliedSeconds); err != nil {
		return err
	}
	if m.timeoutsTotal, err = registerCounterVec(reg, m.timeoutsTotal); err != nil {
		return err
	}
	if m.durationSeconds, err = registerHistogramVec(reg, m.durationSeconds); err != nil {
		return err
	}
	return nil
}

func registerCounter(reg prometheus.Registerer, counter prometheus.Counter) (prometheus.Counter, error) {
	if err := reg.Register(counter); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(prometheus.Counter)
			if !ok {
				return nil, fmt.Errorf("expected counter, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return counter, nil
}

func registerGauge(reg prometheus.Registerer, gauge prometheus.Gauge) (prometheus.Gauge, error) {
	if err := reg.Register(gauge); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(prometheus.Gauge)
			if !ok {
				return nil, fmt.Errorf("expected gauge, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return gauge, nil
}

func registerHistogram(reg prometheus.Registerer, hist prometheus.Histogram) (prometheus.Histogram, error) {
	if err := reg.Register(hist); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(prometheus.Histogram)
			if !ok {
				return nil, fmt.Errorf("expected histogram, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return hist, nil
}

func registerCounterVec(reg prometheus.Registerer, vec *prometheus.CounterVec) (*prometheus.CounterVec, error) {
	if err := reg.Register(vec); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(*prometheus.CounterVec)
			if !ok {
				return nil, fmt.Errorf("expected counter vec, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return vec, nil
}

func registerGaugeVec(reg prometheus.Registerer, vec *prometheus.GaugeVec) (*prometheus.GaugeVec, error) {
	if err := reg.Register(vec); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(*prometheus.GaugeVec)
			if !ok {
				return nil, fmt.Errorf("expected gauge vec, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return vec, nil
}

func registerHistogramVec(reg prometheus.Registerer, vec *prometheus.HistogramVec) (*prometheus.HistogramVec, error) {
	if err := reg.Register(vec); err != nil {
		if are, ok := err.(prometheus.AlreadyRegisteredError); ok {
			existing, ok := are.ExistingCollector.(*prometheus.HistogramVec)
			if !ok {
				return nil, fmt.Errorf("expected histogram vec, got %T", are.ExistingCollector)
			}
			return existing, nil
		}
		return nil, err
	}
	return vec, nil
}
