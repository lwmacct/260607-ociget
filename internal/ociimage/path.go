package ociimage

import (
	"errors"
	"path"
	"strings"
)

var ErrInvalidPath = errors.New("invalid image file path")

// NormalizePath converts an HTTP path parameter into the canonical tar path
// format used by container layers.
func NormalizePath(input string) (string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", ErrInvalidPath
	}
	if strings.Contains(input, "\\") {
		return "", ErrInvalidPath
	}
	if strings.ContainsRune(input, '\x00') {
		return "", ErrInvalidPath
	}

	for _, part := range strings.Split(input, "/") {
		if part == ".." {
			return "", ErrInvalidPath
		}
	}

	cleaned := path.Clean("/" + input)
	if cleaned == "/" || cleaned == "/." {
		return "", ErrInvalidPath
	}
	return strings.TrimPrefix(cleaned, "/"), nil
}

func normalizeLayerPath(input string) string {
	input = strings.TrimSpace(input)
	input = strings.TrimPrefix(input, "./")
	cleaned := path.Clean("/" + input)
	if cleaned == "/" || cleaned == "/." {
		return ""
	}
	return strings.TrimPrefix(cleaned, "/")
}
