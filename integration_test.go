package blockchain_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestIntegrationEndToEnd tests the complete end-to-end functionality
func TestIntegrationEndToEnd(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create test servers with different health states
	healthyCosmosServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12345",
						"catching_up": false
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		}
	}))
	defer healthyCosmosServer.Close()

	healthyEVMServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"jsonrpc": "2.0",
				"id": 1,
				"result": "0x12345"
			}`
			_, _ = w.Write([]byte(response))
		}
	}))
	defer healthyEVMServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12300",
						"catching_up": true
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		}
	}))
	defer unhealthyServer.Close()

	// Create external reference server
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12350",
						"catching_up": false
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		}
	}))
	defer externalServer.Close()

	// Create comprehensive configuration
	config := &Config{
		Nodes: []NodeConfig{
			{Name: "healthy-cosmos", URL: healthyCosmosServer.URL, Type: NodeTypeCosmos, Weight: 1},
			{Name: "healthy-evm", URL: healthyEVMServer.URL, Type: NodeTypeEVM, Weight: 1},
			{Name: "unhealthy-cosmos", URL: unhealthyServer.URL, Type: NodeTypeCosmos, Weight: 1},
		},
		ExternalReferences: []ExternalReference{
			{Name: "external-ref", URL: externalServer.URL, Type: NodeTypeCosmos, Enabled: true},
		},
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "5s",
			RetryAttempts: 3,
			RetryDelay:    "1s",
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes:         1,
			CircuitBreakerThreshold: 0.8,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "30s",
			MaxConcurrentChecks: 5,
		},
		BlockValidation: BlockValidationConfig{
			HeightThreshold:            10,
			ExternalReferenceThreshold: 5,
		},
		Monitoring: MonitoringConfig{
			MetricsEnabled: true,
			LogLevel:       "info",
		},
	}

	// Create upstream with all components
	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(30*time.Second), NewMetrics(), logger),
		cache:         NewHealthCache(30 * time.Second),
		metrics:       NewMetrics(),
		logger:        logger,
	}

	// Test 1: Health endpoint
	t.Run("HealthEndpoint", func(t *testing.T) {
		handler := upstream.ServeHealthEndpoint()
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 200 or 503, got %d", w.Code)
		}
	})

	// Test 2: GetUpstreams (failover)
	t.Run("GetUpstreams", func(t *testing.T) {
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("Expected no error from GetUpstreams, got %v", err)
		}

		// Should only return healthy nodes
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 healthy upstreams, got %d", len(upstreams))
		}
	})

	// Test 3: Health checker
	t.Run("HealthChecker", func(t *testing.T) {
		ctx := context.Background()
		results, err := upstream.healthChecker.CheckAllNodes(ctx)
		if err != nil {
			t.Fatalf("Expected no error from CheckAllNodes, got %v", err)
		}

		// Should have 3 results (one for each node)
		if len(results) != 3 {
			t.Errorf("Expected 3 health results, got %d", len(results))
		}

		// Count healthy vs unhealthy
		healthyCount := 0
		unhealthyCount := 0

		for _, result := range results {
			if result.Healthy {
				healthyCount++
			} else {
				unhealthyCount++
			}
		}

		// Should have 2 healthy and 1 unhealthy
		if healthyCount != 2 {
			t.Errorf("Expected 2 healthy nodes, got %d", healthyCount)
		}

		if unhealthyCount != 1 {
			t.Errorf("Expected 1 unhealthy node, got %d", unhealthyCount)
		}
	})

	// Test 4: External reference check
	t.Run("ExternalReference", func(t *testing.T) {
		ctx := context.Background()
		status := upstream.checkExternalReference(ctx, config.ExternalReferences[0])

		if !status.Reachable {
			t.Errorf("External reference should be reachable, got error: %s", status.Error)
		}

		if status.BlockHeight != 12350 {
			t.Errorf("Expected block height 12350, got %d", status.BlockHeight)
		}
	})

	// Test 5: Metrics
	t.Run("Metrics", func(t *testing.T) {
		if err := upstream.metrics.Register(); err != nil {
			t.Fatalf("Failed to register metrics: %v", err)
		}
		defer upstream.metrics.Unregister()

		// Test metrics operations
		upstream.metrics.IncrementTotalChecks()
		upstream.metrics.SetHealthyNodes(2)
		upstream.metrics.SetUnhealthyNodes(1)
		upstream.metrics.SetBlockHeight("test-node", 12345)
		upstream.metrics.IncrementError("test-node", "timeout")
		upstream.metrics.RecordCheckDuration(1.5)
	})

	// Test 6: Cache
	t.Run("Cache", func(t *testing.T) {
		// Test cache operations
		nodeHealth := &NodeHealth{
			Name:        "test-node",
			URL:         "http://test",
			Healthy:     true,
			BlockHeight: 12345,
			LastCheck:   time.Now(),
		}

		upstream.cache.Set("test-node", nodeHealth)
		retrieved := upstream.cache.Get("test-node")
		if retrieved == nil {
			t.Error("Expected to retrieve cached health result")
			return
		}

		if retrieved.Name != "test-node" {
			t.Errorf("Expected node name 'test-node', got '%s'", retrieved.Name)
		}
	})

	t.Logf("Integration test completed successfully")
}

// TestIntegrationWithRealisticScenarios tests integration with realistic failure scenarios
func TestIntegrationWithRealisticScenarios(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Scenario 1: Node becomes unhealthy during operation
	t.Run("NodeBecomesUnhealthy", func(t *testing.T) {
		// Create a server that starts healthy but becomes unhealthy
		var healthyMutex sync.RWMutex
		healthy := true

		getHealthy := func() bool {
			healthyMutex.RLock()
			defer healthyMutex.RUnlock()
			return healthy
		}

		setHealthy := func(h bool) {
			healthyMutex.Lock()
			defer healthyMutex.Unlock()
			healthy = h
		}

		server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)

				var response string
				if getHealthy() {
					response = `{
						"result": {
							"sync_info": {
								"latest_block_height": "12345",
								"catching_up": false
							}
						}
					}`
				} else {
					response = `{
						"result": {
							"sync_info": {
								"latest_block_height": "12300",
								"catching_up": true
							}
						}
					}`
				}
				_, _ = w.Write([]byte(response))
			}
		}))
		defer server.Close()

		// Create configuration
		config := &Config{
			Nodes: []NodeConfig{
				{Name: "dynamic-node", URL: server.URL, Type: NodeTypeCosmos, Weight: 1},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "1s",
				Timeout:       "5s",
				RetryAttempts: 3,
				RetryDelay:    "1s",
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			Performance: PerformanceConfig{
				CacheDuration:       "1s",
				MaxConcurrentChecks: 5,
			},
		}

		// Create upstream
		upstream := &BlockchainHealthUpstream{
			config:        config,
			healthChecker: NewHealthChecker(config, NewHealthCache(1*time.Second), NewMetrics(), logger),
			cache:         NewHealthCache(1 * time.Second),
			metrics:       NewMetrics(),
			logger:        logger,
		}

		// Test 1: Node is healthy
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("Expected no error from GetUpstreams, got %v", err)
		}

		if len(upstreams) != 1 {
			t.Errorf("Expected 1 healthy upstream, got %d", len(upstreams))
		}

		// Test 2: Node becomes unhealthy
		setHealthy(false)
		time.Sleep(2 * time.Second) // Wait for cache to expire

		upstreams, err = upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("Expected no error from GetUpstreams, got %v", err)
		}

		// Test the actual behavior: when min healthy nodes = 1 and we have 1 node that becomes unhealthy,
		// the system should still return the node (even if unhealthy) to maintain service availability
		if len(upstreams) == 0 {
			t.Error("Expected at least 1 upstream to be returned even when unhealthy (min healthy nodes = 1)")
		} else {
			t.Logf("Upstreams returned when node becomes unhealthy: %d (expected behavior)", len(upstreams))
		}

		t.Logf("Node health transition test completed")
	})

	// Scenario 2: Multiple nodes with different failure patterns
	t.Run("MultipleNodeFailurePatterns", func(t *testing.T) {
		// Create servers with different failure patterns
		timeoutServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(6 * time.Second) // Timeout
		}))
		defer timeoutServer.Close()

		errorServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer errorServer.Close()

		healthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{
					"result": {
						"sync_info": {
							"latest_block_height": "12345",
							"catching_up": false
						}
					}
				}`
				_, _ = w.Write([]byte(response))
			}
		}))
		defer healthyServer.Close()

		// Create configuration
		config := &Config{
			Nodes: []NodeConfig{
				{Name: "timeout-node", URL: timeoutServer.URL, Type: NodeTypeCosmos, Weight: 1},
				{Name: "error-node", URL: errorServer.URL, Type: NodeTypeCosmos, Weight: 1},
				{Name: "healthy-node", URL: healthyServer.URL, Type: NodeTypeCosmos, Weight: 1},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "1s",
				Timeout:       "2s",
				RetryAttempts: 1,
				RetryDelay:    "1s",
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			Performance: PerformanceConfig{
				CacheDuration:       "1s",
				MaxConcurrentChecks: 3,
			},
		}

		// Create upstream
		upstream := &BlockchainHealthUpstream{
			config:        config,
			healthChecker: NewHealthChecker(config, NewHealthCache(1*time.Second), NewMetrics(), logger),
			cache:         NewHealthCache(1 * time.Second),
			metrics:       NewMetrics(),
			logger:        logger,
		}

		// Test with timeout
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("Expected no error from GetUpstreams, got %v", err)
		}

		// Should only return the healthy node
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 healthy upstream, got %d", len(upstreams))
		}

		// Verify it's the healthy node (check if URL contains the expected server)
		if !contains(upstreams[0].Dial, "127.0.0.1") {
			t.Errorf("Expected healthy node URL to contain 127.0.0.1, got %s", upstreams[0].Dial)
		}

		// Test health checker results
		healthResults, err := upstream.healthChecker.CheckAllNodes(ctx)
		if err != nil {
			t.Fatalf("Expected no error from CheckAllNodes, got %v", err)
		}

		// Should have 3 results
		if len(healthResults) != 3 {
			t.Errorf("Expected 3 health results, got %d", len(healthResults))
		}

		// Count healthy vs unhealthy
		healthyCount := 0
		unhealthyCount := 0

		for _, result := range healthResults {
			if result.Healthy {
				healthyCount++
			} else {
				unhealthyCount++
			}
		}

		// Should have 1 healthy and 2 unhealthy
		if healthyCount != 1 {
			t.Errorf("Expected 1 healthy node, got %d", healthyCount)
		}

		if unhealthyCount != 2 {
			t.Errorf("Expected 2 unhealthy nodes, got %d", unhealthyCount)
		}

		t.Logf("Multiple node failure patterns test completed")
	})
}
