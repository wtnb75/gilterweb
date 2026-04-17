package main

import (
	"strings"
	"testing"
)

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
	if err := validate.Execute(); err != nil {
		t.Fatalf("validate execute err: %v", err)
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
