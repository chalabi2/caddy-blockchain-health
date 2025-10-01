package blockchain_health

import (
	"net/http"
	"testing"

	"go.uber.org/zap/zaptest"
)

// TestChainTypeGrouping verifies that nodes are grouped by specific chain_type, not just protocol type
func TestChainTypeGrouping(t *testing.T) {
	logger := zaptest.NewLogger(t)

	t.Run("DifferentEVMChains_NotCompared", func(t *testing.T) {
		// Create servers with vastly different block heights representing different EVM chains
		ethereumServer := createEVMServer(t, 36282000, false) // Ethereum mainnet ~36M blocks
		baseServer := createEVMServer(t, 23485000, false)     // Base chain ~23M blocks
		arbitrumServer := createEVMServer(t, 7829000, false)  // Arbitrum ~7M blocks
		defer ethereumServer.Close()
		defer baseServer.Close()
		defer arbitrumServer.Close()

		// Create nodes with different chain types - they should NOT be compared against each other
		nodes := []NodeConfig{
			{
				Name:      "ethereum-node",
				URL:       ethereumServer.URL,
				Type:      NodeTypeEVM,
				ChainType: "ethereum", // Specific chain type
				Weight:    100,
			},
			{
				Name:      "base-node",
				URL:       baseServer.URL,
				Type:      NodeTypeEVM,
				ChainType: "base", // Different chain type
				Weight:    100,
			},
			{
				Name:      "arbitrum-node",
				URL:       arbitrumServer.URL,
				Type:      NodeTypeEVM,
				ChainType: "arbitrum", // Another different chain type
				Weight:    100,
			},
		}

		// Use the existing test infrastructure
		upstream := createTestUpstream(nodes, logger)

		// Set very strict threshold - would fail if chains were compared
		upstream.config.BlockValidation.HeightThreshold = 5

		// Test GetUpstreams - all should be available since they're in separate chains
		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// All 3 nodes should be available because they're in separate chain groups
		if len(upstreams) != 3 {
			t.Errorf("Expected all 3 nodes to be available (separate chains), got %d", len(upstreams))
		}

		t.Logf("✅ Different EVM chains are properly isolated - all %d nodes available", len(upstreams))
	})

	t.Run("DifferentCosmosChains_NotCompared", func(t *testing.T) {
		// Create servers with different block heights representing different Cosmos chains
		akashServer := createCosmosServer(t, 15000000, false)   // Akash height
		osmosisServer := createCosmosServer(t, 18000000, false) // Osmosis height
		junoServer := createCosmosServer(t, 12000000, false)    // Juno height
		defer akashServer.Close()
		defer osmosisServer.Close()
		defer junoServer.Close()

		// Create nodes with different chain types
		nodes := []NodeConfig{
			{
				Name:      "akash-node",
				URL:       akashServer.URL,
				Type:      NodeTypeCosmos,
				ChainType: "akash",
				Weight:    100,
			},
			{
				Name:      "osmosis-node",
				URL:       osmosisServer.URL,
				Type:      NodeTypeCosmos,
				ChainType: "osmosis",
				Weight:    100,
			},
			{
				Name:      "juno-node",
				URL:       junoServer.URL,
				Type:      NodeTypeCosmos,
				ChainType: "juno",
				Weight:    100,
			},
		}

		upstream := createTestUpstream(nodes, logger)
		upstream.config.BlockValidation.HeightThreshold = 5 // Very strict

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// All 3 nodes should be available because they're in separate chain groups
		if len(upstreams) != 3 {
			t.Errorf("Expected all 3 nodes to be available (separate chains), got %d", len(upstreams))
		}

		t.Logf("✅ Different Cosmos chains are properly isolated - all %d nodes available", len(upstreams))
	})

	t.Run("SameChainType_StillCompared", func(t *testing.T) {
		// Verify that nodes with the SAME chain type are still compared
		leaderServer := createEVMServer(t, 36282000, false)  // Leader
		laggingServer := createEVMServer(t, 36281000, false) // 1000 blocks behind
		defer leaderServer.Close()
		defer laggingServer.Close()

		// Both nodes have the same chain type - they should be compared
		nodes := []NodeConfig{
			{
				Name:      "ethereum-leader",
				URL:       leaderServer.URL,
				Type:      NodeTypeEVM,
				ChainType: "ethereum", // Same chain type
				Weight:    100,
			},
			{
				Name:      "ethereum-lagging",
				URL:       laggingServer.URL,
				Type:      NodeTypeEVM,
				ChainType: "ethereum", // Same chain type
				Weight:    100,
			},
		}

		upstream := createTestUpstream(nodes, logger)
		upstream.config.BlockValidation.HeightThreshold = 500 // Lagging node is 1000 blocks behind

		upstreams, err := upstream.GetUpstreams(&http.Request{})
		if err != nil {
			t.Fatalf("GetUpstreams failed: %v", err)
		}

		// Only 1 node should be available (leader) - lagging should be removed
		if len(upstreams) != 1 {
			t.Errorf("Expected 1 upstream (leader only), got %d", len(upstreams))
		}

		t.Logf("✅ Nodes with same chain type are correctly compared - lagging node removed")
	})
}
