package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	orig := os.Stdout
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	os.Stdout = w
	defer func() { os.Stdout = orig }()

	fn()
	_ = w.Close()
	b, err := io.ReadAll(r)
	if err != nil {
		t.Fatalf("read stdout: %v", err)
	}
	_ = r.Close()
	return string(b)
}

func TestValidateCmdAndCheckCmdBranches(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `server:
  network: tcp
  addr: ":8080"
filters:
  - id: A
    type: static
    params: "ok"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /x
    filter: A
`)

	logLevel := ""
	validate := newValidateCmd(&cfgPath, &logLevel)
	out := captureStdout(t, func() {
		if err := validate.Execute(); err != nil {
			t.Fatalf("validate execute err: %v", err)
		}
	})
	if !strings.Contains(out, "config validation succeeded") {
		t.Fatalf("validate output missing success: %q", out)
	}
	if !strings.Contains(out, "execution plan:") {
		t.Fatalf("validate output missing plan header: %q", out)
	}
	if !strings.Contains(out, "- GET /x -> A") {
		t.Fatalf("validate output missing path info: %q", out)
	}
	if !strings.Contains(out, "filters: A") {
		t.Fatalf("validate output missing filters list: %q", out)
	}

	badLevel := "verbose"
	validateBad := newValidateCmd(&cfgPath, &badLevel)
	if err := validateBad.Execute(); err == nil {
		t.Fatalf("expected validate bad log-level error")
	}

	check := newCheckCmd(&cfgPath, &logLevel)
	check.SetArgs([]string{"--path", "/x"})
	if err := check.Execute(); err != nil {
		t.Fatalf("check success err: %v", err)
	}

	checkNoRoute := newCheckCmd(&cfgPath, &logLevel)
	checkNoRoute.SetArgs([]string{"--path", "/no-route"})
	err := checkNoRoute.Execute()
	if err == nil {
		t.Fatalf("expected no-route error")
	}
	var ex ExitError
	if !AsExitError(err, &ex) || ex.Code != 2 {
		t.Fatalf("expected ExitError code 2, got: %#v", err)
	}

	missing := "/no/such/file.yaml"
	checkMissing := newCheckCmd(&missing, &logLevel)
	checkMissing.SetArgs([]string{"--path", "/x"})
	err = checkMissing.Execute()
	if err == nil {
		t.Fatalf("expected missing config error")
	}
	if !AsExitError(err, &ex) || ex.Code != 1 {
		t.Fatalf("expected ExitError code 1, got: %#v", err)
	}
}

func TestValidateShowsDependencyExpandedFilters(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `server:
  network: tcp
  addr: ":8080"
filters:
  - id: A
    type: static
    params: "ok"
  - id: B
    type: static
    params: "{{ .A }}-b"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /dep
    filter: B
`)
	logLevel := ""
	validate := newValidateCmd(&cfgPath, &logLevel)
	out := captureStdout(t, func() {
		if err := validate.Execute(); err != nil {
			t.Fatalf("validate execute err: %v", err)
		}
	})
	if !strings.Contains(out, "- GET /dep -> B") {
		t.Fatalf("path line missing: %q", out)
	}
	if !strings.Contains(out, "filters: A, B") {
		t.Fatalf("dependency-expanded filter list missing: %q", out)
	}
}

