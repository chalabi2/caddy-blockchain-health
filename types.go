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
	NodeTypeBeacon NodeType = "beacon"
)

// NodeConfig represents the configuration for a blockchain node
type NodeConfig struct {
	Name         string            `json:"name"`
	URL          string            `json:"url"`
	APIURL       string            `json:"api_url,omitempty"`
	WebSocketURL string            `json:"websocket_url,omitempty"`
	Type         NodeType          `json:"type"`
	ChainType    string            `json:"chain_type,omitempty"`
	Weight       int               `json:"weight"`
	Metadata     map[string]string `json:"metadata,omitempty"`
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

// EnvironmentConfig holds environment variable based configuration
type EnvironmentConfig struct {
	RPCServers       string `json:"rpc_servers,omitempty"`
	APIServers       string `json:"api_servers,omitempty"`
	WebSocketServers string `json:"websocket_servers,omitempty"`
	EVMServers       string `json:"evm_servers,omitempty"`
	EVMWSServers     string `json:"evm_ws_servers,omitempty"`
	Servers          string `json:"servers,omitempty"` // Generic server list
}

// ChainConfig holds chain-specific configuration
type ChainConfig struct {
	ChainType           string `json:"chain_type,omitempty"`             // Specific chain identifier for grouping ("ethereum", "base", "akash", etc.)
	NodeType            string `json:"node_type,omitempty"`              // Protocol type for health checker selection ("cosmos", "evm")
	ChainPreset         string `json:"chain_preset,omitempty"`           // "cosmos-hub", "ethereum", "althea"
	AutoDiscoverFromEnv string `json:"auto_discover_from_env,omitempty"` // "COSMOS" looks for COSMOS_*_SERVERS
	ServiceType         string `json:"service_type,omitempty"`           // "rpc", "api", "websocket"
}

// LegacyConfig holds backward compatibility settings
type LegacyConfig struct {
	LegacyMode       bool   `json:"legacy_mode,omitempty"`
	FallbackBehavior string `json:"fallback_behavior,omitempty"` // "disable_health_checks", "fail_startup"
	RequiredEnvVars  string `json:"required_env_vars,omitempty"`
	OptionalEnvVars  string `json:"optional_env_vars,omitempty"`
}

// Config represents the complete module configuration
type Config struct {
	// Traditional node-based configuration
	Nodes              []NodeConfig        `json:"nodes,omitempty"`
	ExternalReferences []ExternalReference `json:"external_references,omitempty"`

	// New environment-based configuration
	Environment EnvironmentConfig `json:"environment,omitempty"`
	Chain       ChainConfig       `json:"chain,omitempty"`
	Legacy      LegacyConfig      `json:"legacy,omitempty"`

	// Configuration sections
	HealthCheck     HealthCheckConfig     `json:"health_check"`
	BlockValidation BlockValidationConfig `json:"block_validation"`
	Performance     PerformanceConfig     `json:"performance"`
	FailureHandling FailureHandlingConfig `json:"failure_handling"`
	Monitoring      MonitoringConfig      `json:"monitoring"`
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
	totalChecks       prometheus.Counter
	healthyNodes      prometheus.Gauge
	unhealthyNodes    prometheus.Gauge
	checkDuration     prometheus.Histogram
	blockHeightGauge  *prometheus.GaugeVec
	errorCount        *prometheus.CounterVec
	configuredNodes   prometheus.Gauge
	upstreamsIncluded *prometheus.CounterVec
	upstreamsExcluded *prometheus.CounterVec
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
	beaconHandler ProtocolHandler
	cache         *HealthCache
	metrics       *Metrics
	logger        *zap.Logger

	// Circuit breakers per node
	circuitBreakers map[string]*CircuitBreaker
	mutex           sync.RWMutex
}

// BlockchainHealthUpstream implements the Caddy UpstreamSource interface
type BlockchainHealthUpstream struct {
	// Traditional configuration
	Nodes              []NodeConfig        `json:"nodes,omitempty"`
	ExternalReferences []ExternalReference `json:"external_references,omitempty"`

	// New environment-based configuration
	Environment EnvironmentConfig `json:"environment,omitempty"`
	Chain       ChainConfig       `json:"chain,omitempty"`
	Legacy      LegacyConfig      `json:"legacy,omitempty"`

	// Configuration sections
	HealthCheck     HealthCheckConfig     `json:"health_check,omitempty"`
	BlockValidation BlockValidationConfig `json:"block_validation,omitempty"`
	Performance     PerformanceConfig     `json:"performance,omitempty"`
	FailureHandling FailureHandlingConfig `json:"failure_handling,omitempty"`
	Monitoring      MonitoringConfig      `json:"monitoring,omitempty"`

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
