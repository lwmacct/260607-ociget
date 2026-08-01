package server

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"log/slog"

	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260614-go-pkg-tlsreload/pkg/tlsreload"
)

const httpTLSMinVersion = tls.VersionTLS12

type runtime struct {
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

	rt.tls, err = newTLSRuntime(ctx, cfg.HTTP.TLS)
	if err != nil {
		return nil, fmt.Errorf("configure tls: %w", err)
	}

	refTTL, err := cfg.ImageStore.RefTTLDuration()
	if err != nil {
		return nil, err
	}
	rt.images, err = imagestore.New(imagestore.Config{
		Dir:    cfg.ImageStore.Dir,
		RefTTL: refTTL,
	})
	if err != nil {
		return nil, err
	}
	return rt, nil
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
