package blockchain_health

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestEnvironmentBasedUpstreams tests the environment variable-based configuration as described in README
func TestEnvironmentBasedUpstreams(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("CosmosServers_AutoDiscovery", func(t *testing.T) {
		// Create test servers to simulate Cosmos nodes
		cosmosRPC1 := createCosmosServer(t, 12345, false)
		cosmosRPC2 := createCosmosServer(t, 12344, false)
		cosmosAPI1 := createCosmosAPIServer(t, 12345, false) // REST API server
		defer cosmosRPC1.Close()
		defer cosmosRPC2.Close()
		defer cosmosAPI1.Close()

		// Set up environment variables as documented in README
		t.Setenv("COSMOS_RPC_SERVERS", fmt.Sprintf("%s %s", cosmosRPC1.URL, cosmosRPC2.URL))
		t.Setenv("COSMOS_API_SERVERS", cosmosAPI1.URL)

		// Create upstream with auto-discovery
		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				AutoDiscoverFromEnv: "COSMOS",
			},
			logger: logger,
		}

		// Process environment configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment configuration: %v", err)
		}

		// Verify nodes were created from environment variables
		if len(upstream.Nodes) != 3 { // 2 RPC + 1 API
			t.Errorf("Expected 3 nodes from environment, got %d", len(upstream.Nodes))
		}

		// Verify node types and URLs
		foundRPCCount := 0
		foundAPICount := 0
		expectedURLs := map[string]bool{
			cosmosRPC1.URL: false,
			cosmosRPC2.URL: false,
			cosmosAPI1.URL: false,
		}

		for _, node := range upstream.Nodes {
			if _, exists := expectedURLs[node.URL]; exists {
				expectedURLs[node.URL] = true
			}

			if node.Metadata["service_type"] == "rpc" {
				foundRPCCount++
			} else if node.Metadata["service_type"] == "api" {
				foundAPICount++
			}
		}

		if foundRPCCount != 2 {
			t.Errorf("Expected 2 RPC nodes, got %d", foundRPCCount)
		}
		if foundAPICount != 1 {
			t.Errorf("Expected 1 API node, got %d", foundAPICount)
		}

		// Verify all URLs were found
		for url, found := range expectedURLs {
			if !found {
				t.Errorf("Expected URL %s not found in nodes", url)
			}
		}

		t.Logf("✅ Cosmos auto-discovery from environment variables works correctly")
	})

	t.Run("EVMServers_Configuration", func(t *testing.T) {
		// Create test EVM servers
		evmServer1 := createEVMServer(t, 0x12345, false)
		evmServer2 := createEVMServer(t, 0x12344, false)
		defer evmServer1.Close()
		defer evmServer2.Close()

		upstream := &BlockchainHealthUpstream{
			Environment: EnvironmentConfig{
				EVMServers: fmt.Sprintf("%s %s", evmServer1.URL, evmServer2.URL),
			},
			Chain: ChainConfig{
				ChainType: "evm",
			},
			logger: logger,
		}

		// Process environment configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment configuration: %v", err)
		}

		// Verify EVM nodes were created
		if len(upstream.Nodes) != 2 {
			t.Errorf("Expected 2 EVM nodes, got %d", len(upstream.Nodes))
		}

		// Verify all nodes are EVM type
		for _, node := range upstream.Nodes {
			if node.Type != NodeTypeEVM {
				t.Errorf("Expected EVM node type, got %s", node.Type)
			}
		}

		t.Logf("✅ EVM server configuration from environment works correctly")
	})

	t.Run("ChainPreset_AppliesDefaults", func(t *testing.T) {
		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				ChainPreset: "cosmos-hub",
			},
			logger: logger,
		}

		// Apply chain preset
		if err := upstream.applyChainPreset("cosmos-hub"); err != nil {
			t.Fatalf("Failed to apply cosmos-hub preset: %v", err)
		}

		// Verify chain type was set
		if upstream.Chain.ChainType != "cosmos" {
			t.Errorf("Expected chain type 'cosmos', got '%s'", upstream.Chain.ChainType)
		}

		// Verify defaults were applied
		if upstream.HealthCheck.Interval != "10s" {
			t.Errorf("Expected interval '10s', got '%s'", upstream.HealthCheck.Interval)
		}

		if upstream.BlockValidation.HeightThreshold != 5 {
			t.Errorf("Expected height threshold 5, got %d", upstream.BlockValidation.HeightThreshold)
		}

		t.Logf("✅ Chain preset correctly applies default configurations")
	})

	t.Run("EnvironmentBasedUpstreams_WorkInLoadBalancer", func(t *testing.T) {
		// Create test servers
		cosmosRPC1 := createCosmosServer(t, 12345, false)
		cosmosRPC2 := createCosmosServer(t, 12344, false)
		unhealthyRPC := createCosmosServer(t, 12300, true) // Catching up
		defer cosmosRPC1.Close()
		defer cosmosRPC2.Close()
		defer unhealthyRPC.Close()

		// Set environment variables
		t.Setenv("COSMOS_RPC_SERVERS", fmt.Sprintf("%s %s %s", cosmosRPC1.URL, cosmosRPC2.URL, unhealthyRPC.URL))

		// Create upstream with environment configuration
		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				AutoDiscoverFromEnv: "COSMOS",
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
				MaxConcurrentChecks: 5,
			},
			logger: logger,
		}

		// Process environment and set up
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment: %v", err)
		}

		config := &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			FailureHandling: upstream.FailureHandling,
			Performance:     upstream.Performance,
			BlockValidation: BlockValidationConfig{
				HeightThreshold:            5,
				ExternalReferenceThreshold: 10,
			},
		}

		upstream.config = config
		upstream.healthChecker = NewHealthChecker(config, NewHealthCache(1*time.Second), NewMetrics(), logger)
		upstream.cache = NewHealthCache(1 * time.Second)
		upstream.metrics = NewMetrics()

		// Test GetUpstreams - should only return healthy nodes
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should return 2 healthy nodes (excluding the catching up one)
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 healthy upstreams from environment config, got %d", len(upstreams))
		}

		// Verify the unhealthy node is not included
		upstreamHosts := make(map[string]bool)
		for _, up := range upstreams {
			upstreamHosts[up.Dial] = true
		}

		unhealthyHost := extractHostFromURL(unhealthyRPC.URL)
		if upstreamHosts[unhealthyHost] {
			t.Errorf("Unhealthy node %s should not be in upstreams", unhealthyHost)
		}

		t.Logf("✅ Environment-based configuration correctly provides healthy upstreams")
	})

	t.Run("MixedProtocol_AltheaConfiguration", func(t *testing.T) {
		// Simulate Althea dual-protocol setup as described in README
		cosmosRPC := createCosmosServer(t, 12345, false)
		evmRPC := createEVMServer(t, 0x12345, false)
		defer cosmosRPC.Close()
		defer evmRPC.Close()

		// Set environment variables for Althea
		t.Setenv("ALTHEA_RPC_SERVERS", cosmosRPC.URL)
		t.Setenv("ALTHEA_EVM_SERVERS", evmRPC.URL)

		// Test Cosmos part
		cosmosUpstream := &BlockchainHealthUpstream{
			Environment: EnvironmentConfig{
				RPCServers: cosmosRPC.URL,
			},
			Chain: ChainConfig{
				ChainType:   "cosmos",
				ServiceType: "rpc",
			},
			logger: logger,
		}

		if err := cosmosUpstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process Cosmos environment: %v", err)
		}

		// Test EVM part
		evmUpstream := &BlockchainHealthUpstream{
			Environment: EnvironmentConfig{
				EVMServers: evmRPC.URL,
			},
			Chain: ChainConfig{
				ChainType:   "evm",
				ServiceType: "evm",
			},
			logger: logger,
		}

		if err := evmUpstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process EVM environment: %v", err)
		}

		// Verify both configurations work
		if len(cosmosUpstream.Nodes) != 1 || cosmosUpstream.Nodes[0].Type != NodeTypeCosmos {
			t.Error("Cosmos configuration failed")
		}

		if len(evmUpstream.Nodes) != 1 || evmUpstream.Nodes[0].Type != NodeTypeEVM {
			t.Error("EVM configuration failed")
		}

		t.Logf("✅ Mixed protocol (Althea-style) configuration works correctly")
	})

	t.Run("WebSocketURL_AutoGeneration", func(t *testing.T) {
		// Test that WebSocket URLs are auto-generated as described in README
		cosmosRPC := createCosmosServer(t, 12345, false)
		defer cosmosRPC.Close()

		upstream := &BlockchainHealthUpstream{
			Environment: EnvironmentConfig{
				RPCServers: cosmosRPC.URL,
			},
			Chain: ChainConfig{
				ChainType: "cosmos",
			},
			logger: logger,
		}

		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment: %v", err)
		}

		// Verify WebSocket URL was generated
		if len(upstream.Nodes) != 1 {
			t.Fatalf("Expected 1 node, got %d", len(upstream.Nodes))
		}

		node := upstream.Nodes[0]
		expectedWSURL := "ws://" + extractHostFromURL(cosmosRPC.URL) + "/websocket"
		if node.WebSocketURL != expectedWSURL {
			t.Errorf("Expected WebSocket URL %s, got %s", expectedWSURL, node.WebSocketURL)
		}

		t.Logf("✅ WebSocket URLs are correctly auto-generated for Cosmos nodes")
	})

	t.Run("ProductionConfiguration_HighAvailability", func(t *testing.T) {
		// Test high-availability configuration as shown in README
		servers := make([]*httptest.Server, 5)
		urls := make([]string, 5)
		for i := 0; i < 5; i++ {
			servers[i] = createCosmosServer(t, uint64(12345-i), false)
			urls[i] = servers[i].URL
		}
		defer func() {
			for _, server := range servers {
				server.Close()
			}
		}()

		t.Setenv("PROD_COSMOS_RPC_SERVERS", fmt.Sprintf("%s %s %s %s %s", urls[0], urls[1], urls[2], urls[3], urls[4]))

		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				AutoDiscoverFromEnv: "PROD_COSMOS",
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "5s",
				Timeout:       "2s",
				RetryAttempts: 3,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes:         3, // High availability requirement
				CircuitBreakerThreshold: 0.7,
			},
			Performance: PerformanceConfig{
				CacheDuration:       "15s",
				MaxConcurrentChecks: 20,
			},
			Monitoring: MonitoringConfig{
				MetricsEnabled: true,
			},
			logger: logger,
		}

		// Process configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process production environment: %v", err)
		}

		// Set up for testing
		config := &Config{
			Nodes:           upstream.Nodes,
			HealthCheck:     upstream.HealthCheck,
			FailureHandling: upstream.FailureHandling,
			Performance:     upstream.Performance,
			BlockValidation: BlockValidationConfig{
				HeightThreshold:            5,
				ExternalReferenceThreshold: 10,
			},
		}
		upstream.config = config
		upstream.healthChecker = NewHealthChecker(config, NewHealthCache(15*time.Second), NewMetrics(), logger)
		upstream.cache = NewHealthCache(15 * time.Second)
		upstream.metrics = NewMetrics()

		// Test that all nodes are healthy and available
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		if len(upstreams) != 5 {
			t.Errorf("Expected 5 upstreams in production setup, got %d", len(upstreams))
		}

		// Verify minimum healthy nodes requirement
		if len(upstreams) < upstream.config.FailureHandling.MinHealthyNodes {
			t.Errorf("Failed to meet minimum healthy nodes requirement: got %d, need %d",
				len(upstreams), upstream.config.FailureHandling.MinHealthyNodes)
		}

		t.Logf("✅ Production high-availability configuration works correctly")
	})
}

