package blockchain_health

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"go.uber.org/zap"
)

// GetUpstreams implements reverseproxy.UpstreamSource
func (b *BlockchainHealthUpstream) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	// Defensive: ensure module is provisioned and logger present
	if b == nil || b.config == nil || b.healthChecker == nil {
		return nil, fmt.Errorf("blockchain_health upstream not provisioned")
	}
	if b.logger == nil {
		b.logger = zap.NewNop()
	}

	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// Get cached health results to avoid running health checks during request processing
	// This prevents interference with WebSocket upgrades and improves performance
	healthResults := b.getCachedHealthResults()

	// If no cached results available, fall back to a quick health check
	if len(healthResults) == 0 {
		b.logger.Debug("no cached health results available, performing quick health check")
		timeout := 5 * time.Second // Shorter timeout for request-time health checks
		if b.config != nil && b.config.HealthCheck.Timeout != "" {
			if parsedTimeout, err := time.ParseDuration(b.config.HealthCheck.Timeout); err == nil && parsedTimeout < timeout {
				timeout = parsedTimeout
			}
		}

		ctx, cancel := context.WithTimeout(context.Background(), timeout)
		defer cancel()

		var err error
		healthResults, err = b.healthChecker.CheckAllNodes(ctx)
		if err != nil {
			b.logger.Error("failed to check node health", zap.Error(err))
			return nil, fmt.Errorf("health check failed: %w", err)
		}
	}

	// Detect if this is a WebSocket upgrade request
	isWebSocketRequest := b.isWebSocketUpgradeRequest(r)

	var upstreams []*reverseproxy.Upstream
	healthyCount := 0
	type selectionInfo struct {
		name        string
		serviceType string
		reason      string
	}
	var selectedInfos []selectionInfo

	for _, health := range healthResults {
		if health.Healthy {
			// Find the corresponding node config for weight and service type
			weight := 1
			var nodeConfig *NodeConfig
			for _, node := range b.config.Nodes {
				if node.Name == health.Name {
					weight = node.Weight
					nodeConfig = &node
					break
				}
			}

			// Filter nodes based on request type
			if nodeConfig != nil {
				serviceType := nodeConfig.Metadata["service_type"]

				// For WebSocket requests, only include WebSocket nodes
				if isWebSocketRequest {
					if serviceType != "websocket" {
						b.logger.Debug("Skipping non-WebSocket node for WebSocket request",
							zap.String("node", health.Name),
							zap.String("service_type", serviceType))
						if b.metrics != nil {
							b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "filtered_websocket").Inc()
						}
						continue
					}
				} else {
					// For HTTP requests, include RPC, API, and nodes without service_type (backward compatibility)
					// but exclude WebSocket-only nodes
					if serviceType == "websocket" {
						b.logger.Debug("Skipping WebSocket node for HTTP request",
							zap.String("node", health.Name),
							zap.String("service_type", serviceType))
						if b.metrics != nil {
							b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "filtered_http").Inc()
						}
						continue
					}
					// Allow: "rpc", "api", "evm", "", or any other non-websocket service type
				}
			}

			healthyCount++

			// Determine the correct URL to use for upstream
			upstreamURL := health.URL

			// For WebSocket nodes, use the actual WebSocket URL for proxy
			if nodeConfig != nil && nodeConfig.Metadata["service_type"] == "websocket" {
				// health.URL should already be the WebSocket URL for WebSocket nodes
				b.logger.Debug("Using WebSocket URL for upstream",
					zap.String("node", health.Name),
					zap.String("websocket_url", upstreamURL))
			}

			// Parse URL for upstream
			parsedURL, err := url.Parse(upstreamURL)
			if err != nil {
				b.logger.Warn("invalid node URL", zap.String("node", health.Name), zap.String("url", upstreamURL))
				if b.metrics != nil {
					serviceType := ""
					if nodeConfig != nil {
						serviceType = nodeConfig.Metadata["service_type"]
					}
					b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "invalid_url").Inc()
				}
				continue
			}
			if parsedURL.Host == "" {
				b.logger.Warn("parsed URL has empty host; skipping upstream", zap.String("node", health.Name), zap.String("url", upstreamURL))
				if b.metrics != nil {
					serviceType := ""
					if nodeConfig != nil {
						serviceType = nodeConfig.Metadata["service_type"]
					}
					b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "empty_host").Inc()
				}
				continue
			}

			upstream := &reverseproxy.Upstream{
				Dial: parsedURL.Host,
			}

			// Add weight if specified
			if weight > 1 {
				upstream.MaxRequests = weight
			}

			upstreams = append(upstreams, upstream)
			if nodeConfig != nil {
				selectedInfos = append(selectedInfos, selectionInfo{
					name:        health.Name,
					serviceType: nodeConfig.Metadata["service_type"],
					reason:      "healthy",
				})
			} else {
				selectedInfos = append(selectedInfos, selectionInfo{
					name:        health.Name,
					serviceType: "",
					reason:      "healthy",
				})
			}
		} else {
			// Count exclusion for unhealthy node
			if b.metrics != nil {
				// Look up service type if available
				st := ""
				for _, node := range b.config.Nodes {
					if node.Name == health.Name {
						st = node.Metadata["service_type"]
						break
					}
				}
				b.metrics.upstreamsExcluded.WithLabelValues(health.Name, st, "unhealthy").Inc()
			}
		}
	}

	// Check minimum healthy nodes requirement
	if healthyCount < b.config.FailureHandling.MinHealthyNodes {
		b.logger.Warn("insufficient healthy nodes",
			zap.Int("healthy", healthyCount),
			zap.Int("minimum_required", b.config.FailureHandling.MinHealthyNodes))

		// Only fallback to unhealthy nodes if we have NO healthy nodes at all
		if healthyCount == 0 {
			b.logger.Info("no healthy nodes available, falling back to all nodes",
				zap.Int("total_nodes", len(healthResults)),
				zap.Int("healthy_nodes", healthyCount))

			// Return all nodes (including unhealthy ones) as last resort
			upstreams = []*reverseproxy.Upstream{}
			selectedInfos = selectedInfos[:0]
			for _, health := range healthResults {
				// Find the corresponding node config for weight
				weight := 1
				serviceType := ""
				for _, node := range b.config.Nodes {
					if node.Name == health.Name {
						weight = node.Weight
						serviceType = node.Metadata["service_type"]
						break
					}
				}

				// Parse URL for upstream
				parsedURL, err := url.Parse(health.URL)
				if err != nil {
					b.logger.Warn("invalid node URL", zap.String("node", health.Name), zap.String("url", health.URL))
					if b.metrics != nil {
						b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "invalid_url").Inc()
					}
					continue
				}
				if parsedURL.Host == "" {
					b.logger.Warn("parsed URL has empty host; skipping fallback upstream", zap.String("node", health.Name), zap.String("url", health.URL))
					if b.metrics != nil {
						b.metrics.upstreamsExcluded.WithLabelValues(health.Name, serviceType, "empty_host").Inc()
					}
					continue
				}

				upstream := &reverseproxy.Upstream{
					Dial: parsedURL.Host,
				}

				// Add weight if specified
				if weight > 1 {
					upstream.MaxRequests = weight
				}

				upstreams = append(upstreams, upstream)
				selectedInfos = append(selectedInfos, selectionInfo{
					name:        health.Name,
					serviceType: serviceType,
					reason:      "fallback_all",
				})
			}
		} else {
			// We have some healthy nodes, just log the warning but keep using only healthy nodes
			b.logger.Info("using available healthy nodes despite insufficient count",
				zap.Int("healthy_nodes", healthyCount),
				zap.Int("minimum_required", b.config.FailureHandling.MinHealthyNodes))
		}
	}

	b.logger.Debug("upstreams selected",
		zap.Int("total_nodes", len(b.config.Nodes)),
		zap.Int("healthy_nodes", healthyCount),
		zap.Int("selected_upstreams", len(upstreams)))

	// Never return an empty upstream list; signal error so caller can 502 gracefully
	if len(upstreams) == 0 {
		return nil, fmt.Errorf("no available upstreams selected")
	}

	// Emit metrics for selected upstreams
	if b.metrics != nil {
		for _, sel := range selectedInfos {
			b.metrics.upstreamsIncluded.WithLabelValues(sel.name, sel.serviceType, sel.reason).Inc()
		}
	}

	return upstreams, nil
}

