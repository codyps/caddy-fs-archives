package caddyfsarchives

import (
	"io/fs"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	"github.com/mholt/archives"
)

func init() {
	caddy.RegisterModule(FS{})
}

// Interface guards
var (
	_ fs.StatFS             = (*FS)(nil)
	_ caddyfile.Unmarshaler = (*FS)(nil)
	_ caddy.Provisioner     = (*FS)(nil)
)

// FS is a Caddy virtual filesystem module for handling archive files.
type FS struct {
	fs.StatFS `json:"-"`

	RootPath string `json:"root_path,omitempty"`
}

// CaddyModule returns the Caddy module information.
func (FS) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.fs.archives",
		New: func() caddy.Module { return new(FS) },
	}
}

// Provision implements the [caddy.Provisioner] interface.
func (fs *FS) Provision(ctx caddy.Context) error {
	fs.StatFS = &archives.DeepFS{
		Root:    fs.RootPath,
		Context: ctx,
	}

	return nil
}

// UnmarshalCaddyfile unmarshals a caddyfile.
func (fs *FS) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	if !d.Next() { // skip block beginning
		return d.ArgErr()
	}

	for nesting := d.Nesting(); d.NextBlock(nesting); {
		switch d.Val() {
		case "root":
			if !d.AllArgs(&fs.RootPath) {
				return d.ArgErr()
			}
		default:
			return d.Errf("%s not a valid caddy.fs.archives option", d.Val())
		}
	}

	return nil
}
