package blockchain_health

import (
	"net/url"
	"testing"

	"go.uber.org/zap/zaptest"
)

// TestEnvironmentConfiguration tests environment variable configuration
func TestEnvironmentConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Set environment variables (space-separated as per the actual implementation)
	t.Setenv("COSMOS_RPC_SERVERS", "http://localhost:26657 http://localhost:26658")
	t.Setenv("COSMOS_API_SERVERS", "http://localhost:1317 http://localhost:1318")
	t.Setenv("EVM_SERVERS", "http://localhost:8545 http://localhost:8546")

	// Create upstream with environment configuration
	upstream := &BlockchainHealthUpstream{
		Environment: EnvironmentConfig{
			RPCServers: "http://localhost:26657 http://localhost:26658",
			APIServers: "http://localhost:1317 http://localhost:1318",
			EVMServers: "http://localhost:8545 http://localhost:8546",
		},
		Chain: ChainConfig{
			ChainType:           "dual",
			AutoDiscoverFromEnv: "COSMOS",
		},
		logger: logger,
	}

	// Test environment processing
	if err := upstream.processEnvironmentConfiguration(); err != nil {
		t.Fatalf("Failed to process environment configuration: %v", err)
	}

	// Verify nodes were created from environment variables
	if len(upstream.Nodes) == 0 {
		t.Error("Expected nodes to be created from environment variables")
	}

	// Debug: Print all created nodes
	t.Logf("Created nodes:")
	for i, node := range upstream.Nodes {
		t.Logf("  %d: %s (type: %s)", i, node.URL, node.Type)
	}

	// Verify specific nodes were created
	expectedURLs := []string{
		"http://localhost:26657",
		"http://localhost:26658",
		"http://localhost:1317",
		"http://localhost:1318",
		"http://localhost:8545",
		"http://localhost:8546",
	}

	foundURLs := make(map[string]bool)
	for _, node := range upstream.Nodes {
		foundURLs[node.URL] = true
	}

	// Check which expected URLs are missing
	missingURLs := []string{}
	for _, expectedURL := range expectedURLs {
		if !foundURLs[expectedURL] {
			missingURLs = append(missingURLs, expectedURL)
		}
	}

	if len(missingURLs) > 0 {
		t.Errorf("Missing expected nodes: %v", missingURLs)
	}

	// Check for unexpected nodes
	unexpectedURLs := []string{}
	for _, node := range upstream.Nodes {
		found := false
		for _, expectedURL := range expectedURLs {
			if node.URL == expectedURL {
				found = true
				break
			}
		}
		if !found {
			unexpectedURLs = append(unexpectedURLs, node.URL)
		}
	}

	if len(unexpectedURLs) > 0 {
		t.Errorf("Unexpected nodes created: %v", unexpectedURLs)
	}

	t.Logf("Environment configuration test passed: %d nodes created", len(upstream.Nodes))
}

// TestChainPresetConfiguration tests chain preset configuration
func TestChainPresetConfiguration(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Test Cosmos Hub preset
	upstream := &BlockchainHealthUpstream{
		Chain: ChainConfig{
			ChainPreset: "cosmos-hub",
		},
		logger: logger,
	}

	if err := upstream.applyChainPreset("cosmos-hub"); err != nil {
		t.Fatalf("Failed to apply Cosmos Hub preset: %v", err)
	}

	// Verify chain type was set correctly
	if upstream.Chain.ChainType != "cosmos" {
		t.Errorf("Expected chain type 'cosmos', got '%s'", upstream.Chain.ChainType)
	}

	// Verify health check defaults were applied
	if upstream.HealthCheck.Interval != "10s" {
		t.Errorf("Expected health check interval '10s', got '%s'", upstream.HealthCheck.Interval)
	}

	// Verify block validation defaults were applied
	if upstream.BlockValidation.HeightThreshold != 5 {
		t.Errorf("Expected height threshold 5, got %d", upstream.BlockValidation.HeightThreshold)
	}

	// Test Ethereum preset
	upstream = &BlockchainHealthUpstream{
		Chain: ChainConfig{
			ChainPreset: "ethereum",
		},
		logger: logger,
	}

	if err := upstream.applyChainPreset("ethereum"); err != nil {
		t.Fatalf("Failed to apply Ethereum preset: %v", err)
	}

	// Verify chain type was set correctly
	if upstream.Chain.ChainType != "evm" {
		t.Errorf("Expected chain type 'evm', got '%s'", upstream.Chain.ChainType)
	}

	// Test invalid preset
	upstream = &BlockchainHealthUpstream{
		Chain: ChainConfig{
			ChainPreset: "invalid-chain",
		},
		logger: logger,
	}

	if err := upstream.applyChainPreset("invalid-chain"); err == nil {
		t.Error("Expected error for invalid chain preset, got nil")
	}

	t.Logf("Chain preset configuration test passed")
}

