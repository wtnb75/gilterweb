package main

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleHealthz(t *testing.T) {
	app := &App{logger: slog.New(slog.DiscardHandler)}
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	app.handleHealthz(rw, req)
	if got := rw.Header().Get("Content-Type"); !strings.Contains(got, "application/json") {
		t.Fatalf("content-type = %q", got)
	}
	if body := strings.TrimSpace(rw.Body.String()); body != `{"status":"ok"}` {
		t.Fatalf("body = %q", body)
	}
}

func TestWriteResultAndError(t *testing.T) {
	rw1 := httptest.NewRecorder()
	writeResult(rw1, "hello")
	if !strings.Contains(rw1.Header().Get("Content-Type"), "text/plain") {
		t.Fatalf("unexpected content-type: %s", rw1.Header().Get("Content-Type"))
	}
	if strings.TrimSpace(rw1.Body.String()) != "hello" {
		t.Fatalf("unexpected body: %q", rw1.Body.String())
	}

	rw2 := httptest.NewRecorder()
	writeResult(rw2, map[string]any{"a": 1})
	if !strings.Contains(rw2.Header().Get("Content-Type"), "application/json") {
		t.Fatalf("unexpected content-type: %s", rw2.Header().Get("Content-Type"))
	}

	rw3 := httptest.NewRecorder()
	writeError(rw3, http.StatusBadRequest, "BAD", "rid-1")
	if got := rw3.Header().Get("X-Request-ID"); got != "rid-1" {
		t.Fatalf("x-request-id = %q", got)
	}
	var m map[string]any
	if err := json.Unmarshal(rw3.Body.Bytes(), &m); err != nil {
		t.Fatalf("decode error body: %v", err)
	}
	if m["code"] != "BAD" || m["request_id"] != "rid-1" {
		t.Fatalf("error body: %#v", m)
	}
}

func TestWithRecoveryAndRenderResultAndTTLCache(t *testing.T) {
	app := &App{logger: slog.New(slog.DiscardHandler)}
	h := app.withRecovery(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		panic("boom")
	}))
	rw := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req = req.WithContext(context.WithValue(req.Context(), requestIDKey{}, "rid-x"))
	h.ServeHTTP(rw, req)
	if rw.Code != http.StatusInternalServerError {
		t.Fatalf("status=%d", rw.Code)
	}

	if got := RenderResult("x"); got != "x" {
		t.Fatalf("RenderResult string = %q", got)
	}
	if got := RenderResult(map[string]any{"a": 1}); !strings.Contains(got, "\"a\":1") {
		t.Fatalf("RenderResult map = %q", got)
	}

	c := NewTTLCache()
	c.Set("k", 20*time.Millisecond, "v")
	if v, ok := c.Get("k"); !ok || v != "v" {
		t.Fatalf("cache get immediately failed: v=%v ok=%v", v, ok)
	}
	time.Sleep(30 * time.Millisecond)
	if _, ok := c.Get("k"); ok {
		t.Fatalf("cache entry should expire")
	}
}
