package imagestore

import (
	"context"
	"fmt"
	"io"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const artifactTitleAnnotation = "org.opencontainers.image.title"

type remoteSource struct {
	extractor ociimage.Extractor
}

func (s *remoteSource) Resolve(ctx context.Context, reference string, opts ociimage.OpenOptions) (*ResolvedObject, error) {
	img, err := s.extractor.Image(ctx, reference, opts)
	if err != nil {
		return nil, fmt.Errorf("pull OCI metadata: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("read manifest digest: %w", err)
	}
	manifest, err := img.Manifest()
	if err != nil {
		return nil, fmt.Errorf("read manifest: %w", err)
	}
	mediaType := manifest.MediaType
	if mediaType == "" {
		mediaType, err = img.MediaType()
		if err != nil {
			return nil, fmt.Errorf("read manifest media type: %w", err)
		}
	}

	resolved := &ResolvedObject{
		Kind:              classifyManifest(manifest),
		ManifestDigest:    digest.String(),
		ManifestMediaType: string(mediaType),
		Annotations:       cloneAnnotations(manifest.Annotations),
	}
	if resolved.Kind == ObjectKindArtifact {
		resolved.ArtifactType = strings.TrimSpace(manifest.ArtifactType)
		if resolved.ArtifactType == "" {
			resolved.ArtifactType = string(manifest.Config.MediaType)
		}
		resolved.Blobs = artifactBlobs(manifest.Layers)
		return resolved, nil
	}

	resolved.Platform = imagePlatformString(opts.Platform)
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("read image layers: %w", err)
	}
	resolved.Layers = make([]ResolvedLayer, 0, len(layers))
	for _, layer := range layers {
		layerDigest, err := layer.Digest()
		if err != nil {
			return nil, fmt.Errorf("read layer digest: %w", err)
		}
		layerMediaType, err := layer.MediaType()
		if err != nil {
			return nil, fmt.Errorf("read layer media type: %w", err)
		}
		layerSize, err := layer.Size()
		if err != nil {
			return nil, fmt.Errorf("read layer size: %w", err)
		}
		resolved.Layers = append(resolved.Layers, ResolvedLayer{
			Descriptor: LayerDescriptor{Digest: layerDigest.String(), MediaType: string(layerMediaType), Size: layerSize},
			Layer:      remoteLayer{layer: layer},
		})
	}
	return resolved, nil
}

func (s *remoteSource) OpenLayer(ctx context.Context, reference string, opts ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error) {
	digest, err := parseDigest(descriptor.Digest)
	if err != nil {
		return nil, fmt.Errorf("parse layer digest: %w", err)
	}
	return s.extractor.OpenLayer(ctx, reference, opts, digest)
}

func (s *remoteSource) OpenBlob(ctx context.Context, reference string, opts ociimage.OpenOptions, descriptor BlobDescriptor) (io.ReadCloser, error) {
	digest, err := parseDigest(descriptor.Digest)
	if err != nil {
		return nil, fmt.Errorf("parse blob digest: %w", err)
	}
	return s.extractor.OpenBlob(ctx, reference, opts, digest)
}

type remoteLayer struct {
	layer v1.Layer
}

func (l remoteLayer) Open() (io.ReadCloser, error) {
	return l.layer.Uncompressed()
}

func classifyManifest(manifest *v1.Manifest) ObjectKind {
	if strings.TrimSpace(manifest.ArtifactType) != "" {
		return ObjectKindArtifact
	}
	switch manifest.MediaType {
	case types.DockerManifestSchema1, types.DockerManifestSchema1Signed:
		return ObjectKindImage
	}
	switch manifest.Config.MediaType {
	case types.OCIConfigJSON, types.DockerConfigJSON:
		return ObjectKindImage
	default:
		return ObjectKindArtifact
	}
}

func artifactBlobs(descriptors []v1.Descriptor) []BlobDescriptor {
	blobs := make([]BlobDescriptor, 0, len(descriptors))
	for _, descriptor := range descriptors {
		name := strings.TrimSpace(descriptor.Annotations[artifactTitleAnnotation])
		if name == "" {
			name = descriptor.Digest.String()
		}
		blobs = append(blobs, BlobDescriptor{
			Digest:      descriptor.Digest.String(),
			MediaType:   string(descriptor.MediaType),
			Size:        descriptor.Size,
			Name:        name,
			Annotations: cloneAnnotations(descriptor.Annotations),
		})
	}
	return blobs
}

func cloneAnnotations(input map[string]string) map[string]string {
	if len(input) == 0 {
		return nil
	}
	result := make(map[string]string, len(input))
	for key, value := range input {
		result[key] = value
	}
	return result
}

func parseDigest(value string) (v1.Hash, error) {
	var digest v1.Hash
	if err := digest.UnmarshalText([]byte(value)); err != nil {
		return v1.Hash{}, err
	}
	return digest, nil
}

func imagePlatformString(platform *v1.Platform) string {
	if platform == nil {
		return "default"
	}
	parts := []string{platform.OS, platform.Architecture}
	if platform.Variant != "" {
		parts = append(parts, platform.Variant)
	}
	return strings.Join(parts, "/")
}
