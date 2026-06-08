package download

import (
	"bytes"
	"context"
	"io"
	"testing"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func TestServiceWriteCachesMissForNextRequest(t *testing.T) {
	source := &fakeImageSource{
		content: []byte("payload"),
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

	source.content = []byte("changed")
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

type fakeImageSource struct {
	content   []byte
	digest    v1.Hash
	modTime   time.Time
	openCalls int
}

func (s *fakeImageSource) OpenFile(_ context.Context, _, filePath string, _ ociimage.OpenOptions) (*ociimage.File, error) {
	s.openCalls++
	return &ociimage.File{
		Path:    filePath,
		Size:    int64(len(s.content)),
		ModTime: s.modTime,
		Reader:  io.NopCloser(bytes.NewReader(s.content)),
	}, nil
}

func (s *fakeImageSource) ImageDigest(_ context.Context, _ string, _ ociimage.OpenOptions) (v1.Hash, error) {
	return s.digest, nil
}
