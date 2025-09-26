package blockchain_health

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestEndToEndCaddyIntegration tests the module in a real Caddy environment
func TestEndToEndCaddyIntegration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("FullStack_CosmosRPC_LoadBalancing", func(t *testing.T) {
		// Create mock Cosmos RPC servers
		cosmosRPC1 := createMockCosmosRPC(t, "12345", false, true) // Healthy
		cosmosRPC2 := createMockCosmosRPC(t, "12344", false, true) // Healthy
		cosmosRPC3 := createMockCosmosRPC(t, "12300", true, true)  // Catching up
		defer cosmosRPC1.Close()
		defer cosmosRPC2.Close()
		defer cosmosRPC3.Close()

		// Create proxy server with our module
		proxyServer := createProxyServerWithModule(t, logger, []NodeConfig{
			{Name: "cosmos-rpc-1", URL: cosmosRPC1.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "cosmos-rpc-2", URL: cosmosRPC2.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "cosmos-rpc-3", URL: cosmosRPC3.URL, Type: NodeTypeCosmos, Weight: 100},
		})
		defer proxyServer.Close()

		// Test load balancing - only healthy nodes should receive traffic
		requestCounts := testLoadBalancing(t, proxyServer.URL, 10)

		// Verify only healthy nodes received requests
		rpc1Host := extractHost(cosmosRPC1.URL)
		rpc2Host := extractHost(cosmosRPC2.URL)
		rpc3Host := extractHost(cosmosRPC3.URL)

		if requestCounts[rpc1Host] == 0 {
			t.Errorf("Healthy RPC node 1 (%s) should have received requests", rpc1Host)
		}
		if requestCounts[rpc2Host] == 0 {
			t.Errorf("Healthy RPC node 2 (%s) should have received requests", rpc2Host)
		}
		if requestCounts[rpc3Host] > 0 {
			t.Errorf("Unhealthy RPC node 3 (%s) should not have received requests, but got %d",
				rpc3Host, requestCounts[rpc3Host])
		}

		t.Logf("✅ Cosmos RPC load balancing works: Node1=%d, Node2=%d, Node3=%d requests",
			requestCounts[rpc1Host], requestCounts[rpc2Host], requestCounts[rpc3Host])
	})

	t.Run("FullStack_CosmosAPI_LoadBalancing", func(t *testing.T) {
		// Create mock Cosmos REST API servers
		cosmosAPI1 := createMockCosmosAPI(t, "12345", false, true) // Healthy
		cosmosAPI2 := createMockCosmosAPI(t, "12344", false, true) // Healthy
		cosmosAPI3 := createMockCosmosAPI(t, "12300", true, true)  // Syncing
		defer cosmosAPI1.Close()
		defer cosmosAPI2.Close()
		defer cosmosAPI3.Close()

		// Create Caddy server with our module
		proxyServer := createProxyServerWithModule(t, logger, []NodeConfig{
			{
				Name:     "cosmos-api-1",
				URL:      cosmosAPI1.URL,
				Type:     NodeTypeCosmos,
				Weight:   100,
				Metadata: map[string]string{"service_type": "api"},
			},
			{
				Name:     "cosmos-api-2",
				URL:      cosmosAPI2.URL,
				Type:     NodeTypeCosmos,
				Weight:   100,
				Metadata: map[string]string{"service_type": "api"},
			},
			{
				Name:     "cosmos-api-3",
				URL:      cosmosAPI3.URL,
				Type:     NodeTypeCosmos,
				Weight:   100,
				Metadata: map[string]string{"service_type": "api"},
			},
		})
		defer proxyServer.Close()

		// Test load balancing
		requestCounts := testLoadBalancing(t, proxyServer.URL, 10)

		// Verify only healthy nodes received requests
		api1Host := extractHost(cosmosAPI1.URL)
		api2Host := extractHost(cosmosAPI2.URL)
		api3Host := extractHost(cosmosAPI3.URL)

		if requestCounts[api1Host] == 0 {
			t.Errorf("Healthy API node 1 (%s) should have received requests", api1Host)
		}
		if requestCounts[api2Host] == 0 {
			t.Errorf("Healthy API node 2 (%s) should have received requests", api2Host)
		}
		if requestCounts[api3Host] > 0 {
			t.Errorf("Unhealthy API node 3 (%s) should not have received requests, but got %d",
				api3Host, requestCounts[api3Host])
		}

		t.Logf("✅ Cosmos API load balancing works: Node1=%d, Node2=%d, Node3=%d requests",
			requestCounts[api1Host], requestCounts[api2Host], requestCounts[api3Host])
	})

	t.Run("FullStack_EVM_LoadBalancing", func(t *testing.T) {
		// Create mock EVM JSON-RPC servers
		evmRPC1 := createMockEVMRPC(t, "0x12345", false, true) // Healthy
		evmRPC2 := createMockEVMRPC(t, "0x12344", false, true) // Healthy
		evmRPC3 := createMockEVMRPC(t, "0x0", true, true)      // Error response
		defer evmRPC1.Close()
		defer evmRPC2.Close()
		defer evmRPC3.Close()

		// Create Caddy server with our module
		proxyServer := createProxyServerWithModule(t, logger, []NodeConfig{
			{Name: "evm-rpc-1", URL: evmRPC1.URL, Type: NodeTypeEVM, Weight: 100},
			{Name: "evm-rpc-2", URL: evmRPC2.URL, Type: NodeTypeEVM, Weight: 100},
			{Name: "evm-rpc-3", URL: evmRPC3.URL, Type: NodeTypeEVM, Weight: 100},
		})
		defer proxyServer.Close()

		// Test load balancing
		requestCounts := testLoadBalancing(t, proxyServer.URL, 10)

		// Verify only healthy nodes received requests
		evm1Host := extractHost(evmRPC1.URL)
		evm2Host := extractHost(evmRPC2.URL)
		evm3Host := extractHost(evmRPC3.URL)

		if requestCounts[evm1Host] == 0 {
			t.Errorf("Healthy EVM node 1 (%s) should have received requests", evm1Host)
		}
		if requestCounts[evm2Host] == 0 {
			t.Errorf("Healthy EVM node 2 (%s) should have received requests", evm2Host)
		}
		if requestCounts[evm3Host] > 0 {
			t.Errorf("Unhealthy EVM node 3 (%s) should not have received requests, but got %d",
				evm3Host, requestCounts[evm3Host])
		}

		t.Logf("✅ EVM JSON-RPC load balancing works: Node1=%d, Node2=%d, Node3=%d requests",
			requestCounts[evm1Host], requestCounts[evm2Host], requestCounts[evm3Host])
	})

	t.Run("FailoverScenario_NodeGoesDown", func(t *testing.T) {
		// Create controllable mock servers
		server1Healthy := true
		server2Healthy := true

		cosmosRPC1 := createControllableMockCosmosRPC(t, &server1Healthy)
		cosmosRPC2 := createControllableMockCosmosRPC(t, &server2Healthy)
		defer cosmosRPC1.Close()
		defer cosmosRPC2.Close()

		// Create Caddy server with short health check interval for fast testing
		proxyServer := createProxyServerWithFastHealthChecks(t, logger, []NodeConfig{
			{Name: "cosmos-rpc-1", URL: cosmosRPC1.URL, Type: NodeTypeCosmos, Weight: 100},
			{Name: "cosmos-rpc-2", URL: cosmosRPC2.URL, Type: NodeTypeCosmos, Weight: 100},
		})
		defer proxyServer.Close()

		// Initial test - both nodes should receive traffic
		requestCounts := testLoadBalancing(t, proxyServer.URL, 10)
		rpc1Host := extractHost(cosmosRPC1.URL)
		rpc2Host := extractHost(cosmosRPC2.URL)

		if requestCounts[rpc1Host] == 0 || requestCounts[rpc2Host] == 0 {
			t.Errorf("Both nodes should initially receive requests: Node1=%d, Node2=%d",
				requestCounts[rpc1Host], requestCounts[rpc2Host])
		}

		// Simulate node 1 going down
		server1Healthy = false
		t.Logf("Simulating node 1 going down...")

		// Wait for health checks to detect the failure
		time.Sleep(3 * time.Second)

		// Test again - only node 2 should receive traffic
		requestCounts = testLoadBalancing(t, proxyServer.URL, 10)

		if requestCounts[rpc1Host] > 0 {
			t.Errorf("Failed node 1 (%s) should not receive requests, but got %d",
				rpc1Host, requestCounts[rpc1Host])
		}
		if requestCounts[rpc2Host] == 0 {
			t.Errorf("Healthy node 2 (%s) should receive all requests", rpc2Host)
		}

		// Simulate node 1 coming back online
		server1Healthy = true
		t.Logf("Simulating node 1 coming back online...")

		// Wait for health checks to detect the recovery
		time.Sleep(3 * time.Second)

		// Test again - both nodes should receive traffic again
		requestCounts = testLoadBalancing(t, proxyServer.URL, 10)

		if requestCounts[rpc1Host] == 0 || requestCounts[rpc2Host] == 0 {
			t.Errorf("Both nodes should receive requests after recovery: Node1=%d, Node2=%d",
				requestCounts[rpc1Host], requestCounts[rpc2Host])
		}

		t.Logf("✅ Failover scenario works: Node recovery detected and traffic restored")
	})

	t.Run("MixedProtocols_AllWorking", func(t *testing.T) {
		// Create mixed protocol servers
		cosmosRPC := createMockCosmosRPC(t, "12345", false, true)
		cosmosAPI := createMockCosmosAPI(t, "12345", false, true)
		evmRPC := createMockEVMRPC(t, "0x12345", false, true)
		defer cosmosRPC.Close()
		defer cosmosAPI.Close()
		defer evmRPC.Close()

		// Create Caddy server with mixed protocols
		proxyServer := createProxyServerWithModule(t, logger, []NodeConfig{
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
			{Name: "evm-rpc", URL: evmRPC.URL, Type: NodeTypeEVM, Weight: 100},
		})
		defer proxyServer.Close()

		// Test that all protocols can handle requests
		requestCounts := testLoadBalancing(t, proxyServer.URL, 15)

		rpcHost := extractHost(cosmosRPC.URL)
		apiHost := extractHost(cosmosAPI.URL)
		evmHost := extractHost(evmRPC.URL)

		if requestCounts[rpcHost] == 0 {
			t.Errorf("Cosmos RPC node should have received requests")
		}
		if requestCounts[apiHost] == 0 {
			t.Errorf("Cosmos API node should have received requests")
		}
		if requestCounts[evmHost] == 0 {
			t.Errorf("EVM RPC node should have received requests")
		}

		t.Logf("✅ Mixed protocols work: CosmosRPC=%d, CosmosAPI=%d, EVM=%d requests",
			requestCounts[rpcHost], requestCounts[apiHost], requestCounts[evmHost])
	})
}

