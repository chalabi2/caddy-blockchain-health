## Integrating a Per‑Request Deadline Handler into `caddy-blockchain-health`

This guide explains how to add a new `http.handlers` module that enforces per‑request deadlines (timeouts) by tier, inside the `caddy-blockchain-health` repository, without changing its existing upstream/health module. The goal is to apply tier‑based time budgets once at the site level in `Caddyfile`, have `reverse_proxy` honor them, and skip WebSocket/gRPC.

### What we’re adding (at a glance)

- A new handler module: `http.handlers.request_deadline`
- Lives in the same repo as `http.reverse_proxy.upstreams.blockchain_health` but is a separate module
- Reads tier (prefer `{http.auth.user.tier}`), maps to a duration, sets a per‑request context deadline
- Skips WebSocket and gRPC; never affects the health checker’s probe timeouts

### Why this, and not the health module’s `timeout`?

- The health module’s `timeout` is only for its internal probes. It doesn’t govern client request lifetimes.
- `reverse_proxy` transport timeouts are static at startup; they do not accept per‑request placeholders.
- A small handler setting `context.WithTimeout` per request is the robust, global, per‑tier solution; `reverse_proxy` will honor the canceled context.

## Repository layout changes (in `caddy-blockchain-health`)

Add a new package for the handler and register both modules during init.

```text
caddy-blockchain-health/
  module.go                        # existing: registers upstream module
  upstream/                        # existing upstream/health implementation
  handler/                         # NEW: per-request deadline handler
    request_deadline.go
    caddyfile.go                   # UnmarshalCaddyfile implementation
    README.md
```

In a central registration file (e.g., `module.go`), ensure both modules register:

```go
package caddyblockchainhealth

import (
    "github.com/caddyserver/caddy/v2"
    // existing imports...
    handler "github.com/chalabi2/caddy-blockchain-health/handler"
)

func init() {
    // existing upstream module registration
    // caddy.RegisterModule(&upstream.BlockchainHealth{})

    // NEW: request deadline handler registration
    caddy.RegisterModule(&handler.RequestDeadline{})
}
```

## Handler design

- **Module ID**: `http.handlers.request_deadline`
- **Implements**: `caddyhttp.MiddlewareHandler`
- **Inputs** (first non‑empty wins):
  - Placeholder: `{http.auth.user.tier}` (preferred with your `jwt_blacklist`)
  - Header: `X-User-Tier`
  - Query: `tier`
- **Tier map**: `FREE=1s`, `BASIC=3s`, `PREMIUM=5s`, `ENTERPRISE=8s`, `UNLIMITED=8s`; configurable with a default fallback
- **Skip**: WebSocket, gRPC, and specific methods (e.g., `OPTIONS`)
- **Behavior**: wrap request `Context` with timeout; if deadline exceeded before writing response headers → return 504; always cancel downstream work
- **Headers (optional)**: `X-Plan-Tier`, `X-Request-Timeout-Seconds`, `X-Request-Deadline-At`

### JSON config (schema sketch)

```json
{
  "handler": "request_deadline",
  "from": [
    { "type": "placeholder", "value": "{http.auth.user.tier}" },
    { "type": "header", "name": "X-User-Tier" },
    { "type": "query", "name": "tier" }
  ],
  "default_timeout": "5s",
  "tiers": {
    "FREE": "1s",
    "BASIC": "3s",
    "PREMIUM": "5s",
    "ENTERPRISE": "8s",
    "UNLIMITED": "8s"
  },
  "skip": { "websocket": true, "grpc": true, "methods": ["OPTIONS"] },
  "add_headers": true,
  "hard_cancel": true,
  "min_timeout": "200ms",
  "max_timeout": "30s",
  "log_level": "info"
}
```

### Caddyfile directive syntax (proposed)

```caddy
request_deadline {
    from placeholder {http.auth.user.tier}
    from header X-User-Tier
    from query tier

    default 5s
    tiers {
        FREE 1s
        BASIC 3s
        PREMIUM 5s
        ENTERPRISE 8s
        UNLIMITED 8s
    }

    skip {
        websocket true
        grpc true
        methods OPTIONS
    }

    add_headers true
    hard_cancel true
    min_timeout 200ms
    max_timeout 30s
    log_level info
}
```

