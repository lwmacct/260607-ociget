package ociimage

import (
	"archive/tar"
	"bytes"
	"errors"
	"io"
	"testing"
)

type fakeLayer struct {
	data []byte
	err  error
}

func (l fakeLayer) Open() (io.ReadCloser, error) {
	if l.err != nil {
		return nil, l.err
	}
	return io.NopCloser(bytes.NewReader(l.data)), nil
}

func TestOpenFileFromLayersUsesTopLayer(t *testing.T) {
	layers := []layerOpener{
		fakeLayer{data: tarBytes(t, regularFile("etc/config", "base"))},
		fakeLayer{data: tarBytes(t, regularFile("etc/config", "top"))},
	}

	file, err := openFileFromLayers(layers, "etc/config")
	if err != nil {
		t.Fatalf("openFileFromLayers() unexpected error: %v", err)
	}
	defer file.Reader.Close()

	got := readAllString(t, file.Reader)
	if got != "top" {
		t.Fatalf("file content = %q, want %q", got, "top")
	}
	if file.Size != 3 {
		t.Fatalf("file size = %d, want 3", file.Size)
	}
}

func TestOpenFileFromLayersWhiteoutHidesLowerFile(t *testing.T) {
	layers := []layerOpener{
		fakeLayer{data: tarBytes(t, regularFile("etc/config", "base"))},
		fakeLayer{data: tarBytes(t, regularFile("etc/.wh.config", ""))},
	}

	_, err := openFileFromLayers(layers, "etc/config")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("openFileFromLayers() error = %v, want ErrNotFound", err)
	}
}

func TestOpenFileFromLayersOpaqueWhiteoutHidesLowerDirectory(t *testing.T) {
	layers := []layerOpener{
		fakeLayer{data: tarBytes(t, regularFile("etc/config", "base"))},
		fakeLayer{data: tarBytes(t, regularFile("etc/.wh..wh..opq", ""))},
	}

	_, err := openFileFromLayers(layers, "etc/config")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("openFileFromLayers() error = %v, want ErrNotFound", err)
	}
}

func TestOpenFileFromLayersRejectsSymlink(t *testing.T) {
	layers := []layerOpener{
		fakeLayer{data: tarBytes(t, symlink("usr/bin/app", "/opt/app"))},
	}

	_, err := openFileFromLayers(layers, "usr/bin/app")
	if !errors.Is(err, ErrUnsupportedFileType) {
		t.Fatalf("openFileFromLayers() error = %v, want ErrUnsupportedFileType", err)
	}
}

func TestOpenFileFromLayersMissing(t *testing.T) {
	layers := []layerOpener{
		fakeLayer{data: tarBytes(t, regularFile("etc/config", "base"))},
	}

	_, err := openFileFromLayers(layers, "usr/bin/app")
	if !errors.Is(err, ErrNotFound) {
		t.Fatalf("openFileFromLayers() error = %v, want ErrNotFound", err)
	}
}

type tarEntry struct {
	header *tar.Header
	body   string
}

func regularFile(name, body string) tarEntry {
	return tarEntry{
		header: &tar.Header{
			Name: name,
			Mode: 0o644,
			Size: int64(len(body)),
		},
		body: body,
	}
}

func symlink(name, linkName string) tarEntry {
	return tarEntry{
		header: &tar.Header{
			Name:     name,
			Typeflag: tar.TypeSymlink,
			Linkname: linkName,
			Mode:     0o777,
		},
	}
}

func tarBytes(t *testing.T, entries ...tarEntry) []byte {
	t.Helper()

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	for _, entry := range entries {
		if entry.header.Typeflag == 0 {
			entry.header.Typeflag = tar.TypeReg
		}
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
	return buf.Bytes()
}

func readAllString(t *testing.T, r io.Reader) string {
	t.Helper()

	data, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("ReadAll() failed: %v", err)
	}
	return string(data)
}
