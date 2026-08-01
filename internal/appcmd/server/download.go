package server

import (
	"archive/tar"
	"crypto/sha256"
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
	app.serveImageFile(w, r, imageID, file, imagestore.OpenRequest{ImageRef: ""})
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
	app.serveImageFile(w, r, image.ImageID, file, options.openRequest(imageRef))
}

func (app *runtime) serveImageFile(w http.ResponseWriter, r *http.Request, imageID string, file *imagestore.File, request imagestore.OpenRequest) {
	entry := file.Entry
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(entry.Name)))
	etag := sha256.Sum256([]byte(imageID + "\n" + entry.Path + "\n" + strconv.FormatInt(entry.Size, 10)))
	w.Header().Set("ETag", strconv.Quote("sha256:"+fmt.Sprintf("%x", etag[:])))
	if !entry.ModTime.IsZero() {
		w.Header().Set("Last-Modified", entry.ModTime.UTC().Format(http.TimeFormat))
	}
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
		return
	}
	start, length, partial, err := imageRange(r.Header.Get("Range"), entry.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", entry.Size))
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	var opened *imagestore.File
	if request.ImageRef != "" {
		opened, err = app.images.OpenFileForRequest(r.Context(), imageID, entry.Path, request)
	} else {
		opened, err = app.images.OpenFileReader(r.Context(), imageID, entry.Path)
	}
	if err != nil {
		writeImagePathError(w, err)
		return
	}
	defer opened.Reader.Close()
	if partial {
		if _, err := io.CopyN(io.Discard, opened.Reader, start); err != nil {
			http.Error(w, "failed to seek image file", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, entry.Size))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
		_, _ = io.CopyN(w, opened.Reader, length)
		return
	}
	w.Header().Set("Content-Length", strconv.FormatInt(entry.Size, 10))
	_, _ = io.CopyN(w, opened.Reader, entry.Size)
}

func imageRange(value string, size int64) (int64, int64, bool, error) {
	if value == "" {
		return 0, size, false, nil
	}
	if !strings.HasPrefix(value, "bytes=") || strings.Contains(value, ",") {
		return 0, 0, false, errors.New("unsupported range")
	}
	parts := strings.SplitN(strings.TrimPrefix(value, "bytes="), "-", 2)
	if len(parts) != 2 {
		return 0, 0, false, errors.New("invalid range")
	}
	if parts[0] == "" {
		suffix, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil || suffix <= 0 {
			return 0, 0, false, errors.New("invalid range")
		}
		if suffix > size {
			suffix = size
		}
		return size - suffix, suffix, true, nil
	}
	start, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil || start < 0 || start >= size {
		return 0, 0, false, errors.New("invalid range")
	}
	end := size - 1
	if parts[1] != "" {
		end, err = strconv.ParseInt(parts[1], 10, 64)
		if err != nil || end < start {
			return 0, 0, false, errors.New("invalid range")
		}
		if end >= size {
			end = size - 1
		}
	}
	return start, end - start + 1, true, nil
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
		_, err := app.images.OpenFile(imageID, filePath)
		if err != nil {
			for _, opened := range files {
				_ = opened.Reader.Close()
			}
			writeImageStoreError(w, err)
			return
		}
		opened, openErr := app.images.OpenFileReader(r.Context(), imageID, filePath)
		if openErr != nil {
			for _, openedFile := range files {
				_ = openedFile.Reader.Close()
			}
			writeImageStoreError(w, openErr)
			return
		}
		files = append(files, opened)
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
