package server

import (
	"archive/tar"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

type imageArchiveInput struct {
	Paths []string `json:"paths"`
}

func (app *runtime) handleImageFile(w http.ResponseWriter, r *http.Request) {
	if app.images == nil {
		http.Error(w, "image store unavailable", http.StatusServiceUnavailable)
		return
	}
	imageID := chi.URLParam(r, "imageID")
	file, err := app.images.OpenFile(imageID, r.URL.Query().Get("path"))
	if err != nil {
		writeImageStoreError(w, err)
		return
	}
	defer file.Reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(file.Entry.Name)))
	w.Header().Set("ETag", strconv.Quote("sha256:"+file.Entry.ContentDigest))
	http.ServeContent(w, r, file.Entry.Name, file.Entry.ModTime, file.Reader)
}

func (app *runtime) handleImagePathDownload(w http.ResponseWriter, r *http.Request) {
	if app.images == nil {
		http.Error(w, "image store unavailable", http.StatusServiceUnavailable)
		return
	}
	imageRef, filePath, err := parseImagePath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	options, err := parseImagePathOptions(r.URL.Query())
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}
	image, err := app.images.Open(r.Context(), options.openRequest(imageRef))
	if err != nil {
		writeImagePathError(w, err)
		return
	}
	file, err := app.images.OpenFile(image.ImageID, filePath)
	if err != nil {
		writeImagePathError(w, err)
		return
	}
	defer file.Reader.Close()

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(file.Entry.Name)))
	w.Header().Set("ETag", strconv.Quote("sha256:"+file.Entry.ContentDigest))
	http.ServeContent(w, r, file.Entry.Name, file.Entry.ModTime, file.Reader)
}

func (app *runtime) handleImageArchive(w http.ResponseWriter, r *http.Request) {
	if app.images == nil {
		http.Error(w, "image store unavailable", http.StatusServiceUnavailable)
		return
	}
	var input imageArchiveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Paths) == 0 {
		http.Error(w, "paths are required", http.StatusBadRequest)
		return
	}
	imageID := chi.URLParam(r, "imageID")
	files := make([]*imagestore.File, 0, len(input.Paths))
	for _, filePath := range input.Paths {
		file, err := app.images.OpenFile(imageID, filePath)
		if err != nil {
			for _, opened := range files {
				_ = opened.Reader.Close()
			}
			writeImageStoreError(w, err)
			return
		}
		files = append(files, file)
	}
	defer func() {
		for _, file := range files {
			_ = file.Reader.Close()
		}
	}()

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", `attachment; filename="image-files.tar"`)
	tarWriter := tar.NewWriter(w)
	for _, file := range files {
		header := &tar.Header{
			Name:    strings.TrimPrefix(file.Entry.Path, "/"),
			Mode:    file.Entry.Mode,
			Size:    file.Entry.Size,
			ModTime: file.Entry.ModTime,
		}
		if err := tarWriter.WriteHeader(header); err != nil {
			slog.Warn("archive header write failed", "imageId", imageID, "path", file.Entry.Path, "error", err)
			return
		}
		if _, err := io.Copy(tarWriter, file.Reader); err != nil {
			slog.Warn("archive file write failed", "imageId", imageID, "path", file.Entry.Path, "error", err)
			return
		}
	}
	if err := tarWriter.Close(); err != nil {
		slog.Warn("archive close failed", "imageId", imageID, "error", err)
	}
}

func writeImageStoreError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		http.Error(w, "invalid path", http.StatusBadRequest)
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrImageNotFound):
		http.Error(w, "path or image not found", http.StatusNotFound)
	case errors.Is(err, imagestore.ErrNotRegularFile):
		http.Error(w, "path is not a regular file", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to read image store", http.StatusInternalServerError)
	}
}

func writeImagePathError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		http.Error(w, "invalid path", http.StatusBadRequest)
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrImageNotFound):
		http.Error(w, "path or image not found", http.StatusNotFound)
	case errors.Is(err, imagestore.ErrNotRegularFile):
		http.Error(w, "path is not a regular file", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to read image", http.StatusBadGateway)
	}
}

func headerFilename(name string) string {
	name = filepath.Base(name)
	name = strings.NewReplacer(`\`, "_", `"`, "_").Replace(name)
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	return name
}
