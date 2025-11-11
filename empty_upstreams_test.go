package blockchain_health

import (
	"net/http"
	"testing"
	"time"

	"go.uber.org/zap/zaptest"
)

// Ensures GetUpstreams returns an error instead of an empty slice,
// which helps upstream reverse proxy handle the condition gracefully.
func TestGetUpstreams_ReturnsErrorWhenNoUpstreamsSelected(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Intentionally provide an invalid URL that parses to an empty host,
	// causing selection to skip all nodes.
	badNode := NodeConfig{
		Name:   "bad",
		URL:    "://", // invalid; url.Parse returns error and the node is skipped
		Type:   NodeTypeCosmos,
		Weight: 100,
	}

	config := &Config{
		Nodes: []NodeConfig{badNode},
		HealthCheck: HealthCheckConfig{
			Timeout:       "1s",
			RetryAttempts: 1,
		},
		FailureHandling: FailureHandlingConfig{
			MinHealthyNodes: 1,
		},
		Performance: PerformanceConfig{
			CacheDuration: "500ms",
		},
	}

	upstream := &BlockchainHealthUpstream{
		config:        config,
		healthChecker: NewHealthChecker(config, NewHealthCache(500*time.Millisecond), nil, logger),
		cache:         NewHealthCache(500 * time.Millisecond),
		logger:        logger,
	}

	_, err := upstream.GetUpstreams(&http.Request{})
	if err == nil {
		t.Fatalf("expected error when no upstreams can be selected, got nil")
	}
}
