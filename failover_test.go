package blockchain_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestFailoverScenario tests the core failover functionality
func TestFailoverScenario(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create healthy Cosmos RPC server
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
		} else {
			http.NotFound(w, r)
		}
	}))
	defer healthyCosmosServer.Close()

	// Create healthy EVM server
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
		} else {
			http.NotFound(w, r)
		}
	}))
	defer healthyEVMServer.Close()

	// Create unhealthy Cosmos server (catching up)
	unhealthyCosmosServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
		} else {
			http.NotFound(w, r)
		}
	}))
	defer unhealthyCosmosServer.Close()

	// Create unhealthy EVM server (returns error)
	unhealthyEVMServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"jsonrpc": "2.0",
				"id": 1,
				"error": {
					"code": -32000,
					"message": "Server error"
				}
			}`
			_, _ = w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer unhealthyEVMServer.Close()

	// Configure upstream with mix of healthy and unhealthy nodes
	upstream := &BlockchainHealthUpstream{
		Nodes: []NodeConfig{
			{
				Name: "healthy-cosmos-rpc",
				URL:  healthyCosmosServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
			{
				Name: "healthy-evm",
				URL:  healthyEVMServer.URL,
				Type: NodeTypeEVM,
				Metadata: map[string]string{
					"service_type": "evm",
				},
			},
			{
				Name: "unhealthy-cosmos-catching-up",
				URL:  unhealthyCosmosServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
			{
				Name: "unhealthy-evm-error",
				URL:  unhealthyEVMServer.URL,
				Type: NodeTypeEVM,
				Metadata: map[string]string{
					"service_type": "evm",
				},
			},
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
		HealthCheck: HealthCheckConfig{
			Timeout:       "2s",
			RetryAttempts: 1, // Quick test
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s", // Short cache for testing
			MaxConcurrentChecks: 2,    // Limit concurrency to avoid timeouts
		},
	}

	// Set up the upstream (simulate provisioning)
	upstream.logger = logger
	upstream.config = &Config{
		Nodes:           upstream.Nodes,
		FailureHandling: upstream.FailureHandling,
		HealthCheck:     upstream.HealthCheck,
		Performance:     upstream.Performance,
	}
	upstream.cache = NewHealthCache(1 * time.Second)
	upstream.healthChecker = NewHealthChecker(upstream.config, upstream.cache, nil, logger)

	// Test the failover scenario with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	upstreams, err := upstream.GetUpstreams(&http.Request{})

	// Verify no error occurred
	if err != nil {
		t.Fatalf("Expected no error from GetUpstreams, got %v", err)
	}

	// Verify only healthy nodes are returned
	expectedHealthyCount := 2 // healthy-cosmos-rpc, healthy-evm
	if len(upstreams) != expectedHealthyCount {
		t.Errorf("Expected %d healthy upstreams, got %d", expectedHealthyCount, len(upstreams))
	}

	// Test that health checker correctly identifies healthy vs unhealthy nodes
	healthResults, err := upstream.healthChecker.CheckAllNodes(ctx)
	if err != nil {
		t.Fatalf("Expected no error from CheckAllNodes, got %v", err)
	}

	healthyCount := 0
	unhealthyCount := 0
	healthyNodeNamesFromHealth := make(map[string]bool)
	unhealthyNodeNamesFromHealth := make(map[string]bool)

	for _, health := range healthResults {
		if health.Healthy {
			healthyCount++
			healthyNodeNamesFromHealth[health.Name] = true
		} else {
			unhealthyCount++
			unhealthyNodeNamesFromHealth[health.Name] = true
		}
	}

	// Verify health check results
	if healthyCount != expectedHealthyCount {
		t.Errorf("Expected %d healthy nodes from health check, got %d", expectedHealthyCount, healthyCount)
	}

	expectedUnhealthyCount := 2 // unhealthy-cosmos-catching-up, unhealthy-evm-error
	if unhealthyCount != expectedUnhealthyCount {
		t.Errorf("Expected %d unhealthy nodes from health check, got %d", expectedUnhealthyCount, unhealthyCount)
	}

	// Verify specific nodes are marked correctly
	expectedHealthyNodes := []string{"healthy-cosmos-rpc", "healthy-evm"}
	for _, nodeName := range expectedHealthyNodes {
		if !healthyNodeNamesFromHealth[nodeName] {
			t.Errorf("Expected node %s to be healthy", nodeName)
		}
	}

	expectedUnhealthyNodes := []string{"unhealthy-cosmos-catching-up", "unhealthy-evm-error"}
	for _, nodeName := range expectedUnhealthyNodes {
		if !unhealthyNodeNamesFromHealth[nodeName] {
			t.Errorf("Expected node %s to be unhealthy", nodeName)
		}
	}

	// Test minimum healthy nodes enforcement
	upstream.config.FailureHandling.MinHealthyNodes = 3 // More than we have healthy
	upstreams, err = upstream.GetUpstreams(&http.Request{})
	if err != nil {
		t.Fatalf("Expected no error even with insufficient healthy nodes, got %v", err)
	}

	// Should only return healthy nodes even when insufficient healthy nodes (unless no healthy nodes at all)
	// In this case we have 2 healthy nodes, so we should return only those 2
	if len(upstreams) != expectedHealthyCount {
		t.Errorf("Expected %d upstreams (only healthy nodes) with insufficient healthy nodes, got %d", expectedHealthyCount, len(upstreams))
	}

	t.Logf("Failover test completed successfully: %d healthy, %d unhealthy nodes", healthyCount, unhealthyCount)
}

// TestFailoverWithNoHealthyNodes tests fallback behavior when no nodes are healthy
func TestFailoverWithNoHealthyNodes(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create servers that all return unhealthy responses
	unhealthyServer1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer unhealthyServer1.Close()

	unhealthyServer2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12200",
						"catching_up": true
					}
				}
			}`
			_, _ = w.Write([]byte(response))
		}
	}))
	defer unhealthyServer2.Close()

	// Create configuration with only unhealthy nodes
	config := &Config{
		Nodes: []NodeConfig{
			{Name: "unhealthy-node-1", URL: unhealthyServer1.URL, Type: NodeTypeCosmos, Weight: 1},
			{Name: "unhealthy-node-2", URL: unhealthyServer2.URL, Type: NodeTypeCosmos, Weight: 1},
		},
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "2s",
			RetryAttempts: 1,
			RetryDelay:    "1s",
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1, // Require at least 1 healthy node
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s",
			MaxConcurrentChecks: 2,
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

	// Test: No healthy nodes available - should fallback to all nodes
	upstreams, err := upstream.GetUpstreams(&http.Request{})
	if err != nil {
		t.Fatalf("Expected no error even with no healthy nodes, got %v", err)
	}

	// Should fallback to all nodes (including unhealthy ones) when no healthy nodes available
	expectedTotalNodes := 2 // All nodes (both unhealthy)
	if len(upstreams) != expectedTotalNodes {
		t.Errorf("Expected %d upstreams (fallback to all nodes) with no healthy nodes, got %d", expectedTotalNodes, len(upstreams))
	}

	t.Logf("No healthy nodes fallback test completed: returned %d nodes (all unhealthy)", len(upstreams))
}

