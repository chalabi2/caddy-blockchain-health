package blockchain_health

import (
	"context"
	"fmt"
	"sync"
	"time"

	"go.uber.org/zap"
)

// NewHealthChecker creates a new health checker instance
func NewHealthChecker(config *Config, cache *HealthCache, metrics *Metrics, logger *zap.Logger) *HealthChecker {
	timeout, err := time.ParseDuration(config.HealthCheck.Timeout)
	if err != nil || timeout == 0 {
		// Default to 10 seconds if no timeout specified or invalid
		timeout = 10 * time.Second
		logger.Debug("using default timeout", zap.Duration("timeout", timeout))
	} else {
		logger.Debug("using configured timeout", zap.Duration("timeout", timeout))
	}

	return &HealthChecker{
		config:          config,
		cosmosHandler:   NewCosmosHandler(timeout, logger),
		evmHandler:      NewEVMHandler(timeout, logger),
		beaconHandler:   NewBeaconHandler(timeout, logger),
		cache:           cache,
		metrics:         metrics,
		logger:          logger,
		circuitBreakers: make(map[string]*CircuitBreaker),
	}
}

// CheckAllNodes performs health checks on all configured nodes
func (h *HealthChecker) CheckAllNodes(ctx context.Context) ([]*NodeHealth, error) {
	nodes := h.config.Nodes
	if len(nodes) == 0 {
		return nil, fmt.Errorf("no nodes configured")
	}

	h.logger.Debug("starting health checks for all nodes",
		zap.Int("total_nodes", len(nodes)))

	// Use semaphore pattern to limit concurrent checks
	sem := make(chan struct{}, h.config.Performance.MaxConcurrentChecks)
	var wg sync.WaitGroup
	results := make([]*NodeHealth, len(nodes))

	// Check each node concurrently with rate limiting
	for i, node := range nodes {
		wg.Add(1)
		go func(idx int, n NodeConfig) {
			defer wg.Done()

			// Acquire semaphore with context cancellation
			select {
			case sem <- struct{}{}:
				defer func() { <-sem }()
			case <-ctx.Done():
				// Context cancelled, return early
				results[idx] = &NodeHealth{
					Name:      n.Name,
					URL:       n.URL,
					Healthy:   false,
					LastError: ctx.Err().Error(),
				}
				return
			}

			h.logger.Debug("checking node health",
				zap.String("node", n.Name),
				zap.String("url", n.URL),
				zap.String("type", string(n.Type)))

			health := h.checkSingleNode(ctx, n)
			results[idx] = health

			h.logger.Debug("node health check completed",
				zap.String("node", n.Name),
				zap.Bool("healthy", health.Healthy),
				zap.String("error", health.LastError))
		}(i, node)
	}

	wg.Wait()

	h.logger.Debug("all health checks completed",
		zap.Int("total_nodes", len(nodes)),
		zap.Int("healthy_nodes", countHealthyNodes(results)))

	// Post-process: validate block heights and update metrics
	if err := h.validateBlockHeights(results); err != nil {
		h.logger.Warn("block height validation failed", zap.Error(err))
	}

	// Update metrics
	if h.metrics != nil {
		h.updateMetrics(results)
	}

	return results, nil
}

// countHealthyNodes counts the number of healthy nodes
func countHealthyNodes(results []*NodeHealth) int {
	count := 0
	for _, health := range results {
		if health.Healthy {
			count++
		}
	}
	return count
}

// checkSingleNode performs health check on a single node with caching and circuit breaker
func (h *HealthChecker) checkSingleNode(ctx context.Context, node NodeConfig) *NodeHealth {
	// Check cache first
	if cached := h.cache.Get(node.Name); cached != nil {
		h.logger.Debug("using cached health result", zap.String("node", node.Name))
		return cached
	}

	// Check circuit breaker
	breaker := h.getCircuitBreaker(node.Name)
	if !breaker.CanExecute() {
		h.logger.Debug("circuit breaker open", zap.String("node", node.Name))
		return &NodeHealth{
			Name:      node.Name,
			URL:       node.URL,
			Healthy:   false,
			LastCheck: time.Now(),
			LastError: "circuit breaker open",
		}
	}

	// Perform health check with retry
	health := h.checkWithRetry(ctx, node)

	// Update circuit breaker
	if health.Healthy {
		breaker.RecordSuccess()
	} else {
		breaker.RecordFailure()
	}

	// Cache the result
	h.cache.Set(node.Name, health)

	return health
}

