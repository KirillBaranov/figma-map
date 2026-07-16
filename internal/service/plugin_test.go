package service

import (
	"archive/zip"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"testing"
)

// buildPluginZip constructs a minimal figma-map-plugin.zip fixture with the
// same layout the release CI step produces (zip -r figma-map-plugin.zip
// figma-map-plugin), so tests exercise EnsurePlugin's real extraction and
// swap logic without depending on an actual Vite build.
func buildPluginZip(t *testing.T, manifestContent string) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	files := map[string]string{
		"figma-map-plugin/manifest.json":   manifestContent,
		"figma-map-plugin/dist/index.html": "<html></html>",
		"figma-map-plugin/dist/code.js":    "// code",
	}
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("create %s in fixture zip: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("write %s in fixture zip: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("close fixture zip: %v", err)
	}
	return buf.Bytes()
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func TestEnsurePlugin(t *testing.T) {
	home := t.TempDir()
	t.Setenv("HOME", home)

	assets := map[string][]byte{}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		data, ok := assets[filepath.Base(r.URL.Path)]
		if !ok {
			http.NotFound(w, r)
			return
		}
		_, _ = w.Write(data)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	// Redirect every request's scheme/host to the local test server,
	// keeping only the final path segment — so release.BaseURL's
	// github.com URLs resolve against fixtures served by basename, with
	// zero changes to production download code.
	target, err := url.Parse(srv.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	prevTransport := http.DefaultTransport
	realTransport := &http.Transport{}
	http.DefaultTransport = roundTripFunc(func(req *http.Request) (*http.Response, error) {
		req2 := req.Clone(req.Context())
		req2.URL.Scheme = target.Scheme
		req2.URL.Host = target.Host
		req2.URL.Path = "/" + filepath.Base(req.URL.Path)
		req2.Host = ""
		return realTransport.RoundTrip(req2)
	})
	t.Cleanup(func() { http.DefaultTransport = prevTransport })

	setFixture := func(manifest string) {
		zipBytes := buildPluginZip(t, manifest)
		sum := sha256.Sum256(zipBytes)
		assets["figma-map-plugin.zip"] = zipBytes
		assets["figma-map-plugin.zip.sha256"] = []byte(hex.EncodeToString(sum[:]) + "\n")
	}

	ctx := context.Background()

	setFixture(`{"name":"v1"}`)
	changed, err := EnsurePlugin(ctx, "v0.0.1-test", false)
	if err != nil {
		t.Fatalf("first EnsurePlugin: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on first install")
	}

	dir, err := pluginDir()
	if err != nil {
		t.Fatalf("pluginDir: %v", err)
	}
	manifestPath := filepath.Join(dir, "manifest.json")
	got, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.json: %v", err)
	}
	if string(got) != `{"name":"v1"}` {
		t.Fatalf("manifest.json = %q, want %q", got, `{"name":"v1"}`)
	}
	if v := readPluginVersion(); v != "v0.0.1-test" {
		t.Fatalf("plugin version = %q, want %q", v, "v0.0.1-test")
	}

	// Re-running with the same version must no-op (no changed report, and
	// no need for the fixture server to even still have the file).
	delete(assets, "figma-map-plugin.zip")
	delete(assets, "figma-map-plugin.zip.sha256")
	changed, err = EnsurePlugin(ctx, "v0.0.1-test", false)
	if err != nil {
		t.Fatalf("no-op EnsurePlugin: %v", err)
	}
	if changed {
		t.Fatal("expected changed=false when version already matches")
	}

	// A version bump must fetch fresh content and fully replace the old
	// directory (no stale files from the previous version left behind).
	setFixture(`{"name":"v2"}`)
	changed, err = EnsurePlugin(ctx, "v0.0.2-test", false)
	if err != nil {
		t.Fatalf("update EnsurePlugin: %v", err)
	}
	if !changed {
		t.Fatal("expected changed=true on version bump")
	}
	got, err = os.ReadFile(manifestPath)
	if err != nil {
		t.Fatalf("read manifest.json after update: %v", err)
	}
	if string(got) != `{"name":"v2"}` {
		t.Fatalf("manifest.json after update = %q, want %q", got, `{"name":"v2"}`)
	}
	if v := readPluginVersion(); v != "v0.0.2-test" {
		t.Fatalf("plugin version after update = %q, want %q", v, "v0.0.2-test")
	}

	// The swap must not leave scratch directories behind.
	for _, leftover := range []string{"plugin.new", "plugin.old"} {
		if _, err := os.Stat(filepath.Join(filepath.Dir(dir), leftover)); !os.IsNotExist(err) {
			t.Fatalf("leftover scratch dir %s should not exist, stat err=%v", leftover, err)
		}
	}
}
