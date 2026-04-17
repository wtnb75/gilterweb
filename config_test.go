package main

import (
	"testing"
)

func TestConfigValidateCycle(t *testing.T) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "static", DependsOn: []string{"B"}, Params: "x"},
		{ID: "B", Type: "static", DependsOn: []string{"A"}, Params: "y"},
	}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "A"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected cycle detection error")
	}
}

func TestConfigValidatePathFilterMissing(t *testing.T) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{{ID: "A", Type: "static", Params: "ok"}}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "B"}}
	if err := cfg.Validate(); err == nil {
		t.Fatalf("expected undefined path filter error")
	}
}
