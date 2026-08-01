package server

import (
	"context"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/lwmacct/260607-ociget/internal/config"
)

// App owns the server process lifecycle.
type App struct {
	cfg *config.Server
}

// NewApp creates a server application for the supplied configuration.
func NewApp(cfg *config.Server) *App {
	return &App{cfg: cfg}
}

// Run validates configuration, starts the runtime, and serves until shutdown.
func (app *App) Run(ctx context.Context) error {
	if err := validateConfig(app.cfg); err != nil {
		return err
	}

	rt, err := newRuntime(ctx, app.cfg)
	if err != nil {
		return err
	}
	defer func() { _ = rt.Close(context.Background()) }()

	srv := newHTTPServer(app.cfg, rt)
	ln, err := (&net.ListenConfig{}).Listen(ctx, "tcp", srv.Addr)
	if err != nil {
		return err
	}
	defer func() { _ = ln.Close() }()

	errCh := make(chan error, 1)
	go func() {
		slog.Info("web backend starting", "listen", srv.Addr, "https", app.cfg.HTTP.UsesTLS())
		var serveErr error
		if app.cfg.HTTP.UsesTLS() {
			serveErr = srv.ServeTLS(ln, "", "")
		} else {
			serveErr = srv.Serve(ln)
		}
		if serveErr != nil && !errors.Is(serveErr, http.ErrServerClosed) {
			errCh <- serveErr
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGTERM, os.Interrupt)
	defer signal.Stop(sigCh)

	select {
	case <-ctx.Done():
		return shutdown(ctx, srv, rt)
	case sig := <-sigCh:
		slog.Info("received shutdown signal", "signal", sig.String())
		return shutdown(ctx, srv, rt)
	case err := <-errCh:
		return err
	}
}

func validateConfig(cfg *config.Server) error {
	if cfg == nil {
		return errors.New("server config is nil")
	}
	if err := cfg.HTTP.Validate(); err != nil {
		return err
	}
	if err := cfg.ImageStore.Validate(); err != nil {
		return err
	}
	return nil
}

func shutdown(ctx context.Context, srv *http.Server, rt *runtime) error {
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 10*time.Second)
	defer cancel()

	var errs []error
	if srv != nil {
		if err := srv.Shutdown(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	if rt != nil {
		if err := rt.Close(shutdownCtx); err != nil {
			errs = append(errs, err)
		}
	}
	slog.Info("web backend stopped")
	return errors.Join(errs...)
}
