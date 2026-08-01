package imagestore

import (
	"context"
	"fmt"
	"io"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

type remoteSource struct {
	extractor ociimage.Extractor
}

func (s *remoteSource) Resolve(ctx context.Context, imageRef string, opts ociimage.OpenOptions) (*ResolvedImage, error) {
	img, err := s.extractor.Image(ctx, imageRef, opts)
	if err != nil {
		return nil, fmt.Errorf("pull image metadata: %w", err)
	}
	digest, err := img.Digest()
	if err != nil {
		return nil, fmt.Errorf("read image digest: %w", err)
	}
	layers, err := img.Layers()
	if err != nil {
		return nil, fmt.Errorf("read image layers: %w", err)
	}
	result := &ResolvedImage{
		ManifestDigest: digest.String(),
		Platform:       imagePlatformString(opts.Platform),
		Layers:         make([]ResolvedLayer, 0, len(layers)),
	}
	for _, layer := range layers {
		layerDigest, err := layer.Digest()
		if err != nil {
			return nil, fmt.Errorf("read layer digest: %w", err)
		}
		mediaType, err := layer.MediaType()
		if err != nil {
			return nil, fmt.Errorf("read layer media type: %w", err)
		}
		layerSize, err := layer.Size()
		if err != nil {
			return nil, fmt.Errorf("read layer size: %w", err)
		}
		result.Layers = append(result.Layers, ResolvedLayer{
			Descriptor: LayerDescriptor{Digest: layerDigest.String(), MediaType: string(mediaType), Size: layerSize},
			Layer:      remoteLayer{layer: layer},
		})
	}
	return result, nil
}

func (s *remoteSource) OpenLayer(ctx context.Context, imageRef, manifestDigest string, opts ociimage.OpenOptions, descriptor LayerDescriptor) (io.ReadCloser, error) {
	var digest v1.Hash
	if err := digest.UnmarshalText([]byte(descriptor.Digest)); err != nil {
		return nil, fmt.Errorf("parse layer digest: %w", err)
	}
	return s.extractor.OpenLayer(ctx, imageRef, opts, digest)
}

type remoteLayer struct {
	layer v1.Layer
}

func (l remoteLayer) Open() (io.ReadCloser, error) {
	return l.layer.Uncompressed()
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
