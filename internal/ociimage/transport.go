package ociimage

import (
	"net/http"
	"strings"
)

type realmFixTransport struct {
	base http.RoundTripper
}

func (t realmFixTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	base := t.base
	if base == nil {
		base = http.DefaultTransport
	}

	resp, err := base.RoundTrip(req)
	if err != nil || resp == nil {
		return resp, err
	}

	header := resp.Header.Get("Www-Authenticate")
	if header == "" || req.URL == nil || req.URL.Host == "" {
		return resp, nil
	}

	from := `realm="http://` + req.URL.Host + `/`
	to := `realm="https://` + req.URL.Host + `/`
	if strings.Contains(header, from) {
		resp.Header.Set("Www-Authenticate", strings.ReplaceAll(header, from, to))
	}
	return resp, nil
}
