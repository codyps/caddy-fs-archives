// Package caddyfss3 implements a Caddy virtual filesystem module for AWS S3 (and compatible) object store.
package caddyfss3

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
)

// FS is a Caddy virtual filesystem module for AWS S3 (and compatible) object store.
type FS struct {
	fs.StatFS `json:"-"`

	RootPath string `json:"root_path,omitempty"`

	// A regex matching files to treat as archives and expose their contents as
	// virtual subdirectories. For example, `\.zip$` would match all .zip files
	// and expose their contents as virtual subdirectories.
	// Default is `\.zip$`.
	//ArchiveRegex string `json:"archive_regex,omitempty"`

	// If true, expose the original archive files as well as the virtual subdirectories. If false, only
	// expose the virtual subdirectories. Default is true.
	//ExposeArchiveFiles *bool `json:"expose_archive_files,omitempty"`
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
	//if fs.ArchiveRegex == "" {
	//	fs.ArchiveRegex = `\.zip$`
	//}

	//if fs.ExposeArchiveFiles == nil {
	//	trueVal := true
	//	fs.ExposeArchiveFiles = &trueVal
	//}

	// ReadSeeker is required by Caddy
	fs.StatFS = &archives.DeepFS{
		Root:    fs.RootPath,
		Context: nil,
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
			return d.Errf("%s not a valid caddy.fs.s3 option", d.Val())
		}
	}

	return nil
}
