# Blockchain Health Dynamic Upstream

[![codecov](https://codecov.io/gh/chalabi2/caddy-blockchain-health/graph/badge.svg?token=18AC52SBNE)](https://codecov.io/gh/chalabi2/caddy-blockchain-health)
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

Basic Caddyfile configuration using environment variables:

```caddy
{
    admin localhost:2019
}

blockchain-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Cosmos Hub preset with automatic configuration
            chain_preset "cosmos-hub"
            servers {$COSMOS_SERVERS}

            # Production settings
            min_healthy_nodes 2
            circuit_breaker_threshold 0.8
            metrics_enabled true
        }
    }

    # Health endpoint
    handle /health {
        reverse_proxy localhost:8080
    }
}

# Ethereum configuration
ethereum-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Ethereum preset with automatic configuration
            chain_preset "ethereum"
            evm_servers {$ETH_SERVERS}

            # Auto-generates WebSocket URLs and external references
            min_healthy_nodes 1
            metrics_enabled true
        }
    }
}
```

Set your environment variables:

```bash
export COSMOS_SERVERS="http://cosmos-1:26657 http://cosmos-2:26657 http://cosmos-3:26657"
export ETH_SERVERS="http://eth-1:8545 http://eth-2:8545"
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
            # Auto-discovery from environment variables
            auto_discover_from_env "COSMOS"

            # Additional EVM servers
            evm_servers {$ETH_SERVERS}

            # Comprehensive health monitoring
            check_interval "10s"
            timeout "3s"
            retry_attempts 3
            block_height_threshold 3

            # Production resilience
            min_healthy_nodes 2
            circuit_breaker_threshold 0.8
            cache_duration "30s"
            max_concurrent_checks 10

            # Full monitoring
            metrics_enabled true
        }
    }
}
```

Environment variables:

```bash
export COSMOS_RPC_SERVERS="http://cosmos-us-east-1:26657 http://cosmos-eu-west-1:26657"
export COSMOS_API_SERVERS="http://cosmos-us-east-1:1317 http://cosmos-eu-west-1:1317"
export COSMOS_WS_SERVERS="ws://cosmos-us-east-1:26657/websocket ws://cosmos-eu-west-1:26657/websocket"
export ETH_SERVERS="http://ethereum-1:8545 http://ethereum-2:8545"
```

#### 2. **Separated Services** (RPC and REST on different endpoints)

```caddy
# Cosmos RPC endpoint
cosmos-rpc.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            rpc_servers {$COSMOS_RPC_SERVERS}
            chain_type "cosmos"
            service_type "rpc"

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
            api_servers {$COSMOS_API_SERVERS}
            chain_type "cosmos"
            service_type "api"

            check_interval "15s"
            min_healthy_nodes 1
            metrics_enabled true
        }
    }
}
```

Environment variables:

```bash
export COSMOS_RPC_SERVERS="http://cosmos-node-1:26657 http://cosmos-node-2:26657"
export COSMOS_API_SERVERS="http://cosmos-node-1:1317 http://cosmos-node-2:1317"
```

#### 3. **Development Configuration**

```caddy
dev-blockchain.localhost {
    reverse_proxy {
        dynamic blockchain_health {
            # Generic server list with auto-detection
            servers {$DEV_SERVERS}

            # Relaxed settings for development
            check_interval "5s"
            timeout "2s"
            block_height_threshold 10
            circuit_breaker_threshold 0.9
            fallback_behavior "disable_health_checks"
            log_level "debug"

            # No minimum nodes required in development
            min_healthy_nodes 0
        }
    }
}
```

Environment variables:

```bash
export DEV_SERVERS="http://localhost:26657 http://localhost:1317 http://localhost:8545"
```

### ⚠️ **Important: Service Separation Behavior**

**Pattern 1 (Multi-Chain)**: ✅ **Full health validation** - Checks all configured endpoints with comprehensive monitoring.

**Pattern 2 (Separated Services)**: ✅ **Service-specific validation** - Only checks the specific service type (RPC or REST) without redundant checks.

**Pattern 3 (Development)**: ✅ **Relaxed validation** - Lenient settings suitable for local development and testing.

**Recommendation**: Use Pattern 1 for production APIs requiring maximum reliability, Pattern 2 for microservice architectures, and Pattern 3 for development environments.

### Configuration Options

#### 🌟 **Environment Variable Configuration** (Recommended)

The plugin now supports simplified environment variable-based configuration:

| Option                   | Description                                                         | Example                 |
| ------------------------ | ------------------------------------------------------------------- | ----------------------- |
| `servers`                | Generic space-separated server list with auto-detection             | `{$BLOCKCHAIN_SERVERS}` |
| `rpc_servers`            | Cosmos RPC servers (port 26657)                                     | `{$COSMOS_RPC_SERVERS}` |
| `api_servers`            | Cosmos REST API servers (port 1317)                                 | `{$COSMOS_API_SERVERS}` |
| `websocket_servers`      | Cosmos WebSocket servers                                            | `{$COSMOS_WS_SERVERS}`  |
| `evm_servers`            | EVM JSON-RPC servers (port 8545)                                    | `{$ETH_SERVERS}`        |
| `evm_ws_servers`         | EVM WebSocket servers (port 8546)                                   | `{$ETH_WS_SERVERS}`     |
| `chain_preset`           | Predefined chain configuration (`cosmos-hub`, `ethereum`, `althea`) | `"cosmos-hub"`          |
| `auto_discover_from_env` | Auto-discover from environment variables with prefix                | `"COSMOS"`              |
| `chain_type`             | Blockchain type (`cosmos`, `evm`)                                   | `"cosmos"`              |
| `legacy_mode`            | Backward compatibility mode                                         | `true`                  |

#### Traditional Node Settings (Legacy)

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
cosmos-combined.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Auto-discovery will find both RPC and API servers
            auto_discover_from_env "COSMOS"

            # Or specify both explicitly
            rpc_servers {$COSMOS_RPC_SERVERS}
            api_servers {$COSMOS_API_SERVERS}
        }
    }
}
```

Environment variables:

```bash
export COSMOS_RPC_SERVERS="http://cosmos-node:26657"
export COSMOS_API_SERVERS="http://cosmos-node:1317"
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
            rpc_servers {$COSMOS_RPC_SERVERS}    # Only RPC
            chain_type "cosmos"
            service_type "rpc"
        }
    }
}

