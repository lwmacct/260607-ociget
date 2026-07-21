package server

import (
	"context"
	"errors"
	"net/http"
	"strings"

	"github.com/danielgtaylor/huma/v2"
	"github.com/lwmacct/260607-ociget/internal/download"
	"github.com/lwmacct/260607-ociget/internal/imagebrowser"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

type listImageFilesInput struct {
	Ref      string `query:"ref" required:"true" doc:"Image reference"`
	Path     string `query:"path" default:"/" doc:"Directory path inside the image"`
	Platform string `query:"platform" doc:"Platform as os/arch or os/arch/variant"`
	Insecure bool   `query:"insecure" doc:"Allow insecure image registry access"`
}

type listImageFilesOutput struct {
	Body imagebrowser.Directory
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

func registerImageRoutes(api huma.API, browser *imagebrowser.Browser) {
	huma.Register(api, huma.Operation{
		OperationID: "list-image-files",
		Method:      http.MethodGet,
		Path:        "/images/files",
		Summary:     "List image directory entries",
		Tags:        []string{"images"},
	}, func(ctx context.Context, input *listImageFilesInput) (*listImageFilesOutput, error) {
		if browser == nil {
			browser = &imagebrowser.Browser{}
		}

		opts := imagebrowser.Options{Insecure: input.Insecure}
		platformParam := strings.TrimSpace(input.Platform)
		if platformParam != "" {
			platform, err := download.ParsePlatform(platformParam)
			if err != nil {
				return nil, huma.Error400BadRequest(err.Error())
			}
			opts.Platform = &platform
		}

		dir, err := browser.List(ctx, imagebrowser.ListRequest{
			ImageRef: input.Ref,
			Path:     input.Path,
			Options:  opts,
		})
		if err != nil {
			return nil, imageBrowserAPIError(err)
		}

		out := &listImageFilesOutput{}
		out.Body = *dir
		return out, nil
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

func imageBrowserAPIError(err error) error {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		return huma.Error400BadRequest("invalid path")
	case errors.Is(err, ociimage.ErrNotFound):
		return huma.Error404NotFound("path not found")
	case errors.Is(err, imagebrowser.ErrNotDirectory):
		return huma.Error422UnprocessableEntity("path is not a directory")
	default:
		return huma.Error502BadGateway("failed to read image")
	}
}
