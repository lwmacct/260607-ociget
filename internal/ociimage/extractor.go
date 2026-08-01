package ociimage

import (
	"archive/tar"
	"context"
	"errors"
	"fmt"
	"io"
	"path"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/google/go-containerregistry/pkg/name"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/remote"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

var (
	ErrNotFound            = errors.New("file not found in image")
	ErrUnsupportedFileType = errors.New("image path is not a regular file")
)

type Extractor struct{}

type OpenOptions struct {
	Platform *v1.Platform
	Insecure bool
}

type Platform struct {
	OS           string `json:"os"`
	Architecture string `json:"architecture"`
	Variant      string `json:"variant,omitempty"`
	OSVersion    string `json:"osVersion,omitempty"`
}

func ParsePlatform(input string) (v1.Platform, error) {
	parts := strings.Split(strings.TrimSpace(input), "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return v1.Platform{}, fmt.Errorf("platform must be os/arch or os/arch/variant")
	}
	platform := v1.Platform{OS: parts[0], Architecture: parts[1]}
	if len(parts) == 3 {
		if parts[2] == "" {
			return v1.Platform{}, fmt.Errorf("platform variant is empty")
		}
		platform.Variant = parts[2]
	}
	return platform, nil
}

func (p Platform) String() string {
	parts := []string{p.OS, p.Architecture}
	if p.OS == "" || p.Architecture == "" {
		return ""
	}
	if p.Variant != "" {
		parts = append(parts, p.Variant)
	}
	return strings.Join(parts, "/")
}

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

func (e *Extractor) Image(ctx context.Context, imageRef string, opts OpenOptions) (v1.Image, error) {
	return e.remoteImage(ctx, imageRef, opts)
}

func (e *Extractor) OpenLayer(ctx context.Context, imageRef string, opts OpenOptions, digest v1.Hash) (io.ReadCloser, error) {
	ref, remoteOpts, err := remoteReferenceOptions(ctx, imageRef, opts, opts.Insecure)
	if err != nil {
		return nil, err
	}
	digestRef, err := name.NewDigest(ref.Context().Name()+"@"+digest.String(), nameOptions(opts.Insecure)...)
	if err != nil {
		return nil, fmt.Errorf("parse layer digest: %w", err)
	}
	layer, err := remote.Layer(digestRef, remoteOpts...)
	if err != nil {
		return nil, err
	}
	return layer.Uncompressed()
}

func (e *Extractor) Platforms(ctx context.Context, imageRef string, opts OpenOptions) ([]Platform, error) {
	desc, err := e.remoteDescriptor(ctx, imageRef, opts)
	if err != nil {
		return nil, fmt.Errorf("pull image descriptor: %w", err)
	}
	switch desc.MediaType {
	case types.OCIManifestSchema1, types.DockerManifestSchema2, types.DockerManifestSchema1, types.DockerManifestSchema1Signed:
		return []Platform{}, nil
	}

	index, err := desc.ImageIndex()
	if err != nil {
		return nil, fmt.Errorf("read image index descriptor: %w", err)
	}
	manifest, err := index.IndexManifest()
	if err != nil {
		return nil, fmt.Errorf("read image index: %w", err)
	}
	return platformsFromManifest(manifest), nil
}

func platformsFromManifest(manifest *v1.IndexManifest) []Platform {
	platforms := make([]Platform, 0, len(manifest.Manifests))
	seen := map[string]struct{}{}
	for _, desc := range manifest.Manifests {
		if desc.Platform == nil {
			continue
		}
		platform := Platform{
			OS:           desc.Platform.OS,
			Architecture: desc.Platform.Architecture,
			Variant:      desc.Platform.Variant,
			OSVersion:    desc.Platform.OSVersion,
		}
		key := platform.String()
		if key == "" || key == "unknown/unknown" {
			continue
		}
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		platforms = append(platforms, platform)
	}
	return platforms
}

func (e *Extractor) OpenFile(ctx context.Context, imageRef, filePath string, opts OpenOptions) (*File, error) {
	target, err := NormalizePath(filePath)
	if err != nil {
		return nil, err
	}

	img, err := e.remoteImage(ctx, imageRef, opts)
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

func (e *Extractor) ImageDigest(ctx context.Context, imageRef string, opts OpenOptions) (v1.Hash, error) {
	img, err := e.remoteImage(ctx, imageRef, opts)
	if err != nil {
		return v1.Hash{}, fmt.Errorf("pull image metadata: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return v1.Hash{}, fmt.Errorf("read image digest: %w", err)
	}
	return digest, nil
}

func (e *Extractor) remoteImage(ctx context.Context, imageRef string, opts OpenOptions) (v1.Image, error) {
	img, err := pullRemoteImage(ctx, imageRef, opts, false)
	if err == nil || opts.Insecure || !shouldRetryInsecure(err) {
		return img, err
	}
	opts.Insecure = true
	return pullRemoteImage(ctx, imageRef, opts, true)
}

func (e *Extractor) remoteDescriptor(ctx context.Context, imageRef string, opts OpenOptions) (*remote.Descriptor, error) {
	desc, err := pullRemoteDescriptor(ctx, imageRef, opts, false)
	if err == nil || opts.Insecure || !shouldRetryInsecure(err) {
		return desc, err
	}
	opts.Insecure = true
	return pullRemoteDescriptor(ctx, imageRef, opts, true)
}

func pullRemoteImage(ctx context.Context, imageRef string, opts OpenOptions, insecure bool) (v1.Image, error) {
	ref, remoteOpts, err := remoteReferenceOptions(ctx, imageRef, opts, insecure)
	if err != nil {
		return nil, err
	}
	return remote.Image(ref, remoteOpts...)
}

func nameOptions(insecure bool) []name.Option {
	if insecure {
		return []name.Option{name.Insecure}
	}
	return nil
}

func pullRemoteDescriptor(ctx context.Context, imageRef string, opts OpenOptions, insecure bool) (*remote.Descriptor, error) {
	ref, remoteOpts, err := remoteReferenceOptions(ctx, imageRef, opts, insecure)
	if err != nil {
		return nil, err
	}
	return remote.Get(ref, remoteOpts...)
}

func remoteReferenceOptions(ctx context.Context, imageRef string, opts OpenOptions, insecure bool) (name.Reference, []remote.Option, error) {
	nameOpts := []name.Option(nil)
	if insecure {
		nameOpts = append(nameOpts, name.Insecure)
	}

	ref, err := name.ParseReference(imageRef, nameOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("parse image reference: %w", err)
	}

	remoteOpts := []remote.Option{
		remote.WithAuthFromKeychain(authn.DefaultKeychain),
		remote.WithContext(ctx),
		remote.WithTransport(realmFixTransport{base: remote.DefaultTransport}),
	}
	if opts.Platform != nil {
		remoteOpts = append(remoteOpts, remote.WithPlatform(*opts.Platform))
	}

	return ref, remoteOpts, nil
}

func shouldRetryInsecure(err error) bool {
	msg := err.Error()
	return strings.Contains(msg, "invalid realm in www-authenticate") &&
		strings.Contains(msg, `realm scheme "http" not allowed`)
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
