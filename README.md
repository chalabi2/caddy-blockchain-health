# Blockchain Health Dynamic Upstream

[![codecov](https://codecov.io/gh/chalabi2/caddy-blockchain-health/branch/main/graph/badge.svg)](https://codecov.io/gh/chalabi2/caddy-blockchain-health)
[![Go Report Card](https://goreportcard.com/badge/github.com/chalabi2/caddy-blockchain-health)](https://goreportcard.com/report/github.com/chalabi2/caddy-blockchain-health)
[![Go Reference](https://pkg.go.dev/badge/github.com/chalabi2/caddy-blockchain-health.svg)](https://pkg.go.dev/github.com/chalabi2/caddy-blockchain-health)

A Caddy dynamic upstream module that intelligently monitors blockchain node health across multiple protocols (Cosmos, EVM) and removes unhealthy nodes from the load balancer pool in real-time. This plugin provides intelligent failover capabilities for blockchain infrastructure, ensuring high availability and optimal performance.

> [!NOTE]  
> This is not an official repository of the Caddy Web Server organization.

## Features

### 🔗 **Multi-Protocol Support**

- **Cosmos SDK chains** - RPC (`/status`) and REST API (`/cosmos/base/tendermint/v1beta1/syncing`) health checks
- **EVM chains** - JSON-RPC (`eth_blockNumber`) validation
- **Flexible endpoints** - Support for separated RPC/REST services or combined nodes
- **Block height comparison** - Within pools and against external references

### 🏥 **Intelligent Health Checking**

- **Sync status monitoring** - Detects `catching_up` state for Cosmos nodes
- **Real-time validation** - Immediate unhealthy node removal from pools
- **External references** - Validate against trusted providers (Infura, Alchemy, public nodes)
- **Concurrent checks** - Parallel health validation with configurable limits

### 🛡️ **Production-Ready Resilience**

- **Circuit breaker pattern** - Prevents overwhelming unhealthy nodes
- **Graceful degradation** - Minimum healthy node enforcement
- **Retry logic** - Exponential backoff with jitter
- **TTL-based caching** - Optimized performance with configurable duration

### 📊 **Monitoring & Observability**

- **Prometheus metrics** - Comprehensive monitoring with node-level granularity
- **Health endpoint** - Real-time status API with detailed diagnostics
- **Structured logging** - Configurable log levels with request tracing
- **Performance tracking** - Response times and error rates

## Installation

Build Caddy with this plugin using xcaddy:

```bash
xcaddy build --with github.com/chalabi2/caddy-blockchain-health
```

> **Migration Note**: This module replaces traditional HTTP health checks with blockchain-aware monitoring. The directive is `dynamic blockchain_health` within reverse proxy configurations.

Or add to your xcaddy.json:

```json
{
  "dependencies": [
    {
      "module": "github.com/chalabi2/caddy-blockchain-health",
      "version": "latest"
    }
  ]
}
```

## Quick Start

Basic Caddyfile configuration:

```caddy
{
    admin localhost:2019
}

blockchain-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Cosmos nodes
            node cosmos-primary {
                url "http://cosmos-1.internal:26657"
                api_url "http://cosmos-1.internal:1317"
                type "cosmos"
                weight 100
            }

            node cosmos-backup {
                url "http://cosmos-2.internal:26657"
                type "cosmos"
                weight 75
            }

            # Ethereum nodes
            node eth-primary {
                url "http://eth-1.internal:8545"
                type "evm"
                weight 100
            }

            # Health monitoring
            check_interval "15s"
            timeout "5s"
            block_height_threshold 5

            # External validation
            external_reference cosmos {
                name "cosmos_mainnet"
                url "https://cosmos-rpc.publicnode.com"
                enabled true
            }

            # Production settings
            min_healthy_nodes 1
            circuit_breaker_threshold 0.8
            metrics_enabled true
        }
    }

    # Health endpoint
    handle /health {
        reverse_proxy localhost:8080
    }
}
```

## Configuration

> **Note**: Complete example configurations are available in the `example_configs/` directory.

### Configuration Patterns

The plugin supports three main usage patterns:

#### 1. **High-Availability Multi-Chain** (Recommended for production APIs)

```caddy
blockchain-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Cosmos nodes with metadata
            node cosmos-us-east-1 {
                url "http://cosmos-1.internal:26657"
                api_url "http://cosmos-1.internal:1317"
                type "cosmos"
                weight 100
                metadata {
                    region "us-east-1"
                    provider "aws"
                }
            }

            node cosmos-eu-west-1 {
                url "http://cosmos-2.internal:26657"
                api_url "http://cosmos-2.internal:1317"
                type "cosmos"
                weight 100
                metadata {
                    region "eu-west-1"
                    provider "gcp"
                }
            }

            # Ethereum nodes
            node eth-primary {
                url "http://ethereum-1.internal:8545"
                type "evm"
                weight 100
            }

            # Comprehensive health monitoring
            check_interval "10s"
            timeout "3s"
            retry_attempts 3
            block_height_threshold 3

            # External validation
            external_reference cosmos {
                name "cosmos_public"
                url "https://cosmos-rpc.publicnode.com"
                enabled true
            }

            external_reference evm {
                name "ethereum_infura"
                url "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"
                enabled true
            }

            # Production resilience
            min_healthy_nodes 2
            circuit_breaker_threshold 0.8
            cache_duration "30s"
            max_concurrent_checks 10
        }
    }
}
```

#### 2. **Separated Services** (RPC and REST on different endpoints)

```caddy
# Cosmos RPC endpoint
cosmos-rpc.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            node cosmos-rpc-1 {
                url "http://cosmos-node-1:26657"
                type "cosmos"
                weight 100
            }

            node cosmos-rpc-2 {
                url "http://cosmos-node-2:26657"
                type "cosmos"
                weight 100
            }

            check_interval "15s"
            min_healthy_nodes 1
            metrics_enabled true
        }
    }
}

# Cosmos REST API endpoint
cosmos-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            node cosmos-api-1 {
                url "http://cosmos-node-1:1317"
                type "cosmos"
                weight 100
            }

            node cosmos-api-2 {
                url "http://cosmos-node-2:1317"
                type "cosmos"
                weight 100
            }

            check_interval "15s"
            min_healthy_nodes 1
            metrics_enabled true
        }
    }
}
```

#### 3. **Development Configuration**

```caddy
dev-blockchain.localhost {
    reverse_proxy {
        dynamic blockchain_health {
            # Local development nodes
            node local-cosmos {
                url "http://localhost:26657"
                api_url "http://localhost:1317"
                type "cosmos"
                weight 100
            }

            node local-eth {
                url "http://localhost:8545"
                type "evm"
                weight 100
            }

            # Relaxed settings for development
            check_interval "5s"
            timeout "2s"
            block_height_threshold 10
            circuit_breaker_threshold 0.9
            log_level "debug"
        }
    }
}
```

### ⚠️ **Important: Service Separation Behavior**

**Pattern 1 (Multi-Chain)**: ✅ **Full health validation** - Checks all configured endpoints with comprehensive monitoring.

**Pattern 2 (Separated Services)**: ✅ **Service-specific validation** - Only checks the specific service type (RPC or REST) without redundant checks.

**Pattern 3 (Development)**: ✅ **Relaxed validation** - Lenient settings suitable for local development and testing.

**Recommendation**: Use Pattern 1 for production APIs requiring maximum reliability, Pattern 2 for microservice architectures, and Pattern 3 for development environments.

### Configuration Options

#### Node Settings

| Option          | Description                                             | Default | Required |
| --------------- | ------------------------------------------------------- | ------- | -------- |
| `name`          | Unique identifier for the node                          | -       | ✅       |
| `url`           | Primary endpoint URL (RPC for Cosmos, JSON-RPC for EVM) | -       | ✅       |
| `api_url`       | Optional REST API URL for Cosmos nodes                  | -       | ❌       |
| `websocket_url` | Optional WebSocket URL for real-time connections        | -       | ❌       |
| `type`          | Node type (`cosmos` or `evm`)                           | -       | ✅       |
| `weight`        | Load balancing weight                                   | `100`   | ❌       |
| `metadata`      | Optional key-value metadata                             | `{}`    | ❌       |

#### 🔍 **Cosmos RPC vs REST API Differentiation**

The plugin intelligently handles Cosmos SDK chains with separate RPC and REST endpoints:

**Scenario 1: Combined Node (Single Service)**

```caddy
node cosmos-combined {
    url "http://cosmos-node:26657"          # RPC endpoint
    api_url "http://cosmos-node:1317"       # REST API endpoint
    type "cosmos"
    weight 100
}
```

- ✅ **Health checks RPC first, REST as fallback** - Tries RPC (`/status`), then REST (`/cosmos/base/tendermint/v1beta1/syncing`) if RPC fails
- ✅ **Fallback redundancy** - Node stays available if either service responds
- ✅ **Recommended for full-node infrastructure**

**Scenario 2: Separated Services (Microservice Architecture)**

```caddy
# Cosmos RPC load balancer
cosmos-rpc.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            node rpc-1 {
                url "http://cosmos-rpc-1:26657"    # Only RPC
                type "cosmos"
            }
            node rpc-2 {
                url "http://cosmos-rpc-2:26657"    # Only RPC
                type "cosmos"
            }
        }
    }
}

# Cosmos REST API load balancer
cosmos-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            node api-1 {
                url "http://cosmos-api-1:1317"     # Only REST
                type "cosmos"
            }
            node api-2 {
                url "http://cosmos-api-2:1317"     # Only REST
                type "cosmos"
            }
        }
    }
}
```

- ✅ **Health checks appropriate endpoint** - RPC or REST based on URL pattern
- ✅ **No redundant checks** - Each service validates its specific protocol
- ✅ **Recommended for microservice deployments**

**Auto-Detection Logic:**

- **Port 26657** or `/status` path → RPC health check
- **Port 1317** or `/cosmos/` path → REST API health check
- **Both `url` and `api_url` specified** → Checks both endpoints

#### 🌐 **WebSocket Support**

The plugin provides comprehensive WebSocket support for real-time blockchain connections:

**Cosmos WebSocket Configuration:**

```caddy
node cosmos-websocket {
    url "http://cosmos-node:26657"
    api_url "http://cosmos-node:1317"
    websocket_url "ws://cosmos-node:26657/websocket"
    type "cosmos"
    weight 100
}
```

- ✅ **Tendermint WebSocket subscriptions** - Tests `tm.event = 'NewBlock'` subscriptions
- ✅ **Real-time event streaming** - Validates connectivity for live event monitoring
- ✅ **Auto scheme conversion** - Converts `http`/`https` to `ws`/`wss` automatically

**EVM WebSocket Configuration:**

```caddy
node ethereum-websocket {
    url "http://geth-node:8545"
    websocket_url "ws://geth-node:8546"
    type "evm"
    weight 100
}
```

- ✅ **JSON-RPC WebSocket** - Tests `eth_subscribe` for `newHeads` subscriptions
- ✅ **Real-time block monitoring** - Validates connectivity for live block feeds
- ✅ **Standard ports** - Uses typical WebSocket ports (8546 for Ethereum)

**WebSocket Health Checking:**

- **Non-blocking** - WebSocket failures don't mark nodes as unhealthy if HTTP works
- **Informational monitoring** - Provides observability into WebSocket connectivity
- **Timeout protection** - 3-second read timeout prevents hanging connections
- **Protocol-specific tests** - Uses appropriate subscription methods per blockchain type

#### 🔗 **EVM JSON-RPC Node Differentiation**

EVM nodes use JSON-RPC protocol and don't have separate RPC/REST endpoints like Cosmos:

**Standard EVM Configuration:**

```caddy
node ethereum-primary {
    url "http://ethereum-node:8545"    # JSON-RPC endpoint
    type "evm"
    weight 100
    metadata {
        client "geth"
        sync_mode "full"
    }
}
```

- ✅ **Single endpoint** - All requests use JSON-RPC over HTTP
- ✅ **Health check via `eth_blockNumber`** - Validates node responsiveness and current block
- ✅ **No separate API URL needed** - EVM protocol is unified

**EVM Service Types (by function, not protocol):**

```caddy
# Archive node for historical data
node ethereum-archive {
    url "http://archive-node:8545"
    type "evm"
    weight 50
    metadata {
        type "archive"
        retention "full_history"
    }
}

# Full node for current state
node ethereum-full {
    url "http://full-node:8545"
    type "evm"
    weight 100
    metadata {
        type "full"
        retention "recent_blocks"
    }
}

# Light client for basic queries
node ethereum-light {
    url "http://light-node:8545"
    type "evm"
    weight 75
    metadata {
        type "light"
        retention "minimal"
    }
}
```

**Key Differences from Cosmos:**

| Aspect              | Cosmos SDK                                            | EVM Chains                     |
| ------------------- | ----------------------------------------------------- | ------------------------------ |
| **Protocol**        | RPC (26657) + REST (1317)                             | JSON-RPC (8545)                |
| **Health Check**    | `/status` + `/cosmos/base/tendermint/v1beta1/syncing` | `eth_blockNumber`              |
| **Endpoints**       | Separate RPC/REST URLs possible                       | Single JSON-RPC endpoint       |
| **Sync Status**     | `catching_up` boolean                                 | Block height comparison        |
| **Differentiation** | Service type (RPC vs REST)                            | Node type (archive/full/light) |

#### 📊 **Block Height Validation Strategy**

The plugin performs **internal pool validation** and **external reference monitoring**:

##### **1. Internal Pool Comparison** ✅ **Affects Load Balancing**

Compares nodes within the same pool and **removes lagging nodes** from the load balancer:

```caddy
dynamic blockchain_health {
    # These nodes will be compared against each other
    node eth-node-1 {
        url "http://eth-1.internal:8545"
        type "evm"
        weight 100
    }

    node eth-node-2 {
        url "http://eth-2.internal:8545"
        type "evm"
        weight 100
    }

    node eth-node-3 {
        url "http://eth-3.internal:8545"
        type "evm"
        weight 100
    }

    # If any node is more than 5 blocks behind the highest in the pool
    block_height_threshold 5
}
```

**Logic**: If `eth-node-1` is at block 18,500,000 and `eth-node-2` is at 18,499,994, then `eth-node-2` is **removed from load balancer** (6 blocks behind > threshold of 5).

##### **2. External Reference Monitoring** ℹ️ **Informational Only**

Monitors your nodes against trusted external sources **for observability** (does not affect load balancing):

**EVM External References:**

```caddy
dynamic blockchain_health {
    node your-eth-node {
        url "http://your-node:8545"
        type "evm"
        weight 100
    }

    # Compare against external trusted sources
    external_reference evm {
        name "infura_mainnet"
        url "https://mainnet.infura.io/v3/YOUR_PROJECT_ID"
        enabled true
    }

    external_reference evm {
        name "alchemy_backup"
        url "https://eth-mainnet.alchemyapi.io/v2/YOUR_API_KEY"
        enabled true
    }

    external_reference evm {
        name "public_ethereum"
        url "https://ethereum-rpc.publicnode.com"
        enabled true
    }

    # If your nodes are more than 10 blocks behind external references
    external_reference_threshold 10
}
```

**Multi-Chain EVM Examples:**

```caddy
# Polygon network
dynamic blockchain_health {
    node polygon-node {
        url "http://polygon-node:8545"
        type "evm"
    }

    external_reference evm {
        name "polygon_alchemy"
        url "https://polygon-mainnet.g.alchemy.com/v2/YOUR_API_KEY"
        enabled true
    }

    external_reference evm {
        name "polygon_public"
        url "https://polygon-rpc.com"
        enabled true
    }
}

# Binance Smart Chain
dynamic blockchain_health {
    node bsc-node {
        url "http://bsc-node:8545"
        type "evm"
    }

    external_reference evm {
        name "bsc_public"
        url "https://bsc-dataseed.binance.org"
        enabled true
    }

    external_reference evm {
        name "bsc_backup"
        url "https://bsc-dataseed1.defibit.io"
        enabled true
    }
}

# Arbitrum network
dynamic blockchain_health {
    node arbitrum-node {
        url "http://arbitrum-node:8545"
        type "evm"
    }

    external_reference evm {
        name "arbitrum_alchemy"
        url "https://arb-mainnet.g.alchemy.com/v2/YOUR_API_KEY"
        enabled true
    }
}
```

**Cosmos External References:**

```caddy
dynamic blockchain_health {
    node cosmos-node {
        url "http://cosmos-node:26657"
        type "cosmos"
    }

    external_reference cosmos {
        name "cosmos_public"
        url "https://cosmos-rpc.publicnode.com"
        enabled true
    }

    external_reference cosmos {
        name "cosmos_polkachu"
        url "https://cosmos-rpc.polkachu.com"
        enabled true
    }
}
```

##### **Validation Flow:**

1. **Internal Check**: Compare all pool nodes → Find highest block height in pool
2. **Remove Internal Laggards**: Nodes > `block_height_threshold` behind pool leader = **removed from load balancer**
3. **External Monitoring**: Query external references → Get external block heights
4. **Flag External Laggards**: Nodes > `external_reference_threshold` behind external references = **flagged in monitoring only**
5. **Final Load Balancing**: Only nodes passing internal validation receive traffic

##### **Example Scenario:**

```
Pool State:
- eth-node-1: Block 18,500,000 (highest in pool)
- eth-node-2: Block 18,499,996 (4 behind, healthy)
- eth-node-3: Block 18,499,990 (10 behind, unhealthy - exceeds threshold 5)

External References:
- infura_mainnet: Block 18,500,002
- alchemy_backup: Block 18,500,001
- Highest external: 18,500,002

External Monitoring (informational):
- eth-node-1: 2 blocks behind external (flagged as up-to-date)
- eth-node-2: 6 blocks behind external (flagged as up-to-date)
- eth-node-3: 12 blocks behind external (flagged as lagging in monitoring)

Final Result: Only eth-node-1 and eth-node-2 receive traffic (based on internal validation only)
```

#### Health Check Settings

| Option           | Description                                | Default | Required |
| ---------------- | ------------------------------------------ | ------- | -------- |
| `check_interval` | How often to check node health             | `15s`   | ❌       |
| `timeout`        | Request timeout for health checks          | `5s`    | ❌       |
| `retry_attempts` | Number of retry attempts for failed checks | `3`     | ❌       |
| `retry_delay`    | Delay between retry attempts               | `1s`    | ❌       |

#### Block Validation Settings

| Option                         | Description                              | Default | Required |
| ------------------------------ | ---------------------------------------- | ------- | -------- |
| `block_height_threshold`       | Maximum blocks behind pool leader        | `5`     | ❌       |
| `external_reference_threshold` | Maximum blocks behind external reference | `10`    | ❌       |

#### External References

**Syntax**: `external_reference <type> { ... }`

| Option    | Description                                              | Default | Required |
| --------- | -------------------------------------------------------- | ------- | -------- |
| `<type>`  | Reference type (`cosmos` or `evm`) specified as argument | -       | ✅       |
| `name`    | Reference identifier                                     | -       | ✅       |
| `url`     | External endpoint URL                                    | -       | ✅       |
| `enabled` | Enable this reference                                    | `true`  | ❌       |

**Example**:

```caddy
external_reference cosmos {
    name "cosmos_public"
    url "https://cosmos-rpc.publicnode.com"
    enabled true
}
```

#### Performance Settings

| Option                  | Description                      | Default | Required |
| ----------------------- | -------------------------------- | ------- | -------- |
| `cache_duration`        | How long to cache health results | `30s`   | ❌       |
| `max_concurrent_checks` | Maximum concurrent health checks | `10`    | ❌       |

#### Failure Handling

| Option                      | Description                           | Default | Required |
| --------------------------- | ------------------------------------- | ------- | -------- |
| `min_healthy_nodes`         | Minimum healthy nodes required        | `1`     | ❌       |
| `grace_period`              | How long to keep unhealthy nodes      | `60s`   | ❌       |
| `circuit_breaker_threshold` | Failure ratio to open circuit breaker | `0.8`   | ❌       |

#### Monitoring Settings

| Option            | Description                              | Default   | Required |
| ----------------- | ---------------------------------------- | --------- | -------- |
| `metrics_enabled` | Enable Prometheus metrics                | `false`   | ❌       |
| `log_level`       | Logging level (debug, info, warn, error) | `info`    | ❌       |
| `health_endpoint` | HTTP endpoint for health status          | `/health` | ❌       |

### Protocol Validation

The plugin performs protocol-specific health checks:

#### Cosmos SDK Chains

```json
{
  "sub": "user_123",
  "jti": "node_abc123",
  "sync_info": {
    "latest_block_height": "12345678",
    "catching_up": false
  },
  "status": "healthy"
}
```

#### EVM Chains

```json
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": "0xbc614e"
}
```

> **Critical**: The plugin validates sync status for Cosmos (`catching_up: false`) and block height for both protocols to ensure nodes are current and healthy.

## Health Endpoint

The module exposes a comprehensive health endpoint:

```bash
curl http://blockchain-api.example.com/health
```

Response:

```json
{
  "status": "healthy",
  "timestamp": "2024-01-15T10:30:00Z",
  "nodes": {
    "total": 4,
    "healthy": 3,
    "unhealthy": 1
  },
  "external_references": {
    "cosmos_mainnet": {
      "reachable": true,
      "block_height": 12345678
    },
    "ethereum_infura": {
      "reachable": true,
      "block_height": 18500000
    }
  },
  "cache": {
    "total_entries": 4,
    "valid_entries": 3,
    "expired_entries": 1,
    "cache_duration": "30s"
  },
  "last_check": "2024-01-15T10:29:45Z"
}
```

## Prometheus Metrics

When `metrics_enabled` is true, the module exposes the following metrics:

- `caddy_blockchain_health_checks_total`: Total number of health checks
- `caddy_blockchain_health_healthy_nodes`: Number of healthy nodes
- `caddy_blockchain_health_unhealthy_nodes`: Number of unhealthy nodes
- `caddy_blockchain_health_check_duration_seconds`: Health check duration
- `caddy_blockchain_health_block_height`: Current block height per node
- `caddy_blockchain_health_errors_total`: Error count by node and type

## Architecture

This plugin implements a **health-first architecture** for optimal blockchain infrastructure management:

1. **Extract node configuration** from Caddyfile/JSON
2. **Concurrent health checks** with protocol-specific validation
3. **Circuit breaker evaluation** per node with failure thresholds
4. **Block height validation** within pools and against external references
5. **Cache results** with TTL to optimize performance
6. **Dynamic upstream selection** based on health status

This design ensures:

- **Fast rejection** of unhealthy nodes (~0.1-1ms)
- **Protocol awareness** - blockchain-specific health validation
- **High availability** - intelligent failover with minimum node enforcement
- **Performance** - cached results with configurable refresh

## Performance

- **Latency**: ~0.1-1ms per request (with caching)
- **Memory**: Minimal overhead with connection pooling
- **Health check operations**: Concurrent with configurable limits
- **Throughput**: Tested at >10,000 RPS with negligible impact
- **Cache efficiency**: Configurable TTL balances freshness vs performance

## Development & Testing

### Setup Development Environment

```bash
git clone https://github.com/chalabi2/caddy-blockchain-health
cd caddy-blockchain-health
make dev-setup
```

### Run Tests

```bash
# Run all tests
make test-all

# Run with coverage
make test-coverage

# Run benchmarks
make benchmark

# Run integration tests (requires Docker)
make test-integration
```

### Integration Testing

```bash
# Build custom Caddy binary
make xcaddy-build

# Start example configuration
make example-start

# Test with example configs
make example-validate
```

### Performance Testing

```bash
# Run performance tests with real load
make perf-test
```

## Migration from Traditional Health Checks

If you're currently using basic HTTP health checks for blockchain nodes:

### Before (Basic HTTP)

```caddy
api.example.com {
    reverse_proxy {
        to http://node1:26657 http://node2:26657
        health_uri /health
        health_interval 30s
    }
}
```

### After (Blockchain-Aware)

```caddy
api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            node node1 {
                url "http://node1:26657"
                type "cosmos"
                weight 100
            }

            node node2 {
                url "http://node2:26657"
                type "cosmos"
                weight 100
            }

            check_interval "15s"
            block_height_threshold 5
            metrics_enabled true
        }
    }
}
```

**Benefits**:

- ✅ **Protocol-specific validation** (sync status, block height)
- ✅ **Intelligent failover** based on blockchain health
- ✅ **External reference validation** against trusted sources
- ✅ **Circuit breaker protection** for unhealthy nodes
- ✅ **Comprehensive monitoring** with Prometheus metrics

## Requirements

- **Caddy**: v2.7.0 or higher
- **Go**: 1.21 or higher
- **Protocols**: Cosmos SDK, Ethereum/EVM JSON-RPC

## License

MIT License - see LICENSE file.

## Contributing

1. Fork the repository
2. Create a feature branch (`git checkout -b feature/amazing-feature`)
3. Add tests for new functionality (`make test`)
4. Ensure all tests pass (`make test-all`)
5. Submit a pull request

### Bug Reports

When reporting bugs, please include:

- Caddy version (`./caddy version`)
- Plugin version and build info
- Configuration (Caddyfile or JSON)
- Blockchain node types and versions
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs with debug level enabled

**Example**:

```bash
# Enable debug logging
make xcaddy-build
./caddy run --config example_configs/Caddyfile
```
