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
					w.Write([]byte(tt.response))
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
				w.Write([]byte(tt.response))
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
		w.Write([]byte(response))
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
		w.Write([]byte(response))
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
