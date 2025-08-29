package blockchain_health

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

// parseCaddyfile parses the Caddyfile configuration
func (b *BlockchainHealthUpstream) parseCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "node":
				node, err := b.parseNode(d)
				if err != nil {
					return fmt.Errorf("parsing node: %w", err)
				}
				b.Nodes = append(b.Nodes, node)

			case "external_reference":
				ref, err := b.parseExternalReference(d)
				if err != nil {
					return fmt.Errorf("parsing external reference: %w", err)
				}
				b.ExternalReferences = append(b.ExternalReferences, ref)

			case "check_interval":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.HealthCheck.Interval = d.Val()

			case "timeout":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.HealthCheck.Timeout = d.Val()

			case "retry_attempts":
				if !d.NextArg() {
					return d.ArgErr()
				}
				attempts, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid retry_attempts: %v", err)
				}
				b.HealthCheck.RetryAttempts = attempts

			case "retry_delay":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.HealthCheck.RetryDelay = d.Val()

			case "block_height_threshold":
				if !d.NextArg() {
					return d.ArgErr()
				}
				threshold, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid block_height_threshold: %v", err)
				}
				b.BlockValidation.HeightThreshold = threshold

			case "external_reference_threshold":
				if !d.NextArg() {
					return d.ArgErr()
				}
				threshold, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid external_reference_threshold: %v", err)
				}
				b.BlockValidation.ExternalReferenceThreshold = threshold

			case "cache_duration":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Performance.CacheDuration = d.Val()

			case "max_concurrent_checks":
				if !d.NextArg() {
					return d.ArgErr()
				}
				checks, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid max_concurrent_checks: %v", err)
				}
				b.Performance.MaxConcurrentChecks = checks

			case "min_healthy_nodes":
				if !d.NextArg() {
					return d.ArgErr()
				}
				nodes, err := strconv.Atoi(d.Val())
				if err != nil {
					return d.Errf("invalid min_healthy_nodes: %v", err)
				}
				b.FailureHandling.MinHealthyNodes = nodes

			case "grace_period":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.FailureHandling.GracePeriod = d.Val()

			case "circuit_breaker_threshold":
				if !d.NextArg() {
					return d.ArgErr()
				}
				threshold, err := strconv.ParseFloat(d.Val(), 64)
				if err != nil {
					return d.Errf("invalid circuit_breaker_threshold: %v", err)
				}
				b.FailureHandling.CircuitBreakerThreshold = threshold

			case "metrics_enabled":
				if !d.NextArg() {
					return d.ArgErr()
				}
				enabled, err := strconv.ParseBool(d.Val())
				if err != nil {
					return d.Errf("invalid metrics_enabled: %v", err)
				}
				b.Monitoring.MetricsEnabled = enabled

			case "log_level":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Monitoring.LogLevel = d.Val()

			case "health_endpoint":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Monitoring.HealthEndpoint = d.Val()

			// Environment-based configuration
			case "servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.Servers = strings.Join(servers, " ")

			case "rpc_servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.RPCServers = strings.Join(servers, " ")

			case "api_servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.APIServers = strings.Join(servers, " ")

			case "websocket_servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.WebSocketServers = strings.Join(servers, " ")

			case "evm_servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.EVMServers = strings.Join(servers, " ")

			case "evm_ws_servers":
				servers := []string{}
				for d.NextArg() {
					servers = append(servers, d.Val())
				}
				b.Environment.EVMWSServers = strings.Join(servers, " ")

			// Chain configuration
			case "chain_type":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Chain.ChainType = d.Val()

			case "chain_preset":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Chain.ChainPreset = d.Val()

			case "auto_discover_from_env":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Chain.AutoDiscoverFromEnv = d.Val()

			case "service_type":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Chain.ServiceType = d.Val()

			// Legacy configuration
			case "legacy_mode":
				if !d.NextArg() {
					return d.ArgErr()
				}
				legacyMode, err := strconv.ParseBool(d.Val())
				if err != nil {
					return d.Errf("invalid legacy_mode: %v", err)
				}
				b.Legacy.LegacyMode = legacyMode

			case "fallback_behavior":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Legacy.FallbackBehavior = d.Val()

			case "required_env_vars":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Legacy.RequiredEnvVars = d.Val()

			case "optional_env_vars":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b.Legacy.OptionalEnvVars = d.Val()

			default:
				return d.Errf("unknown directive: %s", d.Val())
			}
		}
	}

	return nil
}

