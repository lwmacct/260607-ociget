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

func TestStoreIndexesMergedImageFilesystem(t *testing.T) {
	source := newFakeImageSource(t,
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
	object, err := store.Open(context.Background(), OpenRequest{Reference: "example.test/tool:v1"})
	if err != nil {
		t.Fatal(err)
	}
	if object.Kind != ObjectKindImage {
		t.Fatalf("kind = %q, want image", object.Kind)
	}
	directory, err := store.List(object.ObjectID, "/etc")
	if err != nil || len(directory.Entries) != 1 || directory.Entries[0].Path != "/etc/config" {
		t.Fatalf("directory = %#v, error = %v", directory, err)
	}
	entry, err := store.OpenFile(object.ObjectID, "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	if entry.Entry.LayerDigest == "" || entry.Entry.TarPath != "etc/config" {
		t.Fatalf("entry = %#v", entry.Entry)
	}
	reader, err := store.OpenFileReader(context.Background(), object.ObjectID, "/etc/config")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(reader.Reader)
	_ = reader.Reader.Close()
	if string(body) != "upper" {
		t.Fatalf("body = %q, want upper", body)
	}
}

func TestStoreIndexesArtifactWithoutFetchingPayload(t *testing.T) {
	source := newFakeArtifactSource([]byte("raw-elf"))
	store := testStore(t, source)
	object, err := store.Open(context.Background(), OpenRequest{Reference: "example.test/tool:artifact"})
	if err != nil {
		t.Fatal(err)
	}
	if object.Kind != ObjectKindArtifact || object.ArtifactType != "application/vnd.example.binary" {
		t.Fatalf("object = %#v", object)
	}
	if len(object.Blobs) != 1 || object.Blobs[0].Name != "rtty" {
		t.Fatalf("blobs = %#v", object.Blobs)
	}
	if source.blobOpenCount() != 0 {
		t.Fatalf("blob opened while indexing: %d", source.blobOpenCount())
	}
	if _, err := store.List(object.ObjectID, "/"); err != ErrNotImage {
		t.Fatalf("List() error = %v, want ErrNotImage", err)
	}
	byName, err := store.ArtifactBlobBySelector(object.ObjectID, "rtty")
	if err != nil || byName.Digest != object.Blobs[0].Digest {
		t.Fatalf("ArtifactBlobBySelector(name) = %#v, %v", byName, err)
	}
	byDigest, err := store.ArtifactBlobBySelector(object.ObjectID, "@"+object.Blobs[0].Digest)
	if err != nil || byDigest.Digest != object.Blobs[0].Digest {
		t.Fatalf("ArtifactBlobBySelector(digest) = %#v, %v", byDigest, err)
	}
	if _, err := store.ArtifactBlobBySelector(object.ObjectID, "@sha256:bad"); err != ErrInvalidBlobSelector {
		t.Fatalf("invalid digest selector error = %v", err)
	}
	if _, err := store.ArtifactBlobBySelector(object.ObjectID, "../rtty"); err != ErrInvalidBlobSelector {
		t.Fatalf("unsafe name selector error = %v", err)
	}

	blob, err := store.OpenArtifactBlob(context.Background(), object.ObjectID, object.Blobs[0].Digest)
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(blob.Reader)
	_ = blob.Reader.Close()
	if string(body) != "raw-elf" {
		t.Fatalf("blob = %q, want raw-elf", body)
	}
	if _, err := store.OpenArtifactBlob(context.Background(), object.ObjectID, "sha256:ffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff"); err != ErrBlobNotFound {
		t.Fatalf("unlisted blob error = %v, want ErrBlobNotFound", err)
	}
}

func TestStoreRejectsAmbiguousArtifactTitle(t *testing.T) {
	source := newFakeArtifactSource([]byte("first"))
	secondDigest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	source.resolved.Blobs = append(source.resolved.Blobs, BlobDescriptor{
		Digest: secondDigest, MediaType: "application/octet-stream", Size: 6, Name: "rtty",
		Annotations: map[string]string{artifactTitleAnnotation: "./rtty"},
	})
	source.blobs[secondDigest] = []byte("second")
	store := testStore(t, source)
	object, err := store.Open(context.Background(), OpenRequest{Reference: "example.test/tool:artifact"})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := store.ArtifactBlobBySelector(object.ObjectID, "rtty"); err != ErrAmbiguousBlob {
		t.Fatalf("ambiguous selector error = %v", err)
	}
}

func TestStorePersistsReferenceMapping(t *testing.T) {
	root := t.TempDir()
	source := newFakeImageSource(t, tarEntry{name: "file", body: "payload"})
	store, err := NewWithSource(Config{Dir: root, RefTTL: time.Hour}, source)
	if err != nil {
		t.Fatal(err)
	}
	req := OpenRequest{Reference: "example.test/tool:v1"}
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
	if _, err := os.Stat(filepath.Join(root, refsFilename)); err != nil {
		t.Fatalf("reference mapping missing: %v", err)
	}
}

func TestStoreCollapsesConcurrentOpen(t *testing.T) {
	source := newFakeImageSource(t, tarEntry{name: "file", body: "payload"})
	store := testStore(t, source)
	var wait sync.WaitGroup
	wait.Add(2)
	for range 2 {
		go func() {
			defer wait.Done()
			if _, err := store.Open(context.Background(), OpenRequest{Reference: "example.test/tool:v1"}); err != nil {
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
	mu        sync.Mutex
	calls     int
	blobOpens int
	resolved  *ResolvedObject
	layers    map[string][]byte
	blobs     map[string][]byte
	err       error
}

func newFakeImageSource(t *testing.T, entries ...tarEntry) *fakeSource {
	t.Helper()
	layer := tarLayer(t, entries...)
	return &fakeSource{resolved: &ResolvedObject{
		Kind: ObjectKindImage, ManifestDigest: testManifestDigest,
		ManifestMediaType: "application/vnd.oci.image.manifest.v1+json", Platform: "linux/amd64",
		Layers: []ResolvedLayer{{
			Descriptor: LayerDescriptor{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
			Layer:      layer,
		}},
	}, layers: map[string][]byte{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": layer.(memoryLayer)}}
}

func newFakeArtifactSource(payload []byte) *fakeSource {
	digest := "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"
	return &fakeSource{resolved: &ResolvedObject{
		Kind: ObjectKindArtifact, ManifestDigest: testManifestDigest,
		ManifestMediaType: "application/vnd.oci.image.manifest.v1+json",
		ArtifactType:      "application/vnd.example.binary",
		Annotations:       map[string]string{"org.opencontainers.image.created": "2026-08-11T00:00:00Z"},
		Blobs: []BlobDescriptor{{
			Digest: digest, MediaType: "application/octet-stream", Size: int64(len(payload)), Name: "rtty",
			Annotations: map[string]string{artifactTitleAnnotation: "rtty"},
		}},
	}, blobs: map[string][]byte{digest: payload}}
}

func (s *fakeSource) Resolve(context.Context, string, ociimage.OpenOptions) (*ResolvedObject, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	return s.resolved, s.err
}

func (s *fakeSource) OpenLayer(_ context.Context, _ string, _ ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error) {
	data, ok := s.layers[descriptor.Digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeSource) OpenBlob(_ context.Context, _ string, _ ociimage.OpenOptions, descriptor BlobDescriptor) (io.ReadCloser, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.blobOpens++
	data, ok := s.blobs[descriptor.Digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (s *fakeSource) callCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

func (s *fakeSource) blobOpenCount() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.blobOpens
}

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
