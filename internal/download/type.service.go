package download

import (
	"context"
	"fmt"
	"io"
	"log/slog"

	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

func NewService(cacheConfig CacheConfig) (*Service, error) {
	cache, err := newCache(cacheConfig)
	if err != nil {
		return nil, err
	}
	return &Service{cache: cache}, nil
}

func (s *Service) Write(ctx context.Context, req Request, dst io.Writer, beforeWrite func(Metadata)) error {
	if s.cache != nil {
		if handled, err := s.writeCached(ctx, req, dst, beforeWrite); handled || err != nil {
			return err
		}
	}
	return s.writeFromImage(ctx, req, dst, beforeWrite, nil)
}

func (s *Service) writeCached(ctx context.Context, req Request, dst io.Writer, beforeWrite func(Metadata)) (bool, error) {
	key, err := s.cache.key(ctx, req)
	if err != nil {
		return true, err
	}

	result, writtenToThisWriter, err := s.cache.do(key, func() (*cacheResult, error) {
		writer, err := s.cache.writer(key)
		if err != nil {
			return nil, err
		}
		err = s.writeFromImage(ctx, req, dst, beforeWrite, writer)
		if err != nil {
			writer.Abort()
			return nil, err
		}
		cached, err := s.cache.get(key)
		if err != nil {
			slog.Warn("download cache unavailable after stream", "image", req.ImageRef, "path", req.FilePath, "error", err)
			return &cacheResult{written: true}, nil
		}
		return &cacheResult{cached: cached, written: true}, nil
	})
	if err != nil {
		return true, err
	}
	if result.written && writtenToThisWriter {
		return true, nil
	}
	if result.cached == nil {
		return false, nil
	}

	file, cached, err := s.cache.open(key)
	if err != nil {
		return true, err
	}
	defer file.Close()
	notifyBeforeWrite(beforeWrite, Metadata{
		Path:     req.FilePath,
		Size:     cached.size,
		ModTime:  cached.modTime,
		CacheHit: true,
	})
	if _, err := io.Copy(dst, file); err != nil {
		return true, fmt.Errorf("%w: %v", ErrWriterStarted, err)
	}
	return true, nil
}

func (s *Service) writeFromImage(ctx context.Context, req Request, dst io.Writer, beforeWrite func(Metadata), cacheWriter *cacheWriter) error {
	extractor := &ociimage.Extractor{}
	file, err := extractor.OpenFile(ctx, req.ImageRef, req.FilePath, ociOptions(req.Options))
	if err != nil {
		return err
	}
	defer file.Reader.Close()

	notifyBeforeWrite(beforeWrite, Metadata{
		Path:     file.Path,
		Size:     file.Size,
		ModTime:  file.ModTime,
		CacheHit: false,
	})

	reader := io.Reader(file.Reader)
	if cacheWriter != nil {
		defer cacheWriter.Abort()
		reader = io.TeeReader(file.Reader, cacheWriter)
	}

	if _, err := io.Copy(dst, reader); err != nil {
		return fmt.Errorf("%w: %v", ErrWriterStarted, err)
	}
	if cacheWriter != nil {
		if err := cacheWriter.Commit(file.ModTime); err != nil {
			slog.Warn("download cache commit failed", "image", req.ImageRef, "path", req.FilePath, "error", err)
		}
	}
	return nil
}

func notifyBeforeWrite(fn func(Metadata), meta Metadata) {
	if fn != nil {
		fn(meta)
	}
}

func ociOptions(opts Options) ociimage.OpenOptions {
	return ociimage.OpenOptions{
		Platform: opts.Platform,
		Insecure: opts.Insecure,
	}
}