// getCachedHealthResults retrieves cached health results for all nodes
// Returns results only if ALL nodes have cached results, otherwise returns empty slice
func (b *BlockchainHealthUpstream) getCachedHealthResults() []*NodeHealth {
	if b.healthChecker == nil || b.config == nil {
		return nil
	}

	var results []*NodeHealth
	for _, node := range b.config.Nodes {
		cached := b.cache.Get(node.Name)
		if cached == nil {
			// If any node doesn't have cached results, return empty slice
			// This forces a full health check to ensure consistency
			b.logger.Debug("incomplete cached health results, forcing full health check",
				zap.String("missing_node", node.Name),
				zap.Int("total_nodes", len(b.config.Nodes)),
				zap.Int("cached_results", len(results)))
			return nil
		}
		results = append(results, cached)
	}

	b.logger.Debug("retrieved complete cached health results",
		zap.Int("total_nodes", len(b.config.Nodes)),
		zap.Int("cached_results", len(results)))

	return results
}

// isWebSocketUpgradeRequest detects if the incoming request is a WebSocket upgrade request
func (b *BlockchainHealthUpstream) isWebSocketUpgradeRequest(r *http.Request) bool {
	// Check for WebSocket upgrade headers
	connection := r.Header.Get("Connection")
	upgrade := r.Header.Get("Upgrade")

	// WebSocket upgrade requires both headers
	isUpgrade := false
	if connection != "" {
		// Connection header can contain multiple values, check if "upgrade" is one of them
		for _, conn := range strings.Split(strings.ToLower(connection), ",") {
			if strings.TrimSpace(conn) == "upgrade" {
				isUpgrade = true
				break
			}
		}
	}

	isWebSocket := strings.ToLower(strings.TrimSpace(upgrade)) == "websocket"

	result := isUpgrade && isWebSocket

	b.logger.Debug("WebSocket upgrade detection",
		zap.Bool("is_websocket_request", result),
		zap.String("connection", connection),
		zap.String("upgrade", upgrade))

	return result
}