// checkWithRetry performs health check with exponential backoff retry
func (h *HealthChecker) checkWithRetry(ctx context.Context, node NodeConfig) *NodeHealth {
	retryDelay, _ := time.ParseDuration(h.config.HealthCheck.RetryDelay)
	maxAttempts := h.config.HealthCheck.RetryAttempts

	var lastHealth *NodeHealth
	var lastErr error

	for attempt := 1; attempt <= maxAttempts; attempt++ {
		// Select appropriate handler based on node type
		var health *NodeHealth
		var err error

		switch node.Type {
		case NodeTypeCosmos:
			health, err = h.cosmosHandler.CheckHealth(ctx, node)
		case NodeTypeEVM:
			health, err = h.evmHandler.CheckHealth(ctx, node)
		case NodeTypeBeacon:
			health, err = h.beaconHandler.CheckHealth(ctx, node)
		default:
			return &NodeHealth{
				Name:      node.Name,
				URL:       node.URL,
				Healthy:   false,
				LastCheck: time.Now(),
				LastError: fmt.Sprintf("unsupported node type: %s", node.Type),
			}
		}

		if err != nil {
			lastErr = err
			h.logger.Debug("health check attempt failed",
				zap.String("node", node.Name),
				zap.Int("attempt", attempt),
				zap.Error(err))
		} else {
			lastHealth = health
			if health.Healthy {
				// Success, no need to retry
				break
			}
		}

		// Don't sleep after the last attempt
		if attempt < maxAttempts {
			select {
			case <-ctx.Done():
				// Context cancelled, stop retrying
				break
			case <-time.After(retryDelay):
				// Exponential backoff for next attempt
				retryDelay = time.Duration(float64(retryDelay) * 1.5)
			}
		}
	}

	// If we have a health result (even if unhealthy), use it
	if lastHealth != nil {
		return lastHealth
	}

	// If we never got a health result, create one with the last error
	return &NodeHealth{
		Name:      node.Name,
		URL:       node.URL,
		Healthy:   false,
		LastCheck: time.Now(),
		LastError: fmt.Sprintf("all attempts failed: %v", lastErr),
	}
}

// validateBlockHeights validates block heights within the pool and against external references
func (h *HealthChecker) validateBlockHeights(healthResults []*NodeHealth) error {
	if len(healthResults) == 0 {
		return nil
	}

	// Group nodes by chain type for validation (e.g., "ethereum", "base", "akash", "osmosis")
	chainGroups := make(map[string][]*NodeHealth)
	chainNodeTypes := make(map[string]NodeType) // Track the NodeType for each chain

	for _, health := range healthResults {
		if !health.Healthy {
			continue // Skip unhealthy nodes for validation
		}

		// Find the node config to get the chain type
		for _, node := range h.config.Nodes {
			if node.Name == health.Name {
				chainType := node.ChainType
				if chainType == "" {
					// Fallback to generic grouping if no chain type specified
					chainType = string(node.Type)
				}

				// Group nodes by their specific chain type
				if chainGroups[chainType] == nil {
					chainGroups[chainType] = make([]*NodeHealth, 0)
				}
				chainGroups[chainType] = append(chainGroups[chainType], health)
				chainNodeTypes[chainType] = node.Type // Remember the protocol type for this chain
				break
			}
		}
	}

	// Validate each chain group separately
	for chainType, nodes := range chainGroups {
		if len(nodes) > 0 {
			nodeType := chainNodeTypes[chainType]
			if err := h.validateNodeGroup(nodes, nodeType); err != nil {
				h.logger.Warn("chain node validation failed",
					zap.String("chain_type", chainType),
					zap.String("node_type", string(nodeType)),
					zap.Error(err))
			} else {
				h.logger.Debug("chain validation completed",
					zap.String("chain_type", chainType),
					zap.Int("node_count", len(nodes)))
			}
		}
	}

	return nil
}

