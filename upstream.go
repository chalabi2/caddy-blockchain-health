package blockchain_health

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"go.uber.org/zap"
)

// GetUpstreams implements reverseproxy.UpstreamSource
func (b *BlockchainHealthUpstream) GetUpstreams(r *http.Request) ([]*reverseproxy.Upstream, error) {
	b.mutex.RLock()
	defer b.mutex.RUnlock()

	// Get current health status for all nodes
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	healthResults, err := b.healthChecker.CheckAllNodes(ctx)
	if err != nil {
		b.logger.Error("failed to check node health", zap.Error(err))
		return nil, fmt.Errorf("health check failed: %w", err)
	}

	var upstreams []*reverseproxy.Upstream
	healthyCount := 0

	for _, health := range healthResults {
		if health.Healthy {
			healthyCount++

			// Find the corresponding node config for weight
			var weight int = 1
			for _, node := range b.config.Nodes {
				if node.Name == health.Name {
					weight = node.Weight
					break
				}
			}

			// Parse URL for upstream
			parsedURL, err := url.Parse(health.URL)
			if err != nil {
				b.logger.Warn("invalid node URL", zap.String("node", health.Name), zap.String("url", health.URL))
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
		}
	}

	// Check minimum healthy nodes requirement
	if healthyCount < b.config.FailureHandling.MinHealthyNodes {
		b.logger.Warn("insufficient healthy nodes",
			zap.Int("healthy", healthyCount),
			zap.Int("minimum_required", b.config.FailureHandling.MinHealthyNodes))

		// In this case, we might want to return all nodes or fallback nodes
		// For now, we'll return what we have but log the issue
	}

	b.logger.Debug("upstreams selected",
		zap.Int("total_nodes", len(b.config.Nodes)),
		zap.Int("healthy_nodes", healthyCount),
		zap.Int("selected_upstreams", len(upstreams)))

	return upstreams, nil
}

// provision sets up the module after configuration parsing
func (b *BlockchainHealthUpstream) provision(ctx caddy.Context) error {
	// Set up logger
	b.logger = ctx.Logger()

	// Convert parsed config to internal config structure
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
	if b.config.Monitoring.MetricsEnabled {
		b.metrics = NewMetrics()
		if err := b.metrics.Register(); err != nil {
			return fmt.Errorf("failed to register metrics: %w", err)
		}
	}

	// Initialize health checker
	b.healthChecker = NewHealthChecker(b.config, b.cache, b.metrics, b.logger)

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
		if node.Type != NodeTypeCosmos && node.Type != NodeTypeEVM {
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
		if ref.Type != NodeTypeCosmos && ref.Type != NodeTypeEVM {
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
