package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestAcceptsGzip(t *testing.T) {
	cases := []struct {
		header string
		want   bool
	}{
		{"", false},
		{"gzip", true},
		{"br, gzip", true},
		{"*", true},
		{"gzip;q=0", false},
		{"gzip; q=0.0", false},
		{"br", false},
	}
	for _, c := range cases {
		if got := acceptsGzip(c.header); got != c.want {
			t.Fatalf("acceptsGzip(%q)=%v want %v", c.header, got, c.want)
		}
	}
}

func TestMatchesCompressionType(t *testing.T) {
	if !matchesCompressionType("text/plain; charset=utf-8", []string{"text/plain"}) {
		t.Fatalf("expected exact type match")
	}
	if !matchesCompressionType("text/html", []string{"text/*"}) {
		t.Fatalf("expected wildcard match")
	}
	if matchesCompressionType("application/json", []string{"text/*"}) {
		t.Fatalf("unexpected type match")
	}
	if !matchesCompressionType("invalid;=", nil) {
		t.Fatalf("empty allow list should allow all")
	}
}

func TestAppendVaryHeader(t *testing.T) {
	h := http.Header{}
	appendVaryHeader(h, "Accept-Encoding")
	appendVaryHeader(h, "Accept-Encoding")
	if got := h.Values("Vary"); len(got) != 1 {
		t.Fatalf("vary headers=%v", got)
	}
}

func TestEncodeWriteAndCompressionFallback(t *testing.T) {
	b, ct, err := encodeResult("abc")
	if err != nil || string(b) != "abc" || !strings.Contains(ct, "text/plain") {
		t.Fatalf("encode string b=%q ct=%q err=%v", string(b), ct, err)
	}
	b, ct, err = encodeResult(map[string]any{"a": 1})
	if err != nil || !strings.Contains(ct, "application/json") || !strings.Contains(string(b), `"a":1`) {
		t.Fatalf("encode json b=%q ct=%q err=%v", string(b), ct, err)
	}
	if _, _, err = encodeResult(map[string]any{"f": func() {}}); err == nil {
		t.Fatalf("expected encode error")
	}

	rw := httptest.NewRecorder()
	writeResult(rw, map[string]any{"f": func() {}})
	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("writeResult status=%d", rw.Code)
	}

	rw2 := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	route := &PathConfig{Method: "GET", Path: "/x", Filter: "A"}
	cfg := CompressionConfig{
		Enabled:    true,
		MinSize:    1,
		Level:      99,
		Types:      []string{"text/plain"},
		Algorithms: []string{"gzip"},
	}
	writeResultWithCompression(rw2, req.WithContext(context.Background()), route, cfg, "hello")
	if got := rw2.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("unexpected content-encoding=%q", got)
	}
	if strings.TrimSpace(rw2.Body.String()) != "hello" {
		t.Fatalf("unexpected body=%q", rw2.Body.String())
	}
}

func TestShouldCompressResponseBranches(t *testing.T) {
	route := &PathConfig{Method: "GET", Path: "/x", Filter: "A"}
	cfg := CompressionConfig{
		Enabled:    true,
		MinSize:    2,
		Level:      5,
		Types:      []string{"text/plain"},
		Algorithms: []string{"gzip"},
	}
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	h := http.Header{}

	if !shouldCompressResponse(req, route, cfg, "text/plain", 10, h) {
		t.Fatalf("expected compress=true")
	}

	if shouldCompressResponse(req, route, cfg, "text/plain", 1, h) {
		t.Fatalf("expected min-size skip")
	}
	h.Set("Content-Encoding", "br")
	if shouldCompressResponse(req, route, cfg, "text/plain", 10, h) {
		t.Fatalf("expected pre-encoded skip")
	}
	h.Del("Content-Encoding")
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	if shouldCompressResponse(req2, route, cfg, "text/plain", 10, h) {
		t.Fatalf("expected no-accept-encoding skip")
	}
	req3 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req3.Header.Set("Accept-Encoding", "gzip")
	req3.Header.Set("Range", "bytes=0-10")
	if shouldCompressResponse(req3, route, cfg, "text/plain", 10, h) {
		t.Fatalf("expected range skip")
	}
	cfg2 := cfg
	cfg2.Algorithms = []string{"br"}
	if shouldCompressResponse(req, route, cfg2, "text/plain", 10, h) {
		t.Fatalf("expected algorithm skip")
	}
	cfg3 := cfg
	cfg3.Types = []string{"application/json"}
	if shouldCompressResponse(req, route, cfg3, "text/plain", 10, h) {
		t.Fatalf("expected content-type skip")
	}
	disabled := false
	route.Compression = &PathCompressionConfig{Enabled: &disabled}
	if shouldCompressResponse(req, route, cfg, "text/plain", 10, h) {
		t.Fatalf("expected path override disable")
	}
}
