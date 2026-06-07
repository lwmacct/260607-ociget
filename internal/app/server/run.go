package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/danielgtaylor/huma/v2/adapters/humachi"
	"github.com/go-chi/chi/v5"
	"github.com/lwmacct/251207-go-pkg-version/pkg/version"
	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

// Run starts the server application and blocks until shutdown.
func Run(ctx context.Context, cfg *config.Config) error {
	ctx, stop := signal.NotifyContext(ctx, os.Interrupt, syscall.SIGTERM)
	defer stop()

	app := &serverApp{cfg: cfg}
	if err := app.run(ctx); err != nil {
		return err
	}
	return nil
}

type serverApp struct {
	cfg        *config.Config
	db         *bun.DB
	srv        *http.Server
	tlsManager *TLSManager
	control    *controlPlane
}

func (app *serverApp) run(ctx context.Context) error {
	if err := app.prepareDatabase(ctx); err != nil {
		return err
	}
	defer app.close()

	if err := app.buildHTTPServer(); err != nil {
		return err
	}
	if err := app.startControlPlane(ctx); err != nil {
		return err
	}
	defer app.stopControlPlane()

	return app.serve(ctx)
}

func (app *serverApp) prepareDatabase(ctx context.Context) error {
	if err := app.cfg.Server.HTTP.Validate(); err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(app.cfg.Server.Database), 0o755); err != nil {
		return fmt.Errorf("prepare database directory: %w", err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, app.cfg.Server.Database)
	if err != nil {
		return fmt.Errorf("open sqlite database: %w", err)
	}
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return fmt.Errorf("ping sqlite database: %w", err)
	}

	app.db = bun.NewDB(sqldb, sqlitedialect.New())
	return nil
}

func (app *serverApp) close() {
	if app.db != nil {
		_ = app.db.Close()
	}
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

func registerRoutes(api huma.API, db *bun.DB, cfg *config.Config) {
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
		out.Body.Database = cfg.Server.Database
		out.Body.DatabaseState = "up"

		if err := db.DB.PingContext(ctx); err != nil {
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
	}, func(ctx context.Context, _ *struct{}) (*metaOutput, error) {
		out := &metaOutput{}
		out.Body.Name = "Web App Skeleton"
		out.Body.Version = version.AppVersion
		out.Body.Listen = cfg.Server.HTTP.Listen
		out.Body.Database = cfg.Server.Database
		out.Body.DocsPath = "/api"
		return out, nil
	})
}

func serveHTTP(srv *http.Server, cfg *config.Config) error {
	ln, err := net.Listen("tcp", cfg.Server.HTTP.Listen)
	if err != nil {
		return fmt.Errorf("listen tcp: %w", err)
	}

	if !cfg.Server.HTTP.UsesTLS() {
		return srv.Serve(ln)
	}

	return srv.ServeTLS(ln, "", "")
}

func (app *serverApp) buildHTTPServer() error {
	router := chi.NewRouter()
	router.Get("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		_, _ = w.Write([]byte(`{"message":"webapp backend is running","api":"/api"}`))
	})

	apiRouter := chi.NewRouter()
	router.Mount("/api", apiRouter)

	api := humachi.New(apiRouter, huma.DefaultConfig("Web App Skeleton", version.AppVersion))
	registerRoutes(api, app.db, app.cfg)

	app.srv = &http.Server{
		Addr:              app.cfg.Server.HTTP.Listen,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
	}

	if !app.cfg.Server.HTTP.UsesTLS() {
		return nil
	}

	tlsManager, err := NewTLSManager(app.cfg.Server.HTTP.SSLCertFile, app.cfg.Server.HTTP.SSLKeyFile)
	if err != nil {
		return err
	}
	app.tlsManager = tlsManager
	app.srv.TLSConfig = &tls.Config{
		GetCertificate: tlsManager.GetCertificate,
		MinVersion:     tls.VersionTLS12,
	}
	return nil
}

func (app *serverApp) startControlPlane(ctx context.Context) error {
	app.control = newControlPlane(app.cfg.Server.Control.Listen, app.tlsManager)
	return app.control.start(ctx)
}

func (app *serverApp) stopControlPlane() {
	if app.control != nil {
		app.control.stop()
	}
}

func (app *serverApp) serve(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		slog.Info(
			"web backend starting",
			"listen", app.cfg.Server.HTTP.Listen,
			"database", app.cfg.Server.Database,
			"tls", app.cfg.Server.HTTP.UsesTLS(),
			"control", app.cfg.Server.Control.Listen,
		)
		errCh <- serveHTTP(app.srv, app.cfg)
	}()

	select {
	case <-ctx.Done():
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		if err := app.srv.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("shutdown http server: %w", err)
		}
		return nil
	case err := <-errCh:
		if err == nil || errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return fmt.Errorf("run http server: %w", err)
	}
}
