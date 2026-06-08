package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleDownloadWithoutCacheWritesOpenError(t *testing.T) {
	app := &serverApp{}
	req := httptest.NewRequest(http.MethodGet, "/download/!!!/-/etc/passwd", nil)
	rec := httptest.NewRecorder()

	app.handleDownload(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}

func TestHandleDownloadArchiveRejectsMissingTarget(t *testing.T) {
	app := &serverApp{}
	req := httptest.NewRequest(http.MethodPost, "/download/archive", strings.NewReader(`{"ref":"alpine:latest"}`))
	rec := httptest.NewRecorder()

	app.handleDownloadArchive(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadRequest)
	}
}

func TestArchiveOptionsParsesPlatform(t *testing.T) {
	opts, err := archiveOptions(downloadArchiveInput{
		Platform: "linux/arm64/v8",
		Insecure: true,
	})
	if err != nil {
		t.Fatalf("archiveOptions() unexpected error: %v", err)
	}
	if opts.Platform == nil || opts.Platform.OS != "linux" || opts.Platform.Architecture != "arm64" || opts.Platform.Variant != "v8" {
		t.Fatalf("platform = %#v, want linux/arm64/v8", opts.Platform)
	}
	if !opts.Insecure {
		t.Fatalf("insecure = false, want true")
	}
}

func TestDownloadTargetFromQuery(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/download?ref=alpine:latest&path=/etc/alpine-release", nil)

	imageRef, filePath, err := downloadTarget(req)
	if err != nil {
		t.Fatalf("downloadTarget() unexpected error: %v", err)
	}
	if imageRef != "alpine:latest" || filePath != "/etc/alpine-release" {
		t.Fatalf("downloadTarget() = (%q, %q), want (%q, %q)",
			imageRef, filePath, "alpine:latest", "/etc/alpine-release",
		)
	}
}

func TestDownloadTargetFromPath(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/download/alpine:latest/-/etc/alpine-release", nil)

	imageRef, filePath, err := downloadTarget(req)
	if err != nil {
		t.Fatalf("downloadTarget() unexpected error: %v", err)
	}
	if imageRef != "alpine:latest" || filePath != "etc/alpine-release" {
		t.Fatalf("downloadTarget() = (%q, %q), want (%q, %q)",
			imageRef, filePath, "alpine:latest", "etc/alpine-release",
		)
	}
}
