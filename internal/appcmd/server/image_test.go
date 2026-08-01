package server

import (
	"errors"
	"net/http"
	"testing"

	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func TestImageStoreAPIError(t *testing.T) {
	tests := []struct {
		err    error
		status int
	}{
		{err: ociimage.ErrInvalidPath, status: http.StatusBadRequest},
		{err: ociimage.ErrNotFound, status: http.StatusNotFound},
		{err: imagestore.ErrImageNotFound, status: http.StatusNotFound},
		{err: imagestore.ErrNotDirectory, status: http.StatusUnprocessableEntity},
		{err: errors.New("registry failed"), status: http.StatusBadGateway},
	}
	for _, test := range tests {
		err := imageStoreAPIError(test.err)
		statusErr, ok := err.(interface{ GetStatus() int })
		if !ok {
			t.Fatalf("imageStoreAPIError() = %T", err)
		}
		if statusErr.GetStatus() != test.status {
			t.Fatalf("status = %d, want %d", statusErr.GetStatus(), test.status)
		}
	}
}
