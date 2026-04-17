package main

import (
	"io"
	"os"
	"strings"
	"testing"
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
	if !strings.Contains(out, "validation succeeded") {
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
