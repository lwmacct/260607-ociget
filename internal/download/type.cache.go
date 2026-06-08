package download

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
	"golang.org/x/sync/singleflight"
)

type cache struct {
	dir string
	ttl time.Duration
	sf  singleflight.Group
}

type cachedFile struct {
	path    string
	size    int64
	modTime time.Time
}

type cacheResult struct {
	cached  *cachedFile
	written bool
}

type cacheWriter struct {
	file      *os.File
	finalPath string
	tmpPath   string
	err       error
	committed bool
}

func newCache(cfg CacheConfig) (*cache, error) {
	if !cfg.Enabled {
		return nil, nil
	}
	if cfg.Dir == "" {
		return nil, fmt.Errorf("download cache dir is required when enabled")
	}
	if cfg.TTL < 0 {
		return nil, fmt.Errorf("download cache ttl must not be negative")
	}
	if err := os.MkdirAll(cfg.Dir, 0o700); err != nil {
		return nil, fmt.Errorf("prepare download cache directory: %w", err)
	}
	return &cache{dir: cfg.Dir, ttl: cfg.TTL}, nil
}

func (c *cache) key(ctx context.Context, req Request) (string, error) {
	if c == nil {
		return "", os.ErrNotExist
	}
	target, err := ociimage.NormalizePath(req.FilePath)
	if err != nil {
		return "", err
	}

	extractor := &ociimage.Extractor{}
	digest, err := extractor.ImageDigest(ctx, req.ImageRef, ociOptions(req.Options))
	if err != nil {
		return "", err
	}

	platform := "default"
	if req.Options.Platform != nil {
		platform = platformString(*req.Options.Platform)
	}
	sum := sha256.Sum256([]byte(digest.String() + "\n" + platform + "\n" + target))
	return hex.EncodeToString(sum[:]), nil
}

func (c *cache) do(key string, fn func() (*cacheResult, error)) (*cacheResult, bool, error) {
	executed := false
	v, err, shared := c.sf.Do(key, func() (any, error) {
		if cached, err := c.get(key); err == nil {
			return &cacheResult{cached: cached}, nil
		}
		executed = true
		return fn()
	})
	if err != nil {
		return nil, shared, err
	}
	return v.(*cacheResult), executed, nil
}

func (c *cache) get(key string) (*cachedFile, error) {
	if c == nil {
		return nil, os.ErrNotExist
	}
	path := c.pathForKey(key)
	if !c.isFresh(path) {
		return nil, os.ErrNotExist
	}
	return c.stat(path)
}

func (c *cache) writer(key string) (*cacheWriter, error) {
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

func (c *cache) open(key string) (*os.File, *cachedFile, error) {
	cached, err := c.get(key)
	if err != nil {
		return nil, nil, err
	}
	file, err := os.Open(cached.path)
	if err != nil {
		return nil, nil, err
	}
	return file, cached, nil
}

func (c *cache) pathForKey(key string) string {
	return filepath.Join(c.dir, key[:2], key[2:])
}

func (c *cache) isFresh(path string) bool {
	st, err := os.Stat(path)
	if err != nil || !st.Mode().IsRegular() {
		return false
	}
	if c.ttl == 0 {
		return true
	}
	return time.Since(st.ModTime()) <= c.ttl
}

func (c *cache) stat(path string) (*cachedFile, error) {
	st, err := os.Stat(path)
	if err != nil {
		return nil, err
	}
	if !st.Mode().IsRegular() {
		return nil, os.ErrNotExist
	}
	return &cachedFile{
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
