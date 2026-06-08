package download

import (
	"context"
	"errors"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

var ErrWriterStarted = errors.New("download writer already received data")

type Options struct {
	Platform *v1.Platform
	Insecure bool
}

type Request struct {
	ImageRef string
	FilePath string
	Options  Options
}

type ArchiveRequest struct {
	ImageRef string
	Paths    []string
	Options  Options
}

type Metadata struct {
	Path     string
	Size     int64
	ModTime  time.Time
	CacheHit bool
}

type CacheConfig struct {
	Enabled bool
	Dir     string
	TTL     time.Duration
}

type Service struct {
	cache  *cache
	images imageSource
}

type imageSource interface {
	OpenFile(ctx context.Context, imageRef, filePath string, opts ociimage.OpenOptions) (*ociimage.File, error)
	ImageDigest(ctx context.Context, imageRef string, opts ociimage.OpenOptions) (v1.Hash, error)
}
