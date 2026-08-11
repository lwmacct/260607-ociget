package imagestore

import (
	"testing"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/google/go-containerregistry/pkg/v1/types"
)

func TestClassifyManifest(t *testing.T) {
	tests := []struct {
		name     string
		manifest v1.Manifest
		want     ObjectKind
	}{
		{name: "OCI image", manifest: v1.Manifest{Config: v1.Descriptor{MediaType: types.OCIConfigJSON}}, want: ObjectKindImage},
		{name: "Docker image", manifest: v1.Manifest{Config: v1.Descriptor{MediaType: types.DockerConfigJSON}}, want: ObjectKindImage},
		{name: "Docker schema 1", manifest: v1.Manifest{MediaType: types.DockerManifestSchema1}, want: ObjectKindImage},
		{name: "artifact type", manifest: v1.Manifest{ArtifactType: "application/vnd.example", Config: v1.Descriptor{MediaType: types.OCIConfigJSON}}, want: ObjectKindArtifact},
		{name: "empty config", manifest: v1.Manifest{Config: v1.Descriptor{MediaType: "application/vnd.oci.empty.v1+json"}}, want: ObjectKindArtifact},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := classifyManifest(&test.manifest); got != test.want {
				t.Fatalf("classifyManifest() = %q, want %q", got, test.want)
			}
		})
	}
}

func TestArtifactBlobsUsesTitleAndDigestFallback(t *testing.T) {
	descriptors := []v1.Descriptor{
		{Digest: v1.Hash{Algorithm: "sha256", Hex: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa"}, Annotations: map[string]string{artifactTitleAnnotation: "bin/rtty"}},
		{Digest: v1.Hash{Algorithm: "sha256", Hex: "bbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbbb"}},
	}
	blobs := artifactBlobs(descriptors)
	if blobs[0].Name != "bin/rtty" || blobs[1].Name != blobs[1].Digest {
		t.Fatalf("blobs = %#v", blobs)
	}
}
