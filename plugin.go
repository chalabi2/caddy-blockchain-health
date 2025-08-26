package blockchain_health

import (
	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/caddyserver/caddy/v2/modules/caddyhttp/reverseproxy"
)

func init() {
	caddy.RegisterModule(BlockchainHealthUpstream{})
}

// CaddyModule returns the Caddy module information.
func (BlockchainHealthUpstream) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "http.reverse_proxy.upstreams.blockchain_health",
		New: func() caddy.Module { return new(BlockchainHealthUpstream) },
	}
}

// UnmarshalCaddyfile implements caddyfile.Unmarshaler.
func (b *BlockchainHealthUpstream) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	return b.parseCaddyfile(d)
}

// Provision implements caddy.Provisioner.
func (b *BlockchainHealthUpstream) Provision(ctx caddy.Context) error {
	return b.provision(ctx)
}

// Validate implements caddy.Validator.
func (b *BlockchainHealthUpstream) Validate() error {
	return b.validate()
}

// Cleanup implements caddy.CleanerUpper.
func (b *BlockchainHealthUpstream) Cleanup() error {
	return b.cleanup()
}

// Interface guards
var (
	_ caddy.Provisioner           = (*BlockchainHealthUpstream)(nil)
	_ caddy.Validator             = (*BlockchainHealthUpstream)(nil)
	_ caddy.CleanerUpper          = (*BlockchainHealthUpstream)(nil)
	_ caddyfile.Unmarshaler       = (*BlockchainHealthUpstream)(nil)
	_ reverseproxy.UpstreamSource = (*BlockchainHealthUpstream)(nil)
)
