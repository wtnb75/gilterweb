package main

import (
	"strings"
	"testing"
)

func TestValidateFilterParams(t *testing.T) {
	ids := map[string]bool{"OTHER": true}
	cases := []struct {
		name    string
		f       FilterConfig
		wantErr string // substring; empty means no error expected
	}{
		{"static allows any value", FilterConfig{ID: "S", Type: "static", Params: 123}, ""},

		{"env missing name", FilterConfig{ID: "E", Type: "env", Params: map[string]any{}}, "env.name required"},
		{"env not object", FilterConfig{ID: "E", Type: "env", Params: "bad"}, "env params must be object"},
		{"env ok", FilterConfig{ID: "E", Type: "env", Params: map[string]any{"name": "HOME"}}, ""},

		{"http missing url", FilterConfig{ID: "H", Type: "http", Params: map[string]any{}}, "http.url required"},
		{
			"http empty url",
			FilterConfig{ID: "H", Type: "http", Params: map[string]any{"url": ""}},
			"http.url required",
		},
		{"http not object", FilterConfig{ID: "H", Type: "http", Params: "bad"}, "http params must be object"},
		{
			"http ok",
			FilterConfig{ID: "H", Type: "http", Params: map[string]any{"url": "https://example.com"}},
			"",
		},

		{
			"exec missing command",
			FilterConfig{ID: "X", Type: "exec", Params: map[string]any{}},
			"exec.command required",
		},
		{
			"exec empty command",
			FilterConfig{ID: "X", Type: "exec", Params: map[string]any{"command": []any{}}},
			"exec.command required",
		},
		{
			"exec non-string element",
			FilterConfig{ID: "X", Type: "exec", Params: map[string]any{"command": []any{"ls", 1}}},
			"exec.command",
		},
		{"exec not object", FilterConfig{ID: "X", Type: "exec", Params: "bad"}, "exec params must be object"},
		{"exec ok", FilterConfig{ID: "X", Type: "exec", Params: map[string]any{"command": []any{"ls"}}}, ""},

		{"file not object", FilterConfig{ID: "F", Type: "file", Params: "bad"}, "file params must be object"},
		{"file missing path", FilterConfig{ID: "F", Type: "file", Params: map[string]any{}}, "file.path required"},
		{"file ok", FilterConfig{ID: "F", Type: "file", Params: map[string]any{"path": "/tmp/x"}}, ""},

		{"jq not object", FilterConfig{ID: "J", Type: "jq", Params: "bad"}, "jq params must be object"},
		{"jq missing query", FilterConfig{ID: "J", Type: "jq", Params: map[string]any{}}, "jq.query required"},
		{"jq ok", FilterConfig{ID: "J", Type: "jq", Params: map[string]any{"query": "."}}, ""},

		{"base64 not object", FilterConfig{ID: "B", Type: "base64", Params: "bad"}, "base64 params must be object"},
		{
			"base64 bad op",
			FilterConfig{ID: "B", Type: "base64", Params: map[string]any{"op": "bogus"}},
			"base64 op must be encode|decode",
		},
		{"base64 ok encode", FilterConfig{ID: "B", Type: "base64", Params: map[string]any{"op": "encode"}}, ""},
		{"base64 ok decode", FilterConfig{ID: "B", Type: "base64", Params: map[string]any{"op": "decode"}}, ""},

		{"regex not object", FilterConfig{ID: "R", Type: "regex", Params: "bad"}, "regex params must be object"},
		{
			"regex bad op",
			FilterConfig{ID: "R", Type: "regex", Params: map[string]any{"op": "bogus", "pattern": "a"}},
			"regex op must be find|find_all|replace",
		},
		{
			"regex bad pattern",
			FilterConfig{ID: "R", Type: "regex", Params: map[string]any{"op": "find", "pattern": "("}},
			"regex.pattern",
		},
		{
			"regex replace missing replace",
			FilterConfig{ID: "R", Type: "regex", Params: map[string]any{"op": "replace", "pattern": "a"}},
			"replace required",
		},
		{"regex ok", FilterConfig{ID: "R", Type: "regex", Params: map[string]any{"op": "find", "pattern": "a"}}, ""},
		{
			"regex ok multiline",
			FilterConfig{ID: "R", Type: "regex", Params: map[string]any{"op": "find", "pattern": "a", "multiline": true}},
			"",
		},

		{"cache not object", FilterConfig{ID: "C", Type: "cache", Params: "bad"}, "cache params must be object"},
		{
			"cache missing filter",
			FilterConfig{ID: "C", Type: "cache", Params: map[string]any{}},
			"cache.filter required",
		},
		{
			"cache unknown filter",
			FilterConfig{ID: "C", Type: "cache", Params: map[string]any{"filter": "NOPE"}},
			"cache.filter",
		},
		{
			"cache bad ttl",
			FilterConfig{ID: "C", Type: "cache", Params: map[string]any{"filter": "OTHER", "ttl": "bogus"}},
			"cache.ttl",
		},
		{"cache ok", FilterConfig{ID: "C", Type: "cache", Params: map[string]any{"filter": "OTHER"}}, ""},
		{
			"cache ok with ttl",
			FilterConfig{ID: "C", Type: "cache", Params: map[string]any{"filter": "OTHER", "ttl": "5m"}},
			"",
		},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			err := validateFilterParams(c.f, ids)
			if c.wantErr == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), c.wantErr) {
				t.Fatalf("error = %v, want containing %q", err, c.wantErr)
			}
		})
	}
}

func TestConfigValidateCatchesFilterParamErrors(t *testing.T) {
	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{{ID: "H", Type: "http", Params: map[string]any{}}}
	cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "H"}}
	err := cfg.Validate()
	if err == nil || !strings.Contains(err.Error(), "http.url required") {
		t.Fatalf("expected http.url required error, got %v", err)
	}
	if !strings.Contains(err.Error(), "filter 'H'") {
		t.Fatalf("expected error to name the filter id, got %v", err)
	}
}
