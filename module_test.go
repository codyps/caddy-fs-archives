package caddyfsarchives

import (
	"archive/zip"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/caddyserver/caddy/v2"
	"github.com/caddyserver/caddy/v2/caddyconfig"
	"github.com/caddyserver/caddy/v2/caddyconfig/caddyfile"
	_ "github.com/caddyserver/caddy/v2/caddyconfig/httpcaddyfile"
	_ "github.com/caddyserver/caddy/v2/modules/caddyfs"
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp"
	_ "github.com/caddyserver/caddy/v2/modules/caddyhttp/fileserver"
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

func TestOpenArchiveMemberIsSeekable(t *testing.T) {
	const contents = "0123456789abcdef"
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "sample.zip"), "member.txt", contents)

	fsys := provisionFS(t, root)
	file, err := fsys.Open("sample.zip/member.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	seeker, ok := file.(io.ReadSeeker)
	if !ok {
		t.Fatalf("archive member has type %T; want io.ReadSeeker", file)
	}

	if offset, err := seeker.Seek(8, io.SeekStart); err != nil || offset != 8 {
		t.Fatalf("SeekStart = (%d, %v); want (8, nil)", offset, err)
	}
	assertRead(t, seeker, "89ab")

	if offset, err := seeker.Seek(-6, io.SeekCurrent); err != nil || offset != 6 {
		t.Fatalf("backward SeekCurrent = (%d, %v); want (6, nil)", offset, err)
	}
	assertRead(t, seeker, "6789")

	if offset, err := seeker.Seek(-4, io.SeekEnd); err != nil || offset != 12 {
		t.Fatalf("SeekEnd = (%d, %v); want (12, nil)", offset, err)
	}
	assertRead(t, seeker, "cdef")
}

func TestArchiveMemberWorksWithServeContent(t *testing.T) {
	const contents = "0123456789abcdef"
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "sample.zip"), "member.txt", contents)

	fsys := provisionFS(t, root)
	file, err := fsys.Open("sample.zip/member.txt")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		t.Fatal(err)
	}

	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/sample.zip/member.txt", nil)
	request.Header.Set("Range", "bytes=2-5")
	http.ServeContent(recorder, request, info.Name(), info.ModTime(), file.(io.ReadSeeker))

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d; want %d", recorder.Code, http.StatusPartialContent)
	}
	if body := recorder.Body.String(); body != "2345" {
		t.Fatalf("body = %q; want %q", body, "2345")
	}
}

func TestCaddyRoundTripServesArchiveMember(t *testing.T) {
	const contents = "0123456789abcdef"
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "sample.zip"), "member.txt", contents)

	baseURL := startCaddyFileServer(t, root)

	response, err := http.Get(baseURL + "/sample.zip/member.txt")
	if err != nil {
		t.Fatal(err)
	}
	assertResponse(t, response, http.StatusOK, contents)

	request, err := http.NewRequest(http.MethodGet, baseURL+"/sample.zip/member.txt", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Range", "bytes=2-5")
	response, err = http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	if contentRange := response.Header.Get("Content-Range"); contentRange != "bytes 2-5/16" {
		response.Body.Close()
		t.Fatalf("Content-Range = %q; want %q", contentRange, "bytes 2-5/16")
	}
	assertResponse(t, response, http.StatusPartialContent, "2345")
}

