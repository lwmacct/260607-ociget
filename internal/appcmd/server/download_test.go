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
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lwmacct/260607-ociget/internal/config"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const routeTestManifestDigest = "sha256:0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"

func TestImageFileSupportsRanges(t *testing.T) {
	handler, objectID := imageRouteTestHandler(t)
	request := httptest.NewRequest(
		http.MethodGet,
		"/api/objects/"+escapePathSegment(objectID)+"/file?path="+url.QueryEscape("/usr/bin/app"),
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
	handler, objectID := imageRouteTestHandler(t)
	body, _ := json.Marshal(imageArchiveInput{Paths: []string{"/usr/bin/app"}})
	recorder := httptest.NewRecorder()

	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodPost,
		"/api/objects/"+escapePathSegment(objectID)+"/archive",
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

func TestArtifactBlobDownloadsOriginalBytes(t *testing.T) {
	handler, objectID, digest := artifactRouteTestHandler(t)
	request := httptest.NewRequest(http.MethodGet, "/api/objects/"+escapePathSegment(objectID)+"/blobs/"+escapePathSegment(digest), nil)
	request.Header.Set("Range", "bytes=1-3")
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, request)

	if recorder.Code != http.StatusPartialContent {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
	if recorder.Body.String() != "aw-" {
		t.Fatalf("body = %q, want raw-", recorder.Body.String())
	}
	if got := recorder.Header().Get("Content-Type"); got != "application/octet-stream" {
		t.Fatalf("Content-Type = %q", got)
	}
	if got := recorder.Header().Get("Content-Disposition"); got != `attachment; filename=rtty` {
		t.Fatalf("Content-Disposition = %q", got)
	}
	if got := recorder.Header().Get("ETag"); got != `"`+digest+`"` {
		t.Fatalf("ETag = %q", got)
	}
}

func TestArtifactBlobRejectsDigestOutsideManifest(t *testing.T) {
	handler, objectID, _ := artifactRouteTestHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(
		http.MethodGet,
		"/api/objects/"+escapePathSegment(objectID)+"/blobs/sha256%3Affffffffffffffffffffffffffffffffffffffffffffffffffffffffffffffff",
		nil,
	))
	if recorder.Code != http.StatusNotFound {
		t.Fatalf("status = %d, body = %s", recorder.Code, recorder.Body.String())
	}
}

func TestArtifactBlobHeadReturnsMetadataOnly(t *testing.T) {
	handler, objectID, digest := artifactRouteTestHandler(t)
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodHead, "/api/objects/"+escapePathSegment(objectID)+"/blobs/"+escapePathSegment(digest), nil))
	if recorder.Code != http.StatusOK || recorder.Body.Len() != 0 {
		t.Fatalf("status = %d, body = %q", recorder.Code, recorder.Body.String())
	}
	if recorder.Header().Get("Content-Length") != "11" {
		t.Fatalf("Content-Length = %q", recorder.Header().Get("Content-Length"))
	}
}

func TestArtifactFilenameIsSafeForContentDisposition(t *testing.T) {
	name := headerFilename("payloads/evil\r\n\"\\name")
	if strings.ContainsAny(name, "\r\n\"\\/") {
		t.Fatalf("unsafe filename = %q", name)
	}
	disposition := attachmentDisposition("payloads/工具")
	if !strings.HasPrefix(disposition, "attachment;") || strings.ContainsAny(disposition, "\r\n") {
		t.Fatalf("unsafe disposition = %q", disposition)
	}
}

func imageRouteTestHandler(t *testing.T) (http.Handler, string) {
	t.Helper()
	layer := routeTestTar(t)
	source := &routeTestSource{
		resolved: &imagestore.ResolvedObject{
			Kind: imagestore.ObjectKindImage, ManifestDigest: routeTestManifestDigest,
			ManifestMediaType: "application/vnd.oci.image.manifest.v1+json", Platform: "linux/amd64",
			Layers: []imagestore.ResolvedLayer{{
				Descriptor: imagestore.LayerDescriptor{Digest: "sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"},
				Layer:      routeTestLayer(layer),
			}},
		},
		layers: map[string][]byte{"sha256:aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa": layer},
	}
	return routeTestHandler(t, source, "example.test/tool:v1")
}

func artifactRouteTestHandler(t *testing.T) (http.Handler, string, string) {
	t.Helper()
	digest := "sha256:bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"
	payload := []byte("raw-payload")
	source := &routeTestSource{
		resolved: &imagestore.ResolvedObject{
			Kind: imagestore.ObjectKindArtifact, ManifestDigest: routeTestManifestDigest,
			ManifestMediaType: "application/vnd.oci.image.manifest.v1+json",
			ArtifactType:      "application/vnd.example.binary",
			Blobs: []imagestore.BlobDescriptor{{
				Digest: digest, MediaType: "application/octet-stream", Size: int64(len(payload)), Name: "rtty",
			}},
		},
		blobs: map[string][]byte{digest: payload},
	}
	handler, objectID := routeTestHandler(t, source, "example.test/tool:artifact")
	return handler, objectID, digest
}

func routeTestHandler(t *testing.T, source imagestore.Source, reference string) (http.Handler, string) {
	t.Helper()
	store, err := imagestore.NewWithSource(imagestore.Config{Dir: t.TempDir(), RefTTL: time.Hour}, source)
	if err != nil {
		t.Fatal(err)
	}
	object, err := store.Open(context.Background(), imagestore.OpenRequest{Reference: reference})
	if err != nil {
		t.Fatal(err)
	}
	serverConfig := config.DefaultConfig().Server
	return newHTTPHandler(&serverConfig, &runtime{objects: store}), object.ObjectID
}

type routeTestSource struct {
	resolved *imagestore.ResolvedObject
	layers   map[string][]byte
	blobs    map[string][]byte
}

func (source *routeTestSource) Resolve(context.Context, string, ociimage.OpenOptions) (*imagestore.ResolvedObject, error) {
	return source.resolved, nil
}

func (source *routeTestSource) OpenLayer(_ context.Context, _ string, _ ociimage.OpenOptions, descriptor imagestore.LayerDescriptor) (io.ReadCloser, error) {
	data, ok := source.layers[descriptor.Digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func (source *routeTestSource) OpenBlob(_ context.Context, _ string, _ ociimage.OpenOptions, descriptor imagestore.BlobDescriptor) (io.ReadCloser, error) {
	data, ok := source.blobs[descriptor.Digest]
	if !ok {
		return nil, os.ErrNotExist
	}
	return io.NopCloser(bytes.NewReader(data)), nil
}

func routeTestTar(t *testing.T) []byte {
	t.Helper()
	var buffer bytes.Buffer
	writer := tar.NewWriter(&buffer)
	payload := []byte("payload")
	if err := writer.WriteHeader(&tar.Header{Name: "usr/bin/app", Mode: 0o755, Size: int64(len(payload)), Typeflag: tar.TypeReg}); err != nil {
		t.Fatal(err)
	}
	if _, err := writer.Write(payload); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

type routeTestLayer []byte

func (layer routeTestLayer) Open() (io.ReadCloser, error) {
	return io.NopCloser(bytes.NewReader(layer)), nil
}

func escapePathSegment(value string) string {
	return strings.ReplaceAll(url.PathEscape(value), ":", "%3A")
}