// validateNodeGroup validates block heights within a group of nodes of the same type
func (h *HealthChecker) validateNodeGroup(nodes []*NodeHealth, nodeType NodeType) error {
	if len(nodes) <= 1 {
		return nil // Nothing to validate
	}

	// Find the highest block height in the group
	var maxHeight uint64
	for _, node := range nodes {
		if node.BlockHeight > maxHeight {
			maxHeight = node.BlockHeight
		}
	}

	// Check each node against the pool leader
	threshold := uint64(h.config.BlockValidation.HeightThreshold)
	for _, node := range nodes {
		blocksBehind := int64(maxHeight - node.BlockHeight)
		node.BlocksBehindPool = blocksBehind

		if blocksBehind > int64(threshold) {
			node.HeightValid = false
			node.Healthy = false // Mark as unhealthy if too far behind
			h.logger.Warn("node too far behind pool",
				zap.String("node", node.Name),
				zap.Uint64("node_height", node.BlockHeight),
				zap.Uint64("max_height", maxHeight),
				zap.Int64("blocks_behind", blocksBehind))
		} else {
			node.HeightValid = true
		}
	}

	// Validate against external references if configured
	for _, ref := range h.config.ExternalReferences {
		if ref.Type == nodeType && ref.Enabled {
			if err := h.validateAgainstExternal(nodes, ref); err != nil {
				h.logger.Warn("external reference validation failed",
					zap.String("reference", ref.Name),
					zap.Error(err))
			}
		}
	}

	return nil
}

// validateAgainstExternal validates nodes against an external reference
func (h *HealthChecker) validateAgainstExternal(nodes []*NodeHealth, ref ExternalReference) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	var externalHeight uint64
	var err error

	// Get external reference height
	switch ref.Type {
	case NodeTypeCosmos:
		externalHeight, err = h.cosmosHandler.GetBlockHeight(ctx, ref.URL)
	case NodeTypeEVM:
		externalHeight, err = h.evmHandler.GetBlockHeight(ctx, ref.URL)
	case NodeTypeBeacon:
		externalHeight, err = h.beaconHandler.GetBlockHeight(ctx, ref.URL)
	default:
		return fmt.Errorf("unsupported external reference type: %s", ref.Type)
	}

	if err != nil {
		return fmt.Errorf("failed to get external reference height: %w", err)
	}

	// Check each node against external reference
	threshold := uint64(h.config.BlockValidation.ExternalReferenceThreshold)
	for _, node := range nodes {
		blocksBehind := int64(externalHeight - node.BlockHeight)
		node.BlocksBehindExternal = blocksBehind

		if blocksBehind > int64(threshold) {
			node.ExternalReferenceValid = false
			h.logger.Warn("node too far behind external reference",
				zap.String("node", node.Name),
				zap.String("reference", ref.Name),
				zap.Uint64("node_height", node.BlockHeight),
				zap.Uint64("external_height", externalHeight),
				zap.Int64("blocks_behind", blocksBehind))
		} else {
			node.ExternalReferenceValid = true
		}
	}

	return nil
}

// getCircuitBreaker gets or creates a circuit breaker for a node
func (h *HealthChecker) getCircuitBreaker(nodeName string) *CircuitBreaker {
	h.mutex.RLock()
	breaker, exists := h.circuitBreakers[nodeName]
	h.mutex.RUnlock()

	if !exists {
		h.mutex.Lock()
		// Double-check after acquiring write lock
		if breaker, exists = h.circuitBreakers[nodeName]; !exists {
			breaker = NewCircuitBreaker(int(h.config.FailureHandling.CircuitBreakerThreshold * 10))
			h.circuitBreakers[nodeName] = breaker
		}
		h.mutex.Unlock()
	}

	return breaker
}

// updateMetrics updates prometheus metrics based on health check results
func (h *HealthChecker) updateMetrics(results []*NodeHealth) {
	var healthyCount, unhealthyCount int

	for _, health := range results {
		if health.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}

		// Update individual node metrics
		h.metrics.blockHeightGauge.WithLabelValues(health.Name).Set(float64(health.BlockHeight))

		if health.LastError != "" {
			h.metrics.errorCount.WithLabelValues(health.Name, "health_check").Inc()
		}
	}

	h.metrics.healthyNodes.Set(float64(healthyCount))
	h.metrics.unhealthyNodes.Set(float64(unhealthyCount))
	h.metrics.totalChecks.Inc()
}
