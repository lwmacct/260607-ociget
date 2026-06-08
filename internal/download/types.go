package download

import (
	"errors"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
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
	cache *cache
}
