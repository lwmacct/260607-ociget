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
	"time"

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

	if app.cache != nil {
		if handled := app.handleCachedDownload(w, r, imageRef, filePath, openOpts); handled {
			return
		}
	}

	if err := app.streamDownload(w, r, imageRef, filePath, openOpts, nil); err != nil && !errors.Is(err, errDownloadResponseStarted) {
		writeDownloadError(w, err)
		slog.Warn("download failed", "image", imageRef, "path", filePath, "error", err)
	}
}

func (app *serverApp) handleCachedDownload(w http.ResponseWriter, r *http.Request, imageRef, filePath string, openOpts ociimage.OpenOptions) bool {
	key, err := app.cache.key(r.Context(), imageRef, filePath, openOpts)
	if err != nil {
		writeDownloadError(w, err)
		slog.Warn("download cache key failed", "image", imageRef, "path", filePath, "error", err)
		return true
	}

	result, streamedToThisResponse, err := app.cache.do(key, func() (*cacheExtractResult, error) {
		writer, err := app.cache.writer(key)
		if err != nil {
			return nil, err
		}
		err = app.streamDownload(w, r, imageRef, filePath, openOpts, writer)
		if err != nil {
			writer.Abort()
			return nil, err
		}
		cached, err := app.cache.get(key)
		if err != nil {
			slog.Warn("download cache unavailable after stream", "image", imageRef, "path", filePath, "error", err)
			return &cacheExtractResult{streamed: true}, nil
		}
		return &cacheExtractResult{cached: cached, streamed: true}, nil
	})
	if err != nil {
		if !errors.Is(err, errDownloadResponseStarted) {
			writeDownloadError(w, err)
		}
		slog.Warn("download failed", "image", imageRef, "path", filePath, "error", err)
		return true
	}
	if result.streamed && streamedToThisResponse {
		return true
	}
	if result.cached == nil {
		return false
	}
	app.writeDownloadHeaders(w, filepath.Base(filePath), result.cached.size, result.cached.modTime)
	if err := app.cache.serve(w, result.cached); err != nil {
		slog.Warn("cached download stream interrupted", "image", imageRef, "path", filePath, "error", err)
	}
	return true
}

func (app *serverApp) streamDownload(w http.ResponseWriter, r *http.Request, imageRef, filePath string, openOpts ociimage.OpenOptions, cacheWriter *cacheWriter) error {
	extractor := &ociimage.Extractor{}
	file, err := extractor.OpenFile(r.Context(), imageRef, filePath, openOpts)
	if err != nil {
		return err
	}
	defer file.Reader.Close()

	app.writeDownloadHeaders(w, filepath.Base(file.Path), file.Size, file.ModTime)

	reader := io.Reader(file.Reader)
	if cacheWriter != nil {
		defer cacheWriter.Abort()
		reader = io.TeeReader(file.Reader, cacheWriter)
	}

	if _, err := io.Copy(w, reader); err != nil {
		slog.Warn("download stream interrupted", "image", imageRef, "path", filePath, "error", err)
		return fmt.Errorf("%w: %v", errDownloadResponseStarted, err)
	}
	if cacheWriter != nil {
		if err := cacheWriter.Commit(file.ModTime); err != nil {
			slog.Warn("download cache commit failed", "image", imageRef, "path", filePath, "error", err)
		}
	}
	return nil
}

var errDownloadResponseStarted = errors.New("download response already started")

func (app *serverApp) writeDownloadHeaders(w http.ResponseWriter, filename string, size int64, modTime time.Time) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, headerFilename(filename)))
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if !modTime.IsZero() {
		w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
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
