package imagestore

import (
	"archive/tar"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"strings"
)

func (s *Store) applyLayers(ctx context.Context, index *imageIndex, layers []Layer) error {
	for layerIndex, layer := range layers {
		if err := ctx.Err(); err != nil {
			return err
		}
		rc, err := layer.Open()
		if err != nil {
			return fmt.Errorf("open layer %d: %w", layerIndex, err)
		}
		err = s.applyLayer(ctx, index, rc)
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

func (s *Store) applyLayer(ctx context.Context, index *imageIndex, reader io.Reader) error {
	tarReader := tar.NewReader(reader)
	currentLayer := map[string]struct{}{}
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		header, err := tarReader.Next()
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
		if err := s.applyHeader(index.Entries, header, entryPath, tarReader); err != nil {
			return err
		}
		currentLayer[entryPath] = struct{}{}
	}
}

func (s *Store) applyHeader(entries map[string]Entry, header *tar.Header, entryPath string, body io.Reader) error {
	ensureParents(entries, entryPath)
	entry := Entry{
		Name:     path.Base(entryPath),
		Path:     entryPath,
		Size:     header.Size,
		Mode:     header.Mode,
		ModTime:  header.ModTime,
		LinkName: header.Linkname,
	}
	switch header.Typeflag {
	case tar.TypeDir:
		entry.Type = EntryTypeDirectory
		entry.Size = -1
	case tar.TypeReg, tar.TypeRegA:
		entry.Type = EntryTypeFile
		digest, size, err := s.putObject(body)
		if err != nil {
			return fmt.Errorf("store %s: %w", entryPath, err)
		}
		entry.ContentDigest = digest
		entry.Size = size
	case tar.TypeSymlink:
		entry.Type = EntryTypeSymlink
		entry.Size = 0
	case tar.TypeLink:
		target := normalizeLayerPath(header.Linkname)
		linked, ok := entries[target]
		if !ok || linked.Type != EntryTypeFile || linked.ContentDigest == "" {
			entry.Type = EntryTypeOther
			entry.Size = 0
			break
		}
		entry.Type = EntryTypeFile
		entry.Size = linked.Size
		entry.ContentDigest = linked.ContentDigest
	default:
		entry.Type = EntryTypeOther
	}
	if entry.Type != EntryTypeDirectory {
		deleteSubtree(entries, entryPath, false)
	}
	entries[entryPath] = entry
	return nil
}

func (s *Store) putObject(reader io.Reader) (string, int64, error) {
	tmp, err := os.CreateTemp(filepath.Join(s.dir, "objects", "sha256"), ".object-*.tmp")
	if err != nil {
		return "", 0, err
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	hash := sha256.New()
	size, err := io.Copy(io.MultiWriter(tmp, hash), reader)
	if err != nil {
		_ = tmp.Close()
		return "", 0, err
	}
	if err := tmp.Close(); err != nil {
		return "", 0, err
	}
	digest := hex.EncodeToString(hash.Sum(nil))
	final := s.objectPath(digest)
	if _, err := os.Stat(final); err == nil {
		return digest, size, nil
	}
	if err := os.Rename(tmpName, final); err != nil {
		if _, statErr := os.Stat(final); statErr != nil {
			return "", 0, err
		}
	}
	return digest, size, nil
}

func normalizePath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" || input == "/" || input == "." {
		return "/", nil
	}
	if strings.ContainsRune(input, '\x00') {
		return "", fmt.Errorf("invalid image path")
	}
	cleaned := path.Clean("/" + input)
	if cleaned == "/" {
		return "/", nil
	}
	return cleaned, nil
}

func normalizeLayerPath(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "./")
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
	for index := len(missing) - 1; index >= 0; index-- {
		current := missing[index]
		deleteSubtree(entries, current, false)
		entries[current] = Entry{
			Name: path.Base(current), Path: current, Type: EntryTypeDirectory, Size: -1,
		}
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
		dir := parentPath(entryPath)
		deleteLowerSubtree(entries, dir, true, currentLayer)
		return true
	}
	if !strings.HasPrefix(base, ".wh.") {
		return false
	}
	target := path.Join(parentPath(entryPath), strings.TrimPrefix(base, ".wh."))
	deleteLowerSubtree(entries, target, false, currentLayer)
	return true
}

func deleteLowerSubtree(entries map[string]Entry, target string, descendantsOnly bool, currentLayer map[string]struct{}) {
	prefix := target + "/"
	if target == "/" {
		prefix = "/"
	}
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
	if target == "/" {
		prefix = "/"
	}
	for candidate := range entries {
		if strings.HasPrefix(candidate, prefix) || (!descendantsOnly && candidate == target) {
			delete(entries, candidate)
		}
	}
	if _, ok := entries["/"]; !ok {
		entries["/"] = Entry{Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1}
	}
}