# Cosmos REST API load balancer
cosmos-api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            api_servers {$COSMOS_API_SERVERS}    # Only REST
            chain_type "cosmos"
            service_type "api"
        }
    }
}
```

Environment variables:

```bash
export COSMOS_RPC_SERVERS="http://cosmos-rpc-1:26657 http://cosmos-rpc-2:26657"
export COSMOS_API_SERVERS="http://cosmos-api-1:1317 http://cosmos-api-2:1317"
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
cosmos-websocket.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Auto-discovery generates WebSocket URLs automatically
            auto_discover_from_env "COSMOS"

            # Or specify explicit WebSocket servers
            websocket_servers {$COSMOS_WS_SERVERS}
            chain_type "cosmos"
            service_type "websocket"
        }
    }
}
```

Environment variables:

```bash
export COSMOS_RPC_SERVERS="http://cosmos-node:26657"
export COSMOS_API_SERVERS="http://cosmos-node:1317"
export COSMOS_WS_SERVERS="ws://cosmos-node:26657/websocket"
```

- ✅ **Tendermint WebSocket subscriptions** - Tests `tm.event = 'NewBlock'` subscriptions
- ✅ **Real-time event streaming** - Validates connectivity for live event monitoring
- ✅ **Auto scheme conversion** - Converts `http`/`https` to `ws`/`wss` automatically

**EVM WebSocket Configuration:**

```caddy
ethereum-websocket.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # IMPORTANT: Both HTTP and WebSocket servers must be specified
            # for proper health checking and correlation
            evm_servers {$ETH_SERVERS}        # HTTP endpoints for health checks
            evm_ws_servers {$ETH_WS_SERVERS}  # WebSocket endpoints for proxy
            chain_type "evm"
        }
    }
}
```

Environment variables (servers correlated by index/hostname):

```bash
# HTTP JSON-RPC endpoints (used for health checks)
export ETH_SERVERS="http://geth-node1:8545 http://geth-node2:8545"
# WebSocket endpoints (used for proxy, correlated with HTTP endpoints)
export ETH_WS_SERVERS="ws://geth-node1:8546 ws://geth-node2:8546"
```

**Custom Ports Example:**

```bash
# Your production setup with custom ports
export BASE_SERVERS="http://95.216.38.96:13245 http://8.40.118.101:13245"
export BASE_WS_SERVERS="ws://95.216.38.96:13246 ws://8.40.118.101:13246"
```

- ✅ **Intelligent correlation** - Automatically correlates WebSocket and HTTP endpoints by hostname or index
- ✅ **HTTP health checks** - Uses correlated HTTP endpoints for `eth_blockNumber` validation
- ✅ **WebSocket proxy** - Routes WebSocket traffic to healthy WebSocket endpoints
- ✅ **Block height validation** - Full blockchain health checking via HTTP while proxying to WebSocket
- ✅ **Custom ports supported** - No assumptions about standard ports (8545/8546)

**WebSocket Health Checking:**

- **Correlation-based** - WebSocket nodes use correlated HTTP endpoints for health validation
- **Full blockchain validation** - Block height, sync status, and external reference checking
- **Non-blocking WebSocket tests** - Optional WebSocket connectivity verification (informational)
- **Timeout protection** - 3-second read timeout prevents hanging connections
- **Protocol-specific tests** - Uses appropriate subscription methods per blockchain type

#### 🔗 **EVM JSON-RPC Node Differentiation**

EVM nodes use JSON-RPC protocol and don't have separate RPC/REST endpoints like Cosmos:

**Standard EVM Configuration:**

```caddy
ethereum-primary.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Ethereum preset with auto-configuration
            chain_preset "ethereum"
            evm_servers {$ETH_SERVERS}

            # Auto-generates WebSocket URLs and external references
            min_healthy_nodes 1
            metrics_enabled true
        }
    }
}
```

Environment variables:

```bash
export ETH_SERVERS="http://ethereum-node:8545"
```

- ✅ **Single endpoint** - All requests use JSON-RPC over HTTP
- ✅ **Health check via `eth_blockNumber`** - Validates node responsiveness and current block
- ✅ **No separate API URL needed** - EVM protocol is unified

**EVM Service Types (by function, not protocol):**

```caddy
ethereum-mixed.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Mixed node types with different capabilities
            evm_servers {$ETH_ARCHIVE_SERVERS} {$ETH_FULL_SERVERS} {$ETH_LIGHT_SERVERS}
            chain_type "evm"

            # Standard EVM health checking
            check_interval "12s"
            block_height_threshold 2
        }
    }
}
```

Environment variables:

```bash
export ETH_ARCHIVE_SERVERS="http://archive-node:8545"
export ETH_FULL_SERVERS="http://full-node:8545"
export ETH_LIGHT_SERVERS="http://light-node:8545"
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
    evm_servers {$ETH_POOL_SERVERS}
    chain_type "evm"

    # If any node is more than 5 blocks behind the highest in the pool
    block_height_threshold 5
}
```

Environment variables:

```bash
export ETH_POOL_SERVERS="http://eth-1.internal:8545 http://eth-2.internal:8545 http://eth-3.internal:8545"
```

**Logic**: If `eth-node-1` is at block 18,500,000 and `eth-node-2` is at 18,499,994, then `eth-node-2` is **removed from load balancer** (6 blocks behind > threshold of 5).

##### **2. External Reference Monitoring** ℹ️ **Informational Only**

Monitors your nodes against trusted external sources **for observability** (does not affect load balancing):

**EVM External References:**

```caddy
dynamic blockchain_health {
    # Your nodes
    evm_servers {$YOUR_ETH_SERVERS}
    chain_type "evm"

    # Automatically configured external references when using preset
    chain_preset "ethereum"

    # Or manually configure external references
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

Environment variables:

```bash
export YOUR_ETH_SERVERS="http://your-node:8545"
```

**Multi-Chain EVM Examples:**

```caddy
# Polygon network
polygon.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            evm_servers {$POLYGON_SERVERS}
            chain_type "evm"

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
    }
}

# Binance Smart Chain
bsc.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            evm_servers {$BSC_SERVERS}
            chain_type "evm"

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
    }
}

# Arbitrum network
arbitrum.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            evm_servers {$ARBITRUM_SERVERS}
            chain_type "evm"

            external_reference evm {
                name "arbitrum_alchemy"
                url "https://arb-mainnet.g.alchemy.com/v2/YOUR_API_KEY"
                enabled true
            }
        }
    }
}
```

Environment variables:

```bash
export POLYGON_SERVERS="http://polygon-node:8545"
export BSC_SERVERS="http://bsc-node:8545"
export ARBITRUM_SERVERS="http://arbitrum-node:8545"
```

**Cosmos External References:**

```caddy
cosmos-external.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Cosmos Hub preset automatically includes external references
            chain_preset "cosmos-hub"
            servers {$COSMOS_SERVERS}

            # Or add custom external references
            external_reference cosmos {
                name "cosmos_polkachu"
                url "https://cosmos-rpc.polkachu.com"
                enabled true
            }
        }
    }
}
```

Environment variables:

```bash
export COSMOS_SERVERS="http://cosmos-node:26657"
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

### After (Environment Variable Approach)

```caddy
api.example.com {
    reverse_proxy {
        dynamic blockchain_health {
            # Chain preset with automatic configuration
            chain_preset "cosmos-hub"
            servers {$COSMOS_SERVERS}

            # Enhanced settings
            check_interval "15s"
            block_height_threshold 5
            metrics_enabled true
        }
    }
}
```

Environment variables:

```bash
export COSMOS_SERVERS="http://node1:26657 http://node2:26657"
```

### Legacy Node-Based Approach (Still Supported)

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

**Benefits of Environment Variable Approach**:

- ✅ **Simplified configuration** - No manual node definitions
- ✅ **Auto-discovery** - Automatic detection of service types
- ✅ **Chain presets** - Predefined configurations for major blockchains
- ✅ **WebSocket auto-generation** - Automatic WebSocket URL creation
- ✅ **Environment integration** - Better CI/CD and deployment workflows
- ✅ **Backward compatibility** - Legacy mode for existing configurations

**Benefits of Blockchain-Aware Health Checks**:

- ✅ **Protocol-specific validation** (sync status, block height)
- ✅ **Intelligent failover** based on blockchain health
- ✅ **External reference validation** against trusted sources
- ✅ **Circuit breaker protection** for unhealthy nodes
- ✅ **Comprehensive monitoring** with Prometheus metrics

## Troubleshooting

### WebSocket Connection Issues

If you're experiencing WebSocket connection failures (e.g., "Received unexpected status code (200 OK)"):

**Problem**: WebSocket upgrade fails because health checks are being performed on WebSocket URLs instead of HTTP URLs.

**Solution**: Ensure both `evm_servers` and `evm_ws_servers` are configured for proper correlation:

```caddy
handle @websocket {
    reverse_proxy {
        dynamic blockchain_health {
            # ✅ CORRECT: Specify both HTTP and WebSocket servers
            evm_servers {$BASE_SERVERS}        # HTTP for health checks
            evm_ws_servers {$BASE_WS_SERVERS}  # WebSocket for proxy
            chain_type "evm"
        }
    }
}
```

```bash
# ✅ CORRECT: Correlated by hostname/index
export BASE_SERVERS="http://node1:8545 http://node2:8545"
export BASE_WS_SERVERS="ws://node1:8546 ws://node2:8546"
```

**Common Mistakes:**

```caddy
# ❌ WRONG: Only WebSocket servers specified
dynamic blockchain_health {
    evm_ws_servers {$BASE_WS_SERVERS}  # Missing HTTP servers for health checks
    chain_type "evm"
}

# ❌ WRONG: Old service_type approach (deprecated)
dynamic blockchain_health {
    evm_ws_servers {$BASE_WS_SERVERS}
    service_type "evm_websocket"  # ❌ No longer needed - causes issues
    chain_type "evm"
}

# ❌ WRONG: Mismatched server counts
# BASE_SERVERS="http://node1:8545 http://node2:8545"        # 2 servers
# BASE_WS_SERVERS="ws://node1:8546"                         # 1 server - can't correlate
```

**Verification:**

Test your WebSocket connection:

```bash
# Should work after fix
websocat wss://your-domain.com/base -H "Authorization: Bearer YOUR_JWT"
```

### Configuration Validation

**Check correlation in logs:**

```bash
# Enable debug logging to see correlation
./caddy run --config Caddyfile --adapter caddyfile
# Look for: "WebSocket node health check successful via HTTP"
```

**Health endpoint verification:**

```bash
curl http://your-domain.com/health
# Should show both HTTP and WebSocket nodes as healthy
```

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
