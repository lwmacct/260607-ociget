package imagestore

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
)

func applyLayers(ctx context.Context, index *imageIndex, layers []ResolvedLayer) error {
	for layerIndex, resolved := range layers {
		if err := ctx.Err(); err != nil {
			return err
		}
		rc, err := resolved.Layer.Open()
		if err != nil {
			return fmt.Errorf("open layer %d: %w", layerIndex, err)
		}
		err = applyLayer(ctx, index, rc, resolved.Descriptor)
		closeErr := rc.Close()
		if err != nil {
			return fmt.Errorf("scan layer %d: %w", layerIndex, err)
		}
		if closeErr != nil {
			return fmt.Errorf("close layer %d: %w", layerIndex, closeErr)
		}
	}
	return nil
}

func applyLayer(ctx context.Context, index *imageIndex, reader io.Reader, descriptor LayerDescriptor) error {
	tr := tar.NewReader(reader)
	currentLayer := map[string]struct{}{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
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
		if applyWhiteout(index.Entries, entryPath, currentLayer) {
			continue
		}
		applyHeader(index.Entries, header, entryPath, descriptor)
		currentLayer[entryPath] = struct{}{}
	}
}

func applyHeader(entries map[string]Entry, header *tar.Header, entryPath string, descriptor LayerDescriptor) {
	ensureParents(entries, entryPath)
	entry := Entry{
		Name: path.Base(entryPath), Path: entryPath, Size: header.Size, Mode: header.Mode,
		ModTime: header.ModTime, LinkName: header.Linkname,
	}
	switch header.Typeflag {
	case tar.TypeDir:
		entry.Type, entry.Size = EntryTypeDirectory, -1
	case tar.TypeReg, tar.TypeRegA:
		entry.Type, entry.LayerDigest, entry.TarPath = EntryTypeFile, descriptor.Digest, strings.TrimPrefix(normalizeLayerPath(header.Name), "/")
	case tar.TypeSymlink:
		entry.Type, entry.Size = EntryTypeSymlink, 0
	case tar.TypeLink:
		target := normalizeLayerPath(header.Linkname)
		linked, ok := entries[target]
		if !ok || linked.Type != EntryTypeFile || linked.LayerDigest == "" {
			entry.Type, entry.Size = EntryTypeOther, 0
			break
		}
		entry.Type, entry.Size = EntryTypeFile, linked.Size
		entry.LayerDigest, entry.TarPath = linked.LayerDigest, linked.TarPath
	default:
		entry.Type = EntryTypeOther
	}
	if entry.Type != EntryTypeDirectory {
		deleteSubtree(entries, entryPath, false)
	}
	entries[entryPath] = entry
}

func openFileFromLayer(rc io.ReadCloser, tarPath, target string) (*File, error) {
	tr := tar.NewReader(rc)
	for {
		header, err := tr.Next()
		if errors.Is(err, io.EOF) {
			_ = rc.Close()
			return nil, ErrImageNotFound
		}
		if err != nil {
			_ = rc.Close()
			return nil, err
		}
		if normalizeLayerPath(header.Name) != normalizeLayerPath(tarPath) {
			continue
		}
		if header.Typeflag != tar.TypeReg && header.Typeflag != tar.TypeRegA {
			_ = rc.Close()
			return nil, ErrNotRegularFile
		}
		return &File{Entry: Entry{Path: target, Size: header.Size, Mode: header.Mode, ModTime: header.ModTime}, Reader: &layerFileReader{Reader: tr, closer: rc}}, nil
	}
}

type layerFileReader struct {
	io.Reader
	closer io.Closer
}

func (r *layerFileReader) Close() error { return r.closer.Close() }

func normalizePath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "/" || input == "." {
		return "/", nil
	}
	if strings.ContainsRune(input, '\x00') || strings.Contains(input, "\\") {
		return "", fmt.Errorf("invalid image path")
	}
	for _, part := range strings.Split(input, "/") {
		if part == ".." {
			return "", fmt.Errorf("invalid image path")
		}
	}
	return path.Clean("/" + input), nil
}

func normalizeLayerPath(input string) string {
	input = strings.TrimPrefix(strings.TrimSpace(input), "./")
	cleaned := path.Clean("/" + input)
	if cleaned == "/" || cleaned == "/." {
		return ""
	}
	return cleaned
}

func ensureParents(entries map[string]Entry, entryPath string) {
	parent := parentPath(entryPath)
	var missing []string
	for parent != "/" {
		if entry, ok := entries[parent]; ok && entry.Type == EntryTypeDirectory {
			break
		}
		missing = append(missing, parent)
		parent = parentPath(parent)
	}
	for i := len(missing) - 1; i >= 0; i-- {
		current := missing[i]
		deleteSubtree(entries, current, false)
		entries[current] = Entry{Name: path.Base(current), Path: current, Type: EntryTypeDirectory, Size: -1}
	}
}

func parentPath(entryPath string) string {
	parent := path.Dir(entryPath)
	if parent == "." || parent == "" {
		return "/"
	}
	return parent
}

func applyWhiteout(entries map[string]Entry, entryPath string, currentLayer map[string]struct{}) bool {
	base := path.Base(entryPath)
	if base == ".wh..wh..opq" {
		deleteLowerSubtree(entries, parentPath(entryPath), true, currentLayer)
		return true
	}
	if !strings.HasPrefix(base, ".wh.") {
		return false
	}
	deleteLowerSubtree(entries, path.Join(parentPath(entryPath), strings.TrimPrefix(base, ".wh.")), false, currentLayer)
	return true
}

func deleteLowerSubtree(entries map[string]Entry, target string, descendantsOnly bool, currentLayer map[string]struct{}) {
	prefix := target + "/"
	for candidate := range entries {
		if candidate != target && !strings.HasPrefix(candidate, prefix) {
			continue
		}
		if descendantsOnly && candidate == target {
			continue
		}
		if protectedByCurrentLayer(candidate, currentLayer) {
			continue
		}
		delete(entries, candidate)
	}
	if _, ok := entries["/"]; !ok {
		entries["/"] = Entry{Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1}
	}
}

func protectedByCurrentLayer(candidate string, currentLayer map[string]struct{}) bool {
	for current := range currentLayer {
		if candidate == current || strings.HasPrefix(current, candidate+"/") {
			return true
		}
	}
	return false
}

func deleteSubtree(entries map[string]Entry, target string, descendantsOnly bool) {
	prefix := target + "/"
	for candidate := range entries {
		if strings.HasPrefix(candidate, prefix) || (!descendantsOnly && candidate == target) {
			delete(entries, candidate)
		}
	}
	if _, ok := entries["/"]; !ok {
		entries["/"] = Entry{Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1}
	}
}
