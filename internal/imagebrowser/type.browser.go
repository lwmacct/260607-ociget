package imagebrowser

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func (b *Browser) List(ctx context.Context, req ListRequest) (*Directory, error) {
	target, err := normalizeDirectory(req.Path)
	if err != nil {
		return nil, err
	}

	extractor := &ociimage.Extractor{}
	img, err := extractor.Image(ctx, req.ImageRef, ociOptions(req.Options))
	if err != nil {
		return nil, fmt.Errorf("pull image metadata: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("read image layers: %w", err)
	}

	state := &directoryState{
		target:  target,
		entries: map[string]Entry{},
	}
	for i, layer := range layers {
		rc, err := layer.Uncompressed()
		if err != nil {
			return nil, fmt.Errorf("open layer %d: %w", i, err)
		}
		if err := state.scanLayer(rc); err != nil {
			_ = rc.Close()
			return nil, fmt.Errorf("scan layer %d: %w", i, err)
		}
		_ = rc.Close()
	}
	return state.directory()
}

func listFromLayers(target string, layers []io.Reader) (*Directory, error) {
	target, err := normalizeDirectory(target)
	if err != nil {
		return nil, err
	}
	state := &directoryState{
		target:  target,
		entries: map[string]Entry{},
	}
	for i, layer := range layers {
		if err := state.scanLayer(layer); err != nil {
			return nil, fmt.Errorf("scan layer %d: %w", i, err)
		}
	}
	return state.directory()
}

type directoryState struct {
	target       string
	targetExists bool
	targetType   EntryType
	blocked      bool
	entries      map[string]Entry
}

func (s *directoryState) scanLayer(rc io.Reader) error {
	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		entryPath := normalizeLayerPath(header.Name)
		if entryPath == "" {
			continue
		}
		if s.applyWhiteout(entryPath) {
			continue
		}
		if s.blocked {
			continue
		}
		s.applyEntry(header, entryPath)
	}
}

func (s *directoryState) directory() (*Directory, error) {
	if s.blocked {
		return nil, ociimage.ErrNotFound
	}
	if s.targetExists && s.targetType != EntryTypeDirectory {
		return nil, ErrNotDirectory
	}
	if !s.targetExists && len(s.entries) == 0 && s.target != "" {
		return nil, ociimage.ErrNotFound
	}
	return &Directory{
		Path:    s.target,
		Entries: sortedEntries(s.entries),
	}, nil
}

func (s *directoryState) applyWhiteout(entryPath string) bool {
	if target, ok := whiteoutTarget(entryPath); ok {
		if target == s.target {
			s.targetExists = false
			s.targetType = ""
			s.blocked = true
			s.entries = map[string]Entry{}
			return true
		}
		if name, ok := directChild(s.target, target); ok {
			delete(s.entries, name)
			return true
		}
		return true
	}

	if dir, ok := opaqueWhiteoutDir(entryPath); ok {
		if dir == s.target {
			s.targetExists = true
			s.targetType = EntryTypeDirectory
			s.entries = map[string]Entry{}
			return true
		}
		if s.target != "" && dir == "" {
			s.targetExists = false
			s.targetType = ""
			s.blocked = true
			s.entries = map[string]Entry{}
			return true
		}
		return true
	}
	return false
}

func (s *directoryState) applyEntry(header *tar.Header, entryPath string) {
	if entryPath == s.target {
		s.targetExists = true
		s.targetType = entryType(header, false)
		if s.targetType != EntryTypeDirectory {
			s.entries = map[string]Entry{}
		}
		return
	}

	name, ok := directChild(s.target, entryPath)
	if !ok {
		return
	}
	path := childPath(s.target, name)
	syntheticDir := entryPath != path
	if syntheticDir {
		s.entries[name] = Entry{
			Name: name,
			Path: path,
			Type: EntryTypeDirectory,
			Size: -1,
		}
		return
	}

	s.entries[name] = Entry{
		Name:     name,
		Path:     path,
		Type:     entryType(header, false),
		Size:     header.Size,
		Mode:     header.Mode,
		ModTime:  header.ModTime,
		LinkName: header.Linkname,
	}
}

func ociOptions(opts Options) ociimage.OpenOptions {
	return ociimage.OpenOptions{
		Platform: opts.Platform,
		Insecure: opts.Insecure,
	}
}