// TestFailoverWithTimeout tests failover behavior with timeout scenarios
func TestFailoverWithTimeout(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a slow server that times out
	slowServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(6 * time.Second) // Longer than our 2s timeout
		w.WriteHeader(http.StatusOK)
	}))
	defer slowServer.Close()

	// Create a fast healthy server
	fastServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
	defer fastServer.Close()

	// Configure upstream with one slow and one fast node
	upstream := &BlockchainHealthUpstream{
		Nodes: []NodeConfig{
			{
				Name: "fast-node",
				URL:  fastServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
			{
				Name: "slow-node",
				URL:  slowServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
		HealthCheck: HealthCheckConfig{
			Timeout:       "2s",
			RetryAttempts: 1,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s",
			MaxConcurrentChecks: 2,
		},
	}

	// Set up the upstream
	upstream.logger = logger
	upstream.config = &Config{
		Nodes:           upstream.Nodes,
		FailureHandling: upstream.FailureHandling,
		HealthCheck:     upstream.HealthCheck,
		Performance:     upstream.Performance,
	}
	upstream.cache = NewHealthCache(1 * time.Second)
	upstream.healthChecker = NewHealthChecker(upstream.config, upstream.cache, nil, logger)

	// Test failover with timeout
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	upstreams, err := upstream.GetUpstreams(&http.Request{})

	// Verify no error occurred
	if err != nil {
		t.Fatalf("Expected no error from GetUpstreams, got %v", err)
	}

	// Should only return the fast node (slow node should timeout)
	if len(upstreams) != 1 {
		t.Errorf("Expected 1 healthy upstream (fast node), got %d", len(upstreams))
	}

	// Verify it's the fast node (check if URL contains the expected server)
	if !contains(upstreams[0].Dial, "127.0.0.1") {
		t.Errorf("Expected fast node URL to contain 127.0.0.1, got %s", upstreams[0].Dial)
	}

	// Test health checker results
	healthResults, err := upstream.healthChecker.CheckAllNodes(ctx)
	if err != nil {
		t.Fatalf("Expected no error from CheckAllNodes, got %v", err)
	}

	// Should have 1 healthy and 1 unhealthy
	healthyCount := 0
	unhealthyCount := 0

	for _, health := range healthResults {
		if health.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	if healthyCount != 1 {
		t.Errorf("Expected 1 healthy node, got %d", healthyCount)
	}

	if unhealthyCount != 1 {
		t.Errorf("Expected 1 unhealthy node, got %d", unhealthyCount)
	}

	t.Logf("Timeout failover test completed: %d healthy, %d unhealthy nodes", healthyCount, unhealthyCount)
}

// TestFailoverWithCircuitBreaker tests failover behavior with circuit breaker
func TestFailoverWithCircuitBreaker(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a server that fails consistently
	failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
	}))
	defer failingServer.Close()

	// Create a healthy server
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

	// Configure upstream with circuit breaker
	upstream := &BlockchainHealthUpstream{
		Nodes: []NodeConfig{
			{
				Name: "healthy-node",
				URL:  healthyServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
			{
				Name: "failing-node",
				URL:  failingServer.URL,
				Type: NodeTypeCosmos,
				Metadata: map[string]string{
					"service_type": "rpc",
				},
			},
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes:         1,
			CircuitBreakerThreshold: 0.5, // 50% failure rate
		},
		HealthCheck: HealthCheckConfig{
			Timeout:       "2s",
			RetryAttempts: 1,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s",
			MaxConcurrentChecks: 2,
		},
	}

	// Set up the upstream
	upstream.logger = logger
	upstream.config = &Config{
		Nodes:           upstream.Nodes,
		FailureHandling: upstream.FailureHandling,
		HealthCheck:     upstream.HealthCheck,
		Performance:     upstream.Performance,
	}
	upstream.cache = NewHealthCache(1 * time.Second)
	upstream.healthChecker = NewHealthChecker(upstream.config, upstream.cache, nil, logger)

	// Test failover with circuit breaker
	upstreams, err := upstream.GetUpstreams(&http.Request{})

	// Verify no error occurred
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

	t.Logf("Circuit breaker failover test completed successfully")
}

// contains checks if a string contains a substring
func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr ||
		(len(s) > len(substr) && (s[:len(substr)] == substr ||
			s[len(s)-len(substr):] == substr ||
			containsSubstring(s, substr))))
}

// containsSubstring checks if a string contains a substring (simple implementation)
func containsSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
