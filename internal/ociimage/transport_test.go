package ociimage

import (
	"io"
	"net/http"
	"strings"
	"testing"
)

func TestRealmFixTransportRewritesSameHostHTTPRealm(t *testing.T) {
	transport := realmFixTransport{base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}
		resp.Header.Set("Www-Authenticate", `Bearer realm="http://registry.example.com/service/token",service="registry"`)
		return resp, nil
	})}

	req, err := http.NewRequest(http.MethodGet, "https://registry.example.com/v2/", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}

	got := resp.Header.Get("Www-Authenticate")
	want := `Bearer realm="https://registry.example.com/service/token",service="registry"`
	if got != want {
		t.Fatalf("Www-Authenticate = %q, want %q", got, want)
	}
}

func TestRealmFixTransportDoesNotRewriteDifferentHost(t *testing.T) {
	transport := realmFixTransport{base: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		resp := &http.Response{
			StatusCode: http.StatusUnauthorized,
			Header:     http.Header{},
			Body:       io.NopCloser(strings.NewReader("")),
			Request:    req,
		}
		resp.Header.Set("Www-Authenticate", `Bearer realm="http://auth.example.com/token",service="registry"`)
		return resp, nil
	})}

	req, err := http.NewRequest(http.MethodGet, "https://registry.example.com/v2/", nil)
	if err != nil {
		t.Fatalf("NewRequest() failed: %v", err)
	}
	resp, err := transport.RoundTrip(req)
	if err != nil {
		t.Fatalf("RoundTrip() failed: %v", err)
	}

	got := resp.Header.Get("Www-Authenticate")
	want := `Bearer realm="http://auth.example.com/token",service="registry"`
	if got != want {
		t.Fatalf("Www-Authenticate = %q, want %q", got, want)
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}
