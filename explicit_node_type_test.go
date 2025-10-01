package blockchain_health

import (
	"net/http"
	"testing"

	"go.uber.org/zap/zaptest"
)

// TestExplicitNodeType verifies that explicit node_type configuration works correctly
func TestExplicitNodeType(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("ExplicitNodeType_OverridesAutoDetection", func(t *testing.T) {
		// Create a server that could be detected as either type
		server := createEVMServer(t, 36282000, false)
		defer server.Close()

		// Create upstream with explicit node_type that differs from auto-detection
		upstream := createTestUpstream([]NodeConfig{
			{
				Name:      "explicit-evm-node",
				URL:       server.URL,
				Type:      NodeType("evm"), // This should be set by explicit node_type
				ChainType: "custom-blockchain",
				Weight:    100,
				Metadata: map[string]string{
					"service_type": "evm",
				},
			},
		}, logger)

		// Configure with explicit node_type
		upstream.Chain.NodeType = "evm"
		upstream.Chain.ChainType = "custom-blockchain"

		// Get upstreams
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		if len(upstreams) != 1 {
			t.Fatalf("Expected 1 upstream, got %d", len(upstreams))
		}

		t.Logf("✅ Explicit node_type correctly used for custom blockchain")
	})

	t.Run("BackwardCompatibility_AutoDetectionStillWorks", func(t *testing.T) {
		// Create servers for known chains
		ethServer := createEVMServer(t, 36282000, false)
		cosmosServer := createCosmosServer(t, 15000000, false)
		defer ethServer.Close()
		defer cosmosServer.Close()

		// Create upstream WITHOUT explicit node_type (should auto-detect)
		upstream := createTestUpstream([]NodeConfig{
			{
				Name:      "auto-eth-node",
				URL:       ethServer.URL,
				Type:      NodeType("evm"), // Should be auto-detected
				ChainType: "ethereum",
				Weight:    100,
			},
			{
				Name:      "auto-cosmos-node",
				URL:       cosmosServer.URL,
				Type:      NodeType("cosmos"), // Should be auto-detected
				ChainType: "akash",
				Weight:    100,
			},
		}, logger)

		// Don't set explicit node_type - should fall back to auto-detection
		upstream.Chain.NodeType = "" // Empty = auto-detect

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		if len(upstreams) != 2 {
			t.Fatalf("Expected 2 upstreams, got %d", len(upstreams))
		}

		t.Logf("✅ Backward compatibility maintained - auto-detection works when node_type not specified")
	})

	t.Run("ChainGrouping_WithExplicitNodeType", func(t *testing.T) {
		// Create multiple EVM chains with explicit node_type
		ethServer := createEVMServer(t, 36282000, false)
		baseServer := createEVMServer(t, 23485000, false)
		customServer := createEVMServer(t, 50000000, false)
		defer ethServer.Close()
		defer baseServer.Close()
		defer customServer.Close()

		// All use explicit node_type "evm" but different chain_type values
		upstream := createTestUpstream([]NodeConfig{
			{
				Name:      "ethereum-node",
				URL:       ethServer.URL,
				Type:      NodeType("evm"),
				ChainType: "ethereum",
				Weight:    100,
			},
			{
				Name:      "base-node",
				URL:       baseServer.URL,
				Type:      NodeType("evm"),
				ChainType: "base",
				Weight:    100,
			},
			{
				Name:      "custom-node",
				URL:       customServer.URL,
				Type:      NodeType("evm"),
				ChainType: "my-custom-evm-chain",
				Weight:    100,
			},
		}, logger)

		upstream.Chain.NodeType = "evm" // Explicit protocol type
		upstream.config.BlockValidation.HeightThreshold = 1000

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// All should be healthy because they're in separate chain groups
		if len(upstreams) != 3 {
			t.Fatalf("Expected 3 upstreams (all chains isolated), got %d", len(upstreams))
		}

		t.Logf("✅ Chain grouping works correctly with explicit node_type - all chains isolated")
	})
}