// parseNode parses a node block from the Caddyfile
func (b *BlockchainHealthUpstream) parseNode(d *caddyfile.Dispenser) (NodeConfig, error) {
	var node NodeConfig

	// Get the node name
	if !d.NextArg() {
		return node, d.ArgErr()
	}
	node.Name = d.Val()
	node.Weight = 100 // default weight

	// Parse the node block
	for d.NextBlock(1) {
		switch d.Val() {
		case "url":
			if !d.NextArg() {
				return node, d.ArgErr()
			}
			node.URL = d.Val()

		case "api_url":
			if !d.NextArg() {
				return node, d.ArgErr()
			}
			node.APIURL = d.Val()

		case "websocket_url":
			if !d.NextArg() {
				return node, d.ArgErr()
			}
			node.WebSocketURL = d.Val()

		case "type":
			if !d.NextArg() {
				return node, d.ArgErr()
			}
			nodeType := d.Val()
			if nodeType != "cosmos" && nodeType != "evm" {
				return node, d.Errf("invalid node type: %s (must be 'cosmos' or 'evm')", nodeType)
			}
			node.Type = NodeType(nodeType)

		case "weight":
			if !d.NextArg() {
				return node, d.ArgErr()
			}
			weight, err := strconv.Atoi(d.Val())
			if err != nil {
				return node, d.Errf("invalid weight: %v", err)
			}
			if weight <= 0 {
				return node, d.Errf("weight must be positive")
			}
			node.Weight = weight

		case "metadata":
			if node.Metadata == nil {
				node.Metadata = make(map[string]string)
			}

			// Parse metadata block
			for d.NextBlock(2) {
				key := d.Val()
				if !d.NextArg() {
					return node, d.ArgErr()
				}
				value := d.Val()
				node.Metadata[key] = value
			}

		default:
			return node, d.Errf("unknown node directive: %s", d.Val())
		}
	}

	// Validate required fields
	if node.URL == "" {
		return node, d.Errf("node %s: url is required", node.Name)
	}
	if node.Type == "" {
		return node, d.Errf("node %s: type is required", node.Name)
	}

	return node, nil
}

// parseExternalReference parses an external reference block from the Caddyfile
func (b *BlockchainHealthUpstream) parseExternalReference(d *caddyfile.Dispenser) (ExternalReference, error) {
	var ref ExternalReference

	// Get the reference type
	if !d.NextArg() {
		return ref, d.ArgErr()
	}
	refType := d.Val()
	if refType != "cosmos" && refType != "evm" {
		return ref, d.Errf("invalid external reference type: %s (must be 'cosmos' or 'evm')", refType)
	}
	ref.Type = NodeType(refType)
	ref.Enabled = true // default enabled

	// Parse the external reference block
	for d.NextBlock(1) {
		switch d.Val() {
		case "name":
			if !d.NextArg() {
				return ref, d.ArgErr()
			}
			ref.Name = d.Val()

		case "url":
			if !d.NextArg() {
				return ref, d.ArgErr()
			}
			ref.URL = d.Val()

		case "enabled":
			if !d.NextArg() {
				return ref, d.ArgErr()
			}
			enabled, err := strconv.ParseBool(d.Val())
			if err != nil {
				return ref, d.Errf("invalid enabled value: %v", err)
			}
			ref.Enabled = enabled

		default:
			return ref, d.Errf("unknown external reference directive: %s", d.Val())
		}
	}

	// Validate required fields
	if ref.Name == "" {
		return ref, d.Errf("external reference: name is required")
	}
	if ref.URL == "" {
		return ref, d.Errf("external reference %s: url is required", ref.Name)
	}

	return ref, nil
}

// processEnvironmentConfiguration processes environment-based configuration
func (b *BlockchainHealthUpstream) processEnvironmentConfiguration() error {
	// Process auto-discovery from environment variables
	if b.Chain.AutoDiscoverFromEnv != "" {
		if err := b.autoDiscoverFromEnvironment(b.Chain.AutoDiscoverFromEnv); err != nil {
			return fmt.Errorf("auto-discovery failed: %w", err)
		}
	}

	// Process individual server lists
	if err := b.processServerLists(); err != nil {
		return fmt.Errorf("processing server lists: %w", err)
	}

	// Apply chain presets
	if b.Chain.ChainPreset != "" {
		if err := b.applyChainPreset(b.Chain.ChainPreset); err != nil {
			return fmt.Errorf("applying chain preset: %w", err)
		}
	}

	// Generate external references only if explicitly configured
	// Respect external_reference_threshold setting
	if len(b.ExternalReferences) == 0 && b.BlockValidation.ExternalReferenceThreshold > 0 {
		b.generateExternalReferences()
	}

	return nil
}

