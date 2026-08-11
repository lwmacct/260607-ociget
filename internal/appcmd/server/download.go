package server

import (
	"archive/tar"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net/http"
	"net/url"
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
	if app.objects == nil {
		http.Error(w, "image store unavailable", http.StatusServiceUnavailable)
		return
	}
	objectID, err := decodedURLParam(r, "objectID")
	if err != nil {
		http.Error(w, "invalid object id", http.StatusBadRequest)
		return
	}
	file, err := app.objects.OpenFile(objectID, r.URL.Query().Get("path"))
	if err != nil {
		writeImageFilesystemError(w, err)
		return
	}
	app.serveImageFile(w, r, objectID, file, imagestore.OpenRequest{})
}

func (app *runtime) handleImagePathDownload(w http.ResponseWriter, r *http.Request) {
	if app.objects == nil {
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
	object, err := app.objects.Open(r.Context(), options.openRequest(imageRef))
	if err != nil {
		writeImagePathError(w, err)
		return
	}
	file, err := app.objects.OpenFile(object.ObjectID, filePath)
	if err != nil {
		writeImagePathError(w, err)
		return
	}
	app.serveImageFile(w, r, object.ObjectID, file, options.openRequest(imageRef))
}

func (app *runtime) serveImageFile(w http.ResponseWriter, r *http.Request, objectID string, file *imagestore.File, request imagestore.OpenRequest) {
	entry := file.Entry
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(entry.Name)))
	etag := sha256.Sum256([]byte(objectID + "\n" + entry.Path + "\n" + strconv.FormatInt(entry.Size, 10)))
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
	if request.Reference != "" {
		opened, err = app.objects.OpenFileForRequest(r.Context(), objectID, entry.Path, request)
	} else {
		opened, err = app.objects.OpenFileReader(r.Context(), objectID, entry.Path)
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

func (app *runtime) handleArtifactBlob(w http.ResponseWriter, r *http.Request) {
	if app.objects == nil {
		http.Error(w, "OCI object store unavailable", http.StatusServiceUnavailable)
		return
	}
	objectID, err := decodedURLParam(r, "objectID")
	if err != nil {
		http.Error(w, "invalid object id", http.StatusBadRequest)
		return
	}
	digest, err := decodedURLParam(r, "digest")
	if err != nil {
		http.Error(w, "invalid blob digest", http.StatusBadRequest)
		return
	}
	descriptor, err := app.objects.ArtifactBlob(objectID, digest)
	if err != nil {
		writeArtifactBlobError(w, err)
		return
	}
	mediaType := descriptor.MediaType
	if _, _, err := mime.ParseMediaType(mediaType); err != nil {
		mediaType = "application/octet-stream"
	}
	w.Header().Set("Content-Type", mediaType)
	w.Header().Set("Content-Disposition", attachmentDisposition(descriptor.Name))
	w.Header().Set("ETag", strconv.Quote(descriptor.Digest))
	w.Header().Set("Accept-Ranges", "bytes")
	if r.Method == http.MethodHead {
		w.Header().Set("Content-Length", strconv.FormatInt(descriptor.Size, 10))
		return
	}
	start, length, partial, err := imageRange(r.Header.Get("Range"), descriptor.Size)
	if err != nil {
		w.Header().Set("Content-Range", fmt.Sprintf("bytes */%d", descriptor.Size))
		http.Error(w, "invalid range", http.StatusRequestedRangeNotSatisfiable)
		return
	}
	blob, err := app.objects.OpenArtifactBlob(r.Context(), objectID, digest)
	if err != nil {
		writeArtifactBlobError(w, err)
		return
	}
	defer blob.Reader.Close()
	if partial {
		if _, err := io.CopyN(io.Discard, blob.Reader, start); err != nil {
			http.Error(w, "failed to seek artifact blob", http.StatusBadGateway)
			return
		}
		w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, start+length-1, descriptor.Size))
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
		w.WriteHeader(http.StatusPartialContent)
	} else {
		w.Header().Set("Content-Length", strconv.FormatInt(length, 10))
	}
	_, _ = io.CopyN(w, blob.Reader, length)
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
	if app.objects == nil {
		http.Error(w, "image store unavailable", http.StatusServiceUnavailable)
		return
	}
	var input imageArchiveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil || len(input.Paths) == 0 {
		http.Error(w, "paths are required", http.StatusBadRequest)
		return
	}
	objectID, err := decodedURLParam(r, "objectID")
	if err != nil {
		http.Error(w, "invalid object id", http.StatusBadRequest)
		return
	}
	files := make([]*imagestore.File, 0, len(input.Paths))
	for _, filePath := range input.Paths {
		_, err := app.objects.OpenFile(objectID, filePath)
		if err != nil {
			for _, opened := range files {
				_ = opened.Reader.Close()
			}
			writeImageFilesystemError(w, err)
			return
		}
		opened, openErr := app.objects.OpenFileReader(r.Context(), objectID, filePath)
		if openErr != nil {
			for _, openedFile := range files {
				_ = openedFile.Reader.Close()
			}
			writeImageFilesystemError(w, openErr)
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
			slog.Warn("archive header write failed", "objectId", objectID, "path", file.Entry.Path, "error", err)
			return
		}
		if _, err := io.Copy(tarWriter, file.Reader); err != nil {
			slog.Warn("archive file write failed", "objectId", objectID, "path", file.Entry.Path, "error", err)
			return
		}
	}
	if err := tarWriter.Close(); err != nil {
		slog.Warn("archive close failed", "objectId", objectID, "error", err)
	}
}

func writeImageFilesystemError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, ociimage.ErrInvalidPath):
		http.Error(w, "invalid path", http.StatusBadRequest)
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrObjectNotFound):
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
	case errors.Is(err, ociimage.ErrNotFound), errors.Is(err, imagestore.ErrObjectNotFound):
		http.Error(w, "path or image not found", http.StatusNotFound)
	case errors.Is(err, imagestore.ErrNotRegularFile):
		http.Error(w, "path is not a regular file", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to read image", http.StatusBadGateway)
	}
}

func headerFilename(name string) string {
	name = filepath.Base(name)
	name = strings.Map(func(char rune) rune {
		if char < 0x20 || char == 0x7f || char == '\\' || char == '"' {
			return '_'
		}
		return char
	}, name)
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	return name
}

func attachmentDisposition(name string) string {
	value := mime.FormatMediaType("attachment", map[string]string{"filename": headerFilename(name)})
	if value == "" {
		return `attachment; filename="download"`
	}
	return value
}

func decodedURLParam(r *http.Request, name string) (string, error) {
	return url.PathUnescape(chi.URLParam(r, name))
}

func writeArtifactBlobError(w http.ResponseWriter, err error) {
	switch {
	case errors.Is(err, imagestore.ErrObjectNotFound), errors.Is(err, imagestore.ErrBlobNotFound):
		http.Error(w, "OCI object or blob not found", http.StatusNotFound)
	case errors.Is(err, imagestore.ErrNotArtifact):
		http.Error(w, "OCI object is not an artifact", http.StatusUnprocessableEntity)
	default:
		http.Error(w, "failed to read artifact blob", http.StatusBadGateway)
	}
}