// provision sets up the module after configuration parsing
func (b *BlockchainHealthUpstream) provision(ctx caddy.Context) error {
	// Set up logger
	b.logger = ctx.Logger()

	// If an existing config is already present (e.g., tests), preserve its nodes
	if b.config != nil {
		if len(b.Nodes) == 0 && len(b.config.Nodes) > 0 {
			b.Nodes = b.config.Nodes
		}
		if len(b.ExternalReferences) == 0 && len(b.config.ExternalReferences) > 0 {
			b.ExternalReferences = b.config.ExternalReferences
		}
	}

	// Convert parsed config to internal config structure (or refresh from current fields)
	b.config = &Config{
		Nodes:              b.Nodes,
		ExternalReferences: b.ExternalReferences,
		Environment:        b.Environment,
		Chain:              b.Chain,
		Legacy:             b.Legacy,
		HealthCheck:        b.HealthCheck,
		BlockValidation:    b.BlockValidation,
		Performance:        b.Performance,
		FailureHandling:    b.FailureHandling,
		Monitoring:         b.Monitoring,
	}

	// Process environment-based configuration before setting defaults
	if err := b.processEnvironmentConfiguration(); err != nil {
		if b.Legacy.FallbackBehavior == "fail_startup" {
			return fmt.Errorf("environment configuration failed: %w", err)
		}
		b.logger.Warn("environment configuration failed, disabling health checks", zap.Error(err))
		b.Legacy.LegacyMode = true
	}

	// Update config with processed nodes
	b.config.Nodes = b.Nodes
	b.config.ExternalReferences = b.ExternalReferences

	// Set default values
	if err := b.setDefaults(); err != nil {
		return fmt.Errorf("failed to set defaults: %w", err)
	}

	// Initialize cache
	cacheDuration, err := time.ParseDuration(b.config.Performance.CacheDuration)
	if err != nil {
		return fmt.Errorf("invalid cache duration: %w", err)
	}
	b.cache = NewHealthCache(cacheDuration)

	// Initialize metrics if enabled
	b.metrics = NewMetrics()
	if err := b.metrics.Register(); err != nil {
		return fmt.Errorf("failed to register metrics: %w", err)
	}
	// Set configured nodes gauge
	b.metrics.configuredNodes.Set(float64(len(b.config.Nodes)))

	// Initialize health checker
	b.healthChecker = NewHealthChecker(b.config, b.cache, b.metrics, b.logger)

	// Log configuration details for debugging
	b.logger.Info("blockchain health configuration",
		zap.String("log_level", b.Monitoring.LogLevel),
		zap.String("timeout", b.HealthCheck.Timeout),
		zap.String("check_interval", b.HealthCheck.Interval),
		zap.Int("min_healthy_nodes", b.FailureHandling.MinHealthyNodes))

	// Start background health checking
	b.shutdown = make(chan struct{})
	go b.backgroundHealthCheck()

	b.logger.Info("blockchain health upstream provisioned",
		zap.Int("nodes", len(b.config.Nodes)),
		zap.Int("external_references", len(b.config.ExternalReferences)))

	return nil
}

