package imagestore

import (
	"context"
	"errors"
	"io"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

var (
	ErrImageNotFound  = errors.New("image not found in metadata store")
	ErrNotDirectory   = errors.New("image path is not a directory")
	ErrNotRegularFile = errors.New("image path is not a regular file")
)

type EntryType string

const (
	EntryTypeDirectory EntryType = "directory"
	EntryTypeFile      EntryType = "file"
	EntryTypeSymlink   EntryType = "symlink"
	EntryTypeOther     EntryType = "other"
)

type Config struct {
	Dir    string
	RefTTL time.Duration
}

type OpenRequest struct {
	ImageRef string
	Platform *v1.Platform
	Insecure bool
	Refresh  bool
}

type Image struct {
	ImageID        string    `json:"imageId"`
	ManifestDigest string    `json:"manifestDigest"`
	ImageRef       string    `json:"imageRef"`
	Platform       string    `json:"platform"`
	Insecure       bool      `json:"insecure,omitempty"`
	CreatedAt      time.Time `json:"createdAt"`
}

type Entry struct {
	Name        string    `json:"name"`
	Path        string    `json:"path"`
	Type        EntryType `json:"type"`
	Size        int64     `json:"size"`
	Mode        int64     `json:"mode"`
	ModTime     time.Time `json:"modTime,omitempty"`
	LinkName    string    `json:"linkName,omitempty"`
	LayerDigest string    `json:"layerDigest,omitempty"`
	TarPath     string    `json:"tarPath,omitempty"`
}

type Directory struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type File struct {
	Entry  Entry
	Reader io.ReadCloser
}

type Layer interface {
	Open() (io.ReadCloser, error)
}

type LayerDescriptor struct {
	Digest    string `json:"digest"`
	MediaType string `json:"mediaType"`
	Size      int64  `json:"size"`
}

type ResolvedLayer struct {
	Descriptor LayerDescriptor
	Layer      Layer
}

type ResolvedImage struct {
	ManifestDigest string
	Platform       string
	Layers         []ResolvedLayer
}

type Source interface {
	Resolve(ctx context.Context, imageRef string, opts ociimage.OpenOptions) (*ResolvedImage, error)
	OpenLayer(ctx context.Context, imageRef, manifestDigest string, opts ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error)
}
