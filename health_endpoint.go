package blockchain_health

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"go.uber.org/zap"
)

// HealthEndpointResponse represents the response structure for the health endpoint
type HealthEndpointResponse struct {
	Status             string                       `json:"status"`
	Timestamp          time.Time                    `json:"timestamp"`
	Nodes              NodesStatus                  `json:"nodes"`
	ExternalReferences map[string]ExternalRefStatus `json:"external_references"`
	Cache              map[string]interface{}       `json:"cache,omitempty"`
	LastCheck          time.Time                    `json:"last_check"`
}

// NodesStatus represents the status of all nodes
type NodesStatus struct {
	Total     int `json:"total"`
	Healthy   int `json:"healthy"`
	Unhealthy int `json:"unhealthy"`
}

// ExternalRefStatus represents the status of an external reference
type ExternalRefStatus struct {
	Reachable   bool   `json:"reachable"`
	BlockHeight uint64 `json:"block_height,omitempty"`
	Error       string `json:"error,omitempty"`
}

// ServeHealthEndpoint creates an HTTP handler for the health endpoint
func (b *BlockchainHealthUpstream) ServeHealthEndpoint() http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		ctx, cancel := context.WithTimeout(r.Context(), 10*time.Second)
		defer cancel()

		response := b.buildHealthResponse(ctx)

		w.Header().Set("Content-Type", "application/json")

		// Set HTTP status based on overall health
		if response.Status == "healthy" {
			w.WriteHeader(http.StatusOK)
		} else {
			w.WriteHeader(http.StatusServiceUnavailable)
		}

		if err := json.NewEncoder(w).Encode(response); err != nil {
			b.logger.Error("failed to encode health response", zap.Error(err))
			http.Error(w, "Internal server error", http.StatusInternalServerError)
		}
	}
}

// buildHealthResponse builds the health endpoint response
func (b *BlockchainHealthUpstream) buildHealthResponse(ctx context.Context) *HealthEndpointResponse {
	// Get current health status
	healthResults, err := b.healthChecker.CheckAllNodes(ctx)
	if err != nil {
		b.logger.Error("health check failed for endpoint", zap.Error(err))
		return &HealthEndpointResponse{
			Status:    "unhealthy",
			Timestamp: time.Now(),
			Nodes: NodesStatus{
				Total:     len(b.config.Nodes),
				Healthy:   0,
				Unhealthy: len(b.config.Nodes),
			},
			LastCheck: time.Now(),
		}
	}

	// Count healthy and unhealthy nodes
	var healthyCount, unhealthyCount int
	for _, health := range healthResults {
		if health.Healthy {
			healthyCount++
		} else {
			unhealthyCount++
		}
	}

	// Check external references
	externalRefs := make(map[string]ExternalRefStatus)
	for _, ref := range b.config.ExternalReferences {
		if !ref.Enabled {
			continue
		}

		status := b.checkExternalReference(ctx, ref)
		externalRefs[ref.Name] = status
	}

	// Determine overall status
	status := "healthy"
	if healthyCount < b.config.FailureHandling.MinHealthyNodes {
		status = "unhealthy"
	}

	response := &HealthEndpointResponse{
		Status:    status,
		Timestamp: time.Now(),
		Nodes: NodesStatus{
			Total:     len(b.config.Nodes),
			Healthy:   healthyCount,
			Unhealthy: unhealthyCount,
		},
		ExternalReferences: externalRefs,
		LastCheck:          time.Now(),
	}

	// Add cache stats if available
	if b.cache != nil {
		response.Cache = b.cache.GetStats()
	}

	return response
}

// checkExternalReference checks the status of an external reference
func (b *BlockchainHealthUpstream) checkExternalReference(ctx context.Context, ref ExternalReference) ExternalRefStatus {
	var height uint64
	var err error

	switch ref.Type {
	case NodeTypeCosmos:
		height, err = b.healthChecker.cosmosHandler.GetBlockHeight(ctx, ref.URL)
	case NodeTypeEVM:
		height, err = b.healthChecker.evmHandler.GetBlockHeight(ctx, ref.URL)
	default:
		return ExternalRefStatus{
			Reachable: false,
			Error:     fmt.Sprintf("unsupported type: %s", ref.Type),
		}
	}

	if err != nil {
		return ExternalRefStatus{
			Reachable: false,
			Error:     err.Error(),
		}
	}

	return ExternalRefStatus{
		Reachable:   true,
		BlockHeight: height,
	}
}