func TestVersionAndServerCmdBranches(t *testing.T) {
	cmd := newVersionCmd()
	if cmd.Run == nil {
		t.Fatalf("version run func is nil")
	}
	cmd.Run(cmd, nil)

	cfgPath := "/no/such/file.yaml"
	logLevel := ""
	server := newServerCmd(&cfgPath, &logLevel)
	err := server.Execute()
	if err == nil || !strings.Contains(err.Error(), "read config") {
		t.Fatalf("expected read config error, got: %v", err)
	}

	validCfg := writeTestConfigFile(t, `server:
  network: tcp
  addr: ":8080"
filters:
  - id: A
    type: static
    params: "ok"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /x
    filter: A
`)
	badLevel := "verbose"
	serverBadLevel := newServerCmd(&validCfg, &badLevel)
	err = serverBadLevel.Execute()
	if err == nil || !strings.Contains(err.Error(), "invalid --log-level") {
		t.Fatalf("expected invalid log-level error, got: %v", err)
	}

	goodLevel := ""
	serverRunErr := newServerCmd(&validCfg, &goodLevel)
	serverRunErr.SetArgs([]string{"--addr", "bad-addr"})
	err = serverRunErr.Execute()
	if err == nil || !strings.Contains(err.Error(), "listen tcp") {
		t.Fatalf("expected server run error, got: %v", err)
	}
}

func TestValidateCmdCheckHealthzSuccess(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}

	body := "server:\n" +
		"  network: tcp\n" +
		"  addr: \"" + u.Host + "\"\n" +
		"filters:\n" +
		"  - id: A\n" +
		"    type: static\n" +
		"    params: \"ok\"\n" +
		"log:\n" +
		"  level: info\n" +
		"  format: json\n" +
		"paths:\n" +
		"  - method: GET\n" +
		"    path: /x\n" +
		"    filter: A\n"
	cfgPath := writeTestConfigFile(t, body)
	logLevel := ""
	validate := newValidateCmd(&cfgPath, &logLevel)
	validate.SetArgs([]string{"--check-healthz"})
	out := captureStdout(t, func() {
		if err := validate.Execute(); err != nil {
			t.Fatalf("validate with healthz err: %v", err)
		}
	})
	if !strings.Contains(out, "healthz check succeeded") {
		t.Fatalf("healthz success output missing: %q", out)
	}
}

func TestValidateCmdCheckHealthzFailure(t *testing.T) {
	cfgPath := writeTestConfigFile(t, `server:
  network: tcp
  addr: "127.0.0.1:1"
filters:
  - id: A
    type: static
    params: "ok"
log:
  level: info
  format: json
paths:
  - method: GET
    path: /x
    filter: A
`)
	logLevel := ""
	validate := newValidateCmd(&cfgPath, &logLevel)
	validate.SetArgs([]string{"--check-healthz", "--healthz-timeout", "200ms"})
	err := validate.Execute()
	if err == nil {
		t.Fatalf("expected healthz failure")
	}
	if !strings.Contains(err.Error(), "healthz check failed") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestValidateCmdCheckHealthzUnixSuccess(t *testing.T) {
	sock := "/tmp/gilterweb-healthz-" + strconv.FormatInt(time.Now().UnixNano(), 10) + ".sock"
	ln, err := net.Listen("unix", sock)
	if err != nil {
		t.Fatalf("listen unix: %v", err)
	}

	srv := &http.Server{Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})}
	go func() {
		_ = srv.Serve(ln)
	}()
	t.Cleanup(func() {
		_ = srv.Shutdown(context.Background())
		_ = ln.Close()
		_ = os.Remove(sock)
	})

	body := "server:\n" +
		"  network: unix\n" +
		"  unix_socket: \"" + sock + "\"\n" +
		"filters:\n" +
		"  - id: A\n" +
		"    type: static\n" +
		"    params: \"ok\"\n" +
		"log:\n" +
		"  level: info\n" +
		"  format: json\n" +
		"paths:\n" +
		"  - method: GET\n" +
		"    path: /x\n" +
		"    filter: A\n"
	cfgPath := writeTestConfigFile(t, body)
	logLevel := ""
	validate := newValidateCmd(&cfgPath, &logLevel)
	validate.SetArgs([]string{"--check-healthz"})
	out := captureStdout(t, func() {
		if err := validate.Execute(); err != nil {
			t.Fatalf("validate unix healthz err: %v", err)
		}
	})
	if !strings.Contains(out, "healthz check succeeded") {
		t.Fatalf("healthz unix success output missing: %q", out)
	}
}

