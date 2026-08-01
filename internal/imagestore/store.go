package imagestore

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"
	"time"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
	"golang.org/x/sync/singleflight"
)

const indexFilename = "index.json"

type Store struct {
	dir     string
	refTTL  time.Duration
	source  Source
	mu      sync.RWMutex
	refs    map[string]refRecord
	indexes map[string]*imageIndex
	group   singleflight.Group
}

type refRecord struct {
	ImageID    string    `json:"imageId"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

type imageIndex struct {
	Image   Image             `json:"image"`
	Layers  []LayerDescriptor `json:"layers"`
	Entries map[string]Entry  `json:"entries"`
}

func New(cfg Config) (*Store, error) { return newStore(cfg, &remoteSource{}) }

func NewWithSource(cfg Config, source Source) (*Store, error) {
	if source == nil {
		return nil, errors.New("image source is required")
	}
	return newStore(cfg, source)
}

func newStore(cfg Config, source Source) (*Store, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, errors.New("image metadata directory is required")
	}
	if cfg.RefTTL < 0 {
		return nil, errors.New("image metadata ref TTL must not be negative")
	}
	for _, name := range []string{"images", "staging", "locks"} {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, name), 0o700); err != nil {
			return nil, fmt.Errorf("prepare image metadata store: %w", err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(cfg.Dir, "staging")); err == nil {
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(cfg.Dir, "staging", entry.Name()))
		}
	}
	store := &Store{dir: cfg.Dir, refTTL: cfg.RefTTL, source: source, refs: map[string]refRecord{}, indexes: map[string]*imageIndex{}}
	if err := store.loadRefs(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Open(ctx context.Context, req OpenRequest) (*Image, error) {
	req.ImageRef = strings.TrimSpace(req.ImageRef)
	if req.ImageRef == "" {
		return nil, errors.New("image reference is required")
	}
	key := refKey(req)
	if !req.Refresh {
		if image := s.cachedRef(key); image != nil {
			return image, nil
		}
	}
	value, err, _ := s.group.Do("ref:"+key, func() (any, error) {
		if !req.Refresh {
			if image := s.cachedRef(key); image != nil {
				return image, nil
			}
		}
		resolved, err := s.source.Resolve(ctx, req.ImageRef, ociimage.OpenOptions{Platform: req.Platform, Insecure: req.Insecure})
		if err != nil {
			return nil, err
		}
		if !validManifestDigest(resolved.ManifestDigest) {
			return nil, fmt.Errorf("invalid resolved manifest digest %q", resolved.ManifestDigest)
		}
		imageID := sourceImageID(req, resolved.ManifestDigest)
		image, err := s.ensureImage(ctx, imageID, req, resolved)
		if err != nil {
			return nil, err
		}
		if err := s.saveRef(key, refRecord{ImageID: image.ImageID, ResolvedAt: time.Now().UTC()}); err != nil {
			return nil, err
		}
		return image, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*Image), nil
}

func (s *Store) Image(imageID string) (*Image, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	s.touchImage(imageID)
	image := index.Image
	return &image, nil
}

func (s *Store) List(imageID, inputPath string) (*Directory, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	target, err := normalizePath(inputPath)
	if err != nil {
		return nil, err
	}
	entry, ok := index.Entries[target]
	if !ok {
		return nil, ociimage.ErrNotFound
	}
	if entry.Type != EntryTypeDirectory {
		return nil, ErrNotDirectory
	}
	entries := make([]Entry, 0)
	for candidate, item := range index.Entries {
		if candidate == target || parentPath(candidate) != target {
			continue
		}
		item.LayerDigest = ""
		item.TarPath = ""
		entries = append(entries, item)
	}
	sortEntries(entries)
	return &Directory{Path: target, Entries: entries}, nil
}

func (s *Store) OpenFile(imageID, inputPath string) (*File, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	target, err := normalizePath(inputPath)
	if err != nil {
		return nil, err
	}
	entry, ok := index.Entries[target]
	if !ok {
		return nil, ociimage.ErrNotFound
	}
	if entry.Type != EntryTypeFile || entry.LayerDigest == "" {
		return nil, ErrNotRegularFile
	}
	return &File{Entry: entry}, nil
}

func (s *Store) OpenFileReader(ctx context.Context, imageID, inputPath string) (*File, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	target, err := normalizePath(inputPath)
	if err != nil {
		return nil, err
	}
	entry, ok := index.Entries[target]
	if !ok {
		return nil, ociimage.ErrNotFound
	}
	if entry.Type != EntryTypeFile || entry.LayerDigest == "" {
		return nil, ErrNotRegularFile
	}
	options := ociimage.OpenOptions{Insecure: index.Image.Insecure}
	if index.Image.Platform != "" && index.Image.Platform != "default" {
		platform, err := ociimage.ParsePlatform(index.Image.Platform)
		if err != nil {
			return nil, err
		}
		options.Platform = &platform
	}
	reader, err := s.source.OpenLayer(ctx, index.Image.ImageRef, index.Image.ManifestDigest, options, LayerDescriptor{Digest: entry.LayerDigest})
	if err != nil {
		return nil, err
	}
	file, err := openFileFromLayer(reader, entry.TarPath, target)
	if err != nil {
		return nil, err
	}
	file.Entry = entry
	return file, nil
}

func (s *Store) OpenFileForRequest(ctx context.Context, imageID, inputPath string, req OpenRequest) (*File, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	target, err := normalizePath(inputPath)
	if err != nil {
		return nil, err
	}
	entry, ok := index.Entries[target]
	if !ok {
		return nil, ociimage.ErrNotFound
	}
	if entry.Type != EntryTypeFile || entry.LayerDigest == "" {
		return nil, ErrNotRegularFile
	}
	reader, err := s.source.OpenLayer(ctx, req.ImageRef, index.Image.ManifestDigest, ociimage.OpenOptions{Platform: req.Platform, Insecure: req.Insecure}, LayerDescriptor{Digest: entry.LayerDigest})
	if err != nil {
		return nil, err
	}
	file, err := openFileFromLayer(reader, entry.TarPath, target)
	if err != nil {
		return nil, err
	}
	file.Entry = entry
	return file, nil
}

func (s *Store) ensureImage(ctx context.Context, imageID string, req OpenRequest, resolved *ResolvedImage) (*Image, error) {
	if image, err := s.Image(imageID); err == nil {
		return image, nil
	}
	value, err, _ := s.group.Do("image:"+imageID, func() (any, error) {
		if image, err := s.Image(imageID); err == nil {
			return image, nil
		}
		index, err := s.withImageLock(ctx, imageID, func() (*imageIndex, error) {
			if index, err := s.loadIndex(imageID); err == nil {
				return index, nil
			}
			return s.build(ctx, imageID, req, resolved)
		})
		if err != nil {
			return nil, err
		}
		return &index.Image, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*Image), nil
}

func (s *Store) build(ctx context.Context, imageID string, req OpenRequest, resolved *ResolvedImage) (*imageIndex, error) {
	stage, err := os.MkdirTemp(filepath.Join(s.dir, "staging"), "image-*")
	if err != nil {
		return nil, fmt.Errorf("create metadata staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	index := &imageIndex{
		Image:   Image{ImageID: imageID, ManifestDigest: resolved.ManifestDigest, ImageRef: req.ImageRef, Platform: resolved.Platform, Insecure: req.Insecure, CreatedAt: time.Now().UTC()},
		Layers:  make([]LayerDescriptor, 0, len(resolved.Layers)),
		Entries: map[string]Entry{"/": {Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1}},
	}
	for _, layer := range resolved.Layers {
		index.Layers = append(index.Layers, layer.Descriptor)
	}
	if err := applyLayers(ctx, index, resolved.Layers); err != nil {
		return nil, err
	}
	data, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("encode image metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, indexFilename), data, 0o600); err != nil {
		return nil, fmt.Errorf("write image metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, ".access"), nil, 0o600); err != nil {
		return nil, fmt.Errorf("write image access marker: %w", err)
	}
	final := s.imageDir(imageID)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return nil, fmt.Errorf("prepare image metadata directory: %w", err)
	}
	if err := os.Rename(stage, final); err != nil {
		if _, statErr := os.Stat(filepath.Join(final, indexFilename)); statErr != nil {
			return nil, fmt.Errorf("publish image metadata: %w", err)
		}
	}
	s.mu.Lock()
	s.indexes[imageID] = index
	s.mu.Unlock()
	return index, nil
}

func (s *Store) loadIndex(imageID string) (*imageIndex, error) {
	if !validImageID(imageID) {
		return nil, ErrImageNotFound
	}
	s.mu.RLock()
	index := s.indexes[imageID]
	s.mu.RUnlock()
	if index != nil {
		return index, nil
	}
	data, err := os.ReadFile(filepath.Join(s.imageDir(imageID), indexFilename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrImageNotFound
		}
		return nil, err
	}
	index = &imageIndex{}
	if err := json.Unmarshal(data, index); err != nil {
		return nil, fmt.Errorf("decode image metadata: %w", err)
	}
	s.mu.Lock()
	if current := s.indexes[imageID]; current != nil {
		index = current
	} else {
		s.indexes[imageID] = index
	}
	s.mu.Unlock()
	return index, nil
}

func (s *Store) cachedRef(key string) *Image {
	s.mu.RLock()
	record, ok := s.refs[key]
	s.mu.RUnlock()
	if !ok || (s.refTTL > 0 && time.Since(record.ResolvedAt) > s.refTTL) {
		return nil
	}
	image, err := s.Image(record.ImageID)
	if err != nil {
		return nil
	}
	return image
}

func (s *Store) loadRefs() error {
	data, err := os.ReadFile(filepath.Join(s.dir, "refs.json"))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read image refs: %w", err)
	}
	if err := json.Unmarshal(data, &s.refs); err != nil {
		return fmt.Errorf("decode image refs: %w", err)
	}
	return nil
}

func (s *Store) saveRef(key string, record refRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refs[key] = record
	data, err := json.Marshal(s.refs)
	if err != nil {
		return fmt.Errorf("encode image refs: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".refs-*.tmp")
	if err != nil {
		return fmt.Errorf("create image refs file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write image refs: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close image refs: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(s.dir, "refs.json")); err != nil {
		return fmt.Errorf("publish image refs: %w", err)
	}
	return nil
}

func (s *Store) touchImage(imageID string) {
	_ = os.Chtimes(filepath.Join(s.imageDir(imageID), ".access"), time.Now(), time.Now())
}

func (s *Store) imageDir(imageID string) string {
	parts := strings.SplitN(imageID, ":", 2)
	return filepath.Join(s.dir, "images", parts[0], parts[1])
}

func refKey(req OpenRequest) string {
	return fmt.Sprintf("%t\n%s\n%s", req.Insecure, req.ImageRef, platformString(req.Platform))
}

func platformString(platform *v1.Platform) string {
	if platform == nil {
		return "default"
	}
	parts := []string{platform.OS, platform.Architecture}
	if platform.Variant != "" {
		parts = append(parts, platform.Variant)
	}
	return strings.Join(parts, "/")
}

func sourceImageID(req OpenRequest, manifestDigest string) string {
	key := fmt.Sprintf("%t\n%s\n%s\n%s", req.Insecure, req.ImageRef, platformString(req.Platform), manifestDigest)
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validImageID(imageID string) bool {
	parts := strings.SplitN(imageID, ":", 2)
	if len(parts) != 2 || parts[0] != "sha256" || len(parts[1]) != 64 {
		return false
	}
	for _, char := range parts[1] {
		if !strings.ContainsRune("0123456789abcdef", char) {
			return false
		}
	}
	return true
}

func validManifestDigest(value string) bool { return validImageID(value) }

func sortEntries(entries []Entry) {
	sort.Slice(entries, func(i, j int) bool {
		if entries[i].Type == EntryTypeDirectory && entries[j].Type != EntryTypeDirectory {
			return true
		}
		if entries[i].Type != EntryTypeDirectory && entries[j].Type == EntryTypeDirectory {
			return false
		}
		return entries[i].Name < entries[j].Name
	})
}
