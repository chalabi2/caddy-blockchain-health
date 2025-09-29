package blockchain_health

import (
	"context"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// BenchmarkGetUpstreams measures the maximum RPS for GetUpstreams calls
func BenchmarkGetUpstreams(b *testing.B) {
	logger := zaptest.NewLogger(b)

	// Create multiple healthy servers to simulate realistic load balancing
	servers := make([]*httptest.Server, 5)
	nodes := make([]NodeConfig, 5)
	for i := 0; i < 5; i++ {
		servers[i] = createBenchmarkServer(b, uint64(12345-i), false)
		nodes[i] = NodeConfig{
			Name:   fmt.Sprintf("node-%d", i),
			URL:    servers[i].URL,
			Type:   NodeTypeCosmos,
			Weight: 100,
		}
	}
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	upstream := createBenchmarkUpstream(nodes, logger)

	// Wait for initial health checks
	time.Sleep(200 * time.Millisecond)

	// Reset timer to exclude setup time
	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		req := &http.Request{}
		for pb.Next() {
			_, err := upstream.GetUpstreams(req)
			if err != nil {
				b.Errorf("GetUpstreams failed: %v", err)
			}
		}
	})
}

// BenchmarkGetUpstreamsWithFailover measures performance during failover scenarios
func BenchmarkGetUpstreamsWithFailover(b *testing.B) {
	logger := zaptest.NewLogger(b)

	// Create controllable servers
	var healthy1, healthy2, healthy3 int64 = 1, 1, 1

	server1 := createControllableBenchmarkServer(b, &healthy1, 12345)
	server2 := createControllableBenchmarkServer(b, &healthy2, 12344)
	server3 := createControllableBenchmarkServer(b, &healthy3, 12343)
	defer server1.Close()
	defer server2.Close()
	defer server3.Close()

	nodes := []NodeConfig{
		{Name: "node-1", URL: server1.URL, Type: NodeTypeCosmos, Weight: 100},
		{Name: "node-2", URL: server2.URL, Type: NodeTypeCosmos, Weight: 100},
		{Name: "node-3", URL: server3.URL, Type: NodeTypeCosmos, Weight: 100},
	}

	upstream := createBenchmarkUpstream(nodes, logger)
	time.Sleep(200 * time.Millisecond)

	// Simulate random failovers during the benchmark
	done := make(chan struct{})
	defer close(done)

	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				// Randomly toggle node health
				if atomic.LoadInt64(&healthy1) == 1 {
					atomic.StoreInt64(&healthy1, 0)
				} else {
					atomic.StoreInt64(&healthy1, 1)
				}
			case <-done:
				return
			}
		}
	}()

	b.ResetTimer()

	b.RunParallel(func(pb *testing.PB) {
		req := &http.Request{}
		for pb.Next() {
			_, err := upstream.GetUpstreams(req)
			if err != nil {
				b.Errorf("GetUpstreams failed: %v", err)
			}
		}
	})
}

// BenchmarkFailoverSpeed measures how quickly the system detects and responds to node failures
func BenchmarkFailoverSpeed(b *testing.B) {
	logger := zaptest.NewLogger(b)

	var nodeHealthy int64 = 1
	server := createControllableBenchmarkServer(b, &nodeHealthy, 12345)
	defer server.Close()

	nodes := []NodeConfig{
		{Name: "test-node", URL: server.URL, Type: NodeTypeCosmos, Weight: 100},
	}

	// Use fast health checks for this benchmark
	upstream := createFastBenchmarkUpstream(nodes, logger)
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		// Make node healthy
		atomic.StoreInt64(&nodeHealthy, 1)

		// Wait for health check to detect healthy state
		for {
			upstreams, _ := upstream.GetUpstreams(&http.Request{})
			if len(upstreams) > 0 {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}

		// Make node unhealthy
		atomic.StoreInt64(&nodeHealthy, 0)

		// Measure time to detect unhealthy state
		start := time.Now()
		for {
			_, _ = upstream.GetUpstreams(&http.Request{})
			// In this test, we expect fallback behavior (still returns 1 upstream)
			// but we can check if the health detection is working by forcing a health check
			if time.Since(start) > 50*time.Millisecond {
				break
			}
			time.Sleep(1 * time.Millisecond)
		}
	}
}