// autoDiscoverFromEnvironment discovers servers from environment variables
func (b *BlockchainHealthUpstream) autoDiscoverFromEnvironment(prefix string) error {
	// Look for environment variables like COSMOS_RPC_SERVERS, COSMOS_API_SERVERS, etc.
	envVars := map[string]string{
		prefix + "_RPC_SERVERS": "rpc",
		prefix + "_API_SERVERS": "api",
		prefix + "_WS_SERVERS":  "websocket",
		prefix + "_SERVERS":     "generic",
	}

	for envVar, serviceType := range envVars {
		if servers := os.Getenv(envVar); servers != "" {
			if err := b.parseServersFromEnv(servers, serviceType); err != nil {
				return fmt.Errorf("parsing %s: %w", envVar, err)
			}
		}
	}

	return nil
}

// processServerLists processes individual server list configurations
func (b *BlockchainHealthUpstream) processServerLists() error {
	serverConfigs := []struct {
		servers     string
		serviceType string
		chainType   string
	}{
		{b.Environment.Servers, "generic", b.Chain.ChainType},
		{b.Environment.RPCServers, "rpc", "cosmos"},
		{b.Environment.APIServers, "api", "cosmos"},
		{b.Environment.WebSocketServers, "websocket", "cosmos"},
		{b.Environment.EVMServers, "rpc", "evm"},
		{b.Environment.EVMWSServers, "websocket", "evm"},
	}

	for _, config := range serverConfigs {
		if config.servers != "" {
			chainType := config.chainType
			if chainType == "" {
				chainType = b.Chain.ChainType
			}
			if err := b.parseServersFromEnv(config.servers, config.serviceType); err != nil {
				return fmt.Errorf("parsing %s servers: %w", config.serviceType, err)
			}
		}
	}

	return nil
}

// parseServersFromEnv parses a space-separated list of servers and creates nodes
func (b *BlockchainHealthUpstream) parseServersFromEnv(servers, serviceType string) error {
	if servers == "" {
		return nil
	}

	serverList := strings.Fields(servers)
	for i, serverURL := range serverList {
		node, err := b.createNodeFromURL(serverURL, serviceType, i)
		if err != nil {
			return fmt.Errorf("creating node from URL %s: %w", serverURL, err)
		}
		b.Nodes = append(b.Nodes, node)
	}

	return nil
}

// createNodeFromURL creates a node configuration from a URL
func (b *BlockchainHealthUpstream) createNodeFromURL(serverURL, serviceType string, index int) (NodeConfig, error) {
	var node NodeConfig

	// Parse URL to extract information
	parsedURL, err := url.Parse(serverURL)
	if err != nil {
		return node, fmt.Errorf("parsing URL: %w", err)
	}

	// Auto-detect service type and chain type from URL if not specified
	detectedType, detectedChain := b.autoDetectServiceType(parsedURL)

	if serviceType == "generic" {
		serviceType = detectedType
	}

	chainType := b.Chain.ChainType
	if chainType == "" {
		chainType = detectedChain
	}

	// Use detected chain type if chain type is not set or is generic
	actualNodeType := chainType
	if chainType == "" || chainType == "dual" {
		actualNodeType = detectedChain
	}

	// Generate node name
	nodeName := b.generateNodeName(chainType, serviceType, index)

	// Create basic node configuration
	node = NodeConfig{
		Name:   nodeName,
		URL:    serverURL,
		Type:   NodeType(actualNodeType),
		Weight: 100, // Default weight
		Metadata: map[string]string{
			"service_type":   serviceType,
			"auto_generated": "true",
			"source":         "environment",
		},
	}

	// Auto-generate WebSocket URL if needed
	if wsURL := b.generateWebSocketURL(parsedURL, actualNodeType); wsURL != "" {
		node.WebSocketURL = wsURL
	}

	// Set API URL for Cosmos nodes if this is an RPC endpoint
	if actualNodeType == "cosmos" && serviceType == "rpc" {
		if apiURL := b.generateAPIURL(parsedURL); apiURL != "" {
			node.APIURL = apiURL
		}
	}

	return node, nil
}

