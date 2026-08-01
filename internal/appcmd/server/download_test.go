package server

import (
	"archive/tar"
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const routeTestImageID = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestImageFileSupportsRanges(t *testing.T) {
	handler, imageID := imageRouteTestHandler(t)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/images/"+imageID+"/file?path="+url.QueryEscape("/usr/bin/app"),
		nil,
	)
	request.Header.Set("Range", "bytes=1-3")
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, want %d", recorder.Code, http.StatusPartialContent)
	}
	if recorder.Body.String() != "ayl" {
		t.Fatalf("body = %q, want ayl", recorder.Body.String())
	}
	if recorder.Header().Get("ETag") == "" {
		t.Fatal("ETag is missing")
	}
}

func TestImageArchiveReadsMaterializedFiles(t *testing.T) {
	handler, imageID := imageRouteTestHandler(t)
	body, _ := json.Marshal(imageArchiveInput{Paths: []string{"/usr/bin/app"}})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPost,
		"/api/images/"+imageID+"/archive",
		bytes.NewReader(body),
	))

	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	reader := tar.NewReader(bytes.NewReader(recorder.Body.Bytes()))
	header, err := reader.Next()
	if err != nil {
		t.Fatal(err)
	}
	if header.Name != "usr/bin/app" {
		t.Fatalf("archive name = %q", header.Name)
	}
	payload, _ := io.ReadAll(reader)
	if string(payload) != "payload" {
		t.Fatalf("archive payload = %q", payload)
	}
}

func TestImagePathDownload(t *testing.T) {
	handler, _ := imageRouteTestHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"/example.test/tool:v1/-/usr/bin/app?platform=linux%2Famd64",
		nil,
	))
	if recorder.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "payload" {
		t.Fatalf("body = %q, want payload", recorder.Body.String())
	}
}

func imageRouteTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	store, err := imagestore.NewWithSource(imagestore.Config{
		Dir: t.TempDir(), RefTTL: time.Hour,
	}, routeTestSource{})
	if err != nil {
		t.Fatal(err)
	}
	image, err := store.Open(context.Background(), imagestore.OpenRequest{ImageRef: "example.test/tool:v1"})
	if err != nil {
		t.Fatal(err)
	}
	config := config.DefaultConfig().Server
	return newHTTPHandler(&config, &runtime{images: store}), image.ImageID
}

type routeTestSource struct{}

func (routeTestSource) Resolve(context.Context, string, ociimage.OpenOptions) (*imagestore.ResolvedImage, error) {
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	payload := []byte("payload")
	if err := writer.WriteHeader(&tar.Header{
		Name: "usr/bin/app", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg,
	}); err != nil {
		return nil, err
	}
	if _, err := writer.Write(payload); err != nil {
		return nil, err
	}
	if err := writer.Close(); err != nil {
		return nil, err
	}
	return &imagestore.ResolvedImage{
		ImageID:  routeTestImageID,
		Platform: "linux/amd64",
		Layers:   []imagestore.Layer{routeTestLayer(buffer.Bytes())},
	}, nil
}

type routeTestLayer []byte

func (layer routeTestLayer) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(layer)), nil
}
