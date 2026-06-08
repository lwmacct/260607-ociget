package download

import (
	"archive/tar"
	"bytes"
	"context"
	"errors"
	"io"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func TestServiceWriteCachesMissForNextRequest(t *testing.T) {
	source := &fakeImageSource{
		content: map[string][]byte{
			"/usr/local/bin/app": []byte("payload"),
		},
		digest:  v1.Hash{Algorithm: "sha256", Hex: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"},
		modTime: time.Now().Add(-time.Hour).Truncate(time.Second),
	}
	service := &Service{
		cache:  testCache(t, 0),
		images: source,
	}
	req := Request{
		ImageRef: "example.com/app:latest",
		FilePath: "/usr/local/bin/app",
	}

	first := bytes.Buffer{}
	firstMeta := Metadata{}
	if err := service.Write(context.Background(), req, &first, func(meta Metadata) {
		firstMeta = meta
	}); err != nil {
		t.Fatalf("Write() first unexpected error: %v", err)
	}
	if first.String() != "payload" {
		t.Fatalf("first body = %q, want payload", first.String())
	}
	if firstMeta.CacheHit {
		t.Fatalf("first CacheHit = true, want false")
	}
	if source.openCalls != 1 {
		t.Fatalf("open calls after first write = %d, want 1", source.openCalls)
	}

	source.content["/usr/local/bin/app"] = []byte("changed")
	second := bytes.Buffer{}
	secondMeta := Metadata{}
	if err := service.Write(context.Background(), req, &second, func(meta Metadata) {
		secondMeta = meta
	}); err != nil {
		t.Fatalf("Write() second unexpected error: %v", err)
	}
	if second.String() != "payload" {
		t.Fatalf("second body = %q, want cached payload", second.String())
	}
	if !secondMeta.CacheHit {
		t.Fatalf("second CacheHit = false, want true")
	}
	if source.openCalls != 1 {
		t.Fatalf("open calls after second write = %d, want still 1", source.openCalls)
	}
}

func TestServiceWriteArchiveStreamsTarEntries(t *testing.T) {
	source := &fakeImageSource{
		content: map[string][]byte{
			"/etc/a": []byte("a"),
			"/etc/b": []byte("bb"),
		},
		digest:  v1.Hash{Algorithm: "sha256", Hex: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		modTime: time.Now().Add(-time.Hour).Truncate(time.Second),
	}
	service := &Service{images: source}
	var buf bytes.Buffer

	err := service.WriteArchive(context.Background(), ArchiveRequest{
		ImageRef: "example.com/app:latest",
		Paths:    []string{"/etc/a", "/etc/b"},
	}, &buf)
	if err != nil {
		t.Fatalf("WriteArchive() unexpected error: %v", err)
	}

	got := readTarEntries(t, buf.Bytes())
	want := map[string]string{
		"etc/a": "a",
		"etc/b": "bb",
	}
	for name, body := range want {
		if got[name] != body {
			t.Fatalf("tar entry %s = %q, want %q", name, got[name], body)
		}
	}
}

func TestServiceWriteArchiveMarksErrorsAfterFirstEntryAsWriterStarted(t *testing.T) {
	source := &fakeImageSource{
		content: map[string][]byte{
			"/etc/a": []byte("a"),
		},
		errByPath: map[string]error{
			"/etc/missing": ociimage.ErrNotFound,
		},
		digest:  v1.Hash{Algorithm: "sha256", Hex: "abcdef0123456789abcdef0123456789abcdef0123456789abcdef0123456789"},
		modTime: time.Now().Add(-time.Hour).Truncate(time.Second),
	}
	service := &Service{images: source}

	err := service.WriteArchive(context.Background(), ArchiveRequest{
		ImageRef: "example.com/app:latest",
		Paths:    []string{"/etc/a", "/etc/missing"},
	}, &bytes.Buffer{})
	if !errors.Is(err, ErrWriterStarted) {
		t.Fatalf("WriteArchive() error = %v, want ErrWriterStarted", err)
	}
}

type fakeImageSource struct {
	content   map[string][]byte
	errByPath map[string]error
	digest    v1.Hash
	modTime   time.Time
	openCalls int
}

func (s *fakeImageSource) OpenFile(_ context.Context, _, filePath string, _ ociimage.OpenOptions) (*ociimage.File, error) {
	s.openCalls++
	if err := s.errByPath[filePath]; err != nil {
		return nil, err
	}
	content := s.content[filePath]
	return &ociimage.File{
		Path:    filePath,
		Size:    int64(len(content)),
		ModTime: s.modTime,
		Reader:  io.NopCloser(bytes.NewReader(content)),
	}, nil
}

func (s *fakeImageSource) ImageDigest(_ context.Context, _ string, _ ociimage.OpenOptions) (v1.Hash, error) {
	return s.digest, nil
}

func readTarEntries(t *testing.T, data []byte) map[string]string {
	t.Helper()

	entries := map[string]string{}
	tr := tar.NewReader(bytes.NewReader(data))
	for {
		header, err := tr.Next()
		if err == io.EOF {
			return entries
		}
		if err != nil {
			t.Fatalf("tar Next() failed: %v", err)
		}
		body, err := io.ReadAll(tr)
		if err != nil {
			t.Fatalf("ReadAll(%s) failed: %v", header.Name, err)
		}
		entries[header.Name] = string(body)
	}
}
