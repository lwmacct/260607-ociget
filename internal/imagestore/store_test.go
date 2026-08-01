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

const testImageID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestStoreMaterializesMergedFilesystem(t *testing.T) {
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID:  testImageID,
		Platform: "linux/amd64",
		Layers: []Layer{
			tarLayer(t,
				tarEntry{name: "etc/config", body: "base"},
				tarEntry{name: "etc/old", body: "old"},
				tarEntry{name: "var/data/item", body: "shared"},
			),
			tarLayer(t,
				tarEntry{name: "etc/.wh.old"},
				tarEntry{name: "etc/config", body: "upper"},
				tarEntry{name: "usr/bin/app", body: "shared", mode: 0o755},
			),
		},
	}}
	store := testStore(t, source)
	image, err := store.Open(context.Background(), OpenRequest{
		ImageRef: "example.test/tool:v1",
	})
	if err != nil {
		t.Fatalf("Open() unexpected error: %v", err)
	}
	if image.ImageID != testImageID || image.Platform != "linux/amd64" {
		t.Fatalf("Open() = %#v", image)
	}

	directory, err := store.List(image.ImageID, "/etc")
	if err != nil {
		t.Fatalf("List() unexpected error: %v", err)
	}
	if len(directory.Entries) != 1 || directory.Entries[0].Path != "/etc/config" {
		t.Fatalf("entries = %#v", directory.Entries)
	}
	file, err := store.OpenFile(image.ImageID, "/etc/config")
	if err != nil {
		t.Fatalf("OpenFile() unexpected error: %v", err)
	}
	defer file.Reader.Close()
	body, err := io.ReadAll(file.Reader)
	if err != nil || string(body) != "upper" {
		t.Fatalf("OpenFile() body = %q, error = %v", body, err)
	}

	objects, err := filepath.Glob(filepath.Join(store.dir, "objects", "sha256", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 4 {
		t.Fatalf("object count = %d, want 4 (including overwritten content)", len(objects))
	}
}

func TestWhiteoutDoesNotDeleteSameLayerReplacement(t *testing.T) {
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID: testImageID,
		Layers: []Layer{
			tarLayer(t, tarEntry{name: "etc/config", body: "base"}),
			tarLayer(t,
				tarEntry{name: "etc/config", body: "replacement"},
				tarEntry{name: "etc/.wh.config"},
			),
		},
	}}
	store := testStore(t, source)
	if _, err := store.Open(context.Background(), OpenRequest{ImageRef: "example.test/tool:v1"}); err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenFile(testImageID, "/etc/config")
	if err != nil {
		t.Fatalf("OpenFile() unexpected error: %v", err)
	}
	defer file.Reader.Close()
	body, _ := io.ReadAll(file.Reader)
	if string(body) != "replacement" {
		t.Fatalf("body = %q, want replacement", body)
	}
}

func TestStorePersistsReferenceMapping(t *testing.T) {
	root := t.TempDir()
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID: testImageID,
		Layers:  []Layer{tarLayer(t, tarEntry{name: "file", body: "payload"})},
	}}
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
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID: testImageID,
		Layers:  []Layer{tarLayer(t, tarEntry{name: "file", body: "payload"})},
	}}
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

func TestStoreResolvesHardLinks(t *testing.T) {
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID: testImageID,
		Layers: []Layer{tarLayer(t,
			tarEntry{name: "bin/tool", body: "payload"},
			tarEntry{name: "bin/tool-copy", linkName: "bin/tool"},
		)},
	}}
	store := testStore(t, source)
	if _, err := store.Open(context.Background(), OpenRequest{ImageRef: "example.test/tool:v1"}); err != nil {
		t.Fatal(err)
	}
	file, err := store.OpenFile(testImageID, "/bin/tool-copy")
	if err != nil {
		t.Fatalf("OpenFile() unexpected error: %v", err)
	}
	defer file.Reader.Close()
	body, _ := io.ReadAll(file.Reader)
	if string(body) != "payload" {
		t.Fatalf("hard link body = %q", body)
	}
}

func TestStoreRemovesUnreferencedObjectsWhenLimited(t *testing.T) {
	source := &fakeSource{resolved: &ResolvedImage{
		ImageID: testImageID,
		Layers: []Layer{
			tarLayer(t, tarEntry{name: "file", body: "old"}, tarEntry{name: "removed", body: "unused"}),
			tarLayer(t, tarEntry{name: "file", body: "current"}, tarEntry{name: ".wh.removed"}),
		},
	}}
	store, err := NewWithSource(Config{Dir: t.TempDir(), RefTTL: time.Hour, MaxBytes: 1}, source)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.Open(context.Background(), OpenRequest{ImageRef: "example.test/tool:v1"}); err != nil {
		t.Fatal(err)
	}
	objects, err := filepath.Glob(filepath.Join(store.dir, "objects", "sha256", "*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(objects) != 1 {
		t.Fatalf("object count = %d, want only the live object", len(objects))
	}
}

type fakeSource struct {
	mu       sync.Mutex
	calls    int
	resolved *ResolvedImage
	err      error
}

func (s *fakeSource) Resolve(context.Context, string, ociimage.OpenOptions) (*ResolvedImage, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.resolved, s.err
}

func (s *fakeSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

type memoryLayer []byte

func (l memoryLayer) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(l)), nil
}

type tarEntry struct {
	name     string
	body     string
	mode     int64
	linkName string
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
		header := &tar.Header{
			Name: entry.name, Mode: mode, Size: int64(len(entry.body)), Typeflag: typeflag, Linkname: entry.linkName,
		}
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
