package main

import "testing"

func TestValidateServerAndLogBranches(t *testing.T) {
	cfg := defaultConfig()
	cfg.Server.Network = "tcp"
	cfg.Server.Addr = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected tcp addr required")
	}

	cfg = defaultConfig()
	cfg.Server.Network = "unix"
	cfg.Server.UnixSocket = ""
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected unix socket required")
	}

	cfg = defaultConfig()
	cfg.Server.UnixSocketMode = "xyz"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected socket mode error")
	}

	cfg = defaultConfig()
	cfg.Server.RequestTimeout = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected timeout error")
	}

	cfg = defaultConfig()
	cfg.Server.MaxBodySize = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected max body size error")
	}

	cfg = defaultConfig()
	cfg.Server.MaxFilterOutputSize = 0
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected max filter output size error")
	}

	cfg = defaultConfig()
	cfg.Log.Level = "trace"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected log.level error")
	}

	cfg = defaultConfig()
	cfg.Log.Format = "xml"
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected log.format error")
	}
}
