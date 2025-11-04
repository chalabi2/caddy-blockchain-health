package blockchain_health

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	httpcaddyfile "github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp"
)

func init() {
	// Register Caddyfile directive for this handler
	httpcaddyfile.RegisterHandlerDirective("request_deadline", parseRequestDeadlineCaddyfile)
}

func parseRequestDeadlineCaddyfile(h httpcaddyfile.Helper) (caddyhttp.MiddlewareHandler, error) {
	rd := new(RequestDeadline)
	// Delegate parsing to UnmarshalCaddyfile for consistency
	if err := rd.UnmarshalCaddyfile(h.Dispenser); err != nil {
		return nil, err
	}
	return rd, nil
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler for request_deadline
func (h *RequestDeadline) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	for d.Next() {
		for d.NextBlock(0) {
			switch d.Val() {
			case "from":
				// Syntax: from <placeholder|header|query> <value>
				if !d.NextArg() {
					return d.ArgErr()
				}
				typ := d.Val()
				if !d.NextArg() {
					return d.ArgErr()
				}
				val := d.Val()
				s := Source{Type: typ}
				switch typ {
				case "placeholder":
					s.Value = val
				case "header", "query":
					s.Name = val
				default:
					return d.Errf("unknown from type: %s", typ)
				}
				h.Sources = append(h.Sources, s)

			case "default":
				if !d.NextArg() {
					return d.ArgErr()
				}
				dur, err := time.ParseDuration(d.Val())
				if err != nil {
					return d.Errf("invalid default duration: %v", err)
				}
				h.DefaultTimeout = caddy.Duration(dur)

			case "tiers":
				if h.Tiers == nil {
					h.Tiers = make(map[string]string)
				}
				for d.NextBlock(1) {
					name := d.Val()
					if name == "" {
						return d.ArgErr()
					}
					if !d.NextArg() {
						return d.ArgErr()
					}
					h.Tiers[strings.ToUpper(name)] = d.Val()
				}

			case "skip":
				for d.NextBlock(1) {
					switch d.Val() {
					case "websocket":
						if !d.NextArg() {
							return d.ArgErr()
						}
						b, err := strconv.ParseBool(d.Val())
						if err != nil {
							return d.Errf("invalid websocket bool: %v", err)
						}
						h.Skip.WebSocket = b
					case "grpc":
						if !d.NextArg() {
							return d.ArgErr()
						}
						b, err := strconv.ParseBool(d.Val())
						if err != nil {
							return d.Errf("invalid grpc bool: %v", err)
						}
						h.Skip.GRPC = b
					case "methods":
						methods := []string{}
						for d.NextArg() {
							methods = append(methods, d.Val())
						}
						h.Skip.Methods = append(h.Skip.Methods, methods...)
					default:
						return d.Errf("unknown skip directive: %s", d.Val())
					}
				}

			case "add_headers":
				if !d.NextArg() {
					return d.ArgErr()
				}
				b, err := strconv.ParseBool(d.Val())
				if err != nil {
					return d.Errf("invalid add_headers bool: %v", err)
				}
				h.AddHeaders = b

			case "min_timeout":
				if !d.NextArg() {
					return d.ArgErr()
				}
				dur, err := time.ParseDuration(d.Val())
				if err != nil {
					return d.Errf("invalid min_timeout: %v", err)
				}
				h.MinTimeout = caddy.Duration(dur)

			case "max_timeout":
				if !d.NextArg() {
					return d.ArgErr()
				}
				dur, err := time.ParseDuration(d.Val())
				if err != nil {
					return d.Errf("invalid max_timeout: %v", err)
				}
				h.MaxTimeout = caddy.Duration(dur)

			default:
				return d.Errf("unknown directive: %s", d.Val())
			}
		}
	}

	// basic validation
	if err := h.Validate(); err != nil {
		return fmt.Errorf("request_deadline validation: %w", err)
	}
	return nil
}

// Interface guard
var _ caddyfile.Unmarshaler = (*RequestDeadline)(nil)
