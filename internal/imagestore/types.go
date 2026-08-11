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
	ErrObjectNotFound      = errors.New("OCI object not found in metadata store")
	ErrBlobNotFound        = errors.New("artifact blob not found")
	ErrInvalidBlobSelector = errors.New("invalid artifact blob selector")
	ErrAmbiguousBlob       = errors.New("artifact blob selector is ambiguous")
	ErrNotImage            = errors.New("OCI object is not an image")
	ErrNotArtifact         = errors.New("OCI object is not an artifact")
	ErrNotDirectory        = errors.New("image path is not a directory")
	ErrNotRegularFile      = errors.New("image path is not a regular file")
)

type ObjectKind string

const (
	ObjectKindImage    ObjectKind = "image"
	ObjectKindArtifact ObjectKind = "artifact"
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
	Reference string
	Platform  *v1.Platform
	Insecure  bool
	Refresh   bool
}

type OCIObject struct {
	ObjectID          string            `json:"objectId"`
	Kind              ObjectKind        `json:"kind"`
	ManifestDigest    string            `json:"manifestDigest"`
	ManifestMediaType string            `json:"manifestMediaType"`
	Reference         string            `json:"reference"`
	Platform          string            `json:"platform,omitempty"`
	Insecure          bool              `json:"insecure,omitempty"`
	ArtifactType      string            `json:"artifactType,omitempty"`
	Annotations       map[string]string `json:"annotations,omitempty"`
	Blobs             []BlobDescriptor  `json:"blobs,omitempty"`
	CreatedAt         time.Time         `json:"createdAt"`
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

type BlobDescriptor struct {
	Digest      string            `json:"digest"`
	MediaType   string            `json:"mediaType"`
	Size        int64             `json:"size"`
	Name        string            `json:"name"`
	Annotations map[string]string `json:"annotations,omitempty"`
}

type Blob struct {
	Descriptor BlobDescriptor
	Reader     io.ReadCloser
}

type ResolvedLayer struct {
	Descriptor LayerDescriptor
	Layer      Layer
}

type ResolvedObject struct {
	Kind              ObjectKind
	ManifestDigest    string
	ManifestMediaType string
	Platform          string
	ArtifactType      string
	Annotations       map[string]string
	Blobs             []BlobDescriptor
	Layers            []ResolvedLayer
}

type Source interface {
	Resolve(ctx context.Context, reference string, opts ociimage.OpenOptions) (*ResolvedObject, error)
	OpenLayer(ctx context.Context, reference string, opts ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error)
	OpenBlob(ctx context.Context, reference string, opts ociimage.OpenOptions, descriptor BlobDescriptor) (io.ReadCloser, error)
}
