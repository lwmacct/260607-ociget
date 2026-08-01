package server

import (
	"context"
	"net/http"
	"os"
	"path"
	"strings"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/uptrace/bun"

	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
)

func newHTTPServer(cfg *config.Server, rt *runtime) *http.Server {
	srv := &http.Server{
		Addr:              cfg.HTTP.Listen,
		Handler:           newHTTPHandler(cfg, rt),
		ReadHeaderTimeout: 5 * time.Second,
	}
	if rt != nil && rt.tls != nil && rt.tls.config != nil {
		srv.TLSConfig = rt.tls.config
	}
	return srv
}

func newHTTPHandler(cfg *config.Server, rt *runtime) http.Handler {
	if rt == nil {
		rt = &runtime{}
	}
	router := chi.NewRouter()
	apiRouter := chi.NewRouter()
	apiRouter.Get("/images/{imageID}/file", rt.handleImageFile)
	apiRouter.Post("/images/{imageID}/archive", rt.handleImageArchive)
	router.Mount("/api", apiRouter)
	api := humachi.New(apiRouter, huma.DefaultConfig("Web App Skeleton", version.AppVersion))
	registerRoutes(api, rt.db, cfg, rt.images)
	frontend := newFrontendHTTPHandler()
	router.NotFound(frontend.ServeHTTP)
	return router
}

func newFrontendHTTPHandler() http.Handler {
	frontendDir := strings.TrimSpace(os.Getenv("FRONTEND_DIR"))
	if frontendDir == "" {
		return http.NotFoundHandler()
	}
	return frontendFileServer(frontendDir)
}

func frontendFileServer(root string) http.Handler {
	fs := http.Dir(root)
	fileServer := http.FileServer(fs)
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodHead {
			http.NotFound(w, r)
			return
		}
		if r.URL.Path == "/" {
			if !fileExists(fs, "/index.html") {
				http.NotFound(w, r)
				return
			}
			fileServer.ServeHTTP(w, r)
			return
		}
		if fileExists(fs, r.URL.Path) {
			fileServer.ServeHTTP(w, r)
			return
		}
		http.NotFound(w, r)
	})
}

func fileExists(fs http.FileSystem, urlPath string) bool {
	name := strings.TrimPrefix(path.Clean(urlPath), "/")
	if name == "." {
		name = ""
	}
	file, err := fs.Open(name)
	if err != nil {
		return false
	}
	defer func() { _ = file.Close() }()
	info, err := file.Stat()
	return err == nil && !info.IsDir()
}

type healthOutput struct {
	Body struct {
		Status        string `json:"status" example:"ok"`
		Version       string `json:"version" example:"0.1.0"`
		Time          string `json:"time" example:"2026-06-05T12:00:00Z"`
		Database      string `json:"database" example:".local/data/app.db"`
		DatabaseState string `json:"databaseState" example:"up"`
		Error         string `json:"error,omitempty" example:"sqlite ping failed"`
	}
}

type metaOutput struct {
	Body struct {
		Name     string `json:"name" example:"Web App Skeleton"`
		Version  string `json:"version" example:"0.1.0"`
		Listen   string `json:"listen" example:":8080"`
		Database string `json:"database" example:".local/data/app.db"`
		DocsPath string `json:"docsPath" example:"/api"`
	}
}

func registerRoutes(api huma.API, db *bun.DB, cfg *config.Server, images *imagestore.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "get-health",
		Method:      http.MethodGet,
		Path:        "/health",
		Summary:     "Service health",
		Tags:        []string{"system"},
	}, func(ctx context.Context, _ *struct{}) (*healthOutput, error) {
		out := &healthOutput{}
		out.Body.Status = "ok"
		out.Body.Version = version.AppVersion
		out.Body.Time = time.Now().UTC().Format(time.RFC3339)
		out.Body.Database = cfg.Database
		out.Body.DatabaseState = "up"

		if db == nil || db.DB == nil {
			out.Body.Status = "degraded"
			out.Body.DatabaseState = "down"
			out.Body.Error = "database unavailable"
		} else if err := db.DB.PingContext(ctx); err != nil {
			out.Body.Status = "degraded"
			out.Body.DatabaseState = "down"
			out.Body.Error = err.Error()
		}

		return out, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "get-meta",
		Method:      http.MethodGet,
		Path:        "/meta",
		Summary:     "Service metadata",
		Tags:        []string{"system"},
	}, func(context.Context, *struct{}) (*metaOutput, error) {
		out := &metaOutput{}
		out.Body.Name = "Web App Skeleton"
		out.Body.Version = version.AppVersion
		out.Body.Listen = cfg.HTTP.Listen
		out.Body.Database = cfg.Database
		out.Body.DocsPath = "/api"
		return out, nil
	})

	registerImageRoutes(api, images)
}
