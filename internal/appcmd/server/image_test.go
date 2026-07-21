package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/danielgtaylor/huma/v2"
	"github.com/lwmacct/260607-ociget/internal/imagebrowser"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func TestImageBrowserAPIError(t *testing.T) {
	tests := []struct {
		name   string
		err    error
		status int
	}{
		{name: "invalid path", err: ociimage.ErrInvalidPath, status: http.StatusBadRequest},
		{name: "not found", err: ociimage.ErrNotFound, status: http.StatusNotFound},
		{name: "not directory", err: imagebrowser.ErrNotDirectory, status: http.StatusUnprocessableEntity},
		{name: "upstream", err: errors.New("registry failed"), status: http.StatusBadGateway},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := imageBrowserAPIError(tt.err)
			statusErr, ok := err.(huma.StatusError)
			if !ok {
				t.Fatalf("imageBrowserAPIError() = %T, want huma.StatusError", err)
			}
			if statusErr.GetStatus() != tt.status {
				t.Fatalf("status = %d, want %d", statusErr.GetStatus(), tt.status)
			}
		})
	}
}
