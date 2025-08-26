# Blockchain Health Dynamic Upstream Module for Caddy

## Overview

A production-ready Caddy dynamic upstream module that intelligently monitors blockchain node health across multiple protocols (Cosmos, EVM) and removes unhealthy nodes from the load balancer pool in real-time.

## Module Specification

### Module Name and Registration

- **Module Name**: `http.reverse_proxy.upstreams.blockchain_health`
- **Package Path**: `github.com/your-org/caddy-blockchain-health`
- **Registration ID**: `blockchain_health`

### Core Functionality

The module provides intelligent health checking for blockchain nodes by:

1. **Sync Status Monitoring** - Checks if nodes are caught up (`catching_up: false` for Cosmos)
2. **Block Height Validation** - Compares block heights within the pool and against external references
3. **Multi-Protocol Support** - Handles Cosmos RPC/REST and EVM JSON-RPC endpoints
4. **External Reference Checking** - Validates against external providers (Infura, Alchemy, etc.)

## Configuration Schema

### Caddyfile Syntax

```caddy
reverse_proxy {
    dynamic blockchain_health {
        # Node definitions
        node cosmos1 {
            url "http://node1.cosmos:26657"
            api_url "http://node1.cosmos:1317"  # Optional REST API
            type "cosmos"
            weight 100
        }

        node evm1 {
            url "http://node1.eth:8545"
            type "evm"
            weight 100
        }

        # Health check configuration
        check_interval "15s"
        timeout "5s"
        retry_attempts 3
        retry_delay "1s"

        # Block height validation
        block_height_threshold 5          # Max blocks behind pool leader
        external_reference_threshold 10   # Max blocks behind external ref

        # External references
        external_reference cosmos {
            name "cosmos_mainnet"
            url "https://cosmos-rpc.publicnode.com"
            type "cosmos"
            enabled true
        }

        external_reference evm {
            name "ethereum_infura"
            url "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"
            type "evm"
            enabled true
        }

        # Caching and performance
        cache_duration "30s"
        max_concurrent_checks 10

        # Failure handling
        min_healthy_nodes 1
        grace_period "60s"              # Keep unhealthy nodes for this long
        circuit_breaker_threshold 0.8   # Remove from pool if >80% checks fail

        # Monitoring
        metrics_enabled true
        log_level "info"                # debug, info, warn, error
        health_endpoint "/health"       # Internal health endpoint
    }
}
```

### JSON Configuration Schema

```json
{
  "nodes": [
    {
      "name": "cosmos1",
      "url": "http://node1.cosmos:26657",
      "api_url": "http://node1.cosmos:1317",
      "type": "cosmos",
      "weight": 100,
      "metadata": {
        "region": "us-east-1",
        "provider": "aws"
      }
    }
  ],
  "health_check": {
    "interval": "15s",
    "timeout": "5s",
    "retry_attempts": 3,
    "retry_delay": "1s"
  },
  "block_validation": {
    "height_threshold": 5,
    "external_reference_threshold": 10
  },
  "external_references": [
    {
      "name": "cosmos_mainnet",
      "url": "https://cosmos-rpc.publicnode.com",
      "type": "cosmos",
      "enabled": true
    }
  ],
  "performance": {
    "cache_duration": "30s",
    "max_concurrent_checks": 10
  },
  "failure_handling": {
    "min_healthy_nodes": 1,
    "grace_period": "60s",
    "circuit_breaker_threshold": 0.8
  },
  "monitoring": {
    "metrics_enabled": true,
    "log_level": "info",
    "health_endpoint": "/health"
  }
}
```

## Implementation Architecture

### Core Interfaces

#### UpstreamSource Implementation

```go
type BlockchainHealthUpstream struct {
    nodes               []NodeConfig
    externalReferences  []ExternalReference
    healthChecker       *HealthChecker
    cache              *HealthCache
    metrics            *Metrics
    logger             *zap.Logger
    config             *Config
}

func (b *BlockchainHealthUpstream) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error)
```

#### Node Configuration

```go
type NodeConfig struct {
    Name     string            `json:"name"`
    URL      string            `json:"url"`
    APIURL   string            `json:"api_url,omitempty"`
    Type     NodeType          `json:"type"`
    Weight   int               `json:"weight"`
    Metadata map[string]string `json:"metadata,omitempty"`
}

type NodeType string
const (
    NodeTypeCosmos NodeType = "cosmos"
    NodeTypeEVM    NodeType = "evm"
)
```

#### Health Status