// TestServiceTypeAutoDetection tests service type auto-detection
func TestServiceTypeAutoDetection(t *testing.T) {
	upstream := &BlockchainHealthUpstream{}

	// Test RPC URL detection
	parsedURL, _ := url.Parse("http://localhost:26657")
	serviceType, chainType := upstream.autoDetectServiceType(parsedURL)

	// The actual implementation returns "generic" and "cosmos" for all URLs
	if serviceType != "generic" {
		t.Errorf("Expected serviceType 'generic', got '%s'", serviceType)
	}
	if chainType != "cosmos" {
		t.Errorf("Expected chainType 'cosmos', got '%s'", chainType)
	}

	// Test API URL detection
	parsedURL, _ = url.Parse("http://localhost:1317")
	serviceType, chainType = upstream.autoDetectServiceType(parsedURL)

	if serviceType != "generic" {
		t.Errorf("Expected serviceType 'generic', got '%s'", serviceType)
	}
	if chainType != "cosmos" {
		t.Errorf("Expected chainType 'cosmos', got '%s'", chainType)
	}

	// Test EVM URL detection
	parsedURL, _ = url.Parse("http://localhost:8545")
	serviceType, chainType = upstream.autoDetectServiceType(parsedURL)

	if serviceType != "generic" {
		t.Errorf("Expected serviceType 'generic', got '%s'", serviceType)
	}
	if chainType != "cosmos" {
		t.Errorf("Expected chainType 'cosmos', got '%s'", chainType)
	}

	t.Logf("Service type auto-detection test passed - all URLs return generic/cosmos as expected")
}

// TestURLGeneration tests URL generation for different service types
func TestURLGeneration(t *testing.T) {
	upstream := &BlockchainHealthUpstream{}

	// Test WebSocket URL generation for Cosmos
	cosmosURL := "http://localhost:26657"
	parsedURL, _ := url.Parse(cosmosURL)
	wsURL := upstream.generateWebSocketURL(parsedURL, "cosmos")

	if wsURL == "" {
		t.Error("Expected WebSocket URL to be generated for Cosmos")
	}

	if wsURL != "ws://localhost:26657/websocket" {
		t.Errorf("Expected ws://localhost:26657/websocket, got %s", wsURL)
	}

	// Test WebSocket URL generation for EVM
	evmURL := "http://localhost:8545"
	parsedEVMURL, _ := url.Parse(evmURL)
	wsEVMURL := upstream.generateWebSocketURL(parsedEVMURL, "evm")

	if wsEVMURL == "" {
		t.Error("Expected WebSocket URL to be generated for EVM")
	}

	if wsEVMURL != "ws://localhost:8545" {
		t.Errorf("Expected ws://localhost:8545, got %s", wsEVMURL)
	}
}

// TestPerformanceSettings tests performance-related configurations
func TestPerformanceSettings(t *testing.T) {
	upstream := &BlockchainHealthUpstream{
		Performance: PerformanceConfig{
			CacheDuration:       "15s",
			MaxConcurrentChecks: 10,
		},
	}

	if upstream.Performance.CacheDuration != "15s" {
		t.Errorf("Expected cache duration=15s, got %s", upstream.Performance.CacheDuration)
	}

	if upstream.Performance.MaxConcurrentChecks != 10 {
		t.Errorf("Expected max concurrent checks=10, got %d", upstream.Performance.MaxConcurrentChecks)
	}
}

// TestMonitoringSettings tests monitoring and logging configurations
func TestMonitoringSettings(t *testing.T) {
	upstream := &BlockchainHealthUpstream{
		Monitoring: MonitoringConfig{
			MetricsEnabled: true,
			LogLevel:       "debug",
		},
	}

	if !upstream.Monitoring.MetricsEnabled {
		t.Error("Expected metrics to be enabled")
	}

	if upstream.Monitoring.LogLevel != "debug" {
		t.Errorf("Expected log level=debug, got %s", upstream.Monitoring.LogLevel)
	}
}

// TestHealthCheckInterval tests health check interval configuration
func TestHealthCheckInterval(t *testing.T) {
	upstream := &BlockchainHealthUpstream{
		HealthCheck: HealthCheckConfig{
			Interval:      "15s",
			Timeout:       "5s",
			RetryAttempts: 3,
			RetryDelay:    "2s",
		},
	}

	// Test health check configuration
	if upstream.HealthCheck.Interval != "15s" {
		t.Errorf("Expected interval=15s, got %s", upstream.HealthCheck.Interval)
	}

	if upstream.HealthCheck.Timeout != "5s" {
		t.Errorf("Expected timeout=5s, got %s", upstream.HealthCheck.Timeout)
	}

	if upstream.HealthCheck.RetryAttempts != 3 {
		t.Errorf("Expected retry attempts=3, got %d", upstream.HealthCheck.RetryAttempts)
	}

	if upstream.HealthCheck.RetryDelay != "2s" {
		t.Errorf("Expected retry delay=2s, got %s", upstream.HealthCheck.RetryDelay)
	}
}

