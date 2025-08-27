## Blockchain Health Module Redesign Requirements

### 1. **Environment Variable Integration**

```caddy
# Current Problem: Manual node definitions
node cosmos-rpc-1 {
    url "http://cosmos-node-1:26657"
    type "cosmos"
}

# Needed: Automatic parsing from env vars
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}  # Auto-parse space-separated list
    type "cosmos"
    service_type "rpc"
}
```

### 2. **Auto-Discovery and Node Creation**

- **Parse space-separated server lists** from environment variables
- **Auto-generate node names** (e.g., `cosmos-rpc-0`, `cosmos-rpc-1`)
- **Auto-detect service types** based on ports/URLs:
  - Port `26657` → Cosmos RPC
  - Port `1317` → Cosmos API
  - Port `8545` → EVM JSON-RPC
  - Port `8546` → EVM WebSocket
  - `/websocket` path → WebSocket endpoint

### 3. **Simplified Configuration Syntax**

```caddy
# Instead of complex node blocks, simple directive:
dynamic blockchain_health {
    # Auto-parse from environment variables
    rpc_servers {$COSMOS_RPC_SERVERS}
    api_servers {$COSMOS_API_SERVERS}
    ws_servers {$COSMOS_WS_SERVERS}  # Optional

    # Or even simpler:
    chain_type "cosmos"
    auto_discover_from_env "COSMOS"  # Looks for COSMOS_*_SERVERS
}
```

### 4. **WebSocket URL Auto-Generation**

- **Convert HTTP to WebSocket URLs** automatically:
  - `http://node:26657` → `ws://node:26657/websocket` (Cosmos)
  - `http://node:8545` → `ws://node:8546` (EVM, different port)
- **Support explicit WebSocket environment variables** when available
- **Fallback to HTTP** if WebSocket not available

### 5. **Chain-Specific Intelligence**

```caddy
dynamic blockchain_health {
    chain_preset "cosmos-hub"     # Loads cosmos defaults + external refs
    # or
    chain_preset "ethereum"       # Loads ethereum defaults + external refs
    # or
    chain_preset "althea"         # Loads dual cosmos+evm setup

    servers {$CHAIN_SERVERS}      # Generic server list
}
```

### 6. **External Reference Automation**

- **Auto-configure external references** based on chain type
- **Built-in reference URLs** for major chains (Cosmos Hub, Ethereum, etc.)
- **Disable external refs** for private/testnet chains automatically
- **Allow override** of auto-configured references

### 7. **Flexible Environment Variable Patterns**

Support multiple naming patterns:

```bash
# Current pattern
COSMOS_RPC_SERVERS="server1 server2"
COSMOS_API_SERVERS="api1 api2"

# Alternative patterns the module should support
CHAIN_COSMOS_RPC="server1 server2"
COSMOS_ENDPOINTS_RPC="server1 server2"
RPC_COSMOS_URLS="server1 server2"
```

### 8. **Service Detection and Separation**

- **Auto-detect combined vs separated services**:
  - If only `RPC_SERVERS` provided → Check both RPC and API on same nodes
  - If both `RPC_SERVERS` and `API_SERVERS` → Separate service pools
- **Smart health checking** based on detected services
- **Automatic endpoint discovery** on nodes

### 9. **Backward Compatibility Mode**

```caddy
dynamic blockchain_health {
    # Legacy mode - works exactly like current reverse_proxy
    legacy_mode true
    servers {$COSMOS_RPC_SERVERS}

    # Just adds blockchain-aware health checks to existing setup
    # No breaking changes to current configuration
}
```

### 10. **Configuration Inheritance and Defaults**

```caddy
# Global defaults
(blockchain_defaults) {
    check_interval "10s"
    timeout "5s"
    block_height_threshold 5
    metrics_enabled true
}

# Chain-specific config inherits defaults
dynamic blockchain_health {
    import blockchain_defaults
    chain_type "cosmos"
    servers {$COSMOS_RPC_SERVERS}
}
```

### 11. **Multi-Protocol Chain Support**

For chains like Althea (Cosmos + EVM):

```caddy
dynamic blockchain_health {
    chain_type "dual"  # or "cosmos+evm"

    cosmos_rpc {$ALTHEA_RPC_SERVERS}
    cosmos_api {$ALTHEA_API_SERVERS}
    evm_rpc {$ALTHEA_EVM_SERVERS}
    evm_ws {$ALTHEA_EVM_WS_SERVERS}
}
```

### 12. **Health Endpoint Consolidation**

