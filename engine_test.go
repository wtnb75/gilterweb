package main

import (
	"context"
	"reflect"
	"testing"
)

func testEngineConfig() Config {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "static", Params: "2026-04-17"},
		{
			ID:   "R",
			Type: "regex",
			Params: map[string]any{
				"input":   "{{.A}}",
				"pattern": "^(?P<year>\\d{4})-(?P<month>\\d{2})-(?P<day>\\d{2})$",
				"op":      "find",
			},
		},
	}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/", Filter: "R"}}
	return cfg
}

func TestRegexNamedAndNumberedGroups(t *testing.T) {
	cfg := testEngineConfig()
	if err := cfg.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
	idx := map[string]FilterConfig{}
	for _, f := range cfg.Filters {
		idx[f.ID] = f
	}
	eng := NewEngine(cfg, idx, NewTTLCache(), nil)
	v, err := eng.Execute(context.Background(), "R", map[string]any{"req": map[string]any{}})
	if err != nil {
		t.Fatalf("execute: %v", err)
	}
	m, ok := v.(map[string]any)
	if !ok {
		t.Fatalf("result type = %T", v)
	}
	if m["year"] != "2026" || m["1"] != "2026" {
		t.Fatalf("unexpected year groups: %+v", m)
	}
}

func TestStaticDepthLimit(t *testing.T) {
	cfg := defaultConfig()
	// Deeply nested map exceeding static expansion depth limit.
	deep := makeDeepMap(11, "{{.req.path}}")
	cfg.Filters = []FilterConfig{{ID: "S", Type: "static", Params: deep}}
	idx := map[string]FilterConfig{"S": cfg.Filters[0]}
	eng := NewEngine(cfg, idx, NewTTLCache(), nil)
	_, err := eng.Execute(context.Background(), "S", map[string]any{"req": map[string]any{"path": "/x"}})
	if err == nil {
		t.Fatalf("expected depth limit error")
	}
}

func makeDeepMap(depth int, leaf any) any {
	if depth == 0 {
		return leaf
	}
	return map[string]any{"x": makeDeepMap(depth-1, leaf)}
}

func TestCacheFilterHit(t *testing.T) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "static", Params: "value"},
		{ID: "C", Type: "cache", Params: map[string]any{"filter": "A", "ttl": "60s", "key": "{{.req.path}}"}},
	}
	idx := map[string]FilterConfig{"A": cfg.Filters[0], "C": cfg.Filters[1]}
	cache := NewTTLCache()
	eng := NewEngine(cfg, idx, cache, nil)
	v1, err := eng.Execute(context.Background(), "C", map[string]any{"req": map[string]any{"path": "/p"}})
	if err != nil {
		t.Fatalf("1st execute: %v", err)
	}
	v2, err := eng.Execute(context.Background(), "C", map[string]any{"req": map[string]any{"path": "/p"}})
	if err != nil {
		t.Fatalf("2nd execute: %v", err)
	}
	if !reflect.DeepEqual(v1, v2) {
		t.Fatalf("cache mismatch: %v vs %v", v1, v2)
	}
}