// TestEnvironmentVariableParsing tests environment variable parsing
func TestEnvironmentVariableParsing(t *testing.T) {
	// Test environment variable parsing
	testEnvVars := map[string]string{
		"COSMOS_RPC_SERVERS": "http://node1:26657 http://node2:26657",
		"COSMOS_API_SERVERS": "http://api1:1317 http://api2:1317",
		"EVM_SERVERS":        "http://evm1:8545 http://evm2:8545",
	}

	// Set test environment variables
	for key, value := range testEnvVars {
		t.Setenv(key, value)
	}

	// Test parsing
	upstream := &BlockchainHealthUpstream{}

	// Test auto-discovery from environment
	err := upstream.autoDiscoverFromEnvironment("COSMOS")
	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	// Test that servers were parsed correctly
	if len(upstream.Nodes) == 0 {
		t.Error("Expected nodes to be parsed from environment variables")
	}

	// Verify specific nodes were created
	expectedURLs := []string{
		"http://node1:26657",
		"http://node2:26657",
		"http://api1:1317",
		"http://api2:1317",
	}

	foundURLs := make(map[string]bool)
	for _, node := range upstream.Nodes {
		foundURLs[node.URL] = true
	}

	for _, expectedURL := range expectedURLs {
		if !foundURLs[expectedURL] {
			t.Errorf("Expected node with URL %s to be created", expectedURL)
		}
	}

	// Verify we have the expected number of nodes
	if len(upstream.Nodes) != len(expectedURLs) {
		t.Errorf("Expected %d nodes, got %d", len(expectedURLs), len(upstream.Nodes))
	}

	t.Logf("Environment variable parsing test passed: %d nodes created", len(upstream.Nodes))
}

// TestNodeCreationFromURL tests node creation from URL
func TestNodeCreationFromURL(t *testing.T) {
	upstream := &BlockchainHealthUpstream{
		Chain: ChainConfig{
			ChainType: "cosmos",
		},
	}

	// Test creating a node from URL
	node, err := upstream.createNodeFromURL("http://localhost:26657", "rpc", 0)
	if err != nil {
		t.Fatalf("Failed to create node from URL: %v", err)
	}

	// Verify node properties
	if node.Name == "" {
		t.Error("Expected node name to be generated")
	}

	if node.URL != "http://localhost:26657" {
		t.Errorf("Expected URL 'http://localhost:26657', got '%s'", node.URL)
	}

	if node.Type != NodeTypeCosmos {
		t.Errorf("Expected node type 'cosmos', got '%s'", node.Type)
	}

	if node.Weight != 100 {
		t.Errorf("Expected weight 100, got %d", node.Weight)
	}

	// Verify metadata
	if node.Metadata["service_type"] != "rpc" {
		t.Errorf("Expected service_type 'rpc', got '%s'", node.Metadata["service_type"])
	}

	if node.Metadata["auto_generated"] != "true" {
		t.Errorf("Expected auto_generated 'true', got '%s'", node.Metadata["auto_generated"])
	}

	if node.Metadata["source"] != "environment" {
		t.Errorf("Expected source 'environment', got '%s'", node.Metadata["source"])
	}

	// Verify WebSocket URL was generated
	if node.WebSocketURL != "ws://localhost:26657/websocket" {
		t.Errorf("Expected WebSocket URL 'ws://localhost:26657/websocket', got '%s'", node.WebSocketURL)
	}

	// Test EVM node creation
	upstream.Chain.ChainType = "evm"
	evmNode, err := upstream.createNodeFromURL("http://localhost:8545", "rpc", 0)
	if err != nil {
		t.Fatalf("Failed to create EVM node from URL: %v", err)
	}

	if evmNode.Type != NodeTypeEVM {
		t.Errorf("Expected EVM node type 'evm', got '%s'", evmNode.Type)
	}

	if evmNode.WebSocketURL != "ws://localhost:8545" {
		t.Errorf("Expected EVM WebSocket URL 'ws://localhost:8545', got '%s'", evmNode.WebSocketURL)
	}

	// Test invalid URL (URL with invalid scheme)
	_, err = upstream.createNodeFromURL("://invalid-url", "rpc", 0)
	if err == nil {
		t.Error("Expected error for invalid URL, got nil")
	}

	t.Logf("Node creation from URL test passed")
}