```go
type NodeHealth struct {
    Name           string        `json:"name"`
    URL            string        `json:"url"`
    Healthy        bool          `json:"healthy"`
    BlockHeight    uint64        `json:"block_height"`
    CatchingUp     *bool         `json:"catching_up,omitempty"`
    ResponseTime   time.Duration `json:"response_time"`
    LastCheck      time.Time     `json:"last_check"`
    ErrorCount     int           `json:"error_count"`
    LastError      string        `json:"last_error,omitempty"`

    // Validation results
    HeightValid              bool   `json:"height_valid"`
    ExternalReferenceValid   bool   `json:"external_reference_valid"`
    BlocksBehindPool         int64  `json:"blocks_behind_pool"`
    BlocksBehindExternal     int64  `json:"blocks_behind_external"`
}
```

### Protocol Handlers

#### Cosmos Handler

```go
type CosmosHandler struct {
    client *http.Client
    logger *zap.Logger
}

func (c *CosmosHandler) CheckHealth(ctx context.Context, node NodeConfig) (*NodeHealth, error) {
    // GET /status endpoint
    // Parse sync_info.catching_up and sync_info.latest_block_height
}

func (c *CosmosHandler) GetBlockHeight(ctx context.Context, url string) (uint64, error) {
    // GET /status for RPC or /cosmos/base/tendermint/v1beta1/syncing for REST
}
```

#### EVM Handler

```go
type EVMHandler struct {
    client *http.Client
    logger *zap.Logger
}

func (e *EVMHandler) CheckHealth(ctx context.Context, node NodeConfig) (*NodeHealth, error) {
    // JSON-RPC eth_blockNumber call
    // No catching_up concept for EVM
}

func (e *EVMHandler) GetBlockHeight(ctx context.Context, url string) (uint64, error) {
    // JSON-RPC eth_blockNumber
}
```

### Health Checking Engine

#### Primary Health Checker

```go
type HealthChecker struct {
    config          *Config
    cosmosHandler   *CosmosHandler
    evmHandler      *EVMHandler
    cache          *HealthCache
    metrics        *Metrics
    logger         *zap.Logger

    // Circuit breaker per node
    circuitBreakers map[string]*CircuitBreaker
}

func (h *HealthChecker) CheckAllNodes(ctx context.Context) ([]*NodeHealth, error) {
    // Concurrent health checks with proper timeout and rate limiting
}

func (h *HealthChecker) ValidateBlockHeights(nodes []*NodeHealth) error {
    // Compare heights within pool and against external references
}
```

#### Caching Strategy

```go
type HealthCache struct {
    cache     map[string]*CacheEntry
    mutex     sync.RWMutex
    duration  time.Duration
}

type CacheEntry struct {
    Health    *NodeHealth
    ExpiresAt time.Time
}
```

### Error Handling & Resilience

#### Circuit Breaker Pattern

```go
type CircuitBreaker struct {
    failureThreshold int
    failureCount     int
    lastFailureTime  time.Time
    state           CircuitState
}

type CircuitState int
const (
    CircuitClosed CircuitState = iota
    CircuitOpen
    CircuitHalfOpen
)
```

#### Retry Logic

```go
type RetryConfig struct {
    MaxAttempts int           `json:"max_attempts"`
    BaseDelay   time.Duration `json:"base_delay"`
    MaxDelay    time.Duration `json:"max_delay"`
    Multiplier  float64       `json:"multiplier"`
}

func (h *HealthChecker) checkWithRetry(ctx context.Context, node NodeConfig) (*NodeHealth, error) {
    // Exponential backoff with jitter
}
```

### Monitoring & Observability

#### Metrics Collection

```go
type Metrics struct {
    totalChecks         prometheus.Counter
    healthyNodes        prometheus.Gauge
    unhealthyNodes      prometheus.Gauge
    checkDuration       prometheus.Histogram
    blockHeightGauge    *prometheus.GaugeVec
    errorCount          *prometheus.CounterVec
}
```

#### Structured Logging

```go
// Log levels: DEBUG, INFO, WARN, ERROR
logger.Info("node health check completed",
    zap.String("node", node.Name),
    zap.Bool("healthy", health.Healthy),
    zap.Uint64("block_height", health.BlockHeight),
    zap.Duration("response_time", health.ResponseTime),
)
```

#### Health Endpoint

```
GET /health
{
    "status": "healthy",
    "nodes": {
        "total": 5,
        "healthy": 4,
        "unhealthy": 1
    },
    "last_check": "2024-01-15T10:30:00Z",
    "external_references": {
        "cosmos_mainnet": {
            "reachable": true,
            "block_height": 12345678
        }
    }
}
```

## Production Readiness Features

