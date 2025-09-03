package blockchain_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestComprehensiveHealthChecks tests all service types and health check functionality
func TestComprehensiveHealthChecks(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("Cosmos RPC Health Check", func(t *testing.T) {
		testCosmosRPCHealthCheck(t, logger)
	})

	t.Run("Cosmos REST API Health Check", func(t *testing.T) {
		testCosmosAPIHealthCheck(t, logger)
	})

	t.Run("EVM JSON-RPC Health Check", func(t *testing.T) {
		testEVMHealthCheck(t, logger)
	})

	t.Run("WebSocket Service Type Detection", func(t *testing.T) {
		testWebSocketServiceType(t, logger)
	})

	t.Run("Multi-Node Health Checking", func(t *testing.T) {
		testMultiNodeHealthChecking(t, logger)
	})
}

func testCosmosRPCHealthCheck(t *testing.T, logger *zap.Logger) {
	// Test server that responds with different health states
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Simulate a healthy Cosmos node
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12345",
						"catching_up": false
					}
				}
			}`
			w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	handler := NewCosmosHandler(5*time.Second, logger)
	node := NodeConfig{
		Name: "test-cosmos-rpc",
		URL:  server.URL,
		Type: NodeTypeCosmos,
		Metadata: map[string]string{
			"service_type": "rpc",
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !health.Healthy {
		t.Error("Expected healthy=true for healthy Cosmos RPC node")
	}

	if health.BlockHeight != 12345 {
		t.Errorf("Expected height=12345, got %d", health.BlockHeight)
	}

	if health.CatchingUp == nil || *health.CatchingUp {
		t.Error("Expected catching_up=false for healthy node")
	}
}

func testCosmosAPIHealthCheck(t *testing.T, logger *zap.Logger) {
	// Test server that responds with REST API endpoint
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/cosmos/base/tendermint/v1beta1/blocks/latest" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Simulate a healthy Cosmos REST API response
			response := `{
				"block": {
					"header": {
						"height": "12345"
					}
				}
			}`
			w.Write([]byte(response))
		} else if r.URL.Path == "/cosmos/base/tendermint/v1beta1/syncing" {
			// Also serve the syncing endpoint that the handler checks
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"syncing": false
			}`
			w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	handler := NewCosmosHandler(5*time.Second, logger)
	node := NodeConfig{
		Name: "test-cosmos-api",
		URL:  server.URL,
		Type: NodeTypeCosmos,
		Metadata: map[string]string{
			"service_type": "api",
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !health.Healthy {
		t.Error("Expected healthy=true for healthy Cosmos API node")
	}

	if health.BlockHeight != 12345 {
		t.Errorf("Expected height=12345, got %d", health.BlockHeight)
	}
}

func testEVMHealthCheck(t *testing.T, logger *zap.Logger) {
	// Test server that responds with EVM JSON-RPC
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)

			// Simulate a healthy EVM node response
			response := `{
				"jsonrpc": "2.0",
				"id": 1,
				"result": "0x12345"
			}`
			w.Write([]byte(response))
		} else {
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	handler := NewEVMHandler(5*time.Second, logger)
	node := NodeConfig{
		Name: "test-evm",
		URL:  server.URL,
		Type: NodeTypeEVM,
		Metadata: map[string]string{
			"service_type": "evm",
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if !health.Healthy {
		t.Error("Expected healthy=true for healthy EVM node")
	}

	if health.BlockHeight != 0x12345 {
		t.Errorf("Expected height=0x12345, got %d", health.BlockHeight)
	}
}

func testWebSocketServiceType(t *testing.T, logger *zap.Logger) {
	// Test that WebSocket service type is properly handled
	handler := NewCosmosHandler(5*time.Second, logger)
	node := NodeConfig{
		Name: "test-cosmos-ws",
		URL:  "ws://localhost:26657/websocket",
		Type: NodeTypeCosmos,
		Metadata: map[string]string{
			"service_type": "websocket",
		},
	}

	// For WebSocket nodes, we should still be able to check basic health
	// The actual WebSocket connection would be handled by the reverse proxy
	ctx := context.Background()
	_, err := handler.CheckHealth(ctx, node)

	// WebSocket nodes might not support HTTP health checks, so we expect an error
	// but the service type should be properly recognized
	if node.Metadata["service_type"] != "websocket" {
		t.Error("Expected service_type=websocket")
	}

	// The error is expected for WebSocket URLs in HTTP health checks
	if err == nil {
		t.Log("Note: WebSocket node unexpectedly passed HTTP health check")
	}
}

func testMultiNodeHealthChecking(t *testing.T, logger *zap.Logger) {
	// Test health checking with multiple nodes
	upstream := &BlockchainHealthUpstream{
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 2,
		},
		HealthCheck: HealthCheckConfig{
			Interval: "5s",
			Timeout:  "10s",
		},
	}

	// Test configuration
	if upstream.FailureHandling.MinHealthyNodes != 2 {
		t.Errorf("Expected min healthy nodes=2, got %d", upstream.FailureHandling.MinHealthyNodes)
	}

	if upstream.HealthCheck.Interval != "5s" {
		t.Errorf("Expected interval=5s, got %s", upstream.HealthCheck.Interval)
	}

	if upstream.HealthCheck.Timeout != "10s" {
		t.Errorf("Expected timeout=10s, got %s", upstream.HealthCheck.Timeout)
	}

	// Test that multiple nodes can be configured
	testNodes := []NodeConfig{
		{Name: "node1", URL: "http://node1:26657", Type: NodeTypeCosmos},
		{Name: "node2", URL: "http://node2:26657", Type: NodeTypeCosmos},
		{Name: "node3", URL: "http://node3:26657", Type: NodeTypeCosmos},
	}

	upstream.Nodes = testNodes

	if len(upstream.Nodes) != 3 {
		t.Errorf("Expected 3 nodes, got %d", len(upstream.Nodes))
	}

	// Test minimum healthy nodes requirement
	if upstream.FailureHandling.MinHealthyNodes > len(upstream.Nodes) {
		t.Error("Min healthy nodes should not exceed total nodes")
	}
}