- **Single health endpoint** per chain instead of multiple
- **Aggregated health status** across all services
- **Detailed breakdown** available via query parameters
- **Compatible with existing monitoring**

### 13. **Header Passthrough Configuration**

```caddy
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}

    # Auto-detect auth context and pass appropriate headers
    auth_mode "jwt"  # Auto-adds JWT headers
    # or
    auth_mode "public"  # Only basic headers
}
```

### 14. **Migration Compatibility**

The module should provide:

- **Drop-in replacement** for existing `reverse_proxy` blocks
- **Gradual migration path** - can enable features incrementally
- **Configuration validation** - warns about missing env vars
- **Fallback behavior** - graceful degradation if blockchain health fails

### 15. **Environment Variable Validation**

```caddy
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}
    required_env_vars "COSMOS_RPC_SERVERS"  # Validation
    optional_env_vars "COSMOS_API_SERVERS COSMOS_WS_SERVERS"

    # Fail gracefully if required vars missing
    fallback_behavior "disable_health_checks"  # or "fail_startup"
}
```

## Recommended Implementation Priority

1. **Environment variable parsing** (highest priority)
2. **Auto-discovery and node creation**
3. **Chain-specific presets**
4. **WebSocket auto-generation**
5. **External reference automation**
6. **Backward compatibility mode**
7. **Multi-protocol support**
8. **Configuration inheritance**

This redesign would make the module much more compatible with your existing infrastructure while providing the blockchain-aware health benefits.

```plaintext
# Current Problem: Manual node definitions
node cosmos-rpc-1 {
    url "http://cosmos-node-1:26657"
    type "cosmos"
}

# Needed: Automatic parsing from env vars
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}  # Auto-parse space-separated list
    type "cosmos"
    service_type "rpc"
}
```

```plaintext
# Instead of complex node blocks, simple directive:
dynamic blockchain_health {
    # Auto-parse from environment variables
    rpc_servers {$COSMOS_RPC_SERVERS}
    api_servers {$COSMOS_API_SERVERS}
    ws_servers {$COSMOS_WS_SERVERS}  # Optional

    # Or even simpler:
    chain_type "cosmos"
    auto_discover_from_env "COSMOS"  # Looks for COSMOS_*_SERVERS
}
```

```plaintext
dynamic blockchain_health {
    chain_preset "cosmos-hub"     # Loads cosmos defaults + external refs
    # or
    chain_preset "ethereum"       # Loads ethereum defaults + external refs
    # or
    chain_preset "althea"         # Loads dual cosmos+evm setup

    servers {$CHAIN_SERVERS}      # Generic server list
}
```

```shellscript
# Current pattern
COSMOS_RPC_SERVERS="server1 server2"
COSMOS_API_SERVERS="api1 api2"

# Alternative patterns the module should support
CHAIN_COSMOS_RPC="server1 server2"
COSMOS_ENDPOINTS_RPC="server1 server2"
RPC_COSMOS_URLS="server1 server2"
```

```plaintext
dynamic blockchain_health {
    # Legacy mode - works exactly like current reverse_proxy
    legacy_mode true
    servers {$COSMOS_RPC_SERVERS}

    # Just adds blockchain-aware health checks to existing setup
    # No breaking changes to current configuration
}
```

```plaintext
# Global defaults
(blockchain_defaults) {
    check_interval "10s"
    timeout "5s"
    block_height_threshold 5
    metrics_enabled true
}

# Chain-specific config inherits defaults
dynamic blockchain_health {
    import blockchain_defaults
    chain_type "cosmos"
    servers {$COSMOS_RPC_SERVERS}
}
```

```plaintext
dynamic blockchain_health {
    chain_type "dual"  # or "cosmos+evm"

    cosmos_rpc {$ALTHEA_RPC_SERVERS}
    cosmos_api {$ALTHEA_API_SERVERS}
    evm_rpc {$ALTHEA_EVM_SERVERS}
    evm_ws {$ALTHEA_EVM_WS_SERVERS}
}
```

```plaintext
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}

    # Auto-detect auth context and pass appropriate headers
    auth_mode "jwt"  # Auto-adds JWT headers
    # or
    auth_mode "public"  # Only basic headers
}
```

```plaintext
dynamic blockchain_health {
    servers {$COSMOS_RPC_SERVERS}
    required_env_vars "COSMOS_RPC_SERVERS"  # Validation
    optional_env_vars "COSMOS_API_SERVERS COSMOS_WS_SERVERS"

    # Fail gracefully if required vars missing
    fallback_behavior "disable_health_checks"  # or "fail_startup"
}
```
