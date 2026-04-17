package main

import (
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRequestCompressionEnabled(t *testing.T) {
	cfg := testAppConfig()
	cfg.Compression.Enabled = true
	cfg.Compression.MinSize = 1
	cfg.Compression.Types = []string{"text/plain"}
	cfg.Compression.Algorithms = []string{"gzip"}
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: strings.Repeat("x", 64)}}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "rid-c1"))
	app.handleRequest(rw, req)

	if got := rw.Header().Get("Content-Encoding"); got != "gzip" {
		t.Fatalf("content-encoding=%q", got)
	}
	if vary := rw.Header().Get("Vary"); !strings.Contains(vary, "Accept-Encoding") {
		t.Fatalf("vary=%q", vary)
	}
	zr, err := gzip.NewReader(strings.NewReader(rw.Body.String()))
	if err != nil {
		t.Fatalf("gzip reader err: %v", err)
	}
	defer func() { _ = zr.Close() }()
	body, err := io.ReadAll(zr)
	if err != nil {
		t.Fatalf("read gz body err: %v", err)
	}
	if string(body) != strings.Repeat("x", 64) {
		t.Fatalf("body=%q", string(body))
	}
}

func TestHandleRequestCompressionPathOverrideDisabled(t *testing.T) {
	cfg := testAppConfig()
	cfg.Compression.Enabled = true
	cfg.Compression.MinSize = 1
	cfg.Compression.Types = []string{"text/plain"}
	cfg.Compression.Algorithms = []string{"gzip"}
	disabled := false
	cfg.Paths = []PathConfig{{
		Method: "GET",
		Path:   "/x",
		Filter: "A",
		Compression: &PathCompressionConfig{
			Enabled: &disabled,
		},
	}}
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: strings.Repeat("x", 64)}}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req.Header.Set("Accept-Encoding", "gzip")
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "rid-c2"))
	app.handleRequest(rw, req)

	if got := rw.Header().Get("Content-Encoding"); got != "" {
		t.Fatalf("unexpected content-encoding=%q", got)
	}
	if body := strings.TrimSpace(rw.Body.String()); body != strings.Repeat("x", 64) {
		t.Fatalf("body=%q", body)
	}
}
