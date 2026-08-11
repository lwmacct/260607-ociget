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

const (
	indexFilename = "index.json"
	refsFilename  = "object-refs.json"
)

type Store struct {
	dir     string
	refTTL  time.Duration
	source  Source
	mu      sync.RWMutex
	refs    map[string]refRecord
	indexes map[string]*objectIndex
	group   singleflight.Group
}

type refRecord struct {
	ObjectID   string    `json:"objectId"`
	ResolvedAt time.Time `json:"resolvedAt"`
}

type objectIndex struct {
	Object  OCIObject         `json:"object"`
	Layers  []LayerDescriptor `json:"layers,omitempty"`
	Entries map[string]Entry  `json:"entries,omitempty"`
}

func New(cfg Config) (*Store, error) { return newStore(cfg, &remoteSource{}) }

func NewWithSource(cfg Config, source Source) (*Store, error) {
	if source == nil {
		return nil, errors.New("OCI source is required")
	}
	return newStore(cfg, source)
}

func newStore(cfg Config, source Source) (*Store, error) {
	if strings.TrimSpace(cfg.Dir) == "" {
		return nil, errors.New("OCI metadata directory is required")
	}
	if cfg.RefTTL < 0 {
		return nil, errors.New("OCI metadata ref TTL must not be negative")
	}
	for _, name := range []string{"objects", "staging", "locks"} {
		if err := os.MkdirAll(filepath.Join(cfg.Dir, name), 0o700); err != nil {
			return nil, fmt.Errorf("prepare OCI metadata store: %w", err)
		}
	}
	if entries, err := os.ReadDir(filepath.Join(cfg.Dir, "staging")); err == nil {
		for _, entry := range entries {
			_ = os.RemoveAll(filepath.Join(cfg.Dir, "staging", entry.Name()))
		}
	}
	store := &Store{
		dir: cfg.Dir, refTTL: cfg.RefTTL, source: source,
		refs: map[string]refRecord{}, indexes: map[string]*objectIndex{},
	}
	if err := store.loadRefs(); err != nil {
		return nil, err
	}
	return store, nil
}

