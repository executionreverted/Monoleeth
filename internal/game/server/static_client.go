package server

import (
	"fmt"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
)

const clientIndexFile = "index.html"

func newClientStaticHandler(staticDir string) (http.Handler, error) {
	staticDir = strings.TrimSpace(staticDir)
	if staticDir == "" {
		return nil, fmt.Errorf("%s is empty", EnvClientStaticDir)
	}
	info, err := os.Stat(staticDir)
	if err != nil {
		return nil, fmt.Errorf("%s %q: %w", EnvClientStaticDir, staticDir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("%s %q is not a directory", EnvClientStaticDir, staticDir)
	}
	indexPath := filepath.Join(staticDir, clientIndexFile)
	if info, err := os.Stat(indexPath); err != nil {
		return nil, fmt.Errorf("%s %q missing %s: %w", EnvClientStaticDir, staticDir, clientIndexFile, err)
	} else if info.IsDir() {
		return nil, fmt.Errorf("%s %q has directory %s", EnvClientStaticDir, staticDir, clientIndexFile)
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		if isReservedClientStaticPath(r.URL.Path) {
			http.NotFound(w, r)
			return
		}
		requestPath := path.Clean("/" + r.URL.Path)
		relativePath := strings.TrimPrefix(requestPath, "/")
		if relativePath == "" {
			serveClientFile(w, r, staticDir, clientIndexFile)
			return
		}
		if clientStaticFileExists(staticDir, relativePath) {
			serveClientFile(w, r, staticDir, relativePath)
			return
		}
		if path.Ext(requestPath) != "" {
			http.NotFound(w, r)
			return
		}
		serveClientFile(w, r, staticDir, clientIndexFile)
	}), nil
}

func isReservedClientStaticPath(requestPath string) bool {
	return requestPath == "/ws" ||
		requestPath == "/healthz" ||
		requestPath == "/api" ||
		strings.HasPrefix(requestPath, "/api/") ||
		strings.HasPrefix(requestPath, "/ws/")
}

func clientStaticFileExists(staticDir, relativePath string) bool {
	info, err := os.Stat(filepath.Join(staticDir, filepath.FromSlash(relativePath)))
	return err == nil && !info.IsDir()
}

func serveClientFile(w http.ResponseWriter, r *http.Request, staticDir, relativePath string) {
	http.ServeFile(w, r, filepath.Join(staticDir, filepath.FromSlash(relativePath)))
}
