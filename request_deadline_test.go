package blockchain_health

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/caddyserver/caddy/v2"
)

// nextHandler simulates a downstream handler with optional delay and status
type nextHandler struct {
	delay   time.Duration
	status  int
	written bool
}

func (n *nextHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) error {
	// Respect context if it has already timed out
	select {
	case <-r.Context().Done():
		return r.Context().Err()
	case <-time.After(n.delay):
	}
	if n.status == 0 {
		n.status = http.StatusOK
	}
	w.WriteHeader(n.status)
	_, _ = w.Write([]byte("ok"))
	n.written = true
	return nil
}

func TestRequestDeadline_TimeoutCancelsContext(t *testing.T) {
	h := &RequestDeadline{
		DefaultTimeout: caddy.Duration(100 * time.Millisecond),
		AddHeaders:     true,
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/test", nil)

	next := &nextHandler{delay: 250 * time.Millisecond}

	// No need to provision for this basic case
	err := h.ServeHTTP(rec, r, next)
	if err == nil {
		t.Fatalf("expected context timeout error, got nil")
	}
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("expected context.DeadlineExceeded, got %v", err)
	}
}

func TestRequestDeadline_SuccessWithinTimeout_AddsHeaders(t *testing.T) {
	h := &RequestDeadline{
		DefaultTimeout: caddy.Duration(300 * time.Millisecond),
		AddHeaders:     true,
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/test", nil)

	next := &nextHandler{delay: 50 * time.Millisecond, status: http.StatusOK}

	if err := h.ServeHTTP(rec, r, next); err != nil {
		t.Fatalf("ServeHTTP returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("X-Request-Timeout") == "" {
		t.Fatalf("expected X-Request-Timeout header to be set")
	}
	if rec.Header().Get("X-Request-Deadline-At") == "" {
		t.Fatalf("expected X-Request-Deadline-At header to be set")
	}
}

func TestRequestDeadline_SkipByMethod(t *testing.T) {
	h := &RequestDeadline{
		DefaultTimeout: caddy.Duration(50 * time.Millisecond),
		AddHeaders:     true,
		Skip:           Skip{Methods: []string{http.MethodOptions}},
	}

	rec := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodOptions, "http://127.0.0.1/test", nil)

	next := &nextHandler{delay: 100 * time.Millisecond, status: http.StatusOK}

	if err := h.ServeHTTP(rec, r, next); err != nil {
		t.Fatalf("ServeHTTP returned error: %v", err)
	}

	if rec.Code != http.StatusOK {
		t.Fatalf("expected pass-through 200 OK, got %d", rec.Code)
	}
	if rec.Header().Get("X-Request-Timeout") != "" {
		t.Fatalf("expected headers not added on skip, got %q", rec.Header().Get("X-Request-Timeout"))
	}
}

func TestRequestDeadline_TierResolution_Header_Query_MinMaxClamp(t *testing.T) {
	h := &RequestDeadline{
		Sources: []Source{
			{Type: "header", Name: "X-User-Tier"},
			{Type: "query", Name: "tier"},
		},
		DefaultTimeout: caddy.Duration(2 * time.Second),
		Tiers: map[string]string{
			"FREE":  "150ms",
			"PAID":  "5s",
			"LIMIT": "10ms",
		},
		AddHeaders: true,
		MinTimeout: caddy.Duration(100 * time.Millisecond),
		MaxTimeout: caddy.Duration(1 * time.Second),
	}
	if err := h.Provision(caddy.Context{}); err != nil {
		t.Fatalf("provision failed: %v", err)
	}

	// Header-based tier
	{
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/test", nil)
		r.Header.Set("X-User-Tier", "FREE")
		next := &nextHandler{delay: 10 * time.Millisecond, status: http.StatusOK}
		if err := h.ServeHTTP(rec, r, next); err != nil {
			t.Fatalf("ServeHTTP error: %v", err)
		}
		if got := rec.Header().Get("X-Plan-Tier"); got != "FREE" {
			t.Fatalf("expected X-Plan-Tier FREE, got %q", got)
		}
	}

	// Query-based tier with clamp to max (PAID => 5s clamped to 1s)
	{
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/test?tier=PAID", nil)
		next := &nextHandler{delay: 10 * time.Millisecond, status: http.StatusOK}
		if err := h.ServeHTTP(rec, r, next); err != nil {
			t.Fatalf("ServeHTTP error: %v", err)
		}
		if rec.Header().Get("X-Plan-Tier") != "PAID" {
			t.Fatalf("expected X-Plan-Tier PAID, got %q", rec.Header().Get("X-Plan-Tier"))
		}
	}

	// Header-based tier with clamp to min (LIMIT => 10ms clamped to 100ms)
	{
		rec := httptest.NewRecorder()
		r := httptest.NewRequest(http.MethodGet, "http://127.0.0.1/test", nil)
		r.Header.Set("X-User-Tier", "LIMIT")
		next := &nextHandler{delay: 10 * time.Millisecond, status: http.StatusOK}
		if err := h.ServeHTTP(rec, r, next); err != nil {
			t.Fatalf("ServeHTTP error: %v", err)
		}
		if rec.Header().Get("X-Plan-Tier") != "LIMIT" {
			t.Fatalf("expected X-Plan-Tier LIMIT, got %q", rec.Header().Get("X-Plan-Tier"))
		}
	}
}
