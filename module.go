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

var (
	_ fs.StatFS             = (*FS)(nil)
	_ caddyfile.Unmarshaler = (*FS)(nil)
	_ caddy.Provisioner     = (*FS)(nil)
)

// FS is a Caddy virtual filesystem module for handling archive files.
type FS struct {
	fs.StatFS `json:"-"`

	Root string `json:"root_path,omitempty"`
}

func (FS) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.fs.archives",
		New: func() caddy.Module { return new(FS) },
	}
}

func (fs *FS) Provision(ctx caddy.Context) error {
	fs.StatFS = &archives.DeepFS{
		Root:    fs.Root,
		Context: ctx,
	}

	return nil
}

func (fs *FS) UnmarshalCaddyfile(d *caddyfile.Dispenser) error {
	if !d.Next() { // skip block beginning
		return d.ArgErr()
	}

	for nesting := d.Nesting(); d.NextBlock(nesting); {
		switch d.Val() {
		case "root":
			if !d.AllArgs(&fs.Root) {
				return d.ArgErr()
			}
		default:
			return d.Errf("%s not a valid caddy.fs.archives option", d.Val())
		}
	}

	return nil
}
