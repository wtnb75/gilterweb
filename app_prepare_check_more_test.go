package main

import (
	"context"
	"net"
	"os"
	"strings"
	"testing"
)

func TestPrepareUnixListenerInUseAndStale(t *testing.T) {
	sock := "/tmp/gw-inuse-test.sock"
	_ = os.Remove(sock)
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}
	defer func() {
		_ = ln.Close()
		_ = os.Remove(sock)
	}()

	if _, err := prepareUnixListener(sock); err == nil || !strings.Contains(err.Error(), "already in use") {
		t.Fatalf("expected already in use error, got: %v", err)
	}

	_ = ln.Close()
	ln2, err := prepareUnixListener(sock)
	if err != nil {
		t.Fatalf("prepare stale socket err: %v", err)
	}
	defer func() { _ = ln2.Close() }()
}

func TestCheckErrorBranches(t *testing.T) {
	app, err := NewApp(testAppConfig())
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}

	_, err = app.Check(context.Background(), CheckRequest{Method: "GET", Path: "/x", Headers: []string{"bad-header"}})
	if err == nil || !strings.Contains(err.Error(), "invalid --header") {
		t.Fatalf("expected invalid header error, got: %v", err)
	}

	_, err = app.Check(context.Background(), CheckRequest{Method: "GET", Path: "/x", BodyFile: "/no/such/file"})
	if err == nil || !strings.Contains(err.Error(), "read body file") {
		t.Fatalf("expected body-file error, got: %v", err)
	}

	_, err = app.Check(context.Background(), CheckRequest{Method: "GET", Path: "/missing"})
	if err == nil || !strings.Contains(err.Error(), "no matching route") {
		t.Fatalf("expected no route error, got: %v", err)
	}
}
