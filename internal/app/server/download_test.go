package server

import (
	"net/http"
	"net/http/httptest"
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
