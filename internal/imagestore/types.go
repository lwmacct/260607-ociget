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
	ErrImageNotFound  = errors.New("image not found in store")
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
	Dir      string
	RefTTL   time.Duration
	MaxBytes int64
}

type OpenRequest struct {
	ImageRef string
	Platform *v1.Platform
	Insecure bool
	Refresh  bool
}

type Image struct {
	ImageID   string    `json:"imageId"`
	ImageRef  string    `json:"imageRef"`
	Platform  string    `json:"platform"`
	CreatedAt time.Time `json:"createdAt"`
}

type Entry struct {
	Name          string    `json:"name"`
	Path          string    `json:"path"`
	Type          EntryType `json:"type"`
	Size          int64     `json:"size"`
	Mode          int64     `json:"mode"`
	ModTime       time.Time `json:"modTime,omitempty"`
	LinkName      string    `json:"linkName,omitempty"`
	ContentDigest string    `json:"contentDigest,omitempty"`
}

type Directory struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type File struct {
	Entry  Entry
	Reader io.ReadSeekCloser
}

type Layer interface {
	Open() (io.ReadCloser, error)
}

type ResolvedImage struct {
	ImageID  string
	Platform string
	Layers   []Layer
}

type Source interface {
	Resolve(ctx context.Context, imageRef string, opts ociimage.OpenOptions) (*ResolvedImage, error)
}
