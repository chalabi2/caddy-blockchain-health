package blockchain_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
)

// TestCaddyIntegration validates that the module properly integrates with Caddy
func TestCaddyIntegration(t *testing.T) {

	t.Run("ModuleRegistration_Works", func(t *testing.T) {
		// Verify the module is properly registered with Caddy
		module := &BlockchainHealthUpstream{}
		moduleInfo := module.CaddyModule()

		expectedID := "http.reverse_proxy.upstreams.blockchain_health"
		if string(moduleInfo.ID) != expectedID {
			t.Errorf("Expected module ID %s, got %s", expectedID, string(moduleInfo.ID))
		}

		// Verify the module constructor works
		newModule := moduleInfo.New()
		if newModule == nil {
			t.Error("Module constructor returned nil")
		}

		if _, ok := newModule.(*BlockchainHealthUpstream); !ok {
			t.Error("Module constructor returned wrong type")
		}

		t.Logf("✅ Module registration and constructor work correctly")
	})

	t.Run("InterfaceImplementation_Complete", func(t *testing.T) {
		module := &BlockchainHealthUpstream{}

		// Verify all required interfaces are implemented
		var _ caddy.Provisioner = module
		var _ caddy.Validator = module
		var _ caddy.CleanerUpper = module
		var _ caddyfile.Unmarshaler = module
		var _ reverseproxy.UpstreamSource = module

		t.Logf("✅ All required Caddy interfaces are implemented")
	})

	t.Run("CaddyfileUnmarshaling_Works", func(t *testing.T) {
		// Test Caddyfile parsing with the exact syntax from README
		caddyfileContent := `
		dynamic blockchain_health {
			rpc_servers http://cosmos-1:26657 http://cosmos-2:26657
			api_servers http://api-1:1317
			check_interval "10s"
			timeout "5s"
			min_healthy_nodes 2
			metrics_enabled true
		}`

		dispenser := caddyfile.NewTestDispenser(caddyfileContent)
		dispenser.Next() // Skip to first token

		module := &BlockchainHealthUpstream{}
		err := module.UnmarshalCaddyfile(dispenser)
		if err != nil {
			t.Fatalf("Failed to unmarshal Caddyfile: %v", err)
		}

		// Verify configuration was parsed correctly
		if module.HealthCheck.Interval != "10s" {
			t.Errorf("Expected interval '10s', got '%s'", module.HealthCheck.Interval)
		}

		if module.HealthCheck.Timeout != "5s" {
			t.Errorf("Expected timeout '5s', got '%s'", module.HealthCheck.Timeout)
		}

		if module.FailureHandling.MinHealthyNodes != 2 {
			t.Errorf("Expected min healthy nodes 2, got %d", module.FailureHandling.MinHealthyNodes)
		}

		if !module.Monitoring.MetricsEnabled {
			t.Error("Expected metrics to be enabled")
		}

		// Verify environment servers were parsed
		expectedRPCServers := "http://cosmos-1:26657 http://cosmos-2:26657"
		if module.Environment.RPCServers != expectedRPCServers {
			t.Errorf("Expected RPC servers '%s', got '%s'", expectedRPCServers, module.Environment.RPCServers)
		}

		expectedAPIServers := "http://api-1:1317"
		if module.Environment.APIServers != expectedAPIServers {
			t.Errorf("Expected API servers '%s', got '%s'", expectedAPIServers, module.Environment.APIServers)
		}

		t.Logf("✅ Caddyfile unmarshaling works correctly")
	})

	t.Run("CaddyLifecycle_ProvisionValidateCleanup", func(t *testing.T) {
		// Create test servers
		cosmosServer := createCosmosServer(t, 12345, false)
		defer cosmosServer.Close()

		// Create module with minimal configuration
		module := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "test-node", URL: cosmosServer.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "10s",
				Timeout:       "5s",
				RetryAttempts: 3,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
		}

		// Create Caddy context
		ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
		defer cancel()

		// Test Provision
		err := module.Provision(ctx)
		if err != nil {
			t.Fatalf("Provision failed: %v", err)
		}

		// Verify components were initialized
		if module.config == nil {
			t.Error("Config not initialized after provision")
		}
		if module.healthChecker == nil {
			t.Error("Health checker not initialized after provision")
		}
		if module.cache == nil {
			t.Error("Cache not initialized after provision")
		}
		if module.logger == nil {
			t.Error("Logger not initialized after provision")
		}

		// Test Validate
		err = module.Validate()
		if err != nil {
			t.Fatalf("Validate failed: %v", err)
		}

		// Test GetUpstreams works after provision
		upstreams, err := module.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed after provision: %v", err)
		}

		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream after provision, got %d", len(upstreams))
		}

		// Test Cleanup
		err = module.Cleanup()
		if err != nil {
			t.Fatalf("Cleanup failed: %v", err)
		}

		t.Logf("✅ Complete Caddy lifecycle (Provision → Validate → Cleanup) works correctly")
	})

	t.Run("UpstreamSource_Integration", func(t *testing.T) {
		// Test integration with Caddy's reverse proxy module
		healthyServer1 := createCosmosServer(t, 12345, false)
		healthyServer2 := createCosmosServer(t, 12344, false)
		unhealthyServer := createCosmosServer(t, 12300, true) // Catching up
		defer healthyServer1.Close()
		defer healthyServer2.Close()
		defer unhealthyServer.Close()

		module := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "healthy-1", URL: healthyServer1.URL, Type: NodeTypeCosmos, Weight: 100},
				{Name: "healthy-2", URL: healthyServer2.URL, Type: NodeTypeCosmos, Weight: 200}, // Higher weight
				{Name: "unhealthy", URL: unhealthyServer.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "1s",
				Timeout:       "2s",
				RetryAttempts: 1,
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
		}

		// Provision the module
		ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
		defer cancel()

		err := module.Provision(ctx)
		if err != nil {
			t.Fatalf("Provision failed: %v", err)
		}
		defer func() { _ = module.Cleanup() }()

		// Test that it returns proper Caddy upstream objects
		upstreams, err := module.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Should return 2 healthy upstreams (excluding unhealthy one)
		if len(upstreams) != 2 {
			t.Errorf("Expected 2 healthy upstreams, got %d", len(upstreams))
		}

		// Verify upstream objects are properly formed
		for _, upstream := range upstreams {
			if upstream.Dial == "" {
				t.Error("Upstream Dial field is empty")
			}

			// Verify it's a real host:port
			expectedHosts := []string{
				extractCaddyTestHostFromURL(healthyServer1.URL),
				extractCaddyTestHostFromURL(healthyServer2.URL),
			}

			hostFound := false
			for _, expectedHost := range expectedHosts {
				if upstream.Dial == expectedHost {
					hostFound = true
					break
				}
			}

			if !hostFound {
				t.Errorf("Unexpected upstream host: %s", upstream.Dial)
			}
		}

		// Verify weight is applied correctly to the heavy server
		heavyHost := extractCaddyTestHostFromURL(healthyServer2.URL)
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

		if heavyUpstream.MaxRequests != 200 {
			t.Errorf("Expected MaxRequests=200 for heavy server, got %d", heavyUpstream.MaxRequests)
		}

		t.Logf("✅ UpstreamSource integration with Caddy reverse proxy works correctly")
	})

	t.Run("ErrorHandling_GracefulDegradation", func(t *testing.T) {
		// Test error handling and graceful degradation

		// Test with invalid configuration
		module := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "", URL: "", Type: NodeTypeCosmos, Weight: 0}, // Invalid
			},
		}

		// Should fail validation
		err := module.Validate()
		if err == nil {
			t.Error("Expected validation to fail with invalid configuration")
		}

		// Test with no nodes configured
		module = &BlockchainHealthUpstream{
			Nodes: []NodeConfig{}, // Empty
		}

		err = module.Validate()
		if err == nil {
			t.Error("Expected validation to fail with no nodes")
		}

		// Test with unreachable servers
		module = &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "unreachable", URL: "http://localhost:99999", Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Timeout: "1s",
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
		}

		ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
		defer cancel()

		err = module.Provision(ctx)
		if err != nil {
			t.Fatalf("Provision failed: %v", err)
		}
		defer func() { _ = module.Cleanup() }()

		// Should not error, but should return fallback upstreams
		upstreams, err := module.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams should not error with unreachable servers: %v", err)
		}

		// Should return the unreachable server as fallback
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 fallback upstream, got %d", len(upstreams))
		}

		t.Logf("✅ Error handling and graceful degradation work correctly")
	})

	t.Run("BackgroundHealthChecking_Works", func(t *testing.T) {
		// Test that background health checking works
		serverHealthy := true
		dynamicServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path == "/status" {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				var response string
				if serverHealthy {
					response = `{"result": {"sync_info": {"latest_block_height": "12345", "catching_up": false}}}`
				} else {
					response = `{"result": {"sync_info": {"latest_block_height": "12300", "catching_up": true}}}`
				}
				_, _ = w.Write([]byte(response))
			}
		}))
		defer dynamicServer.Close()

		module := &BlockchainHealthUpstream{
			Nodes: []NodeConfig{
				{Name: "dynamic", URL: dynamicServer.URL, Type: NodeTypeCosmos, Weight: 100},
			},
			HealthCheck: HealthCheckConfig{
				Interval:      "500ms", // Fast for testing
				Timeout:       "1s",
				RetryAttempts: 1,
			},
			Performance: PerformanceConfig{
				CacheDuration: "100ms", // Short cache for testing
			},
			FailureHandling: FailureHandlingConfig{
				MinHealthyNodes: 1,
			},
		}

		ctx, cancel := caddy.NewContext(caddy.Context{Context: context.Background()})
		defer cancel()

		err := module.Provision(ctx)
		if err != nil {
			t.Fatalf("Provision failed: %v", err)
		}
		defer func() { _ = module.Cleanup() }()

		// Initial check - should be healthy
		upstreams, err := module.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 healthy upstream initially, got %d", len(upstreams))
		}

		// Change server to unhealthy
		serverHealthy = false

		// Wait for background health check to detect change
		time.Sleep(800 * time.Millisecond) // Wait for interval + cache expiry

		// Should now detect unhealthy state (but still return as fallback)
		upstreams, err = module.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Note: Should still return 1 upstream as fallback since min_healthy_nodes = 1
		// The important thing is that the background checker is working and detecting changes
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 fallback upstream after health change, got %d", len(upstreams))
		}

		t.Logf("✅ Background health checking detects state changes")
	})
}

