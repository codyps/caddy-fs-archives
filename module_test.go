package caddyfsarchives

import (
	"testing"

	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
)

func TestCaddyModule(t *testing.T) {
	info := FS{}.CaddyModule()
	if info.ID != "caddy.fs.archives" {
		t.Errorf("unexpected module ID: %s", info.ID)
	}
	if info.New == nil {
		t.Error("New function is nil")
	}
	if _, ok := info.New().(*FS); !ok {
		t.Error("New() did not return *FS")
	}
}

func TestUnmarshalCaddyfile_Empty(t *testing.T) {
	fs := &FS{}
	d := caddyfile.NewTestDispenser(`archives`)
	if err := fs.UnmarshalCaddyfile(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fs.Root != "" {
		t.Errorf("expected empty Root, got %q", fs.Root)
	}
}

func TestUnmarshalCaddyfile_Root(t *testing.T) {
	fs := &FS{}
	d := caddyfile.NewTestDispenser(`archives {
		root /var/www
	}`)
	if err := fs.UnmarshalCaddyfile(d); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if fs.Root != "/var/www" {
		t.Errorf("expected Root /var/www, got %q", fs.Root)
	}
}

func TestUnmarshalCaddyfile_UnknownOption(t *testing.T) {
	fs := &FS{}
	d := caddyfile.NewTestDispenser(`archives {
		bogus value
	}`)
	if err := fs.UnmarshalCaddyfile(d); err == nil {
		t.Error("expected error for unknown option")
	}
}

func TestUnmarshalCaddyfile_RootMissingArg(t *testing.T) {
	fs := &FS{}
	d := caddyfile.NewTestDispenser(`archives {
		root
	}`)
	if err := fs.UnmarshalCaddyfile(d); err == nil {
		t.Error("expected error when root has no argument")
	}
}
