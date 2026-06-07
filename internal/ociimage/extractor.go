package ociimage

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"runtime"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
)

var (
	ErrNotFound            = errors.New("file not found in image")
	ErrUnsupportedFileType = errors.New("image path is not a regular file")
)

type Extractor struct{}

type File struct {
	Path    string
	Size    int64
	Mode    int64
	ModTime time.Time
	Reader  io.ReadCloser
}

type layerOpener interface {
	Open() (io.ReadCloser, error)
}

type imageLayer struct {
	layer v1.Layer
}

func (l imageLayer) Open() (io.ReadCloser, error) {
	return l.layer.Uncompressed()
}

func (e *Extractor) OpenFile(ctx context.Context, imageRef, filePath string) (*File, error) {
	target, err := NormalizePath(filePath)
	if err != nil {
		return nil, err
	}

	ref, err := name.ParseReference(imageRef)
	if err != nil {
		return nil, fmt.Errorf("parse image reference: %w", err)
	}

	img, err := remote.Image(
		ref,
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
		remote.WithPlatform(v1.Platform{
			OS:           runtime.GOOS,
			Architecture: runtime.GOARCH,
		}),
	)
	if err != nil {
		return nil, fmt.Errorf("pull image metadata: %w", err)
	}

	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("read image layers: %w", err)
	}

	openers := make([]layerOpener, 0, len(layers))
	for _, layer := range layers {
		openers = append(openers, imageLayer{layer: layer})
	}
	return openFileFromLayers(openers, target)
}

func openFileFromLayers(layers []layerOpener, target string) (*File, error) {
	for i := len(layers) - 1; i >= 0; i-- {
		rc, err := layers[i].Open()
		if err != nil {
			return nil, fmt.Errorf("open layer %d: %w", i, err)
		}

		file, hidden, err := scanLayer(rc, target)
		if err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("scan layer %d: %w", i, err)
		}
		if file != nil {
			return file, nil
		}
		_ = rc.Close()
		if hidden {
			return nil, ErrNotFound
		}
	}
	return nil, ErrNotFound
}

func scanLayer(rc io.ReadCloser, target string) (*File, bool, error) {
	target = normalizeLayerPath(target)
	tr := tar.NewReader(rc)

	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil, false, nil
		}
		if err != nil {
			return nil, false, err
		}

		entry := normalizeLayerPath(header.Name)
		if entry == "" {
			continue
		}

		if isWhiteoutForTarget(entry, target) || isOpaqueWhiteoutForTarget(entry, target) {
			return nil, true, nil
		}
		if entry != target {
			continue
		}

		switch header.Typeflag {
		case tar.TypeReg, tar.TypeRegA:
			return &File{
				Path:    target,
				Size:    header.Size,
				Mode:    header.Mode,
				ModTime: header.ModTime,
				Reader:  readCloser{Reader: tr, Closer: rc},
			}, false, nil
		default:
			return nil, false, ErrUnsupportedFileType
		}
	}
}

func isWhiteoutForTarget(entry, target string) bool {
	dir := path.Dir(target)
	if dir == "." {
		dir = ""
	}
	whiteout := ".wh." + path.Base(target)
	if dir != "" {
		whiteout = dir + "/" + whiteout
	}
	return entry == whiteout
}

func isOpaqueWhiteoutForTarget(entry, target string) bool {
	if !strings.HasSuffix(entry, "/.wh..wh..opq") && entry != ".wh..wh..opq" {
		return false
	}

	dir := strings.TrimSuffix(entry, "/.wh..wh..opq")
	if dir == ".wh..wh..opq" {
		dir = ""
	}
	if dir == "" {
		return true
	}
	return target == dir || strings.HasPrefix(target, dir+"/")
}

type readCloser struct {
	io.Reader
	io.Closer
}
