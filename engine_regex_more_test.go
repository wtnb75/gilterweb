package main

import (
	"context"
	"testing"
)

func TestExecRegexFilterFindAllReplaceAndErrors(t *testing.T) {
	e := newTestEngine()
	data := map[string]any{}

	v, err := e.execRegexFilter(FilterConfig{Params: map[string]any{
		"input":   "a-1 b-2",
		"pattern": `(?P<w>\w)-(?P<n>\d)`,
		"op":      "find_all",
	}}, data)
	if err != nil {
		t.Fatalf("find_all err: %v", err)
	}
	arr, ok := v.([]any)
	if !ok || len(arr) != 2 {
		t.Fatalf("find_all result=%#v", v)
	}

	v, err = e.execRegexFilter(FilterConfig{Params: map[string]any{
		"input":   "a-1",
		"pattern": `(?P<w>\w)-(?P<n>\d)`,
		"op":      "replace",
		"replace": "$w:$n",
	}}, data)
	if err != nil || v != "a:1" {
		t.Fatalf("replace result=%v err=%v", v, err)
	}

	_, err = e.execRegexFilter(FilterConfig{Params: map[string]any{
		"input":   "x",
		"pattern": "[",
		"op":      "find",
	}}, data)
	if err == nil {
		t.Fatalf("expected regexp compile error")
	}

	_, err = e.execRegexFilter(FilterConfig{Params: map[string]any{
		"input":   "x",
		"pattern": "x",
		"op":      "replace",
	}}, data)
	if err == nil {
		t.Fatalf("expected replace required error")
	}

	_, err = e.execRegexFilter(FilterConfig{Params: map[string]any{
		"input":   "x",
		"pattern": "x",
		"op":      "unknown",
	}}, data)
	if err == nil {
		t.Fatalf("expected invalid op error")
	}
}

func TestExecFilterDispatchAndErrors(t *testing.T) {
	e := newTestEngine()
	data := map[string]any{"req": map[string]any{"path": "/x"}}

	if _, err := e.execFilter(context.Background(), FilterConfig{Type: "unknown", Params: nil}, data); err == nil {
		t.Fatalf("expected unsupported filter type error")
	}
	if _, err := e.execFilter(context.Background(), FilterConfig{
		Type:   "env",
		Params: map[string]any{"name": "PATH"},
	}, data); err != nil {
		t.Fatalf("env dispatch err=%v", err)
	}
	if _, err := e.execFilter(context.Background(), FilterConfig{
		Type:   "base64",
		Params: map[string]any{"input": "a", "op": "encode"},
	}, data); err != nil {
		t.Fatalf("base64 dispatch err=%v", err)
	}
	if _, err := e.execFilter(context.Background(), FilterConfig{
		Type:   "jq",
		Params: map[string]any{"input": `{"a":1}`, "query": ".a"},
	}, data); err != nil {
		t.Fatalf("jq dispatch err=%v", err)
	}
}
