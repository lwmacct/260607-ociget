package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestParseDownloadPath(t *testing.T) {
	tests := []struct {
		name      string
		path      string
		wantImage string
		wantPath  string
		wantErr   bool
	}{
		{
			name:      "ghcr image",
			path:      "/download/ghcr.io/lwmacct/260606-sshrt:v0.129.260607/-/usr/local/bin/app",
			wantImage: "ghcr.io/lwmacct/260606-sshrt:v0.129.260607",
			wantPath:  "usr/local/bin/app",
		},
		{
			name:      "registry proxy image",
			path:      "/download/1181.s.kuaicdn.cn:11818/ghcr.io/lwmacct/260606-sshrt:v0.129.260607/-/usr/local/bin/app",
			wantImage: "1181.s.kuaicdn.cn:11818/ghcr.io/lwmacct/260606-sshrt:v0.129.260607",
			wantPath:  "usr/local/bin/app",
		},
		{
			name:      "digest image",
			path:      "/download/alpine@sha256:abc123/-/etc/alpine-release",
			wantImage: "alpine@sha256:abc123",
			wantPath:  "etc/alpine-release",
		},
		{
			name:    "missing separator",
			path:    "/download/ghcr.io/lwmacct/image:tag/usr/local/bin/app",
			wantErr: true,
		},
		{
			name:    "empty image",
			path:    "/download/-/usr/local/bin/app",
			wantErr: true,
		},
		{
			name:    "empty path",
			path:    "/download/ghcr.io/lwmacct/image:tag/-/",
			wantErr: true,
		},
		{
			name:    "wrong prefix",
			path:    "/files/ghcr.io/lwmacct/image:tag/-/usr/local/bin/app",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			gotImage, gotPath, err := parseDownloadPath(tt.path)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parseDownloadPath() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parseDownloadPath() unexpected error: %v", err)
			}
			if gotImage != tt.wantImage || gotPath != tt.wantPath {
				t.Fatalf("parseDownloadPath() = (%q, %q), want (%q, %q)",
					gotImage, gotPath, tt.wantImage, tt.wantPath,
				)
			}
		})
	}
}

func TestParsePlatform(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		wantOS   string
		wantArch string
		wantVar  string
		wantErr  bool
	}{
		{name: "os arch", input: "linux/amd64", wantOS: "linux", wantArch: "amd64"},
		{name: "variant", input: "linux/arm/v7", wantOS: "linux", wantArch: "arm", wantVar: "v7"},
		{name: "empty", input: "", wantErr: true},
		{name: "missing arch", input: "linux", wantErr: true},
		{name: "too many", input: "linux/arm/v7/extra", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePlatform(tt.input)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("parsePlatform() expected error")
				}
				return
			}
			if err != nil {
				t.Fatalf("parsePlatform() unexpected error: %v", err)
			}
			if got.OS != tt.wantOS || got.Architecture != tt.wantArch || got.Variant != tt.wantVar {
				t.Fatalf("parsePlatform() = %s/%s/%s, want %s/%s/%s",
					got.OS, got.Architecture, got.Variant,
					tt.wantOS, tt.wantArch, tt.wantVar,
				)
			}
		})
	}
}

func TestHandleDownloadWithoutCacheWritesOpenError(t *testing.T) {
	app := &serverApp{}
	req := httptest.NewRequest(http.MethodGet, "/download/!!!/-/etc/passwd", nil)
	rec := httptest.NewRecorder()

	app.handleDownload(rec, req)

	if rec.Code != http.StatusBadGateway {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusBadGateway)
	}
}