// validate ensures the configuration is valid
func (b *BlockchainHealthUpstream) validate() error {
	// Temporarily process environment configuration for validation
	// This is safe because it doesn't modify persistent state
	tempNodes := make([]NodeConfig, len(b.Nodes))
	copy(tempNodes, b.Nodes)

	// Process environment configuration to generate nodes for validation
	if err := b.processEnvironmentConfiguration(); err != nil {
		// If environment processing fails, only fail if no nodes are manually configured
		if len(b.Nodes) == 0 {
			return fmt.Errorf("no nodes configured and environment configuration failed: %w", err)
		}
	}

	// Now validate that we have at least one node
	if len(b.Nodes) == 0 {
		return fmt.Errorf("at least one node must be configured (either manually or via environment variables)")
	}

	// Restore original nodes for actual provisioning later
	defer func() {
		b.Nodes = tempNodes
	}()

	// Validate node configurations
	for i, node := range b.Nodes {
		if node.Name == "" {
			return fmt.Errorf("node %d: name is required", i)
		}
		if node.URL == "" {
			return fmt.Errorf("node %s: URL is required", node.Name)
		}
		if node.Type != NodeTypeCosmos && node.Type != NodeTypeEVM && node.Type != NodeTypeBeacon {
			return fmt.Errorf("node %s: invalid type %s", node.Name, node.Type)
		}
		if node.Weight <= 0 {
			return fmt.Errorf("node %s: weight must be positive", node.Name)
		}

		// Validate URL format
		if _, err := url.Parse(node.URL); err != nil {
			return fmt.Errorf("node %s: invalid URL: %w", node.Name, err)
		}

		// Validate API URL if provided
		if node.APIURL != "" {
			if _, err := url.Parse(node.APIURL); err != nil {
				return fmt.Errorf("node %s: invalid API URL: %w", node.Name, err)
			}
		}
	}

	// Validate external references
	for i, ref := range b.ExternalReferences {
		if ref.Name == "" {
			return fmt.Errorf("external reference %d: name is required", i)
		}
		if ref.URL == "" {
			return fmt.Errorf("external reference %s: URL is required", ref.Name)
		}
		if ref.Type != NodeTypeCosmos && ref.Type != NodeTypeEVM && ref.Type != NodeTypeBeacon {
			return fmt.Errorf("external reference %s: invalid type %s", ref.Name, ref.Type)
		}

		// Validate URL format
		if _, err := url.Parse(ref.URL); err != nil {
			return fmt.Errorf("external reference %s: invalid URL: %w", ref.Name, err)
		}
	}

	// Validate timing configurations
	if b.HealthCheck.Interval != "" {
		if _, err := time.ParseDuration(b.HealthCheck.Interval); err != nil {
			return fmt.Errorf("invalid check interval: %w", err)
		}
	}
	if b.HealthCheck.Timeout != "" {
		if _, err := time.ParseDuration(b.HealthCheck.Timeout); err != nil {
			return fmt.Errorf("invalid timeout: %w", err)
		}
	}
	if b.HealthCheck.RetryDelay != "" {
		if _, err := time.ParseDuration(b.HealthCheck.RetryDelay); err != nil {
			return fmt.Errorf("invalid retry delay: %w", err)
		}
	}
	if b.Performance.CacheDuration != "" {
		if _, err := time.ParseDuration(b.Performance.CacheDuration); err != nil {
			return fmt.Errorf("invalid cache duration: %w", err)
		}
	}
	if b.FailureHandling.GracePeriod != "" {
		if _, err := time.ParseDuration(b.FailureHandling.GracePeriod); err != nil {
			return fmt.Errorf("invalid grace period: %w", err)
		}
	}

	// Validate thresholds
	if b.FailureHandling.CircuitBreakerThreshold != 0 && (b.FailureHandling.CircuitBreakerThreshold <= 0 || b.FailureHandling.CircuitBreakerThreshold > 1) {
		return fmt.Errorf("circuit breaker threshold must be between 0 and 1")
	}

	return nil
}

