package main

import (
	"context"
	"net"
	"net/http"
	"os"
	"testing"
)

func TestExecHTTPFilterUnixSocket(t *testing.T) {
	sock := "/tmp/gw-http-unix-test.sock"
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	}()

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})}
	defer func() { _ = srv.Shutdown(context.Background()) }()
	go func() { _ = srv.Serve(ln) }()

	e := newTestEngine()
	out, err := e.execHTTPFilter(context.Background(), FilterConfig{Params: map[string]any{
		"method":      "GET",
		"url":         "http://unix/anything",
		"unix_socket": sock,
	}}, map[string]any{})
	if err != nil {
		t.Fatalf("execHTTPFilter unix_socket err: %v", err)
	}
	m, ok := out.(map[string]any)
	if !ok {
		t.Fatalf("result type=%T", out)
	}
	if m["status"] != 200 {
		t.Fatalf("status=%v", m["status"])
	}
	body, ok := m["body"].(map[string]any)
	if !ok || body["ok"] != true {
		t.Fatalf("body=%#v", m["body"])
	}
}
