package blockchain_health

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// TestHealthEndpoint tests the health endpoint functionality
func TestHealthEndpoint(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create healthy and unhealthy test servers
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
			w.Write([]byte(response))
		}
	}))
	defer healthyServer.Close()

	unhealthyServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Write([]byte(response))
		}
	}))
	defer unhealthyServer.Close()

	// Create external reference server
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12350",
						"catching_up": false
					}
				}
			}`
			w.Write([]byte(response))
		}
	}))
	defer externalServer.Close()

	// Create configuration
	config := &Config{
		Nodes: []NodeConfig{
			{Name: "healthy-node", URL: healthyServer.URL, Type: NodeTypeCosmos, Weight: 1},
			{Name: "unhealthy-node", URL: unhealthyServer.URL, Type: NodeTypeCosmos, Weight: 1},
		},
		ExternalReferences: []ExternalReference{
			{Name: "external-ref", URL: externalServer.URL, Type: NodeTypeCosmos, Enabled: true},
		},
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "5s",
			RetryAttempts: 3,
			RetryDelay:    "1s",
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
		Performance: PerformanceConfig{
			CacheDuration:       "30s",
			MaxConcurrentChecks: 5,
		},
	}

	// Create upstream with health endpoint
	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(30*time.Second), NewMetrics(), logger),
		cache:         NewHealthCache(30 * time.Second),
		metrics:       NewMetrics(),
		logger:        logger,
	}

	// Test health endpoint
	handler := upstream.ServeHealthEndpoint()

	// Test GET request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	// Verify response
	if w.Code != http.StatusOK && w.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status 200 or 503, got %d", w.Code)
	}

	// Verify response structure
	var response HealthEndpointResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify response fields
	if response.Status != "healthy" && response.Status != "unhealthy" {
		t.Errorf("Expected status 'healthy' or 'unhealthy', got '%s'", response.Status)
	}

	if response.Nodes.Total != 2 {
		t.Errorf("Expected 2 total nodes, got %d", response.Nodes.Total)
	}

	if len(response.ExternalReferences) != 1 {
		t.Errorf("Expected 1 external reference, got %d", len(response.ExternalReferences))
	}

	// Test POST request (should fail)
	req = httptest.NewRequest("POST", "/health", nil)
	w = httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status 405 for POST request, got %d", w.Code)
	}
}

// TestHealthEndpointResponseStructure tests the health endpoint response structure
func TestHealthEndpointResponseStructure(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create a test server that responds quickly
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Write([]byte(response))
		}
	}))
	defer testServer.Close()

	// Create a simple configuration with the test server
	config := &Config{
		Nodes: []NodeConfig{
			{Name: "test-node", URL: testServer.URL, Type: NodeTypeCosmos, Weight: 1},
		},
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "2s", // Shorter timeout for faster tests
			RetryAttempts: 1,    // Fewer retries for faster tests
			RetryDelay:    "1s",
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
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

	// Test health endpoint
	handler := upstream.ServeHealthEndpoint()

	// Test GET request
	req := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	// Verify response structure
	var response HealthEndpointResponse
	if err := json.Unmarshal(w.Body.Bytes(), &response); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}

	// Verify all required fields are present
	if response.Status == "" {
		t.Error("Expected status field to be present")
	}

	if response.Timestamp.IsZero() {
		t.Error("Expected timestamp field to be present")
	}

	if response.Nodes.Total == 0 {
		t.Error("Expected nodes field to be present")
	}

	if response.LastCheck.IsZero() {
		t.Error("Expected last_check field to be present")
	}
}

// TestExternalReferenceCheck tests external reference checking
func TestExternalReferenceCheck(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create external reference server
	externalServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := `{
				"result": {
					"sync_info": {
						"latest_block_height": "12350",
						"catching_up": false
					}
				}
			}`
			w.Write([]byte(response))
		}
	}))
	defer externalServer.Close()

	// Create a test server for the main node
	testServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
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
			w.Write([]byte(response))
		}
	}))
	defer testServer.Close()

	// Create configuration with external reference
	config := &Config{
		Nodes: []NodeConfig{
			{Name: "test-node", URL: testServer.URL, Type: NodeTypeCosmos, Weight: 1},
		},
		ExternalReferences: []ExternalReference{
			{Name: "external-ref", URL: externalServer.URL, Type: NodeTypeCosmos, Enabled: true},
		},
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "2s", // Shorter timeout for faster tests
			RetryAttempts: 1,    // Fewer retries for faster tests
			RetryDelay:    "1s",
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

	// Test external reference check
	ctx := context.Background()
	status := upstream.checkExternalReference(ctx, config.ExternalReferences[0])

	// Verify external reference status
	if !status.Reachable {
		t.Errorf("External reference should be reachable, got error: %s", status.Error)
	}

	if status.BlockHeight != 12350 {
		t.Errorf("Expected block height 12350, got %d", status.BlockHeight)
	}
}
