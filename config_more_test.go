package main

import (
	"os"
	"path/filepath"
	"testing"
)

func writeTestConfigFile(t *testing.T, body string) string {
	t.Helper()
	d := t.TempDir()
	p := filepath.Join(d, "config.yaml")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatalf("write config: %v", err)
	}
	return p
}

func TestLoadConfigSuccessAndFailures(t *testing.T) {
	okPath := writeTestConfigFile(t, `server:
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
	cfg, err := LoadConfig(okPath)
	if err != nil {
		t.Fatalf("LoadConfig success err: %v", err)
	}
	if len(cfg.Filters) != 1 || cfg.Filters[0].ID != "A" {
		t.Fatalf("unexpected cfg: %#v", cfg)
	}

	if _, err := LoadConfig(filepath.Join(t.TempDir(), "missing.yaml")); err == nil {
		t.Fatalf("expected missing file error")
	}

	badPath := writeTestConfigFile(t, "server: [")
	if _, err := LoadConfig(badPath); err == nil {
		t.Fatalf("expected parse error")
	}
}

func TestValidateMoreBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.Network = "bad"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected invalid network error")
	}

	cfg = defaultConfig()
	cfg.Filters = []FilterConfig{{ID: "A", Type: "weird", Params: "x"}}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "A"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected unsupported filter type error")
	}

	cfg = defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "static", Params: "x"},
		{ID: "A", Type: "static", Params: "y"},
	}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "A"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected duplicate id error")
	}
}