// cleanup stops background processes and cleans up resources
func (b *BlockchainHealthUpstream) cleanup() error {
	if b.shutdown != nil {
		close(b.shutdown)
	}

	if b.metrics != nil {
		b.metrics.Unregister()
	}

	b.logger.Info("blockchain health upstream cleaned up")
	return nil
}

// setDefaults sets default values for configuration fields
func (b *BlockchainHealthUpstream) setDefaults() error {
	// Health check defaults
	if b.config.HealthCheck.Interval == "" {
		b.config.HealthCheck.Interval = "15s"
	}
	if b.config.HealthCheck.Timeout == "" {
		b.config.HealthCheck.Timeout = "5s"
	}
	if b.config.HealthCheck.RetryAttempts == 0 {
		b.config.HealthCheck.RetryAttempts = 3
	}
	if b.config.HealthCheck.RetryDelay == "" {
		b.config.HealthCheck.RetryDelay = "1s"
	}

	// Block validation defaults
	if b.config.BlockValidation.HeightThreshold == 0 {
		b.config.BlockValidation.HeightThreshold = 5
	}
	if b.config.BlockValidation.ExternalReferenceThreshold == 0 {
		b.config.BlockValidation.ExternalReferenceThreshold = 10
	}

	// Performance defaults
	if b.config.Performance.CacheDuration == "" {
		b.config.Performance.CacheDuration = "30s"
	}
	if b.config.Performance.MaxConcurrentChecks == 0 {
		b.config.Performance.MaxConcurrentChecks = 10
	}

	// Failure handling defaults
	if b.config.FailureHandling.MinHealthyNodes == 0 {
		b.config.FailureHandling.MinHealthyNodes = 1
	}
	if b.config.FailureHandling.GracePeriod == "" {
		b.config.FailureHandling.GracePeriod = "60s"
	}
	if b.config.FailureHandling.CircuitBreakerThreshold == 0 {
		b.config.FailureHandling.CircuitBreakerThreshold = 0.8
	}

	// Monitoring defaults
	if b.config.Monitoring.LogLevel == "" {
		b.config.Monitoring.LogLevel = "info"
	}
	if b.config.Monitoring.HealthEndpoint == "" {
		b.config.Monitoring.HealthEndpoint = "/health"
	}

	// Set default weights for nodes
	for i := range b.config.Nodes {
		if b.config.Nodes[i].Weight == 0 {
			b.config.Nodes[i].Weight = 100
		}
	}

	return nil
}

// backgroundHealthCheck runs periodic health checks in the background
func (b *BlockchainHealthUpstream) backgroundHealthCheck() {
	interval, _ := time.ParseDuration(b.config.HealthCheck.Interval)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			_, err := b.healthChecker.CheckAllNodes(ctx)
			if err != nil {
				b.logger.Error("background health check failed", zap.Error(err))
			}
			cancel()

		case <-b.shutdown:
			b.logger.Debug("stopping background health checker")
			return
		}
	}
}
