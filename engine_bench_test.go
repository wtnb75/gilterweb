package main

import (
	"context"
	"log/slog"
	"testing"
)

func benchEngineConfig() (Config, map[string]FilterConfig) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "static", Params: "2026-04-17T{{.req.path}}"},
		{ID: "R", Type: "regex", Params: map[string]any{
			"input":   "{{.A}}",
			"pattern": `^(?P<date>\d{4}-\d{2}-\d{2})T(?P<path>.+)$`,
			"op":      "find",
		}},
		{ID: "J", Type: "jq", Params: map[string]any{
			"input": "{{ toJson .R }}",
			"query": ".date",
		}},
	}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/", Filter: "J"}}
	idx := map[string]FilterConfig{}
	for _, f := range cfg.Filters {
		idx[f.ID] = f
	}
	return cfg, idx
}

// BenchmarkEngineExecute exercises static (template), regex, and jq filters
// together, mirroring a realistic per-request path through the engine. It
// exists to measure the cost of re-parsing templates/regex/jq queries on
// every request versus caching them (see issue #25).
func BenchmarkEngineExecute(b *testing.B) {
	cfg, idx := benchEngineConfig()
	eng := NewEngine(cfg, idx, NewTTLCache(1000), slog.New(slog.DiscardHandler))
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := eng.Execute(ctx, "J", map[string]any{"req": map[string]any{"path": "/x"}}); err != nil {
			b.Fatalf("execute: %v", err)
		}
	}
}