func TestNewHealthzClientAndURLDisablesProxy(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.Network = "tcp"
	cfg.Server.Addr = "127.0.0.1:8080"
	client, _, err := newHealthzClientAndURL(cfg, 500*time.Millisecond)
	if err != nil {
		t.Fatalf("newHealthzClientAndURL err: %v", err)
	}
	tr, ok := client.Transport.(*http.Transport)
	if !ok {
		t.Fatalf("unexpected transport type: %T", client.Transport)
	}
	if tr.Proxy != nil {
		t.Fatalf("proxy must be disabled for check-healthz")
	}
}

func TestNewHealthzClientAndURLErrors(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.Network = "tcp"
	cfg.Server.Addr = "bad-addr"
	if _, _, err := newHealthzClientAndURL(cfg, 500*time.Millisecond); err == nil {
		t.Fatalf("expected invalid server.addr error")
	}

	cfg = defaultConfig()
	cfg.Server.Network = "unix"
	cfg.Server.UnixSocket = ""
	if _, _, err := newHealthzClientAndURL(cfg, 500*time.Millisecond); err == nil {
		t.Fatalf("expected missing unix socket error")
	}

	cfg = defaultConfig()
	cfg.Server.Network = "weird"
	if _, _, err := newHealthzClientAndURL(cfg, 500*time.Millisecond); err == nil {
		t.Fatalf("expected invalid network error")
	}
}

func TestCheckHealthzEndpointStatusFailure(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/healthz" {
			http.NotFound(w, r)
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
	}))
	t.Cleanup(ts.Close)

	u, err := url.Parse(ts.URL)
	if err != nil {
		t.Fatalf("parse test server url: %v", err)
	}
	cfg := defaultConfig()
	cfg.Server.Network = "tcp"
	cfg.Server.Addr = u.Host

	err = checkHealthzEndpoint(cfg, 500*time.Millisecond)
	if err == nil {
		t.Fatalf("expected status failure")
	}
	if !strings.Contains(err.Error(), "unexpected status") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestInitConfigCmdWritesFile(t *testing.T) {
	out := filepath.Join(t.TempDir(), "config.yaml")
	cmd := newInitConfigCmd()
	cmd.SetArgs([]string{"--output", out})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("init-config err: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	s := string(b)
	if !strings.HasPrefix(s, "# yaml-language-server: $schema=") {
		t.Fatalf("schema comment missing: %q", s)
	}
	if !strings.Contains(s, configSchemaURL) {
		t.Fatalf("schema url missing: %q", s)
	}
	if !strings.Contains(s, "server:") || !strings.Contains(s, "id: HELLO") {
		t.Fatalf("generated config content is unexpected: %q", s)
	}
	if !strings.Contains(s, "path: /hello") {
		t.Fatalf("generated config content is unexpected: %q", s)
	}
}

func TestInitConfigCmdRefusesOverwriteWithoutForce(t *testing.T) {
	out := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(out, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}
	cmd := newInitConfigCmd()
	cmd.SetArgs([]string{"--output", out})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected overwrite refusal")
	}
}

func TestInitConfigCmdForceOverwriteAndStdout(t *testing.T) {
	out := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(out, []byte("stale\n"), 0o644); err != nil {
		t.Fatalf("write stale file: %v", err)
	}

	cmd := newInitConfigCmd()
	cmd.SetArgs([]string{"--output", out, "--force"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("force overwrite err: %v", err)
	}
	b, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("read generated config: %v", err)
	}
	if !strings.Contains(string(b), configSchemaURL) {
		t.Fatalf("schema url missing after overwrite")
	}

	stdoutCmd := newInitConfigCmd()
	stdoutCmd.SetArgs([]string{"--output", "-"})
	outText := captureStdout(t, func() {
		if err := stdoutCmd.Execute(); err != nil {
			t.Fatalf("stdout output err: %v", err)
		}
	})
	if !strings.Contains(outText, configSchemaURL) {
		t.Fatalf("stdout output missing schema url: %q", outText)
	}
}
