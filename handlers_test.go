package blockchain_health

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

func TestCosmosHandler_CheckHealth(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name               string
		response           string
		expectedHealthy    bool
		expectedHeight     uint64
		expectedCatchingUp *bool
	}{
		{
			name: "healthy node",
			response: `{
				"result": {
					"sync_info": {
						"latest_block_height": "12345",
						"catching_up": false
					}
				}
			}`,
			expectedHealthy:    true,
			expectedHeight:     12345,
			expectedCatchingUp: boolPtr(false),
		},
		{
			name: "catching up node",
			response: `{
				"result": {
					"sync_info": {
						"latest_block_height": "12300",
						"catching_up": true
					}
				}
			}`,
			expectedHealthy:    false,
			expectedHeight:     12300,
			expectedCatchingUp: boolPtr(true),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.URL.Path == "/status" {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(tt.response))
				} else {
					http.NotFound(w, r)
				}
			}))
			defer server.Close()

			handler := NewCosmosHandler(5*time.Second, logger)
			node := NodeConfig{
				Name: "test-node",
				URL:  server.URL,
				Type: NodeTypeCosmos,
			}

			ctx := context.Background()
			health, err := handler.CheckHealth(ctx, node)

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if health.Healthy != tt.expectedHealthy {
				t.Errorf("Expected healthy=%v, got %v", tt.expectedHealthy, health.Healthy)
			}

			if health.BlockHeight != tt.expectedHeight {
				t.Errorf("Expected height=%d, got %d", tt.expectedHeight, health.BlockHeight)
			}

			if tt.expectedCatchingUp != nil && (health.CatchingUp == nil || *health.CatchingUp != *tt.expectedCatchingUp) {
				t.Errorf("Expected catching_up=%v, got %v", *tt.expectedCatchingUp, health.CatchingUp)
			}
		})
	}
}

func TestEVMHandler_CheckHealth(t *testing.T) {
	logger := zaptest.NewLogger(t)

	tests := []struct {
		name            string
		response        string
		expectedHealthy bool
		expectedHeight  uint64
	}{
		{
			name: "healthy EVM node",
			response: `{
				"jsonrpc": "2.0",
				"id": 1,
				"result": "0x12d687"
			}`,
			expectedHealthy: true,
			expectedHeight:  1234567,
		},
		{
			name: "EVM error response",
			response: `{
				"jsonrpc": "2.0",
				"id": 1,
				"error": {
					"code": -32000,
					"message": "Server error"
				}
			}`,
			expectedHealthy: false,
			expectedHeight:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create test server
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write([]byte(tt.response))
			}))
			defer server.Close()

			handler := NewEVMHandler(5*time.Second, logger)
			node := NodeConfig{
				Name: "test-node",
				URL:  server.URL,
				Type: NodeTypeEVM,
			}

			ctx := context.Background()
			health, err := handler.CheckHealth(ctx, node)

			if err != nil {
				t.Fatalf("Expected no error, got %v", err)
			}

			if health.Healthy != tt.expectedHealthy {
				t.Errorf("Expected healthy=%v, got %v", tt.expectedHealthy, health.Healthy)
			}

			if health.BlockHeight != tt.expectedHeight {
				t.Errorf("Expected height=%d, got %d", tt.expectedHeight, health.BlockHeight)
			}
		})
	}
}

