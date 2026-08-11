package server

import (
	"fmt"
	"net/url"
	"strings"

	v1 "github.com/google/go-containerregistry/pkg/v1"
	"github.com/lwmacct/260607-ociget/internal/imagestore"
	"github.com/lwmacct/260607-ociget/internal/ociimage"
)

const objectPathSeparator = "/-/"

// parseObjectPath splits /<oci-ref>/-/<selector> at the first separator.
func parseObjectPath(urlPath string) (string, string, error) {
	if !strings.HasPrefix(urlPath, "/") {
		return "", "", fmt.Errorf("OCI object path must start with /")
	}
	rest := strings.TrimPrefix(urlPath, "/")
	separatorIndex := strings.Index(rest, objectPathSeparator)
	if separatorIndex < 0 {
		return "", "", fmt.Errorf("OCI object path must be /<oci-ref>/-/<selector>")
	}
	reference := strings.TrimSpace(rest[:separatorIndex])
	selector := strings.TrimSpace(rest[separatorIndex+len(objectPathSeparator):])
	if reference == "" || selector == "" {
		return "", "", fmt.Errorf("OCI reference and selector are required")
	}
	selector, err := ociimage.NormalizePath(selector)
	if err != nil {
		return "", "", err
	}
	return reference, selector, nil
}

type objectPathOptions struct {
	platform *v1.Platform
	insecure bool
	refresh  bool
}

func parseObjectPathOptions(values url.Values) (objectPathOptions, error) {
	var options objectPathOptions
	platformParam := strings.TrimSpace(values.Get("platform"))
	if platformParam != "" {
		platform, err := ociimage.ParsePlatform(platformParam)
		if err != nil {
			return options, err
		}
		options.platform = &platform
	}
	insecure, err := parseObjectPathBool(values, "insecure")
	if err != nil {
		return options, err
	}
	refresh, err := parseObjectPathBool(values, "refresh")
	if err != nil {
		return options, err
	}
	options.insecure = insecure
	options.refresh = refresh
	return options, nil
}

func parseObjectPathBool(values url.Values, key string) (bool, error) {
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

func (o objectPathOptions) openRequest(reference string) imagestore.OpenRequest {
	return imagestore.OpenRequest{
		Reference: reference,
		Platform:  o.platform,
		Insecure:  o.insecure,
		Refresh:   o.refresh,
	}
}
