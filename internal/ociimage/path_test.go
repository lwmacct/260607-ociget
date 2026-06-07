package ociimage

import (
	"errors"
	"testing"
)

func TestNormalizePath(t *testing.T) {
	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "absolute", input: "/usr/local/bin/app", want: "usr/local/bin/app"},
		{name: "relative", input: "etc/passwd", want: "etc/passwd"},
		{name: "clean duplicate slash", input: "etc//passwd", want: "etc/passwd"},
		{name: "empty", input: "", wantErr: true},
		{name: "root", input: "/", wantErr: true},
		{name: "parent segment", input: "etc/../passwd", wantErr: true},
		{name: "backslash", input: `etc\passwd`, wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := NormalizePath(tt.input)
			if tt.wantErr {
				if !errors.Is(err, ErrInvalidPath) {
					t.Fatalf("NormalizePath() error = %v, want ErrInvalidPath", err)
				}
				return
			}
			if err != nil {
				t.Fatalf("NormalizePath() unexpected error: %v", err)
			}
			if got != tt.want {
				t.Fatalf("NormalizePath() = %q, want %q", got, tt.want)
			}
		})
	}
}
