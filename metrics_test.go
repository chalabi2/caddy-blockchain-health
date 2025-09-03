package blockchain_health

import (
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// TestMetrics tests the metrics functionality
func TestMetrics(t *testing.T) {
	// Create metrics instance
	metrics := NewMetrics()

	// Test metrics registration
	if err := metrics.Register(); err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}
	defer metrics.Unregister()

	// Test metrics operations
	metrics.IncrementTotalChecks()
	metrics.SetHealthyNodes(2)
	metrics.SetUnhealthyNodes(1)
	metrics.SetBlockHeight("test-node", 12345)
	metrics.IncrementError("test-node", "timeout")
	metrics.RecordCheckDuration(1.5)

	// Verify metrics are working (basic smoke test)
	// In a real test, you'd collect metrics and verify values
	// For now, we just ensure no panics occur
}

// TestMetricsRegistration tests metrics registration and unregistration
func TestMetricsRegistration(t *testing.T) {
	// Create metrics instance
	metrics := NewMetrics()

	// Test registration
	if err := metrics.Register(); err != nil {
		t.Fatalf("Failed to register metrics: %v", err)
	}

	// Test unregistration
	metrics.Unregister()

	// Test re-registration (should work)
	if err := metrics.Register(); err != nil {
		t.Fatalf("Failed to re-register metrics: %v", err)
	}
	defer metrics.Unregister()
}

// TestMetricsOperations tests individual metrics operations
func TestMetricsOperations(t *testing.T) {
	// Create metrics instance
	metrics := NewMetrics()

	// Test total checks counter
	metrics.IncrementTotalChecks()
	metrics.IncrementTotalChecks()

	// Test healthy nodes gauge
	metrics.SetHealthyNodes(5)
	metrics.SetHealthyNodes(3)

	// Test unhealthy nodes gauge
	metrics.SetUnhealthyNodes(2)
	metrics.SetUnhealthyNodes(1)

	// Test block height gauge
	metrics.SetBlockHeight("node1", 12345)
	metrics.SetBlockHeight("node2", 67890)

	// Test error counter
	metrics.IncrementError("node1", "timeout")
	metrics.IncrementError("node1", "connection")
	metrics.IncrementError("node2", "timeout")

	// Test check duration histogram
	metrics.RecordCheckDuration(0.5)
	metrics.RecordCheckDuration(1.0)
	metrics.RecordCheckDuration(2.5)

	// All operations should complete without panicking
}

// TestMetricsWithLogger tests metrics with logger integration
func TestMetricsWithLogger(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Create metrics instance
	metrics := NewMetrics()

	// Test metrics registration with logger
	if err := metrics.Register(); err != nil {
		logger.Error("Failed to register metrics", zap.Error(err))
		t.Fatalf("Failed to register metrics: %v", err)
	}
	defer metrics.Unregister()

	// Test metrics operations with logger context
	logger.Info("Testing metrics operations")
	metrics.IncrementTotalChecks()
	metrics.SetHealthyNodes(1)
	metrics.SetUnhealthyNodes(0)
	metrics.SetBlockHeight("test-node", 12345)
	metrics.IncrementError("test-node", "test-error")
	metrics.RecordCheckDuration(1.0)

	logger.Info("Metrics operations completed successfully")
}