### Security Considerations

1. **Input Validation** - Strict validation of all configuration parameters
2. **Rate Limiting** - Prevent overwhelming upstream nodes
3. **Authentication** - Support for API keys/tokens for external references
4. **TLS Verification** - Proper certificate validation
5. **Timeout Protection** - Prevent resource exhaustion

### Performance Optimizations

1. **Connection Pooling** - Reuse HTTP connections
2. **Concurrent Checks** - Parallel health checking with limits
3. **Efficient Caching** - TTL-based cache with LRU eviction
4. **Memory Management** - Bounded queues and proper cleanup
5. **CPU Efficiency** - Minimal allocations in hot paths

### Failure Scenarios & Handling

#### Network Failures

- **Timeout handling** with configurable limits
- **DNS resolution failures** with exponential backoff
- **Connection refused** scenarios
- **Partial network connectivity**

#### Node Failures

- **Graceful degradation** when nodes become unhealthy
- **Minimum healthy node** enforcement
- **Automatic recovery** when nodes come back online
- **Split-brain scenarios** in blockchain networks

#### External Reference Failures

- **Fallback to internal validation** when external refs fail
- **Multiple external references** for redundancy
- **External rate limiting** handling

#### Configuration Errors

- **Validation at startup** with clear error messages
- **Hot reload support** for configuration changes
- **Backward compatibility** for configuration versions

### Monitoring & Alerting

#### Key Metrics to Track

- Node availability percentage
- Average response times
- Block height lag
- External reference health
- Cache hit/miss ratios
- Error rates by type

#### Alert Conditions

- All nodes unhealthy
- Block height significantly behind
- External references unreachable
- High error rates
- Memory/CPU usage spikes

### Testing Strategy

#### Unit Tests

- Protocol handler validation
- Health check logic
- Cache behavior
- Error handling paths

#### Integration Tests

- End-to-end health checking
- Multiple protocol scenarios
- Failure injection testing
- Performance benchmarks

#### Load Testing

- High request volume handling
- Concurrent health check limits
- Memory usage under load
- Cache efficiency

## Installation & Usage

### Building with xcaddy

```bash
xcaddy build --with github.com/your-org/caddy-blockchain-health
```

### Example Production Configuration

```caddy
api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Cosmos mainnet nodes
            node cosmos-us-east-1 {
                url "http://cosmos-1.internal:26657"
                api_url "http://cosmos-1.internal:1317"
                type "cosmos"
                weight 100
            }

            node cosmos-eu-west-1 {
                url "http://cosmos-2.internal:26657"
                api_url "http://cosmos-2.internal:1317"
                type "cosmos"
                weight 100
            }

            # Ethereum nodes
            node eth-us-east-1 {
                url "http://ethereum-1.internal:8545"
                type "evm"
                weight 100
            }

            # Health check settings
            check_interval "10s"
            timeout "3s"
            block_height_threshold 3
            external_reference_threshold 5

            # External validation
            external_reference cosmos {
                name "cosmos_public"
                url "https://cosmos-rpc.publicnode.com"
                type "cosmos"
                enabled true
            }

            external_reference evm {
                name "ethereum_infura"
                url "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"
                type "evm"
                enabled true
            }

            # Production settings
            min_healthy_nodes 1
            grace_period "30s"
            circuit_breaker_threshold 0.75
            cache_duration "15s"
            max_concurrent_checks 5

            # Monitoring
            metrics_enabled true
            log_level "info"
        }

        # Fallback to static upstreams if all dynamic fail
        to {$FALLBACK_NODES}

        lb_policy least_conn
        lb_retries 2
        lb_try_duration 10s
    }
}
```

## Development Roadmap

### Phase 1: Core Implementation

- [ ] Basic module structure and Caddy integration
- [ ] Cosmos and EVM protocol handlers
- [ ] Simple health checking without external references
- [ ] Basic caching and error handling

### Phase 2: Advanced Features

- [ ] External reference validation
- [ ] Circuit breaker implementation
- [ ] Comprehensive metrics and logging
- [ ] Configuration hot reload

### Phase 3: Production Hardening

- [ ] Comprehensive test suite
- [ ] Performance optimizations
- [ ] Security audit and hardening
- [ ] Documentation and examples

### Phase 4: Extended Protocol Support

- [ ] Custom protocol handlers
- [ ] Plugin architecture for new protocols

## Conclusion

This blockchain health dynamic upstream module will provide Caddy with intelligent, production-ready load balancing capabilities for blockchain infrastructure, ensuring high availability and optimal performance across multiple protocols and deployment scenarios.
