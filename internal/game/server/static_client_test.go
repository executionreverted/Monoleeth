package server

import (
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestClientStaticHandlerServesBuiltClientAndSPAFallback(t *testing.T) {
	staticDir := t.TempDir()
	writeStaticTestFile(t, filepath.Join(staticDir, "index.html"), "<!doctype html><div id=\"app\"></div>")
	writeStaticTestFile(t, filepath.Join(staticDir, "assets", "app.js"), "console.log('client')")

	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		ClientStaticDir:   staticDir,
		ContentRepository: staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	defer httpServer.Close()

	for _, requestPath := range []string{"/", "/playtest/sector/1-1"} {
		response, err := http.Get(httpServer.URL + requestPath)
		if err != nil {
			t.Fatalf("GET %s: %v", requestPath, err)
		}
		body := readHTTPTestBody(t, response)
		if response.StatusCode != http.StatusOK {
			t.Fatalf("GET %s status = %d body = %q, want 200", requestPath, response.StatusCode, body)
		}
		if !strings.Contains(body, `id="app"`) {
			t.Fatalf("GET %s body = %q, want index.html", requestPath, body)
		}
	}

	response, err := http.Get(httpServer.URL + "/assets/app.js")
	if err != nil {
		t.Fatalf("GET asset: %v", err)
	}
	body := readHTTPTestBody(t, response)
	if response.StatusCode != http.StatusOK || !strings.Contains(body, "client") {
		t.Fatalf("asset response = %d %q, want app.js", response.StatusCode, body)
	}
}

func TestClientStaticHandlerDoesNotMaskAPIRoutesOrMissingAssets(t *testing.T) {
	staticDir := t.TempDir()
	writeStaticTestFile(t, filepath.Join(staticDir, "index.html"), "<!doctype html>")

	gameServer, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		ClientStaticDir:   staticDir,
		ContentRepository: staticContentRepositoryForTest(),
	})
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	httpServer := httptest.NewServer(gameServer.Handler())
	defer httpServer.Close()

	for _, requestPath := range []string{"/api/unknown", "/missing.js"} {
		response, err := http.Get(httpServer.URL + requestPath)
		if err != nil {
			t.Fatalf("GET %s: %v", requestPath, err)
		}
		body := readHTTPTestBody(t, response)
		if response.StatusCode != http.StatusNotFound {
			t.Fatalf("GET %s status = %d body = %q, want 404", requestPath, response.StatusCode, body)
		}
		if strings.Contains(body, "<!doctype html>") {
			t.Fatalf("GET %s body = %q, should not serve SPA index", requestPath, body)
		}
	}
}

func TestNewRejectsInvalidClientStaticDir(t *testing.T) {
	_, err := New(Config{
		AllowedOrigins:    []string{testOrigin},
		ClientStaticDir:   filepath.Join(t.TempDir(), "missing"),
		ContentRepository: staticContentRepositoryForTest(),
	})
	if err == nil {
		t.Fatal("New() error = nil, want missing static dir error")
	}
	if !strings.Contains(err.Error(), EnvClientStaticDir) {
		t.Fatalf("New() error = %q, want %s", err, EnvClientStaticDir)
	}
}

func writeStaticTestFile(t *testing.T, path string, contents string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("mkdir static test file: %v", err)
	}
	if err := os.WriteFile(path, []byte(contents), 0o644); err != nil {
		t.Fatalf("write static test file: %v", err)
	}
}

func readHTTPTestBody(t *testing.T, response *http.Response) string {
	t.Helper()
	defer response.Body.Close()
	body, err := io.ReadAll(response.Body)
	if err != nil {
		t.Fatalf("read response body: %v", err)
	}
	return string(body)
}