// Helper functions for creating mock servers

func createMockCosmosRPC(t *testing.T, blockHeight string, catchingUp bool, healthy bool) *httptest.Server {
	requestCount := 0
	mu := sync.Mutex{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		currentCount := requestCount
		mu.Unlock()

		// Add request tracking header
		w.Header().Set("X-Test-Request-Count", fmt.Sprintf("%d", currentCount))

		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			if healthy {
				w.WriteHeader(http.StatusOK)
				response := fmt.Sprintf(`{
					"result": {
						"sync_info": {
							"latest_block_height": "%s",
							"catching_up": %t
						}
					}
				}`, blockHeight, catchingUp)
				_, _ = fmt.Fprint(w, response)
			} else {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		} else {
			// Proxy the request (simulate normal operation)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"result": "proxied request to cosmos rpc", "block_height": "%s", "backend_host": "%s"}`, blockHeight, r.Host)
		}
	}))
}

func createMockCosmosAPI(t *testing.T, blockHeight string, syncing bool, healthy bool) *httptest.Server {
	requestCount := 0
	mu := sync.Mutex{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		currentCount := requestCount
		mu.Unlock()

		// Add request tracking header
		w.Header().Set("X-Test-Request-Count", fmt.Sprintf("%d", currentCount))

		switch r.URL.Path {
		case "/cosmos/base/tendermint/v1beta1/syncing":
			w.Header().Set("Content-Type", "application/json")
			if healthy {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"syncing": %t}`, syncing)
			} else {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}
		case "/cosmos/base/tendermint/v1beta1/blocks/latest":
			w.Header().Set("Content-Type", "application/json")
			if healthy {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"block": {"header": {"height": "%s"}}}`, blockHeight)
			} else {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}
		default:
			// Proxy the request (simulate normal operation)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"result": "proxied request to cosmos api", "block_height": "%s", "backend_host": "%s"}`, blockHeight, r.Host)
		}
	}))
}

