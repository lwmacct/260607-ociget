package download

import (
	"fmt"
	"path/filepath"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
)

func ParsePath(urlPath string) (string, string, error) {
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

func ParsePlatform(input string) (v1.Platform, error) {
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

func HeaderFilename(name string) string {
	name = strings.ReplaceAll(name, `\`, "_")
	name = strings.ReplaceAll(name, `"`, "_")
	if name == "" || name == "." || name == "/" {
		return "download"
	}
	return name
}

func Filename(path string) string {
	return filepath.Base(path)
}

func platformString(p v1.Platform) string {
	parts := []string{p.OS, p.Architecture}
	if p.Variant != "" {
		parts = append(parts, p.Variant)
	}
	if p.OSVersion != "" {
		parts = append(parts, p.OSVersion)
	}
	if len(p.OSFeatures) > 0 {
		parts = append(parts, strings.Join(p.OSFeatures, ","))
	}
	return strings.Join(parts, "/")
}