## Implementation sketch (Go)

```go
package handler

import (
    "context"
    "encoding/json"
    "net/http"
    "strings"
    "time"

    "github.com/caddyserver/caddy/v2"
    "github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

type Source struct {
    Type  string `json:"type"`   // placeholder|header|query
    Name  string `json:"name"`   // header or query name
    Value string `json:"value"`  // placeholder value like {http.auth.user.tier}
}

type Skip struct {
    WebSocket bool     `json:"websocket"`
    GRPC      bool     `json:"grpc"`
    Methods   []string `json:"methods"`
}

type RequestDeadline struct {
    Sources        []Source          `json:"from,omitempty"`
    DefaultTimeout caddy.Duration    `json:"default_timeout,omitempty"`
    Tiers          map[string]string `json:"tiers,omitempty"`
    Skip           Skip              `json:"skip,omitempty"`
    AddHeaders     bool              `json:"add_headers,omitempty"`
    HardCancel     bool              `json:"hard_cancel,omitempty"`
    MinTimeout     caddy.Duration    `json:"min_timeout,omitempty"`
    MaxTimeout     caddy.Duration    `json:"max_timeout,omitempty"`
    LogLevel       string            `json:"log_level,omitempty"`

    // compiled
    tierDur map[string]time.Duration
}

func (RequestDeadline) CaddyModule() caddy.ModuleInfo {
    return caddy.ModuleInfo{
        ID:  "http.handlers.request_deadline",
        New: func() caddy.Module { return new(RequestDeadline) },
    }
}

func (h *RequestDeadline) Provision(ctx caddy.Context) error {
    h.tierDur = make(map[string]time.Duration)
    for k, v := range h.Tiers {
        d, err := time.ParseDuration(v)
        if err != nil { return err }
        h.tierDur[strings.ToUpper(k)] = d
    }
    return nil
}

func (h *RequestDeadline) Validate() error {
    min := time.Duration(h.MinTimeout)
    max := time.Duration(h.MaxTimeout)
    if min > 0 && max > 0 && min > max { return caddy.Err("min_timeout > max_timeout") }
    return nil
}

func (h *RequestDeadline) ServeHTTP(w http.ResponseWriter, r *http.Request, next caddyhttp.Handler) error {
    if h.shouldSkip(r) { return next.ServeHTTP(w, r) }

    tier := strings.TrimSpace(h.resolveTier(r))
    if tier == "" {
        tier = "__DEFAULT__"
    } else {
        tier = strings.ToUpper(tier)
    }

    timeout := time.Duration(h.DefaultTimeout)
    if d, ok := h.tierDur[tier]; ok { timeout = d }

    if min := time.Duration(h.MinTimeout); min > 0 && timeout < min { timeout = min }
    if max := time.Duration(h.MaxTimeout); max > 0 && timeout > max { timeout = max }

    ctx, cancel := context.WithTimeout(r.Context(), timeout)
    defer cancel()

    if h.AddHeaders {
        deadline := time.Now().Add(timeout).Unix()
        ww := caddyhttp.NewResponseRecorder(w, func(status int, header http.Header) (int, http.Header) {
            header.Set("X-Plan-Tier", tier)
            header.Set("X-Request-Timeout-Seconds", formatSeconds(timeout))
            header.Set("X-Request-Deadline-At", formatUnix(deadline))
            return status, header
        })
        r = r.WithContext(ctx)
        err := next.ServeHTTP(ww, r)
        if err != nil && ctx.Err() == context.DeadlineExceeded && !ww.WroteHeader() {
            caddyhttp.WriteJSON(w, map[string]any{"error": "gateway_timeout"}, http.StatusGatewayTimeout)
            return nil
        }
        return err
    }

    r = r.WithContext(ctx)
    err := next.ServeHTTP(w, r)
    if err != nil && ctx.Err() == context.DeadlineExceeded {
        // best-effort 504 if nothing was written
        if rw, ok := w.(interface{ Written() bool }); ok && !rw.Written() {
            caddyhttp.WriteJSON(w, map[string]any{"error": "gateway_timeout"}, http.StatusGatewayTimeout)
            return nil
        }
    }
    return err
}

func (h *RequestDeadline) shouldSkip(r *http.Request) bool {
    if len(h.Skip.Methods) > 0 {
        for _, m := range h.Skip.Methods { if r.Method == m { return true } }
    }
    if h.Skip.WebSocket {
        if strings.Contains(strings.ToLower(r.Header.Get("Connection")), "upgrade") &&
            strings.EqualFold(r.Header.Get("Upgrade"), "websocket") {
            return true
        }
    }
    if h.Skip.GRPC {
        if strings.HasPrefix(strings.ToLower(r.Header.Get("Content-Type")), "application/grpc") {
            return true
        }
    }
    return false
}

func (h *RequestDeadline) resolveTier(r *http.Request) string {
    repl := caddyhttp.GetVar(r.Context(), caddy.ReplacerCtxKey)
    for _, s := range h.Sources {
        switch s.Type {
        case "placeholder":
            if repl != nil {
                if val := repl.(caddy.Replacer).ReplaceAll(s.Value, ""); val != "" { return val }
            }
        case "header":
            if v := r.Header.Get(s.Name); v != "" { return v }
        case "query":
            if v := r.URL.Query().Get(s.Name); v != "" { return v }
        }
    }
    return ""
}

func formatSeconds(d time.Duration) string { return json.Number((d / time.Second).String()) }
func formatUnix(ts int64) string       { return json.Number((time.Duration(ts)).String()) }
```

