package server

import (
	"context"
	"crypto/tls"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
	"github.com/uptrace/bun"
	"github.com/uptrace/bun/dialect/sqlitedialect"
	"github.com/uptrace/bun/driver/sqliteshim"
)

const httpTLSMinVersion = tls.VersionTLS12

type runtime struct {
	db     *bun.DB
	tls    *tlsRuntime
	images *imagestore.Store
}

func newRuntime(ctx context.Context, cfg *config.Server) (_ *runtime, err error) {
	rt := &runtime{}
	defer func() {
		if err != nil {
			_ = rt.Close(context.Background())
		}
	}()

	rt.db, err = openDatabase(ctx, cfg.Database)
	if err != nil {
		return nil, err
	}
	rt.tls, err = newTLSRuntime(ctx, cfg.HTTP.TLS)
	if err != nil {
		return nil, fmt.Errorf("configure tls: %w", err)
	}

	refTTL, err := cfg.ImageStore.RefTTLDuration()
	if err != nil {
		return nil, err
	}
	rt.images, err = imagestore.New(imagestore.Config{
		Dir:      cfg.ImageStore.Dir,
		RefTTL:   refTTL,
		MaxBytes: cfg.ImageStore.MaxBytes,
	})
	if err != nil {
		return nil, err
	}
	return rt, nil
}

func openDatabase(ctx context.Context, path string) (*bun.DB, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return nil, fmt.Errorf("prepare database directory: %w", err)
	}

	sqldb, err := sql.Open(sqliteshim.ShimName, path)
	if err != nil {
		return nil, fmt.Errorf("open sqlite database: %w", err)
	}
	if err := sqldb.PingContext(ctx); err != nil {
		_ = sqldb.Close()
		return nil, fmt.Errorf("ping sqlite database: %w", err)
	}
	return bun.NewDB(sqldb, sqlitedialect.New()), nil
}

func (rt *runtime) Close(_ context.Context) error {
	if rt == nil {
		return nil
	}
	var errs []error
	if rt.tls != nil {
		if err := rt.tls.Close(); err != nil {
			errs = append(errs, err)
		}
		rt.tls = nil
	}
	if rt.db != nil {
		if err := rt.db.Close(); err != nil {
			errs = append(errs, err)
		}
		rt.db = nil
	}
	return errors.Join(errs...)
}

type tlsRuntime struct {
	config *tls.Config
	store  *tlsreload.Store
}

func newTLSRuntime(ctx context.Context, cfg tlsreload.Config) (*tlsRuntime, error) {
	if !cfg.Enabled {
		return &tlsRuntime{}, nil
	}

	store, err := tlsreload.New(ctx, cfg, tlsreload.WithLogger(slog.Default()))
	if err != nil {
		return nil, err
	}
	return &tlsRuntime{
		config: &tls.Config{
			MinVersion:     httpTLSMinVersion,
			GetCertificate: store.GetCertificate,
		},
		store: store,
	}, nil
}

func (rt *tlsRuntime) Close() error {
	if rt == nil || rt.store == nil {
		return nil
	}
	err := rt.store.Close()
	rt.store = nil
	return err
}
