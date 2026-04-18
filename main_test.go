package main

import (
	"errors"
	"testing"
)

func TestApplyLogLevelOverride(t *testing.T) {
	cfg := defaultConfig()
	if err := applyLogLevelOverride(&cfg, ""); err != nil {
		t.Fatalf("empty override should pass: %v", err)
	}
	if got := cfg.Log.Level; got != "info" {
		t.Fatalf("log level changed unexpectedly: %s", got)
	}
	if err := applyLogLevelOverride(&cfg, "debug"); err != nil {
		t.Fatalf("debug override failed: %v", err)
	}
	if cfg.Log.Level != "debug" {
		t.Fatalf("log level not updated: %s", cfg.Log.Level)
	}
	if err := applyLogLevelOverride(&cfg, "verbose"); err == nil {
		t.Fatalf("invalid override should fail")
	}
}

func TestExitErrorHelpers(t *testing.T) {
	empty := ExitError{}
	if empty.Error() != "" {
		t.Fatalf("empty ExitError should render empty string")
	}

	base := errors.New("boom")
	ex := ExitError{Code: 2, Err: base}
	if ex.Error() != "boom" {
		t.Fatalf("Error()=%q", ex.Error())
	}
	if !errors.Is(ex, base) {
		t.Fatalf("unwrap should expose base error")
	}
	var out ExitError
	if !AsExitError(ex, &out) {
		t.Fatalf("AsExitError should succeed")
	}
	if out.Code != 2 || !errors.Is(out, base) {
		t.Fatalf("unexpected out: %+v", out)
	}
	if AsExitError(errors.New("x"), &out) {
		t.Fatalf("AsExitError should fail for non-ExitError")
	}
	if AsExitError(nil, &out) {
		t.Fatalf("AsExitError should fail for nil")
	}
}

func TestNewRootCmdHasSubcommands(t *testing.T) {
	cmd := newRootCmd()
	names := map[string]bool{}
	for _, c := range cmd.Commands() {
		names[c.Name()] = true
	}
	for _, want := range []string{"server", "check", "validate", "init-config", "version"} {
		if !names[want] {
			t.Fatalf("missing subcommand %q", want)
		}
	}
}

func TestCheckCmdRequiresPath(t *testing.T) {
	cfgPath := "config.yaml"
	logLevel := ""
	cmd := newCheckCmd(&cfgPath, &logLevel)
	cmd.SetArgs([]string{"--method", "GET"})
	if err := cmd.Execute(); err == nil {
		t.Fatalf("expected --path required error")
	}
}
