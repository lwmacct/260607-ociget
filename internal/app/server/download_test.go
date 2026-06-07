package server

import "testing"

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
