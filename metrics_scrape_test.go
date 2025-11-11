package blockchain_health

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// simpleNext is a trivial next handler used to exercise the request_deadline middleware
type simpleNext struct{}

func (s *simpleNext) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
	return nil
}

// TestMetricsAreScrapeable spins up a promhttp server and asserts our
// metrics families are present and can be scraped.
func TestMetricsAreScrapeable(t *testing.T) {
	// Serve the default Prometheus registry
	srv := httptest.NewServer(promhttp.Handler())
	defer srv.Close()

	// Register and touch blockchain health metrics
	m := NewMetrics()
	if err := m.Register(); err != nil {
		t.Fatalf("register metrics: %v", err)
	}
	m.SetHealthyNodes(2)
	m.SetUnhealthyNodes(1)
	m.IncrementTotalChecks()
	m.SetBlockHeight("node-1", 12345)
	m.IncrementError("node-1", "health_check")
	// Touch upstream selection counters directly (same package access)
	m.upstreamsIncluded.WithLabelValues("node-1", "rpc", "healthy").Inc()
	m.upstreamsExcluded.WithLabelValues("node-2", "websocket", "filtered_http").Inc()

	// Register and touch request_deadline metrics by exercising the middleware
	h := &RequestDeadline{
		DefaultTimeout: caddy.Duration(50 * time.Millisecond),
		AddHeaders:     true,
	}
	if err := h.Provision(caddy.Context{}); err != nil {
		t.Fatalf("provision request_deadline: %v", err)
	}
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "http://example.test/", nil)
	if err := h.ServeHTTP(rec, req, caddyhttp.HandlerFunc((&simpleNext{}).ServeHTTP)); err != nil {
		t.Fatalf("request_deadline ServeHTTP error: %v", err)
	}

	// Scrape /metrics
	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatalf("scrape /metrics: %v", err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	text := string(body)

	// Assert key metric names appear in the exposition
	wantNames := []string{
		"caddy_blockchain_health_healthy_nodes",
		"caddy_blockchain_health_unhealthy_nodes",
		"caddy_blockchain_health_upstreams_included_total",
		"caddy_blockchain_health_upstreams_excluded_total",
		"caddy_request_deadline_applied_total",
		"caddy_request_deadline_applied_seconds",
		"caddy_request_deadline_duration_seconds",
	}
	for _, name := range wantNames {
		if !strings.Contains(text, name) {
			t.Fatalf("expected %q to be present in /metrics output", name)
		}
	}
}
