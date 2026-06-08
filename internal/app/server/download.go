package server

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/lwmacct/260607-ociget/internal/download"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func (app *serverApp) handleDownload(w http.ResponseWriter, r *http.Request) {
	imageRef, filePath, err := download.ParsePath(r.URL.Path)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	openOpts, err := downloadOpenOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	service := app.downloads
	if service == nil {
		var err error
		service, err = download.NewService(download.CacheConfig{})
		if err != nil {
			http.Error(w, "failed to initialize downloader", http.StatusInternalServerError)
			return
		}
	}

	req := download.Request{
		ImageRef: imageRef,
		FilePath: filePath,
		Options:  openOpts,
	}
	err = service.Write(r.Context(), req, w, func(meta download.Metadata) {
		app.writeDownloadHeaders(w, download.Filename(meta.Path), meta.Size, meta.ModTime)
	})
	if err != nil && !errors.Is(err, download.ErrWriterStarted) {
		writeDownloadError(w, err)
		slog.Warn("download failed", "image", imageRef, "path", filePath, "error", err)
	}
	if errors.Is(err, download.ErrWriterStarted) {
		slog.Warn("download stream interrupted", "image", imageRef, "path", filePath, "error", err)
	}
}

func (app *serverApp) writeDownloadHeaders(w http.ResponseWriter, filename string, size int64, modTime time.Time) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, download.HeaderFilename(filename)))
	if size >= 0 {
		w.Header().Set("Content-Length", strconv.FormatInt(size, 10))
	}
	if !modTime.IsZero() {
		w.Header().Set("Last-Modified", modTime.UTC().Format(http.TimeFormat))
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

func downloadOpenOptions(r *http.Request) (download.Options, error) {
	var opts download.Options

	platformParam := strings.TrimSpace(r.URL.Query().Get("platform"))
	if platformParam != "" {
		platform, err := download.ParsePlatform(platformParam)
		if err != nil {
			return opts, err
		}
		opts.Platform = &platform
	}

	insecureParam := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("insecure")))
	opts.Insecure = insecureParam == "1" || insecureParam == "true" || insecureParam == "yes"
	return opts, nil
}