> Note: The above is a sketch. Use proper helpers for writing headers and detecting whether headers/body were sent; keep error paths minimal. Ensure any helper types you use conform to Caddy’s interfaces in your version.

### Caddyfile support (`caddyfile.go`)

Implement `caddyfile.Unmarshaler` to parse the directive shown earlier. Keep parsing tolerant and validate durations and enum fields.

## Using the handler in your Caddyfile

Place the handler once per site, after auth and before proxying.

```caddy
api.chandrastation.com {
    route {
        jwt_blacklist { /* ... */ }
        @options method OPTIONS
        handle @options { respond "" 204 }
        usage
        import rate_limit_authenticated

        request_deadline {
            from placeholder {http.auth.user.tier}
            default 5s
            tiers { FREE 1s BASIC 3s PREMIUM 5s ENTERPRISE 8s UNLIMITED 8s }
            skip { websocket true grpc true methods OPTIONS }
            add_headers true
        }

        import chains/private/*.caddy
    }
}
```

For the public site, you can apply a flat cap (e.g., 1s) with no sources:

```caddy
nodes.chandrastation.com {
    route {
        usage
        import rate_limit_public
        request_deadline { default 1s skip { websocket true grpc true methods OPTIONS } }
        import chains/public/*.caddy
    }
}
```

## Build and release

Once the handler is committed in `caddy-blockchain-health` and registered via `init()`:

```bash
xcaddy build \
  --with github.com/chalabi2/caddy-blockchain-health@vX.Y.Z
```

Because both modules live in the same repo and are registered, a single `--with` brings both in.

## Testing strategy

- **Unit tests**: tier resolution, skip logic (WebSocket/gRPC/methods), duration parsing, min/max clamping
- **Integration**: upstream endpoint that sleeps; assert FREE (1s) → 504; PREMIUM (5s) → success; ensure headers are added when enabled
- **Order**: confirm it runs after auth and before any `reverse_proxy`
- **Observability**: confirm access logs show 504 on deadline; optional Prometheus counters

## Operational notes

- Keep this handler independent from the upstream module (single responsibility)
- Do not modify health probe behavior or upstream selection
- The handler only influences per‑request lifetimes via context deadlines
- WebSocket and gRPC are explicitly excluded

---

This approach lets you define tier‑based time budgets once per site, with zero changes to every `reverse_proxy` block and no coupling to health checking logic.