// autoDetectServiceType automatically detects service type and chain type from URL
func (b *BlockchainHealthUpstream) autoDetectServiceType(parsedURL *url.URL) (serviceType, chainType string) {
	// Don't make assumptions about ports - let the environment configuration determine service types
	// The service_type is already specified in the environment variables (rpc_servers, api_servers, etc.)

	// Default to generic service type - the actual type comes from environment config
	return "generic", "cosmos"
}

// generateNodeName generates a unique node name
func (b *BlockchainHealthUpstream) generateNodeName(chainType, serviceType string, index int) string {
	if chainType == "" {
		chainType = "blockchain"
	}
	if serviceType == "" {
		serviceType = "node"
	}
	return fmt.Sprintf("%s-%s-%d", chainType, serviceType, index)
}

// generateWebSocketURL generates WebSocket URL from HTTP URL
func (b *BlockchainHealthUpstream) generateWebSocketURL(parsedURL *url.URL, chainType string) string {
	if chainType == "cosmos" {
		// Cosmos: convert HTTP to WebSocket and add /websocket path
		wsURL := *parsedURL
		switch wsURL.Scheme {
		case "http":
			wsURL.Scheme = "ws"
		case "https":
			wsURL.Scheme = "wss"
		}
		wsURL.Path = "/websocket"
		return wsURL.String()
	} else if chainType == "evm" {
		// EVM: convert HTTP to WebSocket (no path change needed)
		wsURL := *parsedURL
		switch wsURL.Scheme {
		case "http":
			wsURL.Scheme = "ws"
		case "https":
			wsURL.Scheme = "wss"
		}
		return wsURL.String()
	}

	return ""
}

// generateAPIURL generates REST API URL from RPC URL for Cosmos
// Note: This is only used when auto-generating API URLs from RPC URLs
// In most cases, API URLs should be explicitly configured via environment variables
func (b *BlockchainHealthUpstream) generateAPIURL(parsedURL *url.URL) string {
	// Don't make assumptions about ports - let users configure API URLs explicitly
	// This function is kept for backward compatibility but should rarely be used
	return ""
}

// applyChainPreset applies predefined chain configuration
func (b *BlockchainHealthUpstream) applyChainPreset(preset string) error {
	switch preset {
	case "cosmos", "cosmos-hub":
		b.Chain.ChainType = "cosmos"
		b.addCosmosHubDefaults()
	case "ethereum":
		b.Chain.ChainType = "evm"
		b.addEthereumDefaults()
	case "althea":
		// Don't set chain_type for Althea - let auto-detection handle it
		// since Cosmos and EVM services run on different ports
		b.addAltheaDefaults()
	default:
		return fmt.Errorf("unknown chain preset: %s", preset)
	}
	return nil
}

// generateExternalReferences generates external references based on chain type
// Only generates references if explicitly configured - no hardcoded defaults
func (b *BlockchainHealthUpstream) generateExternalReferences() {
	// Only generate external references if they are explicitly configured
	// No hardcoded defaults to avoid rate limiting and chain-specific issues
	if len(b.ExternalReferences) == 0 {
		// No external references configured - this is fine
		return
	}

	// If external references are manually configured, validate them
	for i, ref := range b.ExternalReferences {
		if ref.URL == "" {
			b.ExternalReferences[i].Enabled = false
		}
	}
}

// Chain preset helper functions
func (b *BlockchainHealthUpstream) addCosmosHubDefaults() {
	// Add Cosmos Hub specific defaults
	if b.HealthCheck.Interval == "" {
		b.HealthCheck.Interval = "10s"
	}
	if b.BlockValidation.HeightThreshold == 0 {
		b.BlockValidation.HeightThreshold = 5
	}
}

func (b *BlockchainHealthUpstream) addEthereumDefaults() {
	// Add Ethereum specific defaults
	if b.HealthCheck.Interval == "" {
		b.HealthCheck.Interval = "12s"
	}
	if b.BlockValidation.HeightThreshold == 0 {
		b.BlockValidation.HeightThreshold = 2
	}
}

func (b *BlockchainHealthUpstream) addAltheaDefaults() {
	// Add Althea (dual protocol) specific defaults
	if b.HealthCheck.Interval == "" {
		b.HealthCheck.Interval = "15s"
	}
	if b.BlockValidation.HeightThreshold == 0 {
		b.BlockValidation.HeightThreshold = 5
	}

	// No hardcoded external references - let users configure their own
	// to avoid rate limiting and chain-specific issues
}
