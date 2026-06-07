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

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func (app *serverApp) handleDownload(w http.ResponseWriter, r *http.Request) {
	imageRef := strings.TrimSpace(r.URL.Query().Get("image"))
	filePath := strings.TrimSpace(r.URL.Query().Get("path"))
	if imageRef == "" || filePath == "" {
		http.Error(w, "image and path query parameters are required", http.StatusBadRequest)
		return
	}

	extractor := &ociimage.Extractor{}
	file, err := extractor.OpenFile(r.Context(), imageRef, filePath)
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