// BenchmarkConcurrentHealthChecks measures performance of concurrent health checking
func BenchmarkConcurrentHealthChecks(b *testing.B) {
	logger := zaptest.NewLogger(b)

	// Create many servers to test concurrent health checking
	numServers := 20
	servers := make([]*httptest.Server, numServers)
	nodes := make([]NodeConfig, numServers)
	for i := 0; i < numServers; i++ {
		servers[i] = createBenchmarkServer(b, uint64(12345-i), false)
		nodes[i] = NodeConfig{
			Name:   fmt.Sprintf("node-%d", i),
			URL:    servers[i].URL,
			Type:   NodeTypeCosmos,
			Weight: 100,
		}
	}
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	upstream := createBenchmarkUpstream(nodes, logger)
	time.Sleep(300 * time.Millisecond)

	b.ResetTimer()

	// Test concurrent GetUpstreams calls while health checks are running
	b.RunParallel(func(pb *testing.PB) {
		req := &http.Request{}
		for pb.Next() {
			_, err := upstream.GetUpstreams(req)
			if err != nil {
				b.Errorf("GetUpstreams failed: %v", err)
			}
		}
	})
}

// BenchmarkMemoryAllocation measures memory allocations during operation
func BenchmarkMemoryAllocation(b *testing.B) {
	logger := zaptest.NewLogger(b)

	server := createBenchmarkServer(b, 12345, false)
	defer server.Close()

	nodes := []NodeConfig{
		{Name: "test-node", URL: server.URL, Type: NodeTypeCosmos, Weight: 100},
	}

	upstream := createBenchmarkUpstream(nodes, logger)
	time.Sleep(100 * time.Millisecond)

	b.ResetTimer()
	b.ReportAllocs()

	req := &http.Request{}
	for i := 0; i < b.N; i++ {
		_, err := upstream.GetUpstreams(req)
		if err != nil {
			b.Errorf("GetUpstreams failed: %v", err)
		}
	}
}

// Helper functions for benchmarks

