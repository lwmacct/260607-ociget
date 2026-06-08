package server

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
	"golang.org/x/sync/singleflight"
)

type downloadCache struct {
	dir string
	ttl time.Duration
	sf  singleflight.Group
}

type cachedDownload struct {
	path    string
	size    int64
	modTime time.Time
}

type cacheExtractResult struct {
	cached   *cachedDownload
	streamed bool
}

type cacheWriter struct {
	file      *os.File
	finalPath string
	tmpPath   string
	err       error
	committed bool
}

func newDownloadCache(cfg config.ServerDownloadCache) (*downloadCache, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	if !cfg.Enabled {
		return nil, nil
	}
	ttl, err := cfg.TTLDuration()
	if err != nil {
		return nil, err
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("prepare download cache directory: %w", err)
	}
	return &downloadCache{dir: cfg.Dir, ttl: ttl}, nil
}

func (c *downloadCache) key(ctx context.Context, imageRef, filePath string, opts ociimage.OpenOptions) (string, error) {
	if c == nil {
		return "", os.ErrNotExist
	}
	target, err := ociimage.NormalizePath(filePath)
	if err != nil {
		return "", err
	}

	extractor := &ociimage.Extractor{}
	digest, err := extractor.ImageDigest(ctx, imageRef, opts)
	if err != nil {
		return "", err
	}

	platform := "default"
	if opts.Platform != nil {
		platform = platformString(*opts.Platform)
	}
	sum := sha256.Sum256([]byte(digest.String() + "\n" + platform + "\n" + target))
	return hex.EncodeToString(sum[:]), nil
}

func (c *downloadCache) do(key string, fn func() (*cacheExtractResult, error)) (*cacheExtractResult, bool, error) {
	executed := false
	v, err, shared := c.sf.Do(key, func() (any, error) {
		if cached, err := c.get(key); err == nil {
			return &cacheExtractResult{cached: cached}, nil
		}
		executed = true
		return fn()
	})
	if err != nil {
		return nil, shared, err
	}
	return v.(*cacheExtractResult), executed, nil
}

func (c *downloadCache) get(key string) (*cachedDownload, error) {
	if c == nil {
		return nil, os.ErrNotExist
	}
	path := c.pathForKey(key)
	if !c.isFresh(path) {
		return nil, os.ErrNotExist
	}
	return c.stat(path)
}

func (c *downloadCache) writer(key string) (*cacheWriter, error) {
	finalPath := c.pathForKey(key)
	if err := os.MkdirAll(filepath.Dir(finalPath), 0o700); err != nil {
		return nil, fmt.Errorf("prepare download cache shard: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(finalPath), "."+filepath.Base(finalPath)+".*.tmp")
	if err != nil {
		return nil, fmt.Errorf("create download cache temp file: %w", err)
	}
	return &cacheWriter{
		file:      tmp,
		finalPath: finalPath,
		tmpPath:   tmp.Name(),
	}, nil
}

func (c *downloadCache) serve(w http.ResponseWriter, cached *cachedDownload) error {
	f, err := os.Open(cached.path)
	if err != nil {
		return err
	}
	defer f.Close()
	if cached.size >= 0 {
		w.Header().Set("Content-Length", fmt.Sprintf("%d", cached.size))
	}
	if !cached.modTime.IsZero() {
		w.Header().Set("Last-Modified", cached.modTime.UTC().Format(http.TimeFormat))
	}
	_, err = io.Copy(w, f)
	return err
}

func (c *downloadCache) pathForKey(key string) string {
	return filepath.Join(c.dir, key[:2], key[2:])
}

func (c *downloadCache) isFresh(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	if c.ttl == 0 {
		return true
	}
	return time.Since(st.ModTime()) <= c.ttl
}

func (c *downloadCache) stat(path string) (*cachedDownload, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return &cachedDownload{
		path:    path,
		size:    st.Size(),
		modTime: st.ModTime(),
	}, nil
}

func (w *cacheWriter) Write(p []byte) (int, error) {
	if w == nil || w.file == nil {
		return len(p), nil
	}
	if w.err != nil {
		return len(p), nil
	}
	n, err := w.file.Write(p)
	if err != nil {
		w.err = err
		return len(p), nil
	}
	if n != len(p) {
		w.err = io.ErrShortWrite
	}
	return len(p), nil
}

func (w *cacheWriter) Commit(modTime time.Time) error {
	if w == nil || w.file == nil {
		return nil
	}
	if w.err != nil {
		return w.err
	}
	if !modTime.IsZero() {
		_ = os.Chtimes(w.tmpPath, modTime, modTime)
	}
	if err := w.file.Close(); err != nil {
		return err
	}
	w.file = nil
	if err := os.Rename(w.tmpPath, w.finalPath); err != nil {
		return err
	}
	w.committed = true
	return nil
}

func (w *cacheWriter) Abort() {
	if w == nil || w.committed {
		return
	}
	if w.file != nil {
		_ = w.file.Close()
		w.file = nil
	}
	if w.tmpPath != "" {
		_ = os.Remove(w.tmpPath)
	}
}

func platformString(p v1.Platform) string {
	parts := []string{p.OS, p.Architecture}
	if p.Variant != "" {
		parts = append(parts, p.Variant)
	}
	if p.OSVersion != "" {
		parts = append(parts, p.OSVersion)
	}
	if len(p.OSFeatures) > 0 {
		parts = append(parts, strings.Join(p.OSFeatures, ","))
	}
	return strings.Join(parts, "/")
}
