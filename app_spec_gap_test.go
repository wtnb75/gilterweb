package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRequestMergesFilterAndRouteHeaders(t *testing.T) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{{
		ID:   "A",
		Type: "static",
		Params: map[string]any{
			"headers": map[string]any{
				"X-From-Filter": "yes",
				"X-Override":    "filter",
			},
			"message": "ok",
		},
	}}
	cfg.Paths = []PathConfig{{
		Method: "GET",
		Path:   "/x",
		Filter: "A",
		Headers: map[string]string{
			"X-Route":    "yes",
			"X-Override": "route",
		},
	}}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "rid-h"))
	app.handleRequest(rw, req)

	if got := rw.Header().Get("X-From-Filter"); got != "yes" {
		t.Fatalf("X-From-Filter=%q", got)
	}
	if got := rw.Header().Get("X-Route"); got != "yes" {
		t.Fatalf("X-Route=%q", got)
	}
	if got := rw.Header().Get("X-Override"); got != "route" {
		t.Fatalf("X-Override=%q", got)
	}
}

func TestHandleRequestMapsFilterOutputTooLargeCode(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.MaxFilterOutputSize = 8
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: strings.Repeat("x", 64)}}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "A"}}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/x", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "rid-limit"))
	app.handleRequest(rw, req)

	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rw.Code)
	}
	var body map[string]any
	if err := json.Unmarshal(rw.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if body["code"] != "FILTER_OUTPUT_TOO_LARGE" {
		t.Fatalf("code=%v body=%#v", body["code"], body)
	}
}

func TestAppReloadRules(t *testing.T) {
	base := testAppConfig()
	app, err := NewApp(base)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	next := base
	next.Log.Level = "debug"
	next.Paths = []PathConfig{{Method: "GET", Path: "/y", Filter: "A"}}
	if err := app.Reload(next); err != nil {
		t.Fatalf("Reload err: %v", err)
	}
	if _, err := app.Check(context.Background(), CheckRequest{Method: "GET", Path: "/y"}); err != nil {
		t.Fatalf("Check after reload err: %v", err)
	}

	bad := next
	bad.Server.Addr = ":9999"
	if err := app.Reload(bad); err == nil {
		t.Fatalf("expected non-hot-reloadable server settings error")
	}
}