// TestREADMEExamples validates specific examples from the README
func TestREADMEExamples(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("QuickStart_Example", func(t *testing.T) {
		// Test the exact quick start example from README
		cosmosServer1 := createCosmosServer(t, 12345, false)
		cosmosServer2 := createCosmosServer(t, 12344, false)
		cosmosServer3 := createCosmosServer(t, 12343, false)
		defer cosmosServer1.Close()
		defer cosmosServer2.Close()
		defer cosmosServer3.Close()

		// Set environment as shown in README
		cosmosServers := fmt.Sprintf("%s %s %s", cosmosServer1.URL, cosmosServer2.URL, cosmosServer3.URL)
		t.Setenv("COSMOS_SERVERS", cosmosServers)

		// Configuration matching README quick start
		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				ChainPreset: "cosmos-hub",
			},
			Environment: EnvironmentConfig{
				Servers: cosmosServers,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes:         2,
				CircuitBreakerThreshold: 0.8,
			},
			Monitoring: MonitoringConfig{
				MetricsEnabled: true,
			},
			logger: logger,
		}

		// Apply chain preset
		if err := upstream.applyChainPreset("cosmos-hub"); err != nil {
			t.Fatalf("Failed to apply chain preset: %v", err)
		}

		// Process server configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment: %v", err)
		}

		// Verify setup
		if len(upstream.Nodes) != 3 {
			t.Errorf("Expected 3 nodes from COSMOS_SERVERS, got %d", len(upstream.Nodes))
		}

		// Verify chain type was set by preset
		if upstream.Chain.ChainType != "cosmos" {
			t.Errorf("Expected chain type 'cosmos' from preset, got '%s'", upstream.Chain.ChainType)
		}

		t.Logf("✅ README quick start example configuration validated")
	})

	t.Run("Ethereum_Example", func(t *testing.T) {
		// Test the Ethereum configuration example from README
		ethServer1 := createEVMServer(t, 0x12345, false)
		ethServer2 := createEVMServer(t, 0x12344, false)
		defer ethServer1.Close()
		defer ethServer2.Close()

		ethServers := fmt.Sprintf("%s %s", ethServer1.URL, ethServer2.URL)
		t.Setenv("ETH_SERVERS", ethServers)

		upstream := &BlockchainHealthUpstream{
			Chain: ChainConfig{
				ChainPreset: "ethereum",
			},
			Environment: EnvironmentConfig{
				EVMServers: ethServers,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
			Monitoring: MonitoringConfig{
				MetricsEnabled: true,
			},
			logger: logger,
		}

		// Apply Ethereum preset
		if err := upstream.applyChainPreset("ethereum"); err != nil {
			t.Fatalf("Failed to apply ethereum preset: %v", err)
		}

		// Process configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process environment: %v", err)
		}

		// Verify setup
		if len(upstream.Nodes) != 2 {
			t.Errorf("Expected 2 EVM nodes, got %d", len(upstream.Nodes))
		}

		if upstream.Chain.ChainType != "evm" {
			t.Errorf("Expected chain type 'evm' from preset, got '%s'", upstream.Chain.ChainType)
		}

		// Verify all nodes are EVM type
		for _, node := range upstream.Nodes {
			if node.Type != NodeTypeEVM {
				t.Errorf("Expected EVM node type, got %s", node.Type)
			}
		}

		t.Logf("✅ README Ethereum example configuration validated")
	})

	t.Run("Development_Configuration", func(t *testing.T) {
		// Test development configuration from README
		devServer := createCosmosServer(t, 12345, false)
		defer devServer.Close()

		t.Setenv("DEV_SERVERS", devServer.URL)

		upstream := &BlockchainHealthUpstream{
			Environment: EnvironmentConfig{
				Servers: devServer.URL,
			},
			Chain: ChainConfig{
				ChainType: "cosmos",
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "5s",
				Timeout:       "2s",
				RetryAttempts: 1,
			},
			BlockValidation: BlockValidationConfig{
				HeightThreshold: 10, // Relaxed for development
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes:         0, // No minimum in dev
				CircuitBreakerThreshold: 0.9,
			},
			Monitoring: MonitoringConfig{
				LogLevel: "debug",
			},
			logger: logger,
		}

		// Process configuration
		if err := upstream.processEnvironmentConfiguration(); err != nil {
			t.Fatalf("Failed to process dev environment: %v", err)
		}

		// Verify relaxed development settings
		if upstream.FailureHandling.MinHealthyNodes != 0 {
			t.Errorf("Expected min healthy nodes 0 for dev, got %d", upstream.FailureHandling.MinHealthyNodes)
		}

		if upstream.BlockValidation.HeightThreshold != 10 {
			t.Errorf("Expected relaxed height threshold 10 for dev, got %d", upstream.BlockValidation.HeightThreshold)
		}

		t.Logf("✅ README development configuration validated")
	})
}

// Helper functions that create proper protocol-specific test servers

func createCosmosAPIServer(t *testing.T, blockHeight uint64, syncing bool) *httptest.Server {
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/cosmos/base/tendermint/v1beta1/syncing":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{"syncing": %t}`, syncing)
			w.Write([]byte(response))
		case "/cosmos/base/tendermint/v1beta1/blocks/latest":
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{"block": {"header": {"height": "%d"}}}`, blockHeight)
			w.Write([]byte(response))
		default:
			http.NotFound(w, r)
		}
	}))
}

// Helper function to extract host from URL
func extractHostFromURL(rawURL string) string {
	parsedURL, _ := url.Parse(rawURL)
	return parsedURL.Host
}
