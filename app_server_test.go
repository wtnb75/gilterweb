package main

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"
)

func testAppConfig() Config {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: "ok"}}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "A"}}
	return cfg
}

func TestNewAppCheckAndHandleRequest(t *testing.T) {
	app, err := NewApp(testAppConfig())
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	out, err := app.Check(context.Background(), CheckRequest{Method: "GET", Path: "/x"})
	if err != nil || out != "ok" {
		t.Fatalf("Check out=%v err=%v", out, err)
	}

	rw1 := httptest.NewRecorder()
	req1 := httptest.NewRequest(http.MethodGet, "/not-found", nil)
	req1 = req1.WithContext(context.WithValue(req1.Context(), requestIDKey{}, "rid-a"))
	app.handleRequest(rw1, req1)
	if rw1.Code != http.StatusNotFound {
		t.Fatalf("not found status=%d", rw1.Code)
	}

	rw2 := httptest.NewRecorder()
	req2 := httptest.NewRequest(http.MethodGet, "/x", nil)
	req2 = req2.WithContext(context.WithValue(req2.Context(), requestIDKey{}, "rid-b"))
	app.handleRequest(rw2, req2)
	if rw2.Code != http.StatusOK {
		t.Fatalf("ok status=%d", rw2.Code)
	}
	if body := strings.TrimSpace(rw2.Body.String()); body != "ok" {
		t.Fatalf("body=%q", body)
	}
}

func TestPrepareUnixListenerAndShutdown(t *testing.T) {
	sock := fmt.Sprintf("/tmp/gw-%d-%d.sock", os.Getpid(), time.Now().UnixNano())
	ln, err := prepareUnixListener(sock)
	if err != nil {
		t.Fatalf("prepareUnixListener err: %v", err)
	}
	defer func() { _ = os.Remove(sock) }()
	defer func() { _ = ln.Close() }()
	if _, err := net.Dial("unix", sock); err != nil {
		t.Fatalf("dial unix err: %v", err)
	}

	app := &App{logger: slog.New(slog.DiscardHandler), server: &http.Server{}}
	if err := app.Shutdown(context.Background()); err != nil {
		t.Fatalf("Shutdown err: %v", err)
	}
}
