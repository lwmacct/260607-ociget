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
