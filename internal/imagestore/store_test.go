package imagestore

import (
	"archive/tar"
	"bytes"
	"context"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const testManifestDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestStoreIndexesMergedFilesystemWithoutFileObjects(t *testing.T) {
	source := newFakeSource(t,
		tarEntry{name: "etc/config", body: "base"}, tarEntry{name: "etc/old", body: "old"},
		tarEntry{name: "var/data/item", body: "shared"},
	)
	source.resolved.Layers = append(source.resolved.Layers, ResolvedLayer{
		Descriptor: LayerDescriptor{Digest: "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"},
		Layer: tarLayer(t,
			tarEntry{name: "etc/config", body: "upper"}, tarEntry{name: "etc/.wh.old"}, tarEntry{name: "usr/bin/app", body: "shared", mode: 0o755},
		),
	})
	source.layers["sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"] = source.resolved.Layers[1].Layer.(memoryLayer)
	store := testStore(t, source)
	image, err := store.Open(context.Background(), OpenRequest{ImageRef: "example.test/tool:v1"})
	if err != nil {
		t.Fatal(err)
	}
	directory, err := store.List(image.ImageID, "/etc")
	if err != nil || len(directory.Entries) != 1 || directory.Entries[0].Path != "/etc/config" {
		t.Fatalf("directory = %#v, error = %v", directory, err)
	}
	entry, err := store.OpenFile(image.ImageID, "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Entry.LayerDigest == "" || entry.Entry.TarPath != "etc/config" {
		t.Fatalf("entry = %#v", entry.Entry)
	}
	objects, err := filepath.Glob(filepath.Join(store.dir, "objects", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 0 {
		t.Fatalf("metadata store created file objects: %v", objects)
	}
	reader, err := store.OpenFileReader(context.Background(), image.ImageID, "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(reader.Reader)
	_ = reader.Reader.Close()
	if string(body) != "upper" {
		t.Fatalf("body = %q, want upper", body)
	}
}

func TestStorePersistsReferenceMapping(t *testing.T) {
	root := t.TempDir()
	source := newFakeSource(t, tarEntry{name: "file", body: "payload"})
	store, err := NewWithSource(Config{Dir: root, RefTTL: time.Hour}, source)
	if err != nil {
		t.Fatal(err)
	}
	req := OpenRequest{ImageRef: "example.test/tool:v1"}
	if _, err := store.Open(context.Background(), req); err != nil {
		t.Fatal(err)
	}
	restarted, err := NewWithSource(Config{Dir: root, RefTTL: time.Hour}, &fakeSource{err: os.ErrPermission})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := restarted.Open(context.Background(), req); err != nil {
		t.Fatalf("cached Open() called remote source: %v", err)
	}
}

func TestStoreCollapsesConcurrentOpen(t *testing.T) {
	source := newFakeSource(t, tarEntry{name: "file", body: "payload"})
	store := testStore(t, source)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			if _, err := store.Open(context.Background(), OpenRequest{ImageRef: "example.test/tool:v1"}); err != nil {
				t.Errorf("Open() unexpected error: %v", err)
			}
		}()
	}
	wait.Wait()
	if source.callCount() != 1 {
		t.Fatalf("Resolve() calls = %d, want 1", source.callCount())
	}
}

type fakeSource struct {
	mu       sync.Mutex
	calls    int
	resolved *ResolvedImage
	layers   map[string][]byte
	err      error
}

func newFakeSource(t *testing.T, entries ...tarEntry) *fakeSource {
	t.Helper()
	layer := tarLayer(t, entries...)
	return &fakeSource{resolved: &ResolvedImage{
		ManifestDigest: testManifestDigest,
		Platform:       "linux/amd64",
		Layers:         []ResolvedLayer{{Descriptor: LayerDescriptor{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Layer: layer}},
	}, layers: map[string][]byte{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": layer.(memoryLayer)}}
}

func (s *fakeSource) Resolve(context.Context, string, ociimage.OpenOptions) (*ResolvedImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.resolved, s.err
}

func (s *fakeSource) OpenLayer(_ context.Context, _ string, _ string, _ ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error) {
	data, ok := s.layers[descriptor.Digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeSource) callCount() int { s.mu.Lock(); defer s.mu.Unlock(); return s.calls }

type memoryLayer []byte

func (l memoryLayer) Open() (io.ReadCloser, error) { return io.NopCloser(bytes.NewReader(l)), nil }

type tarEntry struct {
	name, body string
	mode       int64
	linkName   string
}

func tarLayer(t *testing.T, entries ...tarEntry) Layer {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	for _, entry := range entries {
		mode := entry.mode
		if mode == 0 {
			mode = 0o644
		}
		typeflag := byte(tar.TypeReg)
		if entry.linkName != "" {
			typeflag = tar.TypeLink
		}
		header := &tar.Header{Name: entry.name, Mode: mode, Size: int64(len(entry.body)), Typeflag: typeflag, Linkname: entry.linkName}
		if err := writer.WriteHeader(header); err != nil {
			t.Fatal(err)
		}
		if _, err := writer.Write([]byte(entry.body)); err != nil {
			t.Fatal(err)
		}
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return memoryLayer(buffer.Bytes())
}

func testStore(t *testing.T, source Source) *Store {
	t.Helper()
	store, err := NewWithSource(Config{Dir: t.TempDir(), RefTTL: time.Hour}, source)
	if err != nil {
		t.Fatal(err)
	}
	return store
}

var _ Source = (*fakeSource)(nil)
