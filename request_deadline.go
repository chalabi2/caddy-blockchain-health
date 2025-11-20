package blockchain_health

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
	"github.com/prometheus/client_golang/prometheus"
)

// Source describes where to read a tier value from
type Source struct {
	Type  string `json:"type"`  // placeholder|header|query
	Name  string `json:"name"`  // header or query name
	Value string `json:"value"` // placeholder template, e.g. {http.auth.user.tier}
}

// Skip controls which requests are excluded from deadline enforcement
type Skip struct {
	WebSocket bool     `json:"websocket"`
	GRPC      bool     `json:"grpc"`
	Methods   []string `json:"methods"`
}

// RequestDeadline is a middleware that applies per-request context deadlines
// based on configured tiers or a default timeout. It never affects Caddy's
// own transport timeouts unless included in the site routes.
type RequestDeadline struct {
	Sources        []Source          `json:"from,omitempty"`
	DefaultTimeout caddy.Duration    `json:"default_timeout,omitempty"`
	Tiers          map[string]string `json:"tiers,omitempty"`
	Skip           Skip              `json:"skip,omitempty"`
	AddHeaders     bool              `json:"add_headers,omitempty"`
	MinTimeout     caddy.Duration    `json:"min_timeout,omitempty"`
	MaxTimeout     caddy.Duration    `json:"max_timeout,omitempty"`

	// compiled
	tierDur map[string]time.Duration
}

func init() {
	caddy.RegisterModule(&RequestDeadline{})
}

var rdMetrics *RequestDeadlineMetrics

// CaddyModule returns the Caddy module information.
func (*RequestDeadline) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.handlers.request_deadline",
		New: func() caddy.Module { return new(RequestDeadline) },
	}
}

// Provision precomputes tier durations
func (h *RequestDeadline) Provision(ctx caddy.Context) error {
	h.tierDur = make(map[string]time.Duration)
	for k, v := range h.Tiers {
		// Support time strings like "500ms", "2s"
		d, err := time.ParseDuration(v)
		if err != nil {
			return err
		}
		h.tierDur[strings.ToUpper(strings.TrimSpace(k))] = d
	}
	var registerer prometheus.Registerer
	if reg := ctx.GetMetricsRegistry(); reg != nil {
		registerer = reg
	} else {
		registerer = prometheus.DefaultRegisterer
	}
	metrics, err := acquireRequestDeadlineMetrics(registerer)
	if err != nil {
		return err
	}
	rdMetrics = metrics
	return nil
}

// Validate checks configuration correctness, including min/max relation and source types
func (h *RequestDeadline) Validate() error {
	// Check min/max relation when both are provided
	min := time.Duration(h.MinTimeout)
	max := time.Duration(h.MaxTimeout)
	if min > 0 && max > 0 && min > max {
		return fmt.Errorf("min_timeout > max_timeout")
	}

	// Validate source types (allow empty type to be ignored)
	for i, s := range h.Sources {
		if s.Type == "" {
			continue
		}
		switch s.Type {
		case "placeholder", "header", "query":
			// valid
		default:
			return fmt.Errorf("source[%d]: invalid type %q, must be placeholder, header, or query", i, s.Type)
		}
	}
	return nil
}

// ServeHTTP applies the deadline and forwards the request downstream.
func (h *RequestDeadline) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
	if h.shouldSkip(r) {
		return next.ServeHTTP(w, r)
	}

	tier := strings.TrimSpace(h.resolveTier(r))
	if tier == "" {
		tier = "__DEFAULT__"
	} else {
		tier = strings.ToUpper(tier)
	}

	// select timeout
	timeout := time.Duration(h.DefaultTimeout)
	if d, ok := h.tierDur[tier]; ok {
		timeout = d
	}

	// clamp within min/max if configured
	if min := time.Duration(h.MinTimeout); min > 0 && timeout < min {
		timeout = min
	}
	if max := time.Duration(h.MaxTimeout); max > 0 && timeout > max {
		timeout = max
	}

	// if no timeout configured, pass-through
	if timeout <= 0 {
		return next.ServeHTTP(w, r)
	}

	start := time.Now()
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()

	if h.AddHeaders {
		// Set headers early; downstream may overwrite if desired
		w.Header().Set("X-Plan-Tier", tier)
		w.Header().Set("X-Request-Timeout", timeout.String())
		w.Header().Set("X-Request-Deadline-At", time.Now().Add(timeout).UTC().Format(time.RFC3339))
	}

	// Emit applied metrics
	if rdMetrics != nil {
		rdMetrics.appliedTotal.WithLabelValues(tier).Inc()
		rdMetrics.appliedSeconds.WithLabelValues(tier).Observe(timeout.Seconds())
	}

	r = r.WithContext(ctx)
	err := next.ServeHTTP(w, r)

	// Outcome and duration
	outcome := "success"
	if ctx.Err() == context.DeadlineExceeded {
		outcome = "timeout"
		if rdMetrics != nil {
			rdMetrics.timeoutsTotal.WithLabelValues(tier, r.Method, r.Host).Inc()
		}
	}
	if rdMetrics != nil {
		rdMetrics.durationSeconds.WithLabelValues(tier, outcome).Observe(time.Since(start).Seconds())
	}

	return err
}

func (h *RequestDeadline) shouldSkip(r *http.Request) bool {
	// Skip by method
	if len(h.Skip.Methods) > 0 {
		for _, m := range h.Skip.Methods {
			if r.Method == m {
				return true
			}
		}
	}
	// WebSocket upgrade
	if h.Skip.WebSocket {
		if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
			strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
			return true
		}
	}
	// gRPC content-type
	if h.Skip.GRPC {
		if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
			return true
		}
	}
	return false
}

func (h *RequestDeadline) resolveTier(r *http.Request) string {
	// Attempt placeholder via Caddy Replacer if available
	if len(h.Sources) > 0 {
		if replVal := r.Context().Value(caddy.ReplacerCtxKey); replVal != nil {
			if repl, ok := replVal.(caddy.Replacer); ok {
				for _, s := range h.Sources {
					if s.Type == "placeholder" && s.Value != "" {
						if v := strings.TrimSpace(repl.ReplaceAll(s.Value, "")); v != "" {
							return v
						}
					}
				}
			}
		}
	}
	// Fallbacks: header and query
	for _, s := range h.Sources {
		switch s.Type {
		case "header":
			if v := strings.TrimSpace(r.Header.Get(s.Name)); v != "" {
				return v
			}
		case "query":
			if v := strings.TrimSpace(r.URL.Query().Get(s.Name)); v != "" {
				return v
			}
		}
	}
	return ""
}

// Interface guards
var _ caddyhttp.MiddlewareHandler = (*RequestDeadline)(nil)
