package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"regexp"
	"strings"
	"testing"
)

func newTestEngine() *Engine {
	return NewEngine(defaultConfig(), map[string]FilterConfig{}, NewTTLCache(), nil)
}

func TestExecEnvFilter(t *testing.T) {
	t.Setenv("GW_ENV_KEY", "value1")
	v, err := execEnvFilter(FilterConfig{Params: map[string]any{"name": "GW_ENV_KEY"}})
	if err != nil || v != "value1" {
		t.Fatalf("env existing v=%v err=%v", v, err)
	}
	v, err = execEnvFilter(FilterConfig{Params: map[string]any{"name": "NO_SUCH_ENV", "default": "d"}})
	if err != nil || v != "d" {
		t.Fatalf("env default v=%v err=%v", v, err)
	}
}

func TestExecHTTPFilter(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("X-Echo", r.Header.Get("X-Test"))
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	defer ts.Close()

	e := newTestEngine()
	f := FilterConfig{Params: map[string]any{
		"method": "POST",
		"url":    ts.URL,
		"body":   `{{.req.path}}`,
		"headers": map[string]any{
			"X-Test": "abc",
		},
	}}
	out, err := e.execHTTPFilter(context.Background(), f, map[string]any{"req": map[string]any{"path": "/x"}})
	if err != nil {
		t.Fatalf("execHTTPFilter err: %v", err)
	}
	m := out.(map[string]any)
	if m["status"] != 200 {
		t.Fatalf("status = %#v", m["status"])
	}
	headers := m["headers"].(map[string]string)
	if headers["X-Echo"] != "abc" {
		t.Fatalf("headers = %#v", headers)
	}
}

func TestExecExecFilter(t *testing.T) {
	e := newTestEngine()
	f := FilterConfig{Params: map[string]any{
		"command": []any{"sh", "-c", "printf hello"},
	}}
	out, err := e.execExecFilter(context.Background(), f, map[string]any{})
	if err != nil {
		t.Fatalf("execExecFilter err: %v", err)
	}
	m := out.(map[string]any)
	if m["stdout"] != "hello" || m["code"] != 0 {
		t.Fatalf("exec output = %#v", m)
	}
}

func TestExecFileAndJQAndBase64(t *testing.T) {
	e := newTestEngine()
	d := t.TempDir()
	textPath := d + "/a.txt"
	jsonPath := d + "/a.json"
	if err := os.WriteFile(textPath, []byte("abc"), 0o600); err != nil {
		t.Fatalf("write text: %v", err)
	}
	if err := os.WriteFile(jsonPath, []byte(`{"x":1}`), 0o600); err != nil {
		t.Fatalf("write json: %v", err)
	}

	v, err := e.execFileFilter(FilterConfig{Params: map[string]any{"path": textPath}})
	if err != nil || v != "abc" {
		t.Fatalf("file text v=%v err=%v", v, err)
	}
	v, err = e.execFileFilter(FilterConfig{Params: map[string]any{"path": jsonPath, "parse": "json"}})
	if err != nil {
		t.Fatalf("file json err=%v", err)
	}
	if v.(map[string]any)["x"] != float64(1) {
		t.Fatalf("file json v=%#v", v)
	}

	jv, err := e.execJQFilter(FilterConfig{Params: map[string]any{
		"input": `{"a":1,"b":2}`,
		"query": ".b",
	}}, map[string]any{})
	if err != nil || jv != float64(2) {
		t.Fatalf("jq v=%v err=%v", jv, err)
	}

	encoded, err := e.execBase64Filter(FilterConfig{Params: map[string]any{
		"input": "hello",
		"op":    "encode",
	}}, map[string]any{})
	if err != nil {
		t.Fatalf("base64 encode err=%v", err)
	}
	wantEnc := base64.StdEncoding.EncodeToString([]byte("hello"))
	if encoded != wantEnc {
		t.Fatalf("base64 encode=%v want %v", encoded, wantEnc)
	}
	decoded, err := e.execBase64Filter(FilterConfig{Params: map[string]any{
		"input": encoded,
		"op":    "decode",
	}}, map[string]any{})
	if err != nil || decoded != "hello" {
		t.Fatalf("base64 decode=%v err=%v", decoded, err)
	}
}

func TestEngineHelperFunctions(t *testing.T) {
	re := regexpMustCompile(`(?P<word>\w+)-(\d+)`)
	if got := expandRegexReplace(re, "abc-12", "$word:$2"); got != "abc:12" {
		t.Fatalf("expandRegexReplace = %q", got)
	}
	if !tooLarge(strings.Repeat("a", 10), 5) {
		t.Fatalf("tooLarge should be true")
	}
	if tooLarge("a", 100) {
		t.Fatalf("tooLarge should be false")
	}

	m, ok := asMap(map[string]any{"a": 1})
	if !ok || m["a"] != 1 {
		t.Fatalf("asMap failed: %#v %v", m, ok)
	}
	if got := toString("x"); got != "x" {
		t.Fatalf("toString=%q", got)
	}
	sm := toStringMap(map[string]any{"a": 1, "b": true})
	if sm["a"] != "1" || sm["b"] != "true" {
		t.Fatalf("toStringMap=%#v", sm)
	}

	// Smoke check map output for regex find helper.
	out := buildRegexMap(regexpMustCompile(`(a)(b)`), "ab")
	if out["1"] != "a" || out["2"] != "b" {
		t.Fatalf("buildRegexMap=%#v", out)
	}

	b, err := json.Marshal(out)
	if err != nil || len(b) == 0 {
		t.Fatalf("marshal helper output err=%v", err)
	}
}

func regexpMustCompile(p string) *regexp.Regexp {
	re, err := regexp.Compile(p)
	if err != nil {
		panic(err)
	}
	return re
}
