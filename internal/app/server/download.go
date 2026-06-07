package server

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func (app *serverApp) handleDownload(w http.ResponseWriter, r *http.Request) {
	imageRef, filePath, err := parseDownloadPath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	openOpts, err := downloadOpenOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	extractor := &ociimage.Extractor{}
	file, err := extractor.OpenFile(r.Context(), imageRef, filePath, openOpts)
	if err != nil {
		writeDownloadError(w, err)
		slog.Warn("download failed", "image", imageRef, "path", filePath, "error", err)
		return
	}
	defer file.Reader.Close()

	filename := filepath.Base(file.Path)
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(filename)))
	if file.Size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(file.Size, 10))
	}
	if !file.ModTime.IsZero() {
		w.Header().Set("Last-Modified", file.ModTime.UTC().Format(http.TimeFormat))
	}

	if _, err := io.Copy(w, file.Reader); err != nil {
		slog.Warn("download stream interrupted", "image", imageRef, "path", filePath, "error", err)
	}
}

func parseDownloadPath(urlPath string) (string, string, error) {
	const (
		prefix    = "/download/"
		separator = "/-/"
	)

	if !strings.HasPrefix(urlPath, prefix) {
		return "", "", fmt.Errorf("download path must start with %s", prefix)
	}

	rest := strings.TrimPrefix(urlPath, prefix)
	sepIndex := strings.Index(rest, separator)
	if sepIndex < 0 {
		return "", "", fmt.Errorf("download path must be /download/<image>/-/<path>")
	}

	imageRef := strings.TrimSpace(rest[:sepIndex])
	filePath := strings.TrimSpace(rest[sepIndex+len(separator):])
	if imageRef == "" || filePath == "" {
		return "", "", fmt.Errorf("image and path are required")
	}
	return imageRef, filePath, nil
}

func writeDownloadError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		http.Error(w, "invalid path", http.StatusBadRequest)
	case errors.Is(err, ociimage.ErrNotFound):
		http.Error(w, "file not found", http.StatusNotFound)
	case errors.Is(err, ociimage.ErrUnsupportedFileType):
		http.Error(w, "path is not a regular file", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to read image", http.StatusBadGateway)
	}
}

func headerFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.ReplaceAll(name, `"`, "_")
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	return name
}

func downloadOpenOptions(r *http.Request) (ociimage.OpenOptions, error) {
	var opts ociimage.OpenOptions

	platformParam := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platformParam != "" {
		platform, err := parsePlatform(platformParam)
		if err != nil {
			return opts, err
		}
		opts.Platform = &platform
	}

	insecureParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("insecure")))
	opts.Insecure = insecureParam == "1" || insecureParam == "true" || insecureParam == "yes"
	return opts, nil
}

func parsePlatform(input string) (v1.Platform, error) {
	parts := strings.Split(input, "/")
	if len(parts) < 2 || len(parts) > 3 || parts[0] == "" || parts[1] == "" {
		return v1.Platform{}, fmt.Errorf("platform must be os/arch or os/arch/variant")
	}

	platform := v1.Platform{
		OS:           parts[0],
		Architecture: parts[1],
	}
	if len(parts) == 3 {
		if parts[2] == "" {
			return v1.Platform{}, fmt.Errorf("platform variant is empty")
		}
		platform.Variant = parts[2]
	}
	return platform, nil
}
