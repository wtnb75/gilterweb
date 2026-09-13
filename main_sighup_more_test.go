package main

import (
	"bytes"
	"os"
	"strings"
	"sync"
	"syscall"
	"testing"
	"time"
)

// stderrCapture temporarily redirects the process-wide os.Stderr (where
// NewLogger always writes) so a test can assert on log output produced by
// code that does not accept an injectable logger, such as the server
// command's SIGHUP reload loop.
type stderrCapture struct {
	orig *os.File
	w    *os.File
	mu   sync.Mutex
	buf  bytes.Buffer
	done chan struct{}
}

func interceptStderr(t *testing.T) *stderrCapture {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	c := &stderrCapture{orig: os.Stderr, w: w, done: make(chan struct{})}
	os.Stderr = w
	go func() {
		defer close(c.done)
		buf := make([]byte, 4096)
		for {
			n, err := r.Read(buf)
			if n > 0 {
				c.mu.Lock()
				c.buf.Write(buf[:n])
				c.mu.Unlock()
			}
			if err != nil {
				return
			}
		}
	}()
	t.Cleanup(func() {
		os.Stderr = c.orig
		_ = c.w.Close()
		<-c.done
	})
	return c
}

func (c *stderrCapture) contains(s string) bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return strings.Contains(c.buf.String(), s)
}

func (c *stderrCapture) waitFor(t *testing.T, s string) {
	t.Helper()
	deadline := time.Now().Add(3 * time.Second)
	for time.Now().Before(deadline) {
		if c.contains(s) {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}
	c.mu.Lock()
	got := c.buf.String()
	c.mu.Unlock()
	t.Fatalf("timed out waiting for log containing %q; got:\n%s", s, got)
}

func mustSignalSelf(t *testing.T, sig syscall.Signal) {
	t.Helper()
	if err := syscall.Kill(syscall.Getpid(), sig); err != nil {
		t.Fatalf("send signal %v: %v", sig, err)
	}
}

func TestServerCmdSighupReload(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `server:
  network: tcp
  addr: "127.0.0.1:0"
filters:
  - id: A
    type: static
    params: "v1"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /x
    filter: A
`)

	logs := interceptStderr(t)

	logLevel := ""
	cmd := newServerCmd(&cfgPath, &logLevel)
	runDone := make(chan error, 1)
	go func() { runDone <- cmd.Execute() }()

	logs.waitFor(t, "server starting")

	// An invalid config on SIGHUP must be rejected and must not stop the
	// server from continuing to serve the previous config.
	if err := os.WriteFile(cfgPath, []byte("server:\n  network: bogus\n"), 0o600); err != nil {
		t.Fatalf("write invalid config: %v", err)
	}
	mustSignalSelf(t, syscall.SIGHUP)
	logs.waitFor(t, "config reload failed")

	// A valid config on SIGHUP must be applied.
	if err := os.WriteFile(cfgPath, []byte(`server:
  network: tcp
  addr: "127.0.0.1:0"
filters:
  - id: A
    type: static
    params: "v2"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /x
    filter: A
`), 0o600); err != nil {
		t.Fatalf("write updated config: %v", err)
	}
	mustSignalSelf(t, syscall.SIGHUP)
	logs.waitFor(t, "config reload succeeded")

	mustSignalSelf(t, syscall.SIGTERM)
	select {
	case err := <-runDone:
		if err != nil {
			t.Fatalf("server command returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatalf("timed out waiting for graceful shutdown")
	}
}
