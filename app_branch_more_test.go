package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestHandleRequestTooLargeAndFilterFailureAndTimeout(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.MaxBodySize = 1
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: "ok"}}
	cfg.Paths = []PathConfig{{Method: "POST", Path: "/x", Filter: "A"}}
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	rw1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodPost, "/x", strings.NewReader("abcdef"))
	req1 = req1.WithContext(context.WithValue(req1.Context(), requestIDKey{}, "rid-tl"))
	app.handleRequest(rw1, req1)
	if rw1.Code != http.StatusRequestEntityTooLarge {
		t.Fatalf("too large status=%d", rw1.Code)
	}

	cfg2 := defaultConfig()
	cfg2.Filters = []FilterConfig{{
		ID:   "B",
		Type: "base64",
		Params: map[string]any{
			"input": "%%%",
			"op":    "decode",
		},
	}}
	cfg2.Paths = []PathConfig{{Method: "GET", Path: "/e", Filter: "B"}}
	app2, err := NewApp(cfg2)
	if err != nil {
		t.Fatalf("NewApp2 err: %v", err)
	}
	rw2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/e", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), requestIDKey{}, "rid-err"))
	app2.handleRequest(rw2, req2)
	if rw2.Code != http.StatusInternalServerError {
		t.Fatalf("exec failed status=%d", rw2.Code)
	}
	var m2 map[string]any
	if err := json.Unmarshal(rw2.Body.Bytes(), &m2); err != nil {
		t.Fatalf("decode m2: %v", err)
	}
	if m2["code"] != "FILTER_EXECUTION_FAILED" {
		t.Fatalf("unexpected code: %#v", m2)
	}

	delayServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(80 * time.Millisecond)
		_, _ = w.Write([]byte("ok"))
	}))
	defer delayServer.Close()
	cfg3 := defaultConfig()
	cfg3.Server.RequestTimeout = 20 * time.Millisecond
	cfg3.Filters = []FilterConfig{{
		ID:   "H",
		Type: "http",
		Params: map[string]any{
			"method": "GET",
			"url":    delayServer.URL,
		},
	}}
	cfg3.Paths = []PathConfig{{Method: "GET", Path: "/t", Filter: "H"}}
	app3, err := NewApp(cfg3)
	if err != nil {
		t.Fatalf("NewApp3 err: %v", err)
	}
	rw3 := httptest.NewRecorder()
	req3 := httptest.NewRequest(http.MethodGet, "/t", nil)
	req3 = req3.WithContext(context.WithValue(req3.Context(), requestIDKey{}, "rid-timeout"))
	app3.handleRequest(rw3, req3)
	if rw3.Code != http.StatusInternalServerError {
		t.Fatalf("timeout status=%d", rw3.Code)
	}
	var m3 map[string]any
	if err := json.Unmarshal(rw3.Body.Bytes(), &m3); err != nil {
		t.Fatalf("decode m3: %v", err)
	}
	if m3["code"] != "REQUEST_TIMEOUT" {
		t.Fatalf("unexpected timeout code: %#v", m3)
	}
}

func TestRunTcpAndUnix(t *testing.T) {
	cfg := testAppConfig()
	cfg.Server.Network = "tcp"
	cfg.Server.Addr = "127.0.0.1:0"
	app, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp tcp err: %v", err)
	}
	ctx := context.Background()
	done := make(chan error, 1)
	go func() { done <- app.Run(ctx) }()
	time.Sleep(20 * time.Millisecond)
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown tcp err: %v", err)
	}
	if err := <-done; err != nil {
		t.Fatalf("Run tcp err: %v", err)
	}

	cfg2 := testAppConfig()
	cfg2.Server.Network = "unix"
	cfg2.Server.UnixSocket = "/tmp/gw-run-test.sock"
	cfg2.Server.UnixSocketMode = "0660"
	app2, err := NewApp(cfg2)
	if err != nil {
		t.Fatalf("NewApp unix err: %v", err)
	}
	done2 := make(chan error, 1)
	go func() { done2 <- app2.Run(context.Background()) }()
	time.Sleep(20 * time.Millisecond)
	if err := app2.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown unix err: %v", err)
	}
	if err := <-done2; err != nil {
		t.Fatalf("Run unix err: %v", err)
	}
}
