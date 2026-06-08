package server

import (
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lwmacct/260607-ociget/internal/config"
)

func TestNewDownloadCacheDisabled(t *testing.T) {
	cache, err := newDownloadCache(config.ServerDownloadCache{Enabled: false})
	if err != nil {
		t.Fatalf("newDownloadCache() unexpected error: %v", err)
	}
	if cache != nil {
		t.Fatalf("newDownloadCache() = %v, want nil", cache)
	}
}

func TestDownloadCacheWriterCommitAndGet(t *testing.T) {
	cache := testDownloadCache(t, 0)
	key := testCacheKey()
	modTime := time.Now().Add(-time.Hour).Truncate(time.Second)

	writer, err := cache.writer(key)
	if err != nil {
		t.Fatalf("writer() unexpected error: %v", err)
	}
	if _, err := writer.Write([]byte("payload")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}
	if err := writer.Commit(modTime); err != nil {
		t.Fatalf("Commit() unexpected error: %v", err)
	}
	writer.Abort()

	cached, err := cache.get(key)
	if err != nil {
		t.Fatalf("get() unexpected error: %v", err)
	}
	if cached.size != int64(len("payload")) {
		t.Fatalf("cached size = %d, want %d", cached.size, len("payload"))
	}
	if got := readFileString(t, cached.path); got != "payload" {
		t.Fatalf("cached payload = %q, want payload", got)
	}
}

func TestDownloadCacheWriterAbortRemovesTempFile(t *testing.T) {
	cache := testDownloadCache(t, 0)
	writer, err := cache.writer(testCacheKey())
	if err != nil {
		t.Fatalf("writer() unexpected error: %v", err)
	}
	tmpPath := writer.tmpPath
	if _, err := writer.Write([]byte("partial")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}

	writer.Abort()

	if _, err := os.Stat(tmpPath); !os.IsNotExist(err) {
		t.Fatalf("temp stat error = %v, want not exist", err)
	}
}

func TestDownloadCacheTTL(t *testing.T) {
	cache := testDownloadCache(t, time.Hour)
	key := testCacheKey()
	path := cache.pathForKey(key)
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		t.Fatalf("MkdirAll() failed: %v", err)
	}
	if err := os.WriteFile(path, []byte("stale"), 0o600); err != nil {
		t.Fatalf("WriteFile() failed: %v", err)
	}
	old := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(path, old, old); err != nil {
		t.Fatalf("Chtimes() failed: %v", err)
	}

	if _, err := cache.get(key); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("get() error = %v, want os.ErrNotExist", err)
	}
}

func TestDownloadCacheServe(t *testing.T) {
	cache := testDownloadCache(t, 0)
	key := testCacheKey()
	writer, err := cache.writer(key)
	if err != nil {
		t.Fatalf("writer() unexpected error: %v", err)
	}
	if _, err := writer.Write([]byte("payload")); err != nil {
		t.Fatalf("Write() unexpected error: %v", err)
	}
	if err := writer.Commit(time.Now()); err != nil {
		t.Fatalf("Commit() unexpected error: %v", err)
	}
	cached, err := cache.get(key)
	if err != nil {
		t.Fatalf("get() unexpected error: %v", err)
	}

	rec := httptest.NewRecorder()
	if err := cache.serve(rec, cached); err != nil {
		t.Fatalf("serve() unexpected error: %v", err)
	}
	if rec.Body.String() != "payload" {
		t.Fatalf("body = %q, want payload", rec.Body.String())
	}
	if rec.Header().Get("Content-Length") != "7" {
		t.Fatalf("Content-Length = %q, want 7", rec.Header().Get("Content-Length"))
	}
}

func TestDownloadCacheDoSuppressesDuplicateExtraction(t *testing.T) {
	cache := testDownloadCache(t, 0)
	key := testCacheKey()

	started := make(chan struct{})
	release := make(chan struct{})
	var calls int
	var mu sync.Mutex

	fn := func() (*cacheExtractResult, error) {
		mu.Lock()
		calls++
		mu.Unlock()
		close(started)
		<-release
		writer, err := cache.writer(key)
		if err != nil {
			return nil, err
		}
		if _, err := writer.Write([]byte("payload")); err != nil {
			return nil, err
		}
		if err := writer.Commit(time.Now()); err != nil {
			return nil, err
		}
		cached, err := cache.get(key)
		if err != nil {
			return nil, err
		}
		return &cacheExtractResult{cached: cached, streamed: true}, nil
	}

	var wg sync.WaitGroup
	wg.Add(2)
	errs := make(chan error, 2)
	go func() {
		defer wg.Done()
		_, _, err := cache.do(key, fn)
		errs <- err
	}()
	<-started
	go func() {
		defer wg.Done()
		_, _, err := cache.do(key, fn)
		errs <- err
	}()
	close(release)
	wg.Wait()
	close(errs)

	for err := range errs {
		if err != nil {
			t.Fatalf("do() unexpected error: %v", err)
		}
	}
	if calls != 1 {
		t.Fatalf("extract calls = %d, want 1", calls)
	}
}

func testDownloadCache(t *testing.T, ttl time.Duration) *downloadCache {
	t.Helper()

	return &downloadCache{
		dir: t.TempDir(),
		ttl: ttl,
	}
}

func testCacheKey() string {
	return "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
}

func readFileString(t *testing.T, path string) string {
	t.Helper()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile() failed: %v", err)
	}
	return string(data)
}
