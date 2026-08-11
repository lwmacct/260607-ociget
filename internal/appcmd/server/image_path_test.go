package server

import (
	"net/url"
	"testing"
)

func TestParseObjectPath(t *testing.T) {
	tests := []struct {
		name string
		path string
		ref  string
		file string
	}{
		{name: "registry tag", path: "/ghcr.io/acme/tool:v1/-/usr/bin/app", ref: "ghcr.io/acme/tool:v1", file: "usr/bin/app"},
		{name: "digest", path: "/registry.example/team/tool@sha256:0123/-/etc/config", ref: "registry.example/team/tool@sha256:0123", file: "etc/config"},
		{name: "separator in file path", path: "/alpine:latest/-/var/lib/-/state", ref: "alpine:latest", file: "var/lib/-/state"},
		{name: "artifact digest selector", path: "/ghcr.io/acme/tool:v1/-/@sha256:0123", ref: "ghcr.io/acme/tool:v1", file: "@sha256:0123"},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			ref, file, err := parseObjectPath(test.path)
			if err != nil {
				t.Fatal(err)
			}
			if ref != test.ref || file != test.file {
				t.Fatalf("parseObjectPath() = %q, %q; want %q, %q", ref, file, test.ref, test.file)
			}
		})
	}
}

func TestParseObjectPathRejectsMalformedPath(t *testing.T) {
	for _, path := range []string{"/alpine:latest", "/-/etc/config", "/alpine:latest/-/", "alpine:latest/-/etc"} {
		if _, _, err := parseObjectPath(path); err == nil {
			t.Errorf("parseObjectPath(%q) succeeded", path)
		}
	}
}

func TestParseObjectPathOptions(t *testing.T) {
	values := url.Values{
		"platform": {"linux/amd64"},
		"insecure": {"yes"},
		"refresh":  {"1"},
	}
	options, err := parseObjectPathOptions(values)
	if err != nil {
		t.Fatal(err)
	}
	if options.platform == nil || options.platform.String() != "linux/amd64" || !options.insecure || !options.refresh {
		t.Fatalf("options = %#v", options)
	}
	for _, value := range []string{"maybe", "true,false"} {
		if _, err := parseObjectPathOptions(url.Values{"refresh": {value}}); err == nil {
			t.Errorf("parseObjectPathOptions(%q) succeeded", value)
		}
	}
}