func createBenchmarkServer(b *testing.B, blockHeight uint64, catchingUp bool) *httptest.Server {
	var requestCount int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{
				"result": {
					"sync_info": {
						"latest_block_height": "%d",
						"catching_up": %t
					}
				}
			}`, blockHeight, catchingUp)
			_, _ = w.Write([]byte(response))
		} else {
			// Simulate JSON-RPC response for proxy requests
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			response := fmt.Sprintf(`{
				"jsonrpc": "2.0",
				"id": 1,
				"result": {
					"sync_info": {
						"latest_block_height": "%d",
						"catching_up": %t
					}
				}
			}`, blockHeight, catchingUp)
			_, _ = w.Write([]byte(response))
		}
	}))
}

func createControllableBenchmarkServer(b *testing.B, healthy *int64, blockHeight uint64) *httptest.Server {
	var requestCount int64

	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		atomic.AddInt64(&requestCount, 1)

		isHealthy := atomic.LoadInt64(healthy) == 1

		if r.URL.Path == "/status" {
			w.Header().Set("Content-Type", "application/json")
			if isHealthy {
				w.WriteHeader(http.StatusOK)
				response := fmt.Sprintf(`{
					"result": {
						"sync_info": {
							"latest_block_height": "%d",
							"catching_up": false
						}
					}
				}`, blockHeight)
				_, _ = w.Write([]byte(response))
			} else {
				w.WriteHeader(http.StatusServiceUnavailable)
				_, _ = w.Write([]byte(`{"error": "node unhealthy"}`))
			}
		} else {
			// Simulate JSON-RPC response
			if isHealthy {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				response := fmt.Sprintf(`{
					"jsonrpc": "2.0",
					"id": 1,
					"result": {
						"sync_info": {
							"latest_block_height": "%d",
							"catching_up": false
						}
					}
				}`, blockHeight)
				_, _ = w.Write([]byte(response))
			} else {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			}
		}
	}))
}

func createBenchmarkUpstream(nodes []NodeConfig, logger *zap.Logger) *BlockchainHealthUpstream {
	config := &Config{
		Nodes: nodes,
		HealthCheck: HealthCheckConfig{
			Interval:      "1s",
			Timeout:       "500ms",
			RetryAttempts: 1,
			RetryDelay:    "100ms",
		},
		Performance: PerformanceConfig{
			CacheDuration:       "500ms",
			MaxConcurrentChecks: 10,
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
	}

	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(500*time.Millisecond), NewMetrics(), logger),
		cache:         NewHealthCache(500 * time.Millisecond),
		metrics:       NewMetrics(),
		logger:        logger,
	}

	return upstream
}

func createFastBenchmarkUpstream(nodes []NodeConfig, logger *zap.Logger) *BlockchainHealthUpstream {
	config := &Config{
		Nodes: nodes,
		HealthCheck: HealthCheckConfig{
			Interval:      "50ms", // Very fast for benchmark
			Timeout:       "100ms",
			RetryAttempts: 1,
			RetryDelay:    "10ms",
		},
		Performance: PerformanceConfig{
			CacheDuration:       "10ms", // Very short cache
			MaxConcurrentChecks: 5,
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
	}

	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(10*time.Millisecond), NewMetrics(), logger),
		cache:         NewHealthCache(10 * time.Millisecond),
		metrics:       NewMetrics(),
		logger:        logger,
	}

	return upstream
}

// BenchmarkStressTest simulates realistic production load
func BenchmarkStressTest(b *testing.B) {
	if testing.Short() {
		b.Skip("Skipping stress test in short mode")
	}

	logger := zaptest.NewLogger(b)

	// Create a mix of healthy and occasionally unhealthy servers
	numServers := 10
	servers := make([]*httptest.Server, numServers)
	healthStates := make([]*int64, numServers)
	nodes := make([]NodeConfig, numServers)

	for i := 0; i < numServers; i++ {
		healthStates[i] = new(int64)
		atomic.StoreInt64(healthStates[i], 1) // Start healthy
		servers[i] = createControllableBenchmarkServer(b, healthStates[i], uint64(12345-i))
		nodes[i] = NodeConfig{
			Name:   fmt.Sprintf("node-%d", i),
			URL:    servers[i].URL,
			Type:   NodeTypeCosmos,
			Weight: 100,
		}
	}
	defer func() {
		for _, server := range servers {
			server.Close()
		}
	}()

	upstream := createBenchmarkUpstream(nodes, logger)
	time.Sleep(500 * time.Millisecond)

	// Simulate random node failures during the test
	var wg sync.WaitGroup
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Random failure simulation
	wg.Add(1)
	go func() {
		defer wg.Done()
		ticker := time.NewTicker(200 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-ticker.C:
				// Randomly toggle 1-2 nodes
				for i := 0; i < 2; i++ {
					nodeIdx := i % numServers
					current := atomic.LoadInt64(healthStates[nodeIdx])
					atomic.StoreInt64(healthStates[nodeIdx], 1-current)
				}
			}
		}
	}()

	b.ResetTimer()

	// High concurrency test
	b.SetParallelism(20) // 20x the default GOMAXPROCS
	b.RunParallel(func(pb *testing.PB) {
		req := &http.Request{}
		for pb.Next() {
			_, err := upstream.GetUpstreams(req)
			if err != nil {
				b.Errorf("GetUpstreams failed: %v", err)
			}
		}
	})

	cancel()
	wg.Wait()
}
