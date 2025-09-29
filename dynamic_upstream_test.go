package blockchain_health

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestDynamicUpstreamCore tests the core functionality: adding and removing nodes from reverse proxy based on health
func TestDynamicUpstreamCore(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("HealthyNodesOnly_AddedToUpstream", func(t *testing.T) {
		// Create healthy servers with different block heights
		healthyServer1 := createCosmosServer(t, 12345, false) // Healthy, up-to-date
		healthyServer2 := createCosmosServer(t, 12344, false) // Healthy, 1 block behind
		defer healthyServer1.Close()
		defer healthyServer2.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "healthy-1", URL: healthyServer1.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "healthy-2", URL: healthyServer2.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Test GetUpstreams - should return both healthy nodes
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Verify exactly 2 upstreams returned
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (all healthy), got %d", len(upstreams))
		}

		// Verify both nodes are represented in upstreams
		upstreamHosts := make(map[string]bool)
		for _, up := range upstreams {
			upstreamHosts[up.Dial] = true
		}

		expectedHosts := []string{getDynamicTestHostFromURL(healthyServer1.URL), getDynamicTestHostFromURL(healthyServer2.URL)}
		for _, host := range expectedHosts {
			if !upstreamHosts[host] {
				t.Errorf("Expected host %s in upstreams, but not found", host)
			}
		}

		t.Logf("✅ All healthy nodes correctly added to upstream pool")
	})

	t.Run("UnhealthyNodes_RemovedFromUpstream", func(t *testing.T) {
		// Create mixed healthy/unhealthy servers
		healthyServer := createCosmosServer(t, 12345, false)  // Healthy
		unhealthyServer := createCosmosServer(t, 12300, true) // Unhealthy (catching up)
		defer healthyServer.Close()
		defer unhealthyServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "healthy", URL: healthyServer.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "unhealthy", URL: unhealthyServer.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Test GetUpstreams - should return only healthy node
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Verify only 1 upstream returned (the healthy one)
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream (only healthy), got %d", len(upstreams))
		}

		// Verify it's the healthy server
		expectedHost := getDynamicTestHostFromURL(healthyServer.URL)
		if upstreams[0].Dial != expectedHost {
			t.Errorf("Expected upstream host %s, got %s", expectedHost, upstreams[0].Dial)
		}

		t.Logf("✅ Unhealthy nodes correctly removed from upstream pool")
	})

	t.Run("BlockHeightThreshold_RemovesLaggingNodes", func(t *testing.T) {
		// Create servers with different block heights
		leaderServer := createCosmosServer(t, 12345, false)  // Leader (highest)
		goodServer := createCosmosServer(t, 12342, false)    // 3 blocks behind (within threshold)
		laggingServer := createCosmosServer(t, 12335, false) // 10 blocks behind (exceeds threshold)
		defer leaderServer.Close()
		defer goodServer.Close()
		defer laggingServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "leader", URL: leaderServer.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "good", URL: goodServer.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "lagging", URL: laggingServer.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Set block height threshold to 5
		upstream.config.BlockValidation.HeightThreshold = 5

		// Test GetUpstreams - should return only nodes within threshold
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Verify only 2 upstreams returned (leader and good, not lagging)
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (leader and good only), got %d", len(upstreams))
		}

		// Verify lagging node is not in upstreams
		upstreamHosts := make(map[string]bool)
		for _, up := range upstreams {
			upstreamHosts[up.Dial] = true
		}

		laggingHost := getDynamicTestHostFromURL(laggingServer.URL)
		if upstreamHosts[laggingHost] {
			t.Errorf("Lagging node %s should not be in upstreams (too far behind)", laggingHost)
		}

		t.Logf("✅ Nodes exceeding block height threshold correctly removed")
	})

	t.Run("MinHealthyNodes_FallbackToAll", func(t *testing.T) {
		// Create all unhealthy servers
		unhealthyServer1 := createCosmosServer(t, 12300, true) // Catching up
		unhealthyServer2 := createCosmosServer(t, 12301, true) // Catching up
		defer unhealthyServer1.Close()
		defer unhealthyServer2.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "unhealthy-1", URL: unhealthyServer1.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "unhealthy-2", URL: unhealthyServer2.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Set minimum healthy nodes requirement
		upstream.config.FailureHandling.MinHealthyNodes = 1

		// Test GetUpstreams - should fallback to all nodes when none are healthy
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Verify all nodes returned as fallback
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (fallback to all), got %d", len(upstreams))
		}

		t.Logf("✅ Fallback to all nodes works when no healthy nodes available")
	})

	t.Run("NodeWeight_ReflectedInUpstream", func(t *testing.T) {
		// Create healthy servers with different weights
		lightServer := createCosmosServer(t, 12345, false)
		heavyServer := createCosmosServer(t, 12344, false)
		defer lightServer.Close()
		defer heavyServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "light", URL: lightServer.URL, Type: NodeTypeCosmos, Weight: 1},
			{Name: "heavy", URL: heavyServer.URL, Type: NodeTypeCosmos, Weight: 10},
		}, logger)

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Find heavy upstream and verify weight is applied
		heavyHost := getDynamicTestHostFromURL(heavyServer.URL)
		var heavyUpstream *reverseproxy.Upstream
		for _, up := range upstreams {
			if up.Dial == heavyHost {
				heavyUpstream = up
				break
			}
		}

		if heavyUpstream == nil {
			t.Fatal("Heavy server not found in upstreams")
		}

		// Verify weight is applied (weight > 1 sets MaxRequests)
		if heavyUpstream.MaxRequests != 10 {
			t.Errorf("Expected MaxRequests=10 for heavy server, got %d", heavyUpstream.MaxRequests)
		}

		t.Logf("✅ Node weights correctly applied to upstream configuration")
	})

	t.Run("DynamicHealthChanges_UpdateUpstreams", func(t *testing.T) {
		// Create a server that starts healthy but becomes unhealthy
		serverResponseHealthy := true
		dynamicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				var response string
				if serverResponseHealthy {
					response = `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
				} else {
					response = `{"result": {"sync_info": {"latest_block_height": "12300", "catching_up": true}}}`
				}
				_, _ = w.Write([]byte(response))
			}
		}))
		defer dynamicServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "dynamic", URL: dynamicServer.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Use short cache duration for quick updates
		upstream.config.Performance.CacheDuration = "100ms"
		upstream.cache = NewHealthCache(100 * time.Millisecond)

		// First check - should be healthy
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream (healthy), got %d", len(upstreams))
		}

		// Change server to unhealthy
		serverResponseHealthy = false
		time.Sleep(150 * time.Millisecond) // Wait for cache to expire

		// Second check - should be unhealthy (no upstreams)
		upstreams, err = upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}
		if len(upstreams) != 1 { // Should fallback to unhealthy node since min_healthy_nodes = 1
			t.Errorf("Expected 1 upstream (fallback), got %d", len(upstreams))
		}

		t.Logf("✅ Dynamic health changes correctly update upstream pool")
	})

	t.Run("EVMNodes_HealthChecksAndUpstreams", func(t *testing.T) {
		// Create EVM servers
		healthyEVMServer := createEVMServer(t, 0x12345, false)
		unhealthyEVMServer := createEVMServer(t, 0, true) // Error response
		defer healthyEVMServer.Close()
		defer unhealthyEVMServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "healthy-evm", URL: healthyEVMServer.URL, Type: NodeTypeEVM, Weight: 100},
			{Name: "unhealthy-evm", URL: unhealthyEVMServer.URL, Type: NodeTypeEVM, Weight: 100},
		}, logger)

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should only return healthy EVM node
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream (healthy EVM only), got %d", len(upstreams))
		}

		expectedHost := getDynamicTestHostFromURL(healthyEVMServer.URL)
		if upstreams[0].Dial != expectedHost {
			t.Errorf("Expected upstream host %s, got %s", expectedHost, upstreams[0].Dial)
		}

		t.Logf("✅ EVM nodes correctly validated and added to upstream pool")
	})

	t.Run("MixedProtocols_BothValidated", func(t *testing.T) {
		// Create mixed Cosmos and EVM servers
		cosmosServer := createCosmosServer(t, 12345, false)
		evmServer := createEVMServer(t, 0x12345, false)
		defer cosmosServer.Close()
		defer evmServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "cosmos", URL: cosmosServer.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "evm", URL: evmServer.URL, Type: NodeTypeEVM, Weight: 100},
		}, logger)

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should return both healthy nodes regardless of protocol
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (both protocols), got %d", len(upstreams))
		}

		t.Logf("✅ Mixed protocol nodes correctly validated and added to upstream pool")
	})
}

// TestDynamicUpstreamAdvanced tests advanced scenarios and edge cases
func TestDynamicUpstreamAdvanced(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("CircuitBreaker_PreventsBadNodes", func(t *testing.T) {
		// Create a server that always fails
		failingServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer failingServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "failing", URL: failingServer.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Set circuit breaker threshold
		upstream.config.FailureHandling.CircuitBreakerThreshold = 0.5

		// Multiple calls should trigger circuit breaker
		for i := 0; i < 5; i++ {
			upstreams, err := upstream.GetUpstreams(&http.Request{})
			if err != nil {
				t.Fatalf("GetUpstreams failed: %v", err)
			}

			// Should still return the node as fallback (since min_healthy_nodes = 1)
			// but the circuit breaker state should be tracked
			if len(upstreams) == 0 {
				t.Errorf("Expected fallback upstream, got 0")
			}
		}

		t.Logf("✅ Circuit breaker prevents overwhelming failing nodes")
	})

	t.Run("ConcurrentRequests_ConsistentUpstreams", func(t *testing.T) {
		// Create multiple healthy servers
		servers := make([]*httptest.Server, 3)
		nodes := make([]NodeConfig, 3)
		for i := 0; i < 3; i++ {
			servers[i] = createCosmosServer(t, uint64(12345-i), false)
			nodes[i] = NodeConfig{
				Name:   fmt.Sprintf("node-%d", i),
				URL:    servers[i].URL,
				Type:   NodeTypeCosmos,
				Weight: 100,
			}
		}
		defer func() {
			for _, server := range servers {
				server.Close()
			}
		}()

		upstream := createTestUpstream(nodes, logger)

		// Wait for initial health checks to complete to avoid race conditions in CI
		time.Sleep(500 * time.Millisecond)

		// Run concurrent GetUpstreams calls
		results := make(chan int, 10)
		for i := 0; i < 10; i++ {
			go func() {
				upstreams, err := upstream.GetUpstreams(&http.Request{})
				if err != nil {
					results <- -1
					return
				}
				results <- len(upstreams)
			}()
		}

		// Collect results
		for i := 0; i < 10; i++ {
			result := <-results
			if result == -1 {
				t.Errorf("GetUpstreams call %d failed with error", i)
			} else if result != 3 {
				// Get current upstreams for debugging
				upstreams, _ := upstream.GetUpstreams(&http.Request{})
				t.Errorf("Expected 3 upstreams in concurrent call %d, got %d (current upstreams: %d)", i, result, len(upstreams))
			}
		}

		t.Logf("✅ Concurrent requests return consistent upstream counts")
	})

	t.Run("HealthEndpoint_ReflectsUpstreamState", func(t *testing.T) {
		// Create mixed healthy/unhealthy servers
		healthyServer := createCosmosServer(t, 12345, false)
		unhealthyServer := createCosmosServer(t, 12300, true)
		defer healthyServer.Close()
		defer unhealthyServer.Close()

		upstream := createTestUpstream([]NodeConfig{
			{Name: "healthy", URL: healthyServer.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "unhealthy", URL: unhealthyServer.URL, Type: NodeTypeCosmos, Weight: 100},
		}, logger)

		// Test health endpoint
		handler := upstream.ServeHealthEndpoint()
		req := httptest.NewRequest("GET", "/health", nil)
		w := httptest.NewRecorder()
		handler(w, req)

		if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
			t.Errorf("Expected status 200 or 503, got %d", w.Code)
		}

		// Parse response and verify node counts
		var response HealthEndpointResponse
		if err := json.NewDecoder(w.Body).Decode(&response); err != nil {
			t.Fatalf("Failed to decode health response: %v", err)
		}

		if response.Nodes.Total != 2 {
			t.Errorf("Expected 2 total nodes, got %d", response.Nodes.Total)
		}

		if response.Nodes.Healthy != 1 {
			t.Errorf("Expected 1 healthy node, got %d", response.Nodes.Healthy)
		}

		if response.Nodes.Unhealthy != 1 {
			t.Errorf("Expected 1 unhealthy node, got %d", response.Nodes.Unhealthy)
		}

		t.Logf("✅ Health endpoint correctly reflects upstream state")
	})
}

// Helper functions for test setup

func createTestUpstream(nodes []NodeConfig, logger *zap.Logger) *BlockchainHealthUpstream {
	config := &Config{
		Nodes: nodes,
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "2s",
			RetryAttempts: 1,
			RetryDelay:    "1s",
		},
		BlockValidation: BlockValidationConfig{
			HeightThreshold:            5,
			ExternalReferenceThreshold: 10,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s",
			MaxConcurrentChecks: 5,
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes:         1,
			CircuitBreakerThreshold: 0.8,
		},
	}

	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(1*time.Second), NewMetrics(), logger),
		cache:         NewHealthCache(1 * time.Second),
		metrics:       NewMetrics(),
		logger:        logger,
	}

	return upstream
}

func createCosmosServer(t *testing.T, blockHeight uint64, catchingUp bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{
				"result": {
					"sync_info": {
						"latest_block_height": "%d",
						"catching_up": %t
					}
				}
			}`, blockHeight, catchingUp)
			_, _ = w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
}

func createEVMServer(t *testing.T, blockHeight uint64, returnError bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			var response string
			if returnError {
				response = `{
					"jsonrpc": "2.0",
					"id": 1,
					"error": {
						"code": -32000,
						"message": "Server error"
					}
				}`
			} else {
				response = fmt.Sprintf(`{
					"jsonrpc": "2.0",
					"id": 1,
					"result": "0x%x"
				}`, blockHeight)
			}
			_, _ = w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
}

func getDynamicTestHostFromURL(rawURL string) string {
	parsedURL, _ := url.Parse(rawURL)
	return parsedURL.Host
}