func TestCaddyRoundTripBrowseListing(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "sample.zip"), "member.txt", "contents")
	if err := os.WriteFile(filepath.Join(root, "ordinary.txt"), []byte("contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	request, err := http.NewRequest(http.MethodGet, startCaddyFileServer(t, root)+"/", nil)
	if err != nil {
		t.Fatal(err)
	}
	request.Header.Set("Accept", "application/json")
	response, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(response.Body)
		t.Fatalf("status = %d; want %d; body = %q", response.StatusCode, http.StatusOK, body)
	}

	var entries []struct {
		Name  string `json:"name"`
		IsDir bool   `json:"is_dir"`
	}
	if err := json.NewDecoder(response.Body).Decode(&entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	for _, entry := range entries {
		switch entry.Name {
		case "sample.zip/":
			if !entry.IsDir {
				t.Error("Caddy did not represent sample.zip as a directory")
			}
		case "ordinary.txt":
			if entry.IsDir {
				t.Error("Caddy represented ordinary.txt as a directory")
			}
		default:
			t.Errorf("unexpected entry %q", entry.Name)
		}
	}
}

func TestBrowseListingMarksArchivesAsDirectories(t *testing.T) {
	root := t.TempDir()
	writeZip(t, filepath.Join(root, "sample.zip"), "member.txt", "contents")
	if err := os.WriteFile(filepath.Join(root, "ordinary.txt"), []byte("contents"), 0o600); err != nil {
		t.Fatal(err)
	}

	fsys := provisionFS(t, root)
	file, err := fsys.Open(".")
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()
	dir, ok := file.(fs.ReadDirFile)
	if !ok {
		t.Fatalf("root directory has type %T; want fs.ReadDirFile", file)
	}

	entries, err := dir.ReadDir(-1)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("got %d entries; want 2", len(entries))
	}
	for _, entry := range entries {
		switch entry.Name() {
		case "sample.zip":
			if !entry.IsDir() {
				t.Error("sample.zip is not represented as a directory")
			}
		case "ordinary.txt":
			if entry.IsDir() {
				t.Error("ordinary.txt is represented as a directory")
			}
		default:
			t.Errorf("unexpected entry %q", entry.Name())
		}
	}
}

func TestUnprovisionedFilesystemReturnsErrors(t *testing.T) {
	fsys := &FS{}
	if _, err := fsys.Open("."); err == nil {
		t.Error("Open succeeded before provisioning")
	}
	if _, err := fsys.Stat("."); err == nil {
		t.Error("Stat succeeded before provisioning")
	}
	if _, err := fsys.ReadDir("."); err == nil {
		t.Error("ReadDir succeeded before provisioning")
	}
}

func provisionFS(t *testing.T, root string) *FS {
	t.Helper()
	fsys := &FS{Root: root}
	if err := fsys.Provision(caddy.Context{Context: context.Background()}); err != nil {
		t.Fatal(err)
	}
	return fsys
}

func startCaddyFileServer(t *testing.T, root string) string {
	t.Helper()
	adapter := caddyconfig.GetAdapter("caddyfile")
	if adapter == nil {
		t.Fatal("Caddyfile adapter is not registered")
	}

	for range 10 {
		port := unusedTCPPort(t)
		config := fmt.Sprintf(`{
	admin off
	auto_https off
	persist_config off
	filesystem test_archives archives {
		root %q
	}
}

http://127.0.0.1:%d {
	file_server browse {
		fs test_archives
	}
}
`, root, port)
		configJSON, warnings, err := adapter.Adapt([]byte(config), nil)
		if err != nil {
			t.Fatalf("adapting Caddyfile: %v", err)
		}
		for _, warning := range warnings {
			t.Logf("Caddyfile warning: %s", warning)
		}
		if err := caddy.Load(configJSON, true); err != nil {
			if strings.Contains(err.Error(), "address already in use") {
				continue
			}
			t.Fatalf("loading Caddy config: %v", err)
		}
		t.Cleanup(func() {
			if err := caddy.Stop(); err != nil {
				t.Errorf("stopping Caddy: %v", err)
			}
		})

		return fmt.Sprintf("http://127.0.0.1:%d", port)
	}
	t.Fatal("loading Caddy config: could not reserve a TCP port")
	return ""
}

func unusedTCPPort(t *testing.T) int {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	port := listener.Addr().(*net.TCPAddr).Port
	if err := listener.Close(); err != nil {
		t.Fatal(err)
	}
	return port
}

func assertResponse(t *testing.T, response *http.Response, expectedStatus int, expectedBody string) {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatal(err)
	}
	if response.StatusCode != expectedStatus {
		t.Fatalf("status = %d; want %d; body = %q", response.StatusCode, expectedStatus, body)
	}
	if string(body) != expectedBody {
		t.Fatalf("body = %q; want %q", body, expectedBody)
	}
}

func writeZip(t *testing.T, filename, memberName, contents string) {
	t.Helper()
	output, err := os.Create(filename)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(output)
	member, err := writer.Create(memberName)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.WriteString(member, contents); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	if err := output.Close(); err != nil {
		t.Fatal(err)
	}
}

func assertRead(t *testing.T, reader io.Reader, expected string) {
	t.Helper()
	buf := make([]byte, len(expected))
	if _, err := io.ReadFull(reader, buf); err != nil {
		t.Fatal(err)
	}
	if actual := string(buf); actual != expected {
		t.Fatalf("read %q; want %q", actual, expected)
	}
}
