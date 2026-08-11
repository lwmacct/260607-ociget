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

type createObjectInput struct {
	Body struct {
		Ref      string `json:"ref" required:"true" doc:"OCI image or artifact reference"`
		Platform string `json:"platform,omitempty" doc:"Platform as os/arch or os/arch/variant"`
		Insecure bool   `json:"insecure,omitempty" doc:"Allow insecure registry access"`
		Refresh  bool   `json:"refresh,omitempty" doc:"Resolve the mutable reference again"`
	}
}

type createObjectOutput struct {
	Body imagestore.OCIObject
}

type listObjectEntriesInput struct {
	ObjectID string `path:"objectID" doc:"Immutable OCI object metadata identifier"`
	Path     string `query:"path" default:"/" doc:"Directory path inside the image"`
}

type listObjectEntriesOutput struct {
	Body imagestore.Directory
}

type listObjectPlatformsInput struct {
	Ref      string `query:"ref" required:"true" doc:"OCI image or artifact reference"`
	Insecure bool   `query:"insecure" doc:"Allow insecure registry access"`
}

type listObjectPlatformsOutput struct {
	Body struct {
		Platforms []ociimage.Platform `json:"platforms"`
	}
}

func registerImageRoutes(api huma.API, objects *imagestore.Store) {
	huma.Register(api, huma.Operation{
		OperationID: "create-object",
		Method:      http.MethodPost,
		Path:        "/objects",
		Summary:     "Resolve an OCI image or artifact",
		Tags:        []string{"objects"},
	}, func(ctx context.Context, input *createObjectInput) (*createObjectOutput, error) {
		if objects == nil {
			return nil, huma.Error503ServiceUnavailable("OCI object store unavailable")
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
		object, err := objects.Open(ctx, imagestore.OpenRequest{
			Reference: input.Body.Ref,
			Platform:  platform,
			Insecure:  input.Body.Insecure,
			Refresh:   input.Body.Refresh,
		})
		if err != nil {
			return nil, objectStoreAPIError(err)
		}
		return &createObjectOutput{Body: *object}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-object-entries",
		Method:      http.MethodGet,
		Path:        "/objects/{objectID}/entries",
		Summary:     "List an indexed image directory",
		Tags:        []string{"objects"},
	}, func(_ context.Context, input *listObjectEntriesInput) (*listObjectEntriesOutput, error) {
		if objects == nil {
			return nil, huma.Error503ServiceUnavailable("OCI object store unavailable")
		}
		directory, err := objects.List(input.ObjectID, input.Path)
		if err != nil {
			return nil, objectStoreAPIError(err)
		}
		return &listObjectEntriesOutput{Body: *directory}, nil
	})

	huma.Register(api, huma.Operation{
		OperationID: "list-object-platforms",
		Method:      http.MethodGet,
		Path:        "/objects/platforms",
		Summary:     "List OCI image platforms",
		Tags:        []string{"objects"},
	}, func(ctx context.Context, input *listObjectPlatformsInput) (*listObjectPlatformsOutput, error) {
		extractor := &ociimage.Extractor{}
		platforms, err := extractor.Platforms(ctx, input.Ref, ociimage.OpenOptions{Insecure: input.Insecure})
		if err != nil {
			return nil, huma.Error502BadGateway("failed to read OCI object platforms")
		}
		out := &listObjectPlatformsOutput{}
		out.Body.Platforms = platforms
		return out, nil
	})
}

func objectStoreAPIError(err error) error {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		return huma.Error400BadRequest("invalid path")
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrObjectNotFound), errors.Is(err, imagestore.ErrBlobNotFound):
		return huma.Error404NotFound("OCI object or path not found")
	case errors.Is(err, imagestore.ErrNotImage):
		return huma.Error422UnprocessableEntity("OCI artifact does not contain an image filesystem")
	case errors.Is(err, imagestore.ErrNotDirectory):
		return huma.Error422UnprocessableEntity("path is not a directory")
	case errors.Is(err, imagestore.ErrNotRegularFile):
		return huma.Error422UnprocessableEntity("path is not a regular file")
	default:
		return huma.Error502BadGateway("failed to resolve OCI object")
	}
}
