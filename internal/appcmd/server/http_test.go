package server

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

	"github.com/lwmacct/260607-ociget/internal/config"
)

func TestHTTPHandlerServesFrontendFiles(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "index.html"), []byte("frontend-index"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "assets", "app.js"), []byte("frontend-asset"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRONTEND_DIR", root)

	cfg := config.DefaultConfig().Server
	handler := newHTTPHandler(&cfg, nil)

	tests := []struct {
		name       string
		method     string
		path       string
		wantStatus int
		wantBody   string
	}{
		{name: "index", method: http.MethodGet, path: "/", wantStatus: http.StatusOK, wantBody: "frontend-index"},
		{name: "index head", method: http.MethodHead, path: "/", wantStatus: http.StatusOK},
		{name: "asset", method: http.MethodGet, path: "/assets/app.js", wantStatus: http.StatusOK, wantBody: "frontend-asset"},
		{name: "asset head", method: http.MethodHead, path: "/assets/app.js", wantStatus: http.StatusOK},
		{name: "missing", method: http.MethodGet, path: "/missing.js", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
		{name: "post asset", method: http.MethodPost, path: "/assets/app.js", wantStatus: http.StatusNotFound, wantBody: "404 page not found\n"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			handler.ServeHTTP(recorder, httptest.NewRequest(test.method, test.path, nil))
			if recorder.Code != test.wantStatus {
				t.Fatalf("status = %d, want %d", recorder.Code, test.wantStatus)
			}
			if recorder.Body.String() != test.wantBody {
				t.Fatalf("body = %q, want %q", recorder.Body.String(), test.wantBody)
			}
		})
	}
}

func TestHTTPHandlerReturnsNotFoundWithoutFrontend(t *testing.T) {
	t.Setenv("FRONTEND_DIR", "")
	cfg := config.DefaultConfig().Server
	recorder := httptest.NewRecorder()

	newHTTPHandler(&cfg, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusNotFound)
	}
}

func TestHTTPHandlerKeepsAPIRoutesAheadOfFrontend(t *testing.T) {
	root := t.TempDir()
	if err := os.Mkdir(filepath.Join(root, "api"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "api", "health"), []byte("frontend-health"), 0o600); err != nil {
		t.Fatal(err)
	}
	t.Setenv("FRONTEND_DIR", root)
	cfg := config.DefaultConfig().Server
	recorder := httptest.NewRecorder()

	newHTTPHandler(&cfg, nil).ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/health", nil))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() == "frontend-health" {
		t.Fatal("API request was served by the frontend file handler")
	}
}