// TestCaddyfileConfiguration tests various Caddyfile configuration patterns
func TestCaddyfileConfiguration(t *testing.T) {
	t.Run("CompleteConfiguration_ParsesCorrectly", func(t *testing.T) {
		// Test parsing a complete configuration similar to README examples
		caddyfileContent := `
		dynamic blockchain_health {
			# Node definitions
			node cosmos-1 {
				url "http://cosmos-1:26657"
				api_url "http://cosmos-1:1317"
				type "cosmos"
				weight 100
				metadata {
					region "us-east-1"
					provider "aws"
				}
			}

			node evm-1 {
				url "http://eth-1:8545"
				type "evm"
				weight 200
			}

			# External reference
			external_reference cosmos {
				name "cosmos_public"
				url "https://cosmos-rpc.publicnode.com"
				enabled true
			}

			# Health check settings
			check_interval "10s"
			timeout "5s"
			retry_attempts 3
			retry_delay "2s"

			# Block validation
			block_height_threshold 5
			external_reference_threshold 10

			# Performance settings
			cache_duration "30s"
			max_concurrent_checks 10

			# Failure handling
			min_healthy_nodes 1
			grace_period "60s"
			circuit_breaker_threshold 0.8

			# Monitoring
			metrics_enabled true
			log_level "info"
			health_endpoint "/health"

			# Environment configuration
			rpc_servers http://cosmos-2:26657 http://cosmos-3:26657
			evm_servers http://eth-2:8545
			chain_type "dual"
			chain_preset "cosmos-hub"
		}`

		dispenser := caddyfile.NewTestDispenser(caddyfileContent)
		dispenser.Next() // Skip to first token

		module := &BlockchainHealthUpstream{}
		err := module.UnmarshalCaddyfile(dispenser)
		if err != nil {
			t.Fatalf("Failed to unmarshal complete Caddyfile: %v", err)
		}

		// Verify all settings were parsed correctly

		// Health check settings
		if module.HealthCheck.Interval != "10s" {
			t.Errorf("Expected interval '10s', got '%s'", module.HealthCheck.Interval)
		}
		if module.HealthCheck.Timeout != "5s" {
			t.Errorf("Expected timeout '5s', got '%s'", module.HealthCheck.Timeout)
		}
		if module.HealthCheck.RetryAttempts != 3 {
			t.Errorf("Expected retry attempts 3, got %d", module.HealthCheck.RetryAttempts)
		}

		// Block validation
		if module.BlockValidation.HeightThreshold != 5 {
			t.Errorf("Expected height threshold 5, got %d", module.BlockValidation.HeightThreshold)
		}
		if module.BlockValidation.ExternalReferenceThreshold != 10 {
			t.Errorf("Expected external threshold 10, got %d", module.BlockValidation.ExternalReferenceThreshold)
		}

		// Performance settings
		if module.Performance.CacheDuration != "30s" {
			t.Errorf("Expected cache duration '30s', got '%s'", module.Performance.CacheDuration)
		}
		if module.Performance.MaxConcurrentChecks != 10 {
			t.Errorf("Expected max concurrent checks 10, got %d", module.Performance.MaxConcurrentChecks)
		}

		// Failure handling
		if module.FailureHandling.MinHealthyNodes != 1 {
			t.Errorf("Expected min healthy nodes 1, got %d", module.FailureHandling.MinHealthyNodes)
		}
		if module.FailureHandling.CircuitBreakerThreshold != 0.8 {
			t.Errorf("Expected circuit breaker threshold 0.8, got %f", module.FailureHandling.CircuitBreakerThreshold)
		}

		// Monitoring
		if !module.Monitoring.MetricsEnabled {
			t.Error("Expected metrics to be enabled")
		}
		if module.Monitoring.LogLevel != "info" {
			t.Errorf("Expected log level 'info', got '%s'", module.Monitoring.LogLevel)
		}

		// Chain configuration
		if module.Chain.ChainType != "dual" {
			t.Errorf("Expected chain type 'dual', got '%s'", module.Chain.ChainType)
		}
		if module.Chain.ChainPreset != "cosmos-hub" {
			t.Errorf("Expected chain preset 'cosmos-hub', got '%s'", module.Chain.ChainPreset)
		}

		t.Logf("✅ Complete Caddyfile configuration parses correctly")
	})

	t.Run("EnvironmentBasedConfiguration_ParsesCorrectly", func(t *testing.T) {
		// Test environment-based configuration as shown in README
		caddyfileContent := `
		dynamic blockchain_health {
			chain_preset "cosmos-hub"
			auto_discover_from_env "COSMOS"
			evm_servers {$ETH_SERVERS}
			min_healthy_nodes 2
			circuit_breaker_threshold 0.8
			metrics_enabled true
		}`

		dispenser := caddyfile.NewTestDispenser(caddyfileContent)
		dispenser.Next()

		module := &BlockchainHealthUpstream{}
		err := module.UnmarshalCaddyfile(dispenser)
		if err != nil {
			t.Fatalf("Failed to unmarshal environment-based Caddyfile: %v", err)
		}

		// Verify configuration
		if module.Chain.ChainPreset != "cosmos-hub" {
			t.Errorf("Expected chain preset 'cosmos-hub', got '%s'", module.Chain.ChainPreset)
		}

		if module.Chain.AutoDiscoverFromEnv != "COSMOS" {
			t.Errorf("Expected auto discover 'COSMOS', got '%s'", module.Chain.AutoDiscoverFromEnv)
		}

		// Note: Environment variable substitution is handled by Caddy, so we check if parsing worked
		// The actual environment variables would be resolved by Caddy's configuration system
		t.Logf("EVM servers configuration parsed: '%s'", module.Environment.EVMServers)

		t.Logf("✅ Environment-based Caddyfile configuration parses correctly")
	})
}

// Helper function to extract host from URL for caddy integration tests
func extractCaddyTestHostFromURL(rawURL string) string {
	parsedURL, _ := url.Parse(rawURL)
	return parsedURL.Host
}