func (s *Store) Open(ctx context.Context, req OpenRequest) (*OCIObject, error) {
	req.Reference = strings.TrimSpace(req.Reference)
	if req.Reference == "" {
		return nil, errors.New("OCI reference is required")
	}
	key := refKey(req)
	if !req.Refresh {
		if object := s.cachedRef(key); object != nil {
			return object, nil
		}
	}
	value, err, _ := s.group.Do("ref:"+key, func() (any, error) {
		if !req.Refresh {
			if object := s.cachedRef(key); object != nil {
				return object, nil
			}
		}
		resolved, err := s.source.Resolve(ctx, req.Reference, ociimage.OpenOptions{Platform: req.Platform, Insecure: req.Insecure})
		if err != nil {
			return nil, err
		}
		if !validDigest(resolved.ManifestDigest) {
			return nil, fmt.Errorf("invalid resolved manifest digest %q", resolved.ManifestDigest)
		}
		if resolved.Kind != ObjectKindImage && resolved.Kind != ObjectKindArtifact {
			return nil, fmt.Errorf("unsupported OCI object kind %q", resolved.Kind)
		}
		objectID := sourceObjectID(req, resolved.ManifestDigest)
		object, err := s.ensureObject(ctx, objectID, req, resolved)
		if err != nil {
			return nil, err
		}
		if err := s.saveRef(key, refRecord{ObjectID: object.ObjectID, ResolvedAt: time.Now().UTC()}); err != nil {
			return nil, err
		}
		return object, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*OCIObject), nil
}

func (s *Store) Object(objectID string) (*OCIObject, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	s.touchObject(objectID)
	object := index.Object
	return &object, nil
}

func (s *Store) List(objectID, inputPath string) (*Directory, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	if index.Object.Kind != ObjectKindImage {
		return nil, ErrNotImage
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

func (s *Store) OpenFile(objectID, inputPath string) (*File, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	if index.Object.Kind != ObjectKindImage {
		return nil, ErrNotImage
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

func (s *Store) OpenFileReader(ctx context.Context, objectID, inputPath string) (*File, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	file, err := s.OpenFile(objectID, inputPath)
	if err != nil {
		return nil, err
	}
	reader, err := s.source.OpenLayer(ctx, index.Object.Reference, objectOpenOptions(index.Object), LayerDescriptor{Digest: file.Entry.LayerDigest})
	if err != nil {
		return nil, err
	}
	opened, err := openFileFromLayer(reader, file.Entry.TarPath, file.Entry.Path)
	if err != nil {
		return nil, err
	}
	opened.Entry = file.Entry
	return opened, nil
}

func (s *Store) OpenFileForRequest(ctx context.Context, objectID, inputPath string, req OpenRequest) (*File, error) {
	file, err := s.OpenFile(objectID, inputPath)
	if err != nil {
		return nil, err
	}
	reader, err := s.source.OpenLayer(ctx, req.Reference, ociimage.OpenOptions{Platform: req.Platform, Insecure: req.Insecure}, LayerDescriptor{Digest: file.Entry.LayerDigest})
	if err != nil {
		return nil, err
	}
	opened, err := openFileFromLayer(reader, file.Entry.TarPath, file.Entry.Path)
	if err != nil {
		return nil, err
	}
	opened.Entry = file.Entry
	return opened, nil
}

func (s *Store) ArtifactBlob(objectID, digest string) (*BlobDescriptor, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	if index.Object.Kind != ObjectKindArtifact {
		return nil, ErrNotArtifact
	}
	for _, blob := range index.Object.Blobs {
		if blob.Digest == digest {
			result := blob
			return &result, nil
		}
	}
	return nil, ErrBlobNotFound
}

func (s *Store) OpenArtifactBlob(ctx context.Context, objectID, digest string) (*Blob, error) {
	index, err := s.loadIndex(objectID)
	if err != nil {
		return nil, err
	}
	descriptor, err := s.ArtifactBlob(objectID, digest)
	if err != nil {
		return nil, err
	}
	reader, err := s.source.OpenBlob(ctx, index.Object.Reference, objectOpenOptions(index.Object), *descriptor)
	if err != nil {
		return nil, err
	}
	return &Blob{Descriptor: *descriptor, Reader: reader}, nil
}

func (s *Store) ensureObject(ctx context.Context, objectID string, req OpenRequest, resolved *ResolvedObject) (*OCIObject, error) {
	if object, err := s.Object(objectID); err == nil {
		return object, nil
	}
	value, err, _ := s.group.Do("object:"+objectID, func() (any, error) {
		if object, err := s.Object(objectID); err == nil {
			return object, nil
		}
		index, err := s.withObjectLock(ctx, objectID, func() (*objectIndex, error) {
			if index, err := s.loadIndex(objectID); err == nil {
				return index, nil
			}
			return s.build(ctx, objectID, req, resolved)
		})
		if err != nil {
			return nil, err
		}
		return &index.Object, nil
	})
	if err != nil {
		return nil, err
	}
	return value.(*OCIObject), nil
}

func (s *Store) build(ctx context.Context, objectID string, req OpenRequest, resolved *ResolvedObject) (*objectIndex, error) {
	stage, err := os.MkdirTemp(filepath.Join(s.dir, "staging"), "object-*")
	if err != nil {
		return nil, fmt.Errorf("create metadata staging directory: %w", err)
	}
	defer os.RemoveAll(stage)
	index := &objectIndex{
		Object: OCIObject{
			ObjectID: objectID, Kind: resolved.Kind, ManifestDigest: resolved.ManifestDigest,
			ManifestMediaType: resolved.ManifestMediaType, Reference: req.Reference,
			Platform: resolved.Platform, Insecure: req.Insecure, ArtifactType: resolved.ArtifactType,
			Annotations: cloneAnnotations(resolved.Annotations), Blobs: cloneBlobs(resolved.Blobs),
			CreatedAt: time.Now().UTC(),
		},
	}
	if resolved.Kind == ObjectKindImage {
		index.Layers = make([]LayerDescriptor, 0, len(resolved.Layers))
		index.Entries = map[string]Entry{"/": {Name: "/", Path: "/", Type: EntryTypeDirectory, Size: -1}}
		for _, layer := range resolved.Layers {
			index.Layers = append(index.Layers, layer.Descriptor)
		}
		if err := applyLayers(ctx, index, resolved.Layers); err != nil {
			return nil, err
		}
	}
	data, err := json.Marshal(index)
	if err != nil {
		return nil, fmt.Errorf("encode OCI metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, indexFilename), data, 0o600); err != nil {
		return nil, fmt.Errorf("write OCI metadata: %w", err)
	}
	if err := os.WriteFile(filepath.Join(stage, ".access"), nil, 0o600); err != nil {
		return nil, fmt.Errorf("write OCI access marker: %w", err)
	}
	final := s.objectDir(objectID)
	if err := os.MkdirAll(filepath.Dir(final), 0o700); err != nil {
		return nil, fmt.Errorf("prepare OCI metadata directory: %w", err)
	}
	if err := os.Rename(stage, final); err != nil {
		if _, statErr := os.Stat(filepath.Join(final, indexFilename)); statErr != nil {
			return nil, fmt.Errorf("publish OCI metadata: %w", err)
		}
	}
	s.mu.Lock()
	s.indexes[objectID] = index
	s.mu.Unlock()
	return index, nil
}

func (s *Store) loadIndex(objectID string) (*objectIndex, error) {
	if !validDigest(objectID) {
		return nil, ErrObjectNotFound
	}
	s.mu.RLock()
	index := s.indexes[objectID]
	s.mu.RUnlock()
	if index != nil {
		return index, nil
	}
	data, err := os.ReadFile(filepath.Join(s.objectDir(objectID), indexFilename))
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, ErrObjectNotFound
		}
		return nil, err
	}
	index = &objectIndex{}
	if err := json.Unmarshal(data, index); err != nil {
		return nil, fmt.Errorf("decode OCI metadata: %w", err)
	}
	if index.Object.Kind != ObjectKindImage && index.Object.Kind != ObjectKindArtifact {
		return nil, fmt.Errorf("decode OCI metadata: invalid kind %q", index.Object.Kind)
	}
	s.mu.Lock()
	if current := s.indexes[objectID]; current != nil {
		index = current
	} else {
		s.indexes[objectID] = index
	}
	s.mu.Unlock()
	return index, nil
}

func (s *Store) cachedRef(key string) *OCIObject {
	s.mu.RLock()
	record, ok := s.refs[key]
	s.mu.RUnlock()
	if !ok || (s.refTTL > 0 && time.Since(record.ResolvedAt) > s.refTTL) {
		return nil
	}
	object, err := s.Object(record.ObjectID)
	if err != nil {
		return nil
	}
	return object
}

func (s *Store) loadRefs() error {
	data, err := os.ReadFile(filepath.Join(s.dir, refsFilename))
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return fmt.Errorf("read OCI refs: %w", err)
	}
	if err := json.Unmarshal(data, &s.refs); err != nil {
		return fmt.Errorf("decode OCI refs: %w", err)
	}
	return nil
}

func (s *Store) saveRef(key string, record refRecord) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.refs[key] = record
	data, err := json.Marshal(s.refs)
	if err != nil {
		return fmt.Errorf("encode OCI refs: %w", err)
	}
	tmp, err := os.CreateTemp(s.dir, ".object-refs-*.tmp")
	if err != nil {
		return fmt.Errorf("create OCI refs file: %w", err)
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("write OCI refs: %w", err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("close OCI refs: %w", err)
	}
	if err := os.Rename(tmpName, filepath.Join(s.dir, refsFilename)); err != nil {
		return fmt.Errorf("publish OCI refs: %w", err)
	}
	return nil
}

func (s *Store) touchObject(objectID string) {
	_ = os.Chtimes(filepath.Join(s.objectDir(objectID), ".access"), time.Now(), time.Now())
}

func (s *Store) objectDir(objectID string) string {
	parts := strings.SplitN(objectID, ":", 2)
	return filepath.Join(s.dir, "objects", parts[0], parts[1])
}

func refKey(req OpenRequest) string {
	return fmt.Sprintf("%t\n%s\n%s", req.Insecure, req.Reference, platformString(req.Platform))
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

func sourceObjectID(req OpenRequest, manifestDigest string) string {
	key := fmt.Sprintf("%t\n%s\n%s\n%s", req.Insecure, req.Reference, platformString(req.Platform), manifestDigest)
	digest := sha256.Sum256([]byte(key))
	return "sha256:" + hex.EncodeToString(digest[:])
}

func validDigest(value string) bool {
	parts := strings.SplitN(value, ":", 2)
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

func objectOpenOptions(object OCIObject) ociimage.OpenOptions {
	options := ociimage.OpenOptions{Insecure: object.Insecure}
	if object.Platform != "" && object.Platform != "default" {
		platform, err := ociimage.ParsePlatform(object.Platform)
		if err == nil {
			options.Platform = &platform
		}
	}
	return options
}

func cloneBlobs(input []BlobDescriptor) []BlobDescriptor {
	if len(input) == 0 {
		return nil
	}
	result := make([]BlobDescriptor, len(input))
	for index, blob := range input {
		result[index] = blob
		result[index].Annotations = cloneAnnotations(blob.Annotations)
	}
	return result
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
