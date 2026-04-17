package main

import (
	"log/slog"
	"net/http"
	"net/http/httptest"
	"regexp"
	"testing"
)

func TestWithAccessLogUsesClientRequestID(t *testing.T) {
	app := &App{logger: slog.New(slog.DiscardHandler)}
	const wantID = "client-provided-id-123"
	var gotCtxID string
	h := app.withAccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCtxID = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	req.Header.Set("X-Request-ID", wantID)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	if got := rw.Header().Get("X-Request-ID"); got != wantID {
		t.Fatalf("response X-Request-ID = %q, want %q", got, wantID)
	}
	if gotCtxID != wantID {
		t.Fatalf("context request id = %q, want %q", gotCtxID, wantID)
	}
}

func TestWithAccessLogGeneratesUUIDv4(t *testing.T) {
	app := &App{logger: slog.New(slog.DiscardHandler)}
	var gotCtxID string
	h := app.withAccessLog(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotCtxID = requestIDFromContext(r.Context())
		w.WriteHeader(http.StatusNoContent)
	}))

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rw := httptest.NewRecorder()
	h.ServeHTTP(rw, req)

	got := rw.Header().Get("X-Request-ID")
	if got == "" {
		t.Fatalf("response X-Request-ID is empty")
	}
	if gotCtxID != got {
		t.Fatalf("context request id = %q, want %q", gotCtxID, got)
	}
	pattern := `^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`
	if ok, _ := regexp.MatchString(pattern, got); !ok {
		t.Fatalf("generated request id = %q, not UUIDv4", got)
	}
}
