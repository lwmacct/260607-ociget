package imagebrowser

import (
	"errors"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

var ErrNotDirectory = errors.New("image path is not a directory")

type Options struct {
	Platform *v1.Platform
	Insecure bool
}

type ListRequest struct {
	ImageRef string
	Path     string
	Options  Options
}

type EntryType string

const (
	EntryTypeDirectory EntryType = "directory"
	EntryTypeFile      EntryType = "file"
	EntryTypeSymlink   EntryType = "symlink"
	EntryTypeOther     EntryType = "other"
)

type Entry struct {
	Name     string    `json:"name"`
	Path     string    `json:"path"`
	Type     EntryType `json:"type"`
	Size     int64     `json:"size"`
	Mode     int64     `json:"mode"`
	ModTime  time.Time `json:"modTime,omitempty"`
	LinkName string    `json:"linkName,omitempty"`
}

type Directory struct {
	Path    string  `json:"path"`
	Entries []Entry `json:"entries"`
}

type Browser struct{}
