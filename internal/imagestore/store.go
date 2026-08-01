package imagestore

import (
	"context"
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
	dir      string
	refTTL   time.Duration
	maxBytes int64
	source   Source
	mu       sync.RWMutex
	refs     map[string]refRecord
	indexes  map[string]*imageIndex
	group    singleflight.Group
}

type refRecord struct {
	ImageID    string    `json:"imageId"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

type imageIndex struct {
	Image   Image            `json:"image"`
	Entries map[string]Entry `json:"entries"`
}

func New(cfg Config) (*Store, error) {
	return newStore(cfg, &remoteSource{})
}

func NewWithSource(cfg Config, source Source) (*Store, error) {
	if source == nil {
		return nil, errors.New("image source is required")
	}
	return newStore(cfg, source)
}

func newStore(cfg Config, source Source) (*Store, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, errors.New("image store dir is required")
	}
	if cfg.RefTTL < 0 {
		return nil, errors.New("image store ref TTL must not be negative")
	}
	if cfg.MaxBytes < 0 {
		return nil, errors.New("image store max bytes must not be negative")
	}
	for _, name := range []string{"images", "objects/sha256", "staging", "locks"} {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, name), 0o700); err != nil {
			return nil, fmt.Errorf("prepare image store: %w", err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(cfg.Dir, "staging")); err == nil {
		for _, entry := range entries {
			if err := os.RemoveAll(filepath.Join(cfg.Dir, "staging", entry.Name())); err != nil {
				return nil, fmt.Errorf("clean image staging directory: %w", err)
			}
		}
	}
	store := &Store{
		dir:      cfg.Dir,
		refTTL:   cfg.RefTTL,
		maxBytes: cfg.MaxBytes,
		source:   source,
		refs:     map[string]refRecord{},
		indexes:  map[string]*imageIndex{},
	}
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
		resolved, err := s.source.Resolve(ctx, req.ImageRef, ociimage.OpenOptions{
			Platform: req.Platform,
			Insecure: req.Insecure,
		})
		if err != nil {
			return nil, err
		}
		if !validImageID(resolved.ImageID) {
			return nil, fmt.Errorf("invalid resolved image digest %q", resolved.ImageID)
		}
		image, err := s.ensureImage(ctx, req.ImageRef, resolved)
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
	image := index.Image
	s.touchImage(imageID)
	return &image, nil
}

func (s *Store) List(imageID, inputPath string) (*Directory, error) {
	index, err := s.loadIndex(imageID)
	if err != nil {
		return nil, err
	}
	s.touchImage(imageID)
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
		item.ContentDigest = ""
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
	s.touchImage(imageID)
	target, err := normalizePath(inputPath)
	if err != nil {
		return nil, err
	}
	entry, ok := index.Entries[target]
	if !ok {
		return nil, ociimage.ErrNotFound
	}
	if entry.Type != EntryTypeFile || entry.ContentDigest == "" {
		return nil, ErrNotRegularFile
	}
	file, err := os.Open(s.objectPath(entry.ContentDigest))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrImageNotFound
		}
		return nil, err
	}
	return &File{Entry: entry, Reader: file}, nil
}

func (s *Store) ensureImage(ctx context.Context, imageRef string, resolved *ResolvedImage) (*Image, error) {
	if image, err := s.Image(resolved.ImageID); err == nil {
		return image, nil
	}
	value, err, _ := s.group.Do("image:"+resolved.ImageID, func() (any, error) {
		if image, err := s.Image(resolved.ImageID); err == nil {
			return image, nil
		}
		index, err := s.withImageLock(ctx, resolved.ImageID, func() (*imageIndex, error) {
			if image, err := s.loadIndex(resolved.ImageID); err == nil {
				return image, nil
			}
			return s.build(ctx, imageRef, resolved)
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

func (s *Store) build(ctx context.Context, imageRef string, resolved *ResolvedImage) (*imageIndex, error) {
	stage, err := os.MkdirTemp(filepath.Join(s.dir, "staging"), "image-*")
	if err != nil {
		return nil, fmt.Errorf("create image staging directory: %w", err)
	}
	defer os.RemoveAll(stage)

	index := &imageIndex{
		Image: Image{
			ImageID:   resolved.ImageID,
			ImageRef:  imageRef,
			Platform:  resolved.Platform,
			CreatedAt: time.Now().UTC(),
		},
		Entries: map[string]Entry{
			"/": {Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1},
		},
	}
	if err := s.applyLayers(ctx, index, resolved.Layers); err != nil {
		return nil, err
	}
	data, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("encode image index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, indexFilename), data, 0o600); err != nil {
		return nil, fmt.Errorf("write image index: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, ".access"), nil, 0o600); err != nil {
		return nil, fmt.Errorf("write image access marker: %w", err)
	}

	final := s.imageDir(resolved.ImageID)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return nil, fmt.Errorf("prepare image index directory: %w", err)
	}
	if err := os.Rename(stage, final); err != nil {
		if _, statErr := os.Stat(filepath.Join(final, indexFilename)); statErr != nil {
			return nil, fmt.Errorf("publish image index: %w", err)
		}
	}
	s.mu.Lock()
	s.indexes[resolved.ImageID] = index
	s.mu.Unlock()
	if err := s.enforceLimit(ctx, resolved.ImageID); err != nil {
		return nil, err
	}
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
		return nil, fmt.Errorf("decode image index: %w", err)
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
	path := filepath.Join(s.imageDir(imageID), ".access")
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

func (s *Store) enforceLimit(ctx context.Context, keepImageID string) error {
	if s.maxBytes == 0 {
		return nil
	}
	type imageUsage struct {
		id        string
		access    time.Time
		objects   map[string]struct{}
		protected bool
	}
	entries, err := os.ReadDir(filepath.Join(s.dir, "images", "sha256"))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil
		}
		return err
	}
	protected := map[string]struct{}{keepImageID: {}}
	s.mu.RLock()
	for _, ref := range s.refs {
		protected[ref.ImageID] = struct{}{}
	}
	s.mu.RUnlock()
	usage := make([]imageUsage, 0, len(entries))
	allObjects := map[string]int64{}
	var total int64
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id := "sha256:" + entry.Name()
		dir := filepath.Join(s.dir, "images", "sha256", entry.Name())
		access := time.Time{}
		if stat, statErr := os.Stat(filepath.Join(dir, ".access")); statErr == nil {
			access = stat.ModTime()
		}
		index, indexErr := s.loadIndex(id)
		if indexErr != nil {
			continue
		}
		objects := map[string]struct{}{}
		for _, item := range index.Entries {
			if item.ContentDigest == "" {
				continue
			}
			objects[item.ContentDigest] = struct{}{}
			if size, sizeErr := objectSize(s.objectPath(item.ContentDigest)); sizeErr == nil {
				allObjects[item.ContentDigest] = size
			}
		}
		_, isProtected := protected[id]
		usage = append(usage, imageUsage{id: id, access: access, objects: objects, protected: isProtected})
	}
	physicalObjects, err := os.ReadDir(filepath.Join(s.dir, "objects", "sha256"))
	if err != nil {
		return err
	}
	for _, object := range physicalObjects {
		if object.IsDir() {
			continue
		}
		size, sizeErr := objectSize(filepath.Join(s.dir, "objects", "sha256", object.Name()))
		if sizeErr != nil {
			return sizeErr
		}
		if _, referenced := allObjects[object.Name()]; !referenced {
			_ = os.Remove(filepath.Join(s.dir, "objects", "sha256", object.Name()))
			continue
		}
		total += size
	}
	if total <= s.maxBytes {
		return nil
	}
	sort.Slice(usage, func(i, j int) bool { return usage[i].access.Before(usage[j].access) })
	active := make(map[string]bool, len(usage))
	for _, item := range usage {
		active[item.id] = true
	}
	for _, item := range usage {
		if total <= s.maxBytes {
			break
		}
		if item.protected {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.RemoveAll(s.imageDir(item.id)); err != nil {
			return fmt.Errorf("remove evicted image: %w", err)
		}
		active[item.id] = false
		s.mu.Lock()
		delete(s.indexes, item.id)
		s.mu.Unlock()
		for digest := range item.objects {
			stillReferenced := false
			for _, other := range usage {
				if !active[other.id] {
					continue
				}
				if _, ok := other.objects[digest]; ok {
					stillReferenced = true
					break
				}
			}
			if !stillReferenced {
				if size, ok := allObjects[digest]; ok {
					total -= size
					delete(allObjects, digest)
				}
				_ = os.Remove(s.objectPath(digest))
			}
		}
	}
	return nil
}

func objectSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

func (s *Store) imageDir(imageID string) string {
	parts := strings.SplitN(imageID, ":", 2)
	return filepath.Join(s.dir, "images", parts[0], parts[1])
}

func (s *Store) objectPath(digest string) string {
	return filepath.Join(s.dir, "objects", "sha256", digest)
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
