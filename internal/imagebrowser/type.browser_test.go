package imagebrowser

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func TestListFromLayersRoot(t *testing.T) {
	dir, err := listFromLayers("/", []io.Reader{
		tarBytes(t,
			dirEntry("etc"),
			fileEntry("etc/config", "base"),
			fileEntry("usr/bin/app", "app"),
		),
	})
	if err != nil {
		t.Fatalf("listFromLayers() unexpected error: %v", err)
	}

	assertEntryNames(t, dir.Entries, []string{"etc", "usr"})
	if dir.Entries[0].Type != EntryTypeDirectory {
		t.Fatalf("etc type = %s, want directory", dir.Entries[0].Type)
	}
}

func TestListFromLayersDirectory(t *testing.T) {
	dir, err := listFromLayers("/etc", []io.Reader{
		tarBytes(t,
			fileEntry("etc/config", "base"),
			fileEntry("etc/hosts", "hosts"),
		),
	})
	if err != nil {
		t.Fatalf("listFromLayers() unexpected error: %v", err)
	}

	assertEntryNames(t, dir.Entries, []string{"config", "hosts"})
	if dir.Entries[0].Path != "etc/config" {
		t.Fatalf("entry path = %q, want etc/config", dir.Entries[0].Path)
	}
}

func TestListFromLayersUpperLayerOverridesEntry(t *testing.T) {
	dir, err := listFromLayers("/etc", []io.Reader{
		tarBytes(t, fileEntry("etc/config", "base")),
		tarBytes(t, dirEntry("etc/config")),
	})
	if err != nil {
		t.Fatalf("listFromLayers() unexpected error: %v", err)
	}

	if len(dir.Entries) != 1 {
		t.Fatalf("entries = %d, want 1", len(dir.Entries))
	}
	if dir.Entries[0].Type != EntryTypeDirectory {
		t.Fatalf("config type = %s, want directory", dir.Entries[0].Type)
	}
}

func TestListFromLayersWhiteoutRemovesEntry(t *testing.T) {
	dir, err := listFromLayers("/etc", []io.Reader{
		tarBytes(t, fileEntry("etc/config", "base"), fileEntry("etc/hosts", "hosts")),
		tarBytes(t, fileEntry("etc/.wh.config", "")),
	})
	if err != nil {
		t.Fatalf("listFromLayers() unexpected error: %v", err)
	}

	assertEntryNames(t, dir.Entries, []string{"hosts"})
}

func TestListFromLayersOpaqueWhiteoutClearsDirectory(t *testing.T) {
	dir, err := listFromLayers("/etc", []io.Reader{
		tarBytes(t, fileEntry("etc/config", "base"), fileEntry("etc/hosts", "hosts")),
		tarBytes(t, fileEntry("etc/.wh..wh..opq", ""), fileEntry("etc/new", "new")),
	})
	if err != nil {
		t.Fatalf("listFromLayers() unexpected error: %v", err)
	}

	assertEntryNames(t, dir.Entries, []string{"new"})
}

func TestListFromLayersFileIsNotDirectory(t *testing.T) {
	_, err := listFromLayers("/etc/config", []io.Reader{
		tarBytes(t, fileEntry("etc/config", "base")),
	})
	if !errors.Is(err, ErrNotDirectory) {
		t.Fatalf("listFromLayers() error = %v, want ErrNotDirectory", err)
	}
}

func TestListFromLayersMissing(t *testing.T) {
	_, err := listFromLayers("/missing", []io.Reader{
		tarBytes(t, fileEntry("etc/config", "base")),
	})
	if !errors.Is(err, ociimage.ErrNotFound) {
		t.Fatalf("listFromLayers() error = %v, want ErrNotFound", err)
	}
}

type testTarEntry struct {
	header *tar.Header
	body   string
}

func dirEntry(name string) testTarEntry {
	return testTarEntry{
		header: &tar.Header{
			Name:     name,
			Typeflag: tar.TypeDir,
			Mode:     0o755,
		},
	}
}

func fileEntry(name, body string) testTarEntry {
	return testTarEntry{
		header: &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Mode:     0o644,
			Size:     int64(len(body)),
		},
		body: body,
	}
}

func tarBytes(t *testing.T, entries ...testTarEntry) *bytes.Reader {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		if err := tw.WriteHeader(entry.header); err != nil {
			t.Fatalf("WriteHeader() failed: %v", err)
		}
		if entry.body != "" {
			if _, err := tw.Write([]byte(entry.body)); err != nil {
				t.Fatalf("Write() failed: %v", err)
			}
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("Close() failed: %v", err)
	}
	return bytes.NewReader(buf.Bytes())
}

func assertEntryNames(t *testing.T, entries []Entry, want []string) {
	t.Helper()

	if len(entries) != len(want) {
		t.Fatalf("entry count = %d, want %d", len(entries), len(want))
	}
	for i := range want {
		if entries[i].Name != want[i] {
			t.Fatalf("entry[%d] = %q, want %q", i, entries[i].Name, want[i])
		}
	}
}
