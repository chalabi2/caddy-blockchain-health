package blockchain_health

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestUpstreamValidation validates the core dynamic upstream functionality with protocol-specific health checks
// This test focuses on proving the module works exactly to spec with real-world scenarios
func TestUpstreamValidation(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("CosmosRPC_vs_CosmosAPI_DifferentHealthChecks", func(t *testing.T) {
		// Test that RPC nodes are checked via /status and API nodes via REST endpoints

		// Create RPC server (responds to /status)
		cosmosRPCServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer cosmosRPCServer.Close()

		// Create API server (responds to REST endpoints, NOT /status)
		cosmosAPIServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/cosmos/base/tendermint/v1beta1/syncing":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `{"syncing": false}`)
			case "/cosmos/base/tendermint/v1beta1/blocks/latest":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `{"block": {"header": {"height": "12345"}}}`)
			case "/status":
				// API servers don't typically respond to /status
				http.NotFound(w, r)
			default:
				http.NotFound(w, r)
			}
		}))
		defer cosmosAPIServer.Close()

		// Configure upstream with both types
		upstream := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{
					Name:     "cosmos-rpc",
					URL:      cosmosRPCServer.URL,
					Type:     NodeTypeCosmos,
					Weight:   100,
					Metadata: map[string]string{"service_type": "rpc"},
				},
				{
					Name:     "cosmos-api",
					URL:      cosmosAPIServer.URL,
					Type:     NodeTypeCosmos,
					Weight:   100,
					Metadata: map[string]string{"service_type": "api"},
				},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "1s",
				Timeout:       "2s",
				RetryAttempts: 1,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			logger: logger,
		}

		// Set up the upstream
		upstream.config = &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			FailureHandling: upstream.FailureHandling,
		}
		upstream.healthChecker = NewHealthChecker(upstream.config, NewHealthCache(1*time.Second), nil, logger)

		// Test GetUpstreams - both should be healthy via their respective protocols
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		if len(upstreams) != 2 {
			t.Errorf("Expected 2 healthy upstreams (RPC + API), got %d", len(upstreams))
		}

		t.Logf("✅ RPC nodes checked via /status, API nodes checked via REST endpoints")
	})

	t.Run("EVM_JSONRPCHealthCheck_vs_CosmosRPC", func(t *testing.T) {
		// Test that EVM nodes use JSON-RPC eth_blockNumber while Cosmos uses /status

		// Create EVM server (JSON-RPC)
		evmServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				// Respond to eth_blockNumber
				response := `{"jsonrpc": "2.0", "id": 1, "result": "0x12345"}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer evmServer.Close()

		// Create Cosmos RPC server
		cosmosServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "75557", "catching_up": false}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer cosmosServer.Close()

		upstream := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "evm-node", URL: evmServer.URL, Type: NodeTypeEVM, Weight: 100},
				{Name: "cosmos-node", URL: cosmosServer.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Timeout:       "2s",
				RetryAttempts: 1,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			logger: logger,
		}

		upstream.config = &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			FailureHandling: upstream.FailureHandling,
		}
		upstream.healthChecker = NewHealthChecker(upstream.config, NewHealthCache(1*time.Second), nil, logger)

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		if len(upstreams) != 2 {
			t.Errorf("Expected 2 healthy upstreams (EVM + Cosmos), got %d", len(upstreams))
		}

		t.Logf("✅ EVM nodes checked via JSON-RPC eth_blockNumber, Cosmos via /status")
	})

	t.Run("BlockHeightLag_RemovesFromUpstream", func(t *testing.T) {
		// Test that nodes lagging too far behind in block height are removed from upstream pool

		// Create nodes with different block heights
		leaderNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "100000", "catching_up": false}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer leaderNode.Close()

		goodNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "99998", "catching_up": false}}}` // 2 blocks behind
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer goodNode.Close()

		laggingNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "99990", "catching_up": false}}}` // 10 blocks behind
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer laggingNode.Close()

		config := &Config{
			Nodes: []NodeConfig{
				{Name: "leader", URL: leaderNode.URL, Type: NodeTypeCosmos, Weight: 100},
				{Name: "good", URL: goodNode.URL, Type: NodeTypeCosmos, Weight: 100},
				{Name: "lagging", URL: laggingNode.URL, Type: NodeTypeCosmos, Weight: 100},
			},
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

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should only return leader and good nodes (lagging node excluded)
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (excluding lagging node), got %d", len(upstreams))
		}

		// Verify lagging node is not included
		upstreamHosts := make(map[string]bool)
		for _, up := range upstreams {
			upstreamHosts[up.Dial] = true
		}

		laggingHost := getHostFromValidationTestURL(laggingNode.URL)
		if upstreamHosts[laggingHost] {
			t.Errorf("Lagging node %s should not be in upstreams", laggingHost)
		}

		t.Logf("✅ Nodes lagging beyond block height threshold correctly removed from upstream pool")
	})

	t.Run("CatchingUp_CosmosNode_RemovedFromUpstream", func(t *testing.T) {
		// Test that Cosmos nodes with catching_up=true are removed from upstream

		healthyNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer healthyNode.Close()

		catchingUpNode := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "12340", "catching_up": true}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer catchingUpNode.Close()

		config := &Config{
			Nodes: []NodeConfig{
				{Name: "healthy", URL: healthyNode.URL, Type: NodeTypeCosmos, Weight: 100},
				{Name: "catching-up", URL: catchingUpNode.URL, Type: NodeTypeCosmos, Weight: 100},
			},
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

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should only return healthy node (catching up node excluded)
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream (excluding catching up node), got %d", len(upstreams))
		}

		expectedHost := getHostFromValidationTestURL(healthyNode.URL)
		if upstreams[0].Dial != expectedHost {
			t.Errorf("Expected upstream host %s, got %s", expectedHost, upstreams[0].Dial)
		}

		t.Logf("✅ Cosmos nodes with catching_up=true correctly removed from upstream pool")
	})

	t.Run("FailedNode_FallbackBehavior", func(t *testing.T) {
		// Test fallback behavior when no nodes are healthy but min_healthy_nodes > 0

		failedNode1 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		}))
		defer failedNode1.Close()

		failedNode2 := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		}))
		defer failedNode2.Close()

		upstream := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "failed-1", URL: failedNode1.URL, Type: NodeTypeCosmos, Weight: 100},
				{Name: "failed-2", URL: failedNode2.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Timeout:       "1s",
				RetryAttempts: 1,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1, // Require at least 1 healthy node
			},
			logger: logger,
		}

		upstream.config = &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			FailureHandling: upstream.FailureHandling,
		}
		upstream.healthChecker = NewHealthChecker(upstream.config, NewHealthCache(1*time.Second), nil, logger)

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams should not error on fallback: %v", err)
		}

		// Should fallback to all nodes when no healthy nodes available
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 upstreams (fallback to all), got %d", len(upstreams))
		}

		t.Logf("✅ Fallback to all nodes works when no healthy nodes available")
	})

	t.Run("RealWorld_MixedServiceTypes", func(t *testing.T) {
		// Test a real-world scenario with mixed service types as described in README

		// Cosmos RPC (port 26657)
		cosmosRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer cosmosRPC.Close()

		// Cosmos REST API (port 1317)
		cosmosAPI := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/cosmos/base/tendermint/v1beta1/syncing":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"syncing": false}`))
			case "/cosmos/base/tendermint/v1beta1/blocks/latest":
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(`{"block": {"header": {"height": "12345"}}}`))
			default:
				http.NotFound(w, r)
			}
		}))
		defer cosmosAPI.Close()

		// EVM JSON-RPC (port 8545)
		evmRPC := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"jsonrpc": "2.0", "id": 1, "result": "0x12345"}`
				_, _ = w.Write([]byte(response))
			} else {
				http.NotFound(w, r)
			}
		}))
		defer evmRPC.Close()

		// Create a failing EVM node
		failingEVM := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == "POST" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := `{"jsonrpc": "2.0", "id": 1, "error": {"code": -32000, "message": "Server error"}}`
				_, _ = w.Write([]byte(response))
			}
		}))
		defer failingEVM.Close()

		config := &Config{
			Nodes: []NodeConfig{
				{
					Name:     "cosmos-rpc",
					URL:      cosmosRPC.URL,
					Type:     NodeTypeCosmos,
					Weight:   100,
					Metadata: map[string]string{"service_type": "rpc"},
				},
				{
					Name:     "cosmos-api",
					URL:      cosmosAPI.URL,
					Type:     NodeTypeCosmos,
					Weight:   100,
					Metadata: map[string]string{"service_type": "api"},
				},
				{
					Name:   "evm-healthy",
					URL:    evmRPC.URL,
					Type:   NodeTypeEVM,
					Weight: 100,
				},
				{
					Name:   "evm-failing",
					URL:    failingEVM.URL,
					Type:   NodeTypeEVM,
					Weight: 100,
				},
			},
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
				MinHealthyNodes:         2,
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

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should return 3 healthy nodes (2 Cosmos + 1 EVM, excluding failing EVM)
		if len(upstreams) != 3 {
			t.Errorf("Expected 3 healthy upstreams, got %d", len(upstreams))
		}

		// Verify failing EVM is not included
		upstreamHosts := make(map[string]bool)
		for _, up := range upstreams {
			upstreamHosts[up.Dial] = true
		}

		failingHost := getHostFromValidationTestURL(failingEVM.URL)
		if upstreamHosts[failingHost] {
			t.Errorf("Failing EVM node %s should not be in upstreams", failingHost)
		}

		t.Logf("✅ Mixed service types correctly validated with protocol-specific health checks")
	})

	t.Run("CacheEffectiveness_ReducesHealthCheckCalls", func(t *testing.T) {
		// Test that caching reduces redundant health check calls
		callCount := 0

		cachedServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			callCount++
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
			_, _ = w.Write([]byte(response))
		}))
		defer cachedServer.Close()

		upstream := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "cached-node", URL: cachedServer.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Timeout:       "2s",
				RetryAttempts: 1,
			},
			Performance: PerformanceConfig{
				CacheDuration: "5s", // Long cache for this test
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			logger: logger,
		}

		upstream.config = &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			Performance:     upstream.Performance,
			FailureHandling: upstream.FailureHandling,
		}
		upstream.cache = NewHealthCache(5 * time.Second)
		upstream.healthChecker = NewHealthChecker(upstream.config, upstream.cache, nil, logger)

		// First call - should hit the server
		_, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("First GetUpstreams failed: %v", err)
		}

		firstCallCount := callCount

		// Second call immediately - should use cache
		_, err = upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("Second GetUpstreams failed: %v", err)
		}

		secondCallCount := callCount

		// Verify cache was used (no additional server calls)
		if secondCallCount != firstCallCount {
			t.Errorf("Expected cached result (no additional calls), but call count increased from %d to %d",
				firstCallCount, secondCallCount)
		}

		t.Logf("✅ Caching effectively reduces redundant health check calls")
	})
}

// Helper function that gets the host from a URL string (simplified for test servers)
func getHostFromValidationTestURL(rawURL string) string {
	// Extract host from URL (simplified for test servers)
	parts := strings.Split(rawURL, "://")
	if len(parts) != 2 {
		return rawURL
	}
	hostPort := parts[1]
	return hostPort
}
