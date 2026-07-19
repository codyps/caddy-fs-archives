package caddyfsarchives

import (
	"errors"
	"fmt"
	"io"
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
	_ fs.ReadDirFS          = (*FS)(nil)
	_ caddyfile.Unmarshaler = (*FS)(nil)
	_ caddy.Provisioner     = (*FS)(nil)
)

// FS is a Caddy virtual filesystem module for handling archive files.
type FS struct {
	Root string `json:"root_path,omitempty"`

	deep *archives.DeepFS
}

func (FS) CaddyModule() caddy.ModuleInfo {
	return caddy.ModuleInfo{
		ID:  "caddy.fs.archives",
		New: func() caddy.Module { return new(FS) },
	}
}

func (fs *FS) Provision(ctx caddy.Context) error {
	fs.deep = &archives.DeepFS{
		Root:    fs.Root,
		Context: ctx,
	}

	return nil
}

// Open opens name and adapts the result to the additional file contracts used
// by Caddy's file server. In particular, regular files must be seekable and
// directory listings must go through DeepFS so archives appear as directories.
func (fsys *FS) Open(name string) (fs.File, error) {
	if fsys.deep == nil {
		return nil, errors.New("archives filesystem is not provisioned")
	}

	file, err := fsys.deep.Open(name)
	if err != nil {
		return nil, err
	}

	info, err := file.Stat()
	if err != nil {
		file.Close()
		return nil, err
	}

	if info.IsDir() {
		return &readDirFile{File: file, fsys: fsys.deep, name: name}, nil
	}
	if _, ok := file.(io.ReadSeeker); ok {
		return file, nil
	}

	return &seekableFile{
		file: file,
		fsys: fsys.deep,
		name: name,
		info: info,
	}, nil
}

// Stat returns information about name.
func (fsys *FS) Stat(name string) (fs.FileInfo, error) {
	if fsys.deep == nil {
		return nil, errors.New("archives filesystem is not provisioned")
	}
	return fsys.deep.Stat(name)
}

// ReadDir returns directory entries with archives represented as directories.
func (fsys *FS) ReadDir(name string) ([]fs.DirEntry, error) {
	if fsys.deep == nil {
		return nil, errors.New("archives filesystem is not provisioned")
	}
	return fsys.deep.ReadDir(name)
}

// readDirFile ensures callers which list an opened directory, including Caddy's
// browse handler, use DeepFS.ReadDir instead of the underlying operating-system
// directory handle.
type readDirFile struct {
	fs.File
	fsys    *archives.DeepFS
	name    string
	entries []fs.DirEntry
	offset  int
	loaded  bool
}

func (file *readDirFile) ReadDir(n int) ([]fs.DirEntry, error) {
	if !file.loaded {
		entries, err := file.fsys.ReadDir(file.name)
		if err != nil {
			return nil, err
		}
		file.entries = entries
		file.loaded = true
	}

	if n <= 0 {
		entries := file.entries[file.offset:]
		file.offset = len(file.entries)
		return entries, nil
	}
	if file.offset >= len(file.entries) {
		return nil, io.EOF
	}

	end := min(file.offset+n, len(file.entries))
	entries := file.entries[file.offset:end]
	file.offset = end
	if len(entries) < n {
		return entries, io.EOF
	}
	return entries, nil
}

// seekableFile adds seeking to streaming archive members without buffering the
// whole member in memory. Seek updates the logical offset; the next Read reopens
// the member when necessary and discards bytes up to that offset.
type seekableFile struct {
	file       fs.File
	fsys       *archives.DeepFS
	name       string
	info       fs.FileInfo
	offset     int64
	fileOffset int64
	closed     bool
}

func (file *seekableFile) Read(buf []byte) (int, error) {
	if file.closed {
		return 0, fs.ErrClosed
	}
	if len(buf) == 0 {
		return 0, nil
	}
	if file.offset >= file.info.Size() {
		return 0, io.EOF
	}

	if err := file.positionUnderlyingFile(); err != nil {
		return 0, err
	}
	n, err := file.file.Read(buf)
	file.offset += int64(n)
	file.fileOffset += int64(n)
	return n, err
}

func (file *seekableFile) Seek(offset int64, whence int) (int64, error) {
	if file.closed {
		return 0, fs.ErrClosed
	}

	var base int64
	switch whence {
	case io.SeekStart:
	case io.SeekCurrent:
		base = file.offset
	case io.SeekEnd:
		base = file.info.Size()
	default:
		return 0, fmt.Errorf("invalid seek whence %d", whence)
	}

	target := base + offset
	if (offset > 0 && target < base) || (offset < 0 && target > base) {
		return 0, errors.New("seek offset overflow")
	}
	if target < 0 {
		return 0, errors.New("negative seek offset")
	}
	file.offset = target
	return target, nil
}

func (file *seekableFile) Stat() (fs.FileInfo, error) {
	if file.closed {
		return nil, fs.ErrClosed
	}
	return file.info, nil
}

func (file *seekableFile) Close() error {
	if file.closed {
		return fs.ErrClosed
	}
	file.closed = true
	return file.file.Close()
}

func (file *seekableFile) positionUnderlyingFile() error {
	if file.fileOffset > file.offset {
		reopened, err := file.fsys.Open(file.name)
		if err != nil {
			return err
		}
		if err := file.file.Close(); err != nil {
			reopened.Close()
			return err
		}
		file.file = reopened
		file.fileOffset = 0
	}

	if remaining := file.offset - file.fileOffset; remaining > 0 {
		copied, err := io.CopyN(io.Discard, file.file, remaining)
		file.fileOffset += copied
		if err != nil {
			return err
		}
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