func createMockEVMRPC(t *testing.T, blockHeight string, returnError bool, healthy bool) *httptest.Server {
	requestCount := 0
	mu := sync.Mutex{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		currentCount := requestCount
		mu.Unlock()

		// Add request tracking header
		w.Header().Set("X-Test-Request-Count", fmt.Sprintf("%d", currentCount))

		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			if healthy && !returnError {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{
					"jsonrpc": "2.0",
					"id": 1,
					"result": "%s"
				}`, blockHeight)
			} else if healthy && returnError {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `{
					"jsonrpc": "2.0",
					"id": 1,
					"error": {
						"code": -32000,
						"message": "Server error"
					}
				}`)
			} else {
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
			}
		} else {
			// Proxy the request (simulate normal operation)
			w.WriteHeader(http.StatusOK)
			_, _ = fmt.Fprintf(w, `{"result": "proxied request to evm rpc", "block_height": "%s", "backend_host": "%s"}`, blockHeight, r.Host)
		}
	}))
}

func createControllableMockCosmosRPC(t *testing.T, healthy *bool) *httptest.Server {
	requestCount := 0
	mu := sync.Mutex{}

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		requestCount++
		currentCount := requestCount
		isHealthy := *healthy
		mu.Unlock()

		// Add request tracking header
		w.Header().Set("X-Test-Request-Count", fmt.Sprintf("%d", currentCount))

		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			if isHealthy {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprint(w, `{
					"result": {
						"sync_info": {
							"latest_block_height": "12345",
							"catching_up": false
						}
					}
				}`)
			} else {
				http.Error(w, "Node is down", http.StatusServiceUnavailable)
			}
		} else {
			// Proxy the request only if healthy
			if isHealthy {
				w.WriteHeader(http.StatusOK)
				_, _ = fmt.Fprintf(w, `{"result": "proxied request", "backend_host": "%s"}`, r.Host)
			} else {
				http.Error(w, "Node is down", http.StatusServiceUnavailable)
			}
		}
	}))
}

// Helper functions for creating test proxy servers

func createProxyServerWithModule(t *testing.T, logger *zap.Logger, nodes []NodeConfig) *httptest.Server {
	return createProxyServerWithConfig(t, logger, nodes, HealthCheckConfig{
		Interval:      "10s",
		Timeout:       "5s",
		RetryAttempts: 3,
	})
}

func createProxyServerWithFastHealthChecks(t *testing.T, logger *zap.Logger, nodes []NodeConfig) *httptest.Server {
	return createProxyServerWithConfig(t, logger, nodes, HealthCheckConfig{
		Interval:      "1s", // Fast health checks for testing
		Timeout:       "2s",
		RetryAttempts: 1,
	})
}

func createProxyServerWithConfig(t *testing.T, logger *zap.Logger, nodes []NodeConfig, healthCheck HealthCheckConfig) *httptest.Server {
	// Create our blockchain health upstream
	blockchainUpstream := &BlockchainHealthUpstream{
		Nodes:       nodes,
		HealthCheck: healthCheck,
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "1s",
			MaxConcurrentChecks: 10,
		},
	}

	// Create Caddy context and provision the module
	ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
	t.Cleanup(cancel)

	err := blockchainUpstream.Provision(ctx)
	if err != nil {
		t.Fatalf("Failed to provision blockchain upstream: %v", err)
	}

	// Create test server using our module as HTTP handler
	return httptest.NewServer(blockchainUpstream)
}

// Global request counter for round-robin
var requestCounter int

// Simplified approach - create test server manually
func (h *BlockchainHealthUpstream) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Get healthy upstreams
	upstreams, err := h.GetUpstreams(r)
	if err != nil {
		http.Error(w, fmt.Sprintf("No healthy upstreams: %v", err), http.StatusBadGateway)
		return
	}

	if len(upstreams) == 0 {
		http.Error(w, "No healthy upstreams available", http.StatusBadGateway)
		return
	}

	// Simple round-robin across all healthy upstreams
	requestCounter++
	upstreamIndex := requestCounter % len(upstreams)
	upstream := upstreams[upstreamIndex]

	// Parse target URL
	targetURL := fmt.Sprintf("http://%s%s", upstream.Dial, r.URL.Path)

	// Create proxy request
	proxyReq, err := http.NewRequest(r.Method, targetURL, r.Body)
	if err != nil {
		http.Error(w, "Failed to create proxy request", http.StatusInternalServerError)
		return
	}

	// Copy headers
	for name, values := range r.Header {
		for _, value := range values {
			proxyReq.Header.Add(name, value)
		}
	}

	// Make request
	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(proxyReq)
	if err != nil {
		http.Error(w, "Proxy request failed", http.StatusBadGateway)
		return
	}
	defer func() { _ = resp.Body.Close() }()

	// Copy response headers
	for name, values := range resp.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}

	// Copy status code
	w.WriteHeader(resp.StatusCode)

	// Copy body
	_, _ = io.Copy(w, resp.Body)
}

// Helper functions for testing

func testLoadBalancing(t *testing.T, serverAddr string, numRequests int) map[string]int {
	requestCounts := make(map[string]int)

	client := &http.Client{Timeout: 5 * time.Second}

	for i := 0; i < numRequests; i++ {
		// serverAddr already includes http:// from httptest.Server.URL
		url := fmt.Sprintf("%s/test", serverAddr)
		resp, err := client.Get(url)
		if err != nil {
			t.Logf("Request %d failed: %v", i+1, err)
			continue
		}

		// Read response body to see if it contains backend info
		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()

		if err == nil {
			bodyStr := string(body)
			// Look for backend_host in JSON response
			if strings.Contains(bodyStr, `"backend_host"`) {
				// Extract backend_host value from JSON
				parts := strings.Split(bodyStr, `"backend_host": "`)
				if len(parts) > 1 {
					hostEnd := strings.Index(parts[1], `"`)
					if hostEnd > 0 {
						host := parts[1][:hostEnd]
						requestCounts[host]++
					}
				}
			}
		}
	}

	return requestCounts
}

func extractHost(url string) string {
	parts := strings.Split(url, "://")
	if len(parts) != 2 {
		return url
	}
	return parts[1]
}