func TestCosmosHandler_GetBlockHeight(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"result": {
				"sync_info": {
					"latest_block_height": "999999",
					"catching_up": false
				}
			}
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	handler := NewCosmosHandler(5*time.Second, logger)

	ctx := context.Background()
	height, err := handler.GetBlockHeight(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if height != 999999 {
		t.Errorf("Expected height=999999, got %d", height)
	}
}

func TestEVMHandler_GetBlockHeight(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		response := `{
			"jsonrpc": "2.0",
			"id": 1,
			"result": "0xf4240"
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	defer server.Close()

	handler := NewEVMHandler(5*time.Second, logger)

	ctx := context.Background()
	height, err := handler.GetBlockHeight(ctx, server.URL)

	if err != nil {
		t.Fatalf("Expected no error, got %v", err)
	}

	if height != 1000000 {
		t.Errorf("Expected height=1000000, got %d", height)
	}
}

// Helper function to create bool pointer
func boolPtr(b bool) *bool {
	return &b
}

func TestEVMHandler_CheckHealth_WebSocketOnly(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewEVMHandler(5*time.Second, logger)

	// Test WebSocket-only node (service_type = "websocket")
	node := NodeConfig{
		Name: "test-evm-ws",
		URL:  "ws://localhost:8546", // WebSocket URL
		Type: NodeTypeEVM,
		Metadata: map[string]string{
			"service_type": "websocket",
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	// Should not return an error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Health should be returned (even if unhealthy due to no actual WebSocket server)
	if health == nil {
		t.Fatal("Expected health result, got nil")
	}

	// Verify it's recognized as WebSocket-only node
	if health.BlockHeight != 0 {
		t.Errorf("Expected block height 0 for WebSocket-only node, got %d", health.BlockHeight)
	}

	// The node will be unhealthy since there's no actual WebSocket server running
	// but it should not crash or try HTTP requests
	if health.LastError != "WebSocket connection failed" && health.Healthy {
		t.Logf("Note: WebSocket connection unexpectedly succeeded (health: %v, error: %s)",
			health.Healthy, health.LastError)
	}

	t.Logf("✅ EVM WebSocket-only node handled correctly: healthy=%v, error=%s",
		health.Healthy, health.LastError)
}

func TestEVMHandler_WebSocketWithHTTPCorrelation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewEVMHandler(5*time.Second, logger)

	// Mock HTTP server for health checks (simulating the correlated HTTP endpoint)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Simulate successful eth_blockNumber response
		response := `{
			"jsonrpc": "2.0",
			"result": "0x12345",
			"id": 1
		}`
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(response))
	}))
	defer httpServer.Close()

	// Test WebSocket node with correlated HTTP URL in metadata
	// This simulates the scenario where:
	// BASE_SERVERS="http://node:13245"
	// BASE_WS_SERVERS="http://node:13246"
	// The WebSocket node gets the HTTP URL in metadata for health checks
	node := NodeConfig{
		Name: "test-evm-ws-correlated",
		URL:  "ws://localhost:13246", // WebSocket URL (would be converted from http://localhost:13246)
		Type: NodeTypeEVM,
		Metadata: map[string]string{
			"service_type": "websocket",
			"http_url":     httpServer.URL, // Correlated HTTP URL for health checks
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	// Should not return an error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Health should be returned
	if health == nil {
		t.Fatal("Expected health result, got nil")
	}

	// Should be healthy since HTTP endpoint is working
	if !health.Healthy {
		t.Errorf("Expected healthy node, got unhealthy: %s", health.LastError)
	}

	// Should have block height from HTTP endpoint
	expectedHeight := uint64(0x12345)
	if health.BlockHeight != expectedHeight {
		t.Errorf("Expected block height %d, got %d", expectedHeight, health.BlockHeight)
	}

	// Should have reasonable response time
	if health.ResponseTime <= 0 {
		t.Error("Expected positive response time")
	}

	t.Logf("✅ EVM WebSocket node with HTTP correlation: healthy=%v, height=%d, response_time=%v",
		health.Healthy, health.BlockHeight, health.ResponseTime)
}

func TestEVMHandler_WebSocketWithoutHTTPCorrelation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewEVMHandler(5*time.Second, logger)

	// Test WebSocket node WITHOUT correlated HTTP URL in metadata
	// This simulates a misconfiguration where evm_ws_servers is provided
	// but evm_servers is missing or correlation failed
	node := NodeConfig{
		Name: "test-evm-ws-no-correlation",
		URL:  "ws://localhost:13246",
		Type: NodeTypeEVM,
		Metadata: map[string]string{
			"service_type": "websocket",
			// Missing "http_url" - correlation failed
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	// Should not return an error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Health should be returned but unhealthy
	if health == nil {
		t.Fatal("Expected health result, got nil")
	}

	// Should be unhealthy due to missing HTTP URL correlation
	if health.Healthy {
		t.Error("Expected unhealthy node due to missing HTTP correlation")
	}

	// Should have descriptive error message
	expectedError := "no corresponding HTTP URL found for WebSocket node - check evm_servers configuration"
	if health.LastError != expectedError {
		t.Errorf("Expected error '%s', got '%s'", expectedError, health.LastError)
	}

	// Block height should be 0 since health check failed
	if health.BlockHeight != 0 {
		t.Errorf("Expected block height 0 for failed health check, got %d", health.BlockHeight)
	}

	t.Logf("✅ EVM WebSocket node without HTTP correlation correctly failed: error=%s",
		health.LastError)
}

func TestEVMHandler_WebSocketWithFailedHTTPCorrelation(t *testing.T) {
	logger := zaptest.NewLogger(t)
	handler := NewEVMHandler(5*time.Second, logger)

	// Mock HTTP server that returns errors (simulating unhealthy HTTP endpoint)
	httpServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	}))
	defer httpServer.Close()

	// Test WebSocket node with correlated HTTP URL that fails health checks
	node := NodeConfig{
		Name: "test-evm-ws-failed-http",
		URL:  "ws://localhost:13246",
		Type: NodeTypeEVM,
		Metadata: map[string]string{
			"service_type": "websocket",
			"http_url":     httpServer.URL, // HTTP URL that will fail
		},
	}

	ctx := context.Background()
	health, err := handler.CheckHealth(ctx, node)

	// Should not return an error
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}

	// Health should be returned but unhealthy
	if health == nil {
		t.Fatal("Expected health result, got nil")
	}

	// Should be unhealthy due to HTTP endpoint failure
	if health.Healthy {
		t.Error("Expected unhealthy node due to HTTP endpoint failure")
	}

	// Should have error message about HTTP failure
	if health.LastError == "" {
		t.Error("Expected error message for failed HTTP health check")
	}

	// Block height should be 0 since health check failed
	if health.BlockHeight != 0 {
		t.Errorf("Expected block height 0 for failed health check, got %d", health.BlockHeight)
	}

	t.Logf("✅ EVM WebSocket node with failed HTTP correlation correctly failed: error=%s",
		health.LastError)
}
