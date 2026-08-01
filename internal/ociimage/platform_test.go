package ociimage

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func TestPlatformsFromManifestFiltersAndDeduplicates(t *testing.T) {
	got := platformsFromManifest(&v1.IndexManifest{
		Manifests: []v1.Descriptor{
			{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}},
			{Platform: &v1.Platform{OS: "unknown", Architecture: "unknown"}},
			{Platform: &v1.Platform{OS: "linux", Architecture: "amd64"}},
			{Platform: &v1.Platform{OS: "linux", Architecture: "arm", Variant: "v7"}},
			{},
		},
	})

	if len(got) != 2 {
		t.Fatalf("platform count = %d, want 2", len(got))
	}
	if got[0].String() != "linux/amd64" {
		t.Fatalf("platform[0] = %q, want linux/amd64", got[0].String())
	}
	if got[1].String() != "linux/arm/v7" {
		t.Fatalf("platform[1] = %q, want linux/arm/v7", got[1].String())
	}
}

func TestPlatformStringRequiresOSAndArchitecture(t *testing.T) {
	if got := (Platform{OS: "linux"}).String(); got != "" {
		t.Fatalf("String() = %q, want empty", got)
	}
}

func TestParsePlatform(t *testing.T) {
	platform, err := ParsePlatform("linux/arm/v7")
	if err != nil {
		t.Fatalf("ParsePlatform() unexpected error: %v", err)
	}
	if platform.OS != "linux" || platform.Architecture != "arm" || platform.Variant != "v7" {
		t.Fatalf("ParsePlatform() = %#v", platform)
	}
	if _, err := ParsePlatform("linux"); err == nil {
		t.Fatal("ParsePlatform() expected error")
	}
}
