package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

type createImageInput struct {
	Body struct {
		Ref      string `json:"ref" required:"true" doc:"Image reference"`
		Platform string `json:"platform,omitempty" doc:"Platform as os/arch or os/arch/variant"`
		Insecure bool   `json:"insecure,omitempty" doc:"Allow insecure image registry access"`
		Refresh  bool   `json:"refresh,omitempty" doc:"Resolve the mutable reference again"`
	}
}

type createImageOutput struct {
	Body imagestore.Image
}

type listImageEntriesInput struct {
	ImageID string `path:"imageID" doc:"Immutable source-bound metadata image identifier"`
	Path    string `query:"path" default:"/" doc:"Directory path inside the image"`
}

type listImageEntriesOutput struct {
	Body imagestore.Directory
}

type listImagePlatformsInput struct {
	Ref      string `query:"ref" required:"true" doc:"Image reference"`
	Insecure bool   `query:"insecure" doc:"Allow insecure image registry access"`
}

type listImagePlatformsOutput struct {
	Body struct {
		Platforms []ociimage.Platform `json:"platforms"`
	}
}

func registerImageRoutes(api huma.API, images *imagestore.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "create-image",
		Method:      http.MethodPost,
		Path:        "/images",
		Summary:     "Resolve an image and index its filesystem metadata",
		Tags:        []string{"images"},
	}, func(ctx context.Context, input *createImageInput) (*createImageOutput, error) {
		if images == nil {
			return nil, huma.Error503ServiceUnavailable("image store unavailable")
		}
		var platform *v1.Platform
		platformParam := strings.TrimSpace(input.Body.Platform)
		if platformParam != "" {
			parsed, err := ociimage.ParsePlatform(platformParam)
			if err != nil {
				return nil, huma.Error400BadRequest(err.Error())
			}
			platform = &parsed
		}
		image, err := images.Open(ctx, imagestore.OpenRequest{
			ImageRef: input.Body.Ref,
			Platform: platform,
			Insecure: input.Body.Insecure,
			Refresh:  input.Body.Refresh,
		})
		if err != nil {
			return nil, imageStoreAPIError(err)
		}
		return &createImageOutput{Body: *image}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-image-entries",
		Method:      http.MethodGet,
		Path:        "/images/{imageID}/entries",
		Summary:     "List an indexed image directory",
		Tags:        []string{"images"},
	}, func(_ context.Context, input *listImageEntriesInput) (*listImageEntriesOutput, error) {
		if images == nil {
			return nil, huma.Error503ServiceUnavailable("image store unavailable")
		}
		directory, err := images.List(input.ImageID, input.Path)
		if err != nil {
			return nil, imageStoreAPIError(err)
		}
		return &listImageEntriesOutput{Body: *directory}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-image-platforms",
		Method:      http.MethodGet,
		Path:        "/images/platforms",
		Summary:     "List image platforms",
		Tags:        []string{"images"},
	}, func(ctx context.Context, input *listImagePlatformsInput) (*listImagePlatformsOutput, error) {
		extractor := &ociimage.Extractor{}
		platforms, err := extractor.Platforms(ctx, input.Ref, ociimage.OpenOptions{Insecure: input.Insecure})
		if err != nil {
			return nil, huma.Error502BadGateway("failed to read image platforms")
		}
		out := &listImagePlatformsOutput{}
		out.Body.Platforms = platforms
		return out, nil
	})
}

func imageStoreAPIError(err error) error {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		return huma.Error400BadRequest("invalid path")
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrImageNotFound):
		return huma.Error404NotFound("path or image not found")
	case errors.Is(err, imagestore.ErrNotDirectory):
		return huma.Error422UnprocessableEntity("path is not a directory")
	case errors.Is(err, imagestore.ErrNotRegularFile):
		return huma.Error422UnprocessableEntity("path is not a regular file")
	default:
		return huma.Error502BadGateway("failed to index image metadata")
	}
}
