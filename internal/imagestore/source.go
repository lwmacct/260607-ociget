package imagestore

import (
	"context"
	"fmt"
	"io"

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
		ImageID:  digest.String(),
		Platform: platformString(opts.Platform),
		Layers:   make([]Layer, 0, len(layers)),
	}
	for _, layer := range layers {
		result.Layers = append(result.Layers, imageLayer{layer: layer})
	}
	return result, nil
}

type imageLayer struct {
	layer v1.Layer
}

func (l imageLayer) Open() (io.ReadCloser, error) {
	return l.layer.Uncompressed()
}
