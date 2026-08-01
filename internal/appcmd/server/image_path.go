package server

import (
	"fmt"
	"net/url"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const imagePathSeparator = "/-/"

// parseImagePath splits /<image-ref>/-/<image-path> at the first separator.
func parseImagePath(urlPath string) (string, string, error) {
	if !strings.HasPrefix(urlPath, "/") {
		return "", "", fmt.Errorf("image path must start with /")
	}
	rest := strings.TrimPrefix(urlPath, "/")
	separatorIndex := strings.Index(rest, imagePathSeparator)
	if separatorIndex < 0 {
		return "", "", fmt.Errorf("image path must be /<image-ref>/-/<path>")
	}
	imageRef := strings.TrimSpace(rest[:separatorIndex])
	filePath := strings.TrimSpace(rest[separatorIndex+len(imagePathSeparator):])
	if imageRef == "" || filePath == "" {
		return "", "", fmt.Errorf("image and path are required")
	}
	filePath, err := ociimage.NormalizePath(filePath)
	if err != nil {
		return "", "", err
	}
	return imageRef, filePath, nil
}

type imagePathOptions struct {
	platform *v1.Platform
	insecure bool
	refresh  bool
}

func parseImagePathOptions(values url.Values) (imagePathOptions, error) {
	var options imagePathOptions
	platformParam := strings.TrimSpace(values.Get("platform"))
	if platformParam != "" {
		platform, err := ociimage.ParsePlatform(platformParam)
		if err != nil {
			return options, err
		}
		options.platform = &platform
	}
	insecure, err := parseImagePathBool(values, "insecure")
	if err != nil {
		return options, err
	}
	refresh, err := parseImagePathBool(values, "refresh")
	if err != nil {
		return options, err
	}
	options.insecure = insecure
	options.refresh = refresh
	return options, nil
}

func parseImagePathBool(values url.Values, key string) (bool, error) {
	value, ok := values[key]
	if !ok {
		return false, nil
	}
	if len(value) != 1 {
		return false, fmt.Errorf("query parameter %s must be a single boolean value", key)
	}
	switch strings.ToLower(strings.TrimSpace(value[0])) {
	case "1", "true", "yes", "on":
		return true, nil
	case "0", "false", "no", "off":
		return false, nil
	default:
		return false, fmt.Errorf("query parameter %s must be true or false", key)
	}
}

func (o imagePathOptions) openRequest(imageRef string) imagestore.OpenRequest {
	return imagestore.OpenRequest{
		ImageRef: imageRef,
		Platform: o.platform,
		Insecure: o.insecure,
		Refresh:  o.refresh,
	}
}
