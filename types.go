package blockchain_health

import (
	"context"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"go.uber.org/zap"
)

// NodeType represents the type of blockchain node
type NodeType string

const (
	NodeTypeCosmos NodeType = "cosmos"
	NodeTypeEVM    NodeType = "evm"
)

// NodeConfig represents the configuration for a blockchain node
type NodeConfig struct {
	Name     string            `json:"name"`
	URL      string            `json:"url"`
	APIURL   string            `json:"api_url,omitempty"`
	Type     NodeType          `json:"type"`
	Weight   int               `json:"weight"`
	Metadata map[string]string `json:"metadata,omitempty"`
}

// ExternalReference represents an external blockchain endpoint for validation
type ExternalReference struct {
	Name    string   `json:"name"`
	URL     string   `json:"url"`
	Type    NodeType `json:"type"`
	Enabled bool     `json:"enabled"`
}

// HealthCheckConfig holds health check configuration
type HealthCheckConfig struct {
	Interval      string `json:"interval"`
	Timeout       string `json:"timeout"`
	RetryAttempts int    `json:"retry_attempts"`
	RetryDelay    string `json:"retry_delay"`
}

// BlockValidationConfig holds block height validation configuration
type BlockValidationConfig struct {
	HeightThreshold            int `json:"height_threshold"`
	ExternalReferenceThreshold int `json:"external_reference_threshold"`
}

// PerformanceConfig holds performance-related configuration
type PerformanceConfig struct {
	CacheDuration       string `json:"cache_duration"`
	MaxConcurrentChecks int    `json:"max_concurrent_checks"`
}

// FailureHandlingConfig holds failure handling configuration
type FailureHandlingConfig struct {
	MinHealthyNodes         int     `json:"min_healthy_nodes"`
	GracePeriod             string  `json:"grace_period"`
	CircuitBreakerThreshold float64 `json:"circuit_breaker_threshold"`
}

// MonitoringConfig holds monitoring configuration
type MonitoringConfig struct {
	MetricsEnabled bool   `json:"metrics_enabled"`
	LogLevel       string `json:"log_level"`
	HealthEndpoint string `json:"health_endpoint"`
}

// Config represents the complete module configuration
type Config struct {
	Nodes              []NodeConfig          `json:"nodes"`
	ExternalReferences []ExternalReference   `json:"external_references"`
	HealthCheck        HealthCheckConfig     `json:"health_check"`
	BlockValidation    BlockValidationConfig `json:"block_validation"`
	Performance        PerformanceConfig     `json:"performance"`
	FailureHandling    FailureHandlingConfig `json:"failure_handling"`
	Monitoring         MonitoringConfig      `json:"monitoring"`
}

// NodeHealth represents the health status of a node
type NodeHealth struct {
	Name         string        `json:"name"`
	URL          string        `json:"url"`
	Healthy      bool          `json:"healthy"`
	BlockHeight  uint64        `json:"block_height"`
	CatchingUp   *bool         `json:"catching_up,omitempty"`
	ResponseTime time.Duration `json:"response_time"`
	LastCheck    time.Time     `json:"last_check"`
	ErrorCount   int           `json:"error_count"`
	LastError    string        `json:"last_error,omitempty"`

	// Validation results
	HeightValid            bool  `json:"height_valid"`
	ExternalReferenceValid bool  `json:"external_reference_valid"`
	BlocksBehindPool       int64 `json:"blocks_behind_pool"`
	BlocksBehindExternal   int64 `json:"blocks_behind_external"`
}

// CircuitState represents the state of a circuit breaker
type CircuitState int

const (
	CircuitClosed CircuitState = iota
	CircuitOpen
	CircuitHalfOpen
)

// CircuitBreaker implements the circuit breaker pattern for node health checks
type CircuitBreaker struct {
	failureThreshold int
	failureCount     int
	lastFailureTime  time.Time
	state            CircuitState
	mutex            sync.RWMutex
}

// CacheEntry represents a cached health check result
type CacheEntry struct {
	Health    *NodeHealth
	ExpiresAt time.Time
}

// HealthCache provides TTL-based caching for health check results
type HealthCache struct {
	cache    map[string]*CacheEntry
	mutex    sync.RWMutex
	duration time.Duration
}

// Metrics holds prometheus metrics for the module
type Metrics struct {
	totalChecks      prometheus.Counter
	healthyNodes     prometheus.Gauge
	unhealthyNodes   prometheus.Gauge
	checkDuration    prometheus.Histogram
	blockHeightGauge *prometheus.GaugeVec
	errorCount       *prometheus.CounterVec
}

// ProtocolHandler defines the interface for protocol-specific health checks
type ProtocolHandler interface {
	CheckHealth(ctx context.Context, node NodeConfig) (*NodeHealth, error)
	GetBlockHeight(ctx context.Context, url string) (uint64, error)
}

// HealthChecker manages health checking for all nodes
type HealthChecker struct {
	config        *Config
	cosmosHandler ProtocolHandler
	evmHandler    ProtocolHandler
	cache         *HealthCache
	metrics       *Metrics
	logger        *zap.Logger

	// Circuit breakers per node
	circuitBreakers map[string]*CircuitBreaker
	mutex           sync.RWMutex
}

// BlockchainHealthUpstream implements the Caddy UpstreamSource interface
type BlockchainHealthUpstream struct {
	// Configuration
	Nodes              []NodeConfig          `json:"nodes,omitempty"`
	ExternalReferences []ExternalReference   `json:"external_references,omitempty"`
	HealthCheck        HealthCheckConfig     `json:"health_check,omitempty"`
	BlockValidation    BlockValidationConfig `json:"block_validation,omitempty"`
	Performance        PerformanceConfig     `json:"performance,omitempty"`
	FailureHandling    FailureHandlingConfig `json:"failure_handling,omitempty"`
	Monitoring         MonitoringConfig      `json:"monitoring,omitempty"`

	// Runtime components
	config        *Config
	healthChecker *HealthChecker
	cache         *HealthCache
	metrics       *Metrics
	logger        *zap.Logger

	// Internal state
	mutex    sync.RWMutex
	shutdown chan struct{}
}
