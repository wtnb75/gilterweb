package main

import (
	"context"
	"testing"
)

func TestExpandStaticAndRenderTemplateErrorBranches(t *testing.T) {
	e := newTestEngine()
	data := map[string]any{"req": map[string]any{"x": "v"}}

	v, err := e.expandStatic(map[string]any{
		"a": "{{.req.x}}",
		"b": []any{"{{.req.x}}", 1},
	}, data, 0)
	if err != nil {
		t.Fatalf("expandStatic err: %v", err)
	}
	m := v.(map[string]any)
	if m["a"] != "v" {
		t.Fatalf("expandStatic map result=%#v", v)
	}

	if _, err := renderTemplate("{{", data, e.renderFuncs); err == nil {
		t.Fatalf("expected template parse error")
	}
	if _, err := renderTemplate(`{{ required "x" .nope }}`, data, e.renderFuncs); err == nil {
		t.Fatalf("expected template execute error")
	}
}

func TestExecFilterErrorBranches(t *testing.T) {
	e := newTestEngine()
	ctx := context.Background()

	if _, err := execEnvFilter(FilterConfig{Params: "bad"}); err == nil {
		t.Fatalf("expected env params error")
	}
	if _, err := execEnvFilter(FilterConfig{Params: map[string]any{"default": "x"}}); err == nil {
		t.Fatalf("expected env.name required error")
	}

	if _, err := e.execHTTPFilter(ctx, FilterConfig{Params: "bad"}, map[string]any{}); err == nil {
		t.Fatalf("expected http params error")
	}
	if _, err := e.execHTTPFilter(ctx, FilterConfig{Params: map[string]any{}}, map[string]any{}); err == nil {
		t.Fatalf("expected http.url required error")
	}

	if _, err := e.execExecFilter(ctx, FilterConfig{Params: "bad"}, map[string]any{}); err == nil {
		t.Fatalf("expected exec params error")
	}
	if _, err := e.execExecFilter(ctx, FilterConfig{Params: map[string]any{}}, map[string]any{}); err == nil {
		t.Fatalf("expected exec.command required error")
	}
	if _, err := e.execExecFilter(ctx, FilterConfig{Params: map[string]any{
		"command": []any{"echo", "ok"},
		"timeout": "bad",
	}}, map[string]any{}); err == nil {
		t.Fatalf("expected exec timeout parse error")
	}
	if out, err := e.execExecFilter(ctx, FilterConfig{Params: map[string]any{
		"command": []any{"sh", "-c", "exit 7"},
	}}, map[string]any{}); err != nil {
		t.Fatalf("unexpected exec exit err: %v", err)
	} else if out.(map[string]any)["code"] != 7 {
		t.Fatalf("expected exit code 7, got: %#v", out)
	}
	if out, err := e.execExecFilter(ctx, FilterConfig{Params: map[string]any{
		"command": []any{"sh", "-c", "sleep 1"},
		"timeout": "5ms",
	}}, map[string]any{}); err != nil {
		t.Fatalf("unexpected exec timeout err: %v", err)
	} else {
		m := out.(map[string]any)
		if m["timeout"] != true || m["code"] != -1 {
			t.Fatalf("expected timeout result, got: %#v", out)
		}
	}

	if _, err := e.execFileFilter(FilterConfig{Params: "bad"}); err == nil {
		t.Fatalf("expected file params error")
	}
	if _, err := e.execFileFilter(FilterConfig{Params: map[string]any{}}); err == nil {
		t.Fatalf("expected file.path required error")
	}

	if _, err := e.execJQFilter(FilterConfig{Params: "bad"}, map[string]any{}); err == nil {
		t.Fatalf("expected jq params error")
	}
	if _, err := e.execJQFilter(FilterConfig{Params: map[string]any{"input": "{}"}}, map[string]any{}); err == nil {
		t.Fatalf("expected jq.query required error")
	}
	if _, err := e.execJQFilter(FilterConfig{
		Params: map[string]any{"input": "x", "query": ".a"},
	}, map[string]any{}); err == nil {
		t.Fatalf("expected jq input unmarshal error")
	}
	if _, err := e.execJQFilter(FilterConfig{
		Params: map[string]any{"input": "{}", "query": "["},
	}, map[string]any{}); err == nil {
		t.Fatalf("expected jq parse error")
	}
	if v, err := e.execJQFilter(FilterConfig{
		Params: map[string]any{"input": "{}", "query": "empty"},
	}, map[string]any{}); err != nil || v != nil {
		t.Fatalf("expected jq empty => nil, got v=%v err=%v", v, err)
	}

	if _, err := e.execBase64Filter(FilterConfig{Params: "bad"}, map[string]any{}); err == nil {
		t.Fatalf("expected base64 params error")
	}
	if _, err := e.execBase64Filter(FilterConfig{
		Params: map[string]any{"input": "x", "op": "bad"},
	}, map[string]any{}); err == nil {
		t.Fatalf("expected base64 op error")
	}

	if _, err := e.execCacheFilter(ctx, FilterConfig{Params: "bad"}, map[string]any{}); err == nil {
		t.Fatalf("expected cache params error")
	}
	if _, err := e.execCacheFilter(ctx, FilterConfig{Params: map[string]any{}}, map[string]any{}); err == nil {
		t.Fatalf("expected cache.filter required error")
	}
	if _, err := e.execCacheFilter(ctx, FilterConfig{
		Params: map[string]any{"filter": "A", "ttl": "bad"},
	}, map[string]any{}); err == nil {
		t.Fatalf("expected cache ttl parse error")
	}

	if m := buildRegexMap(regexpMustCompile(`x`), "y"); len(m) != 0 {
		t.Fatalf("expected empty regex map, got: %#v", m)
	}
	if tooLarge(make(chan int), 1) {
		t.Fatalf("marshal error path should return false")
	}
}
