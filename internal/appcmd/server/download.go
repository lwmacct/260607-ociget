package server

import (
	"encoding/json"
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

type downloadArchiveInput struct {
	Ref      string   `json:"ref"`
	Paths    []string `json:"paths"`
	Platform string   `json:"platform,omitempty"`
	Insecure bool     `json:"insecure,omitempty"`
}

func (app *runtime) handleDownload(w http.ResponseWriter, r *http.Request) {
	imageRef, filePath, err := downloadTarget(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	openOpts, err := downloadOpenOptions(r)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	service, err := app.downloadService()
	if err != nil {
		http.Error(w, "failed to initialize downloader", http.StatusInternalServerError)
		return
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

func (app *runtime) handleDownloadArchive(w http.ResponseWriter, r *http.Request) {
	var input downloadArchiveInput
	if err := json.NewDecoder(r.Body).Decode(&input); err != nil {
		http.Error(w, "invalid archive request", http.StatusBadRequest)
		return
	}
	input.Ref = strings.TrimSpace(input.Ref)
	if input.Ref == "" || len(input.Paths) == 0 {
		http.Error(w, "image and paths are required", http.StatusBadRequest)
		return
	}

	opts, err := archiveOptions(input)
	if err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	service, err := app.downloadService()
	if err != nil {
		http.Error(w, "failed to initialize downloader", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/x-tar")
	w.Header().Set("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, download.HeaderFilename("image-files.tar")))
	err = service.WriteArchive(r.Context(), download.ArchiveRequest{
		ImageRef: input.Ref,
		Paths:    input.Paths,
		Options:  opts,
	}, w)
	if err != nil && !errors.Is(err, download.ErrWriterStarted) {
		writeDownloadError(w, err)
		slog.Warn("archive download failed", "image", input.Ref, "paths", len(input.Paths), "error", err)
	}
	if errors.Is(err, download.ErrWriterStarted) {
		slog.Warn("archive download stream interrupted", "image", input.Ref, "paths", len(input.Paths), "error", err)
	}
}

func downloadTarget(r *http.Request) (string, string, error) {
	if strings.TrimSpace(r.URL.Query().Get("ref")) != "" || strings.TrimSpace(r.URL.Query().Get("path")) != "" {
		return download.ParseQuery(r.URL.Query())
	}
	return download.ParsePath(r.URL.Path)
}

func archiveOptions(input downloadArchiveInput) (download.Options, error) {
	var opts download.Options
	platformParam := strings.TrimSpace(input.Platform)
	if platformParam != "" {
		platform, err := download.ParsePlatform(platformParam)
		if err != nil {
			return opts, err
		}
		opts.Platform = &platform
	}
	opts.Insecure = input.Insecure
	return opts, nil
}

func (app *runtime) downloadService() (*download.Service, error) {
	if app.downloads != nil {
		return app.downloads, nil
	}
	return download.NewService(download.CacheConfig{})
}

func (app *runtime) writeDownloadHeaders(w http.ResponseWriter, filename string, size int64, modTime time.Time) {
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
