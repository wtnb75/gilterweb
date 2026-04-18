package main

import (
	"errors"
	"reflect"
	"strings"
	"testing"
)

func TestTemplateFuncMapHasCoreFunctions(t *testing.T) {
	fm := templateFuncMap()
	keys := []string{
		"default", "coalesce", "required", "dig", "toJson", "urlquery", "trim",
		"trimSpace", "trimPrefix", "trimSuffix", "removePrefix", "removeSuffix",
		"split", "indent", "contains", "hasPrefix", "hasSuffix", "sha256",
	}
	for _, k := range keys {
		if _, ok := fm[k]; !ok {
			t.Fatalf("missing function %q", k)
		}
	}
}

func TestTplTrimFamily(t *testing.T) {
	if got := tplTrim("/", "//a/b//"); got != "a/b" {
		t.Fatalf("trim = %q", got)
	}
	if got := tplTrimSpace("  a b  "); got != "a b" {
		t.Fatalf("trimSpace = %q", got)
	}
	if got := tplTrimPrefix("/", "///a/b"); got != "a/b" {
		t.Fatalf("trimPrefix = %q", got)
	}
	if got := tplTrimSuffix("/", "a/b///"); got != "a/b" {
		t.Fatalf("trimSuffix = %q", got)
	}
}

func TestTplRemovePrefixSuffix(t *testing.T) {
	if got := tplRemovePrefix("ab", "ababa"); got != "aba" {
		t.Fatalf("removePrefix = %q", got)
	}
	if got := tplRemoveSuffix("ab", "ababa"); got != "ababa" {
		t.Fatalf("removeSuffix = %q", got)
	}
	if got := tplRemovePrefix("xy", "ababa"); got != "ababa" {
		t.Fatalf("removePrefix unchanged = %q", got)
	}
	if got := tplRemoveSuffix("xy", "ababa"); got != "ababa" {
		t.Fatalf("removeSuffix unchanged = %q", got)
	}
}

func TestTplSplit(t *testing.T) {
	got := tplSplit("/", "a/b/c")
	want := []string{"a", "b", "c"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("split = %#v want %#v", got, want)
	}
}

func TestTplIndent(t *testing.T) {
	if got := tplIndent(2, "a\nb"); got != "  a\n  b" {
		t.Fatalf("indent multiline = %q", got)
	}
	if got := tplIndent(0, "a\nb"); got != "a\nb" {
		t.Fatalf("indent width 0 = %q", got)
	}
	if got := tplIndent(-1, "x"); got != "x" {
		t.Fatalf("indent negative = %q", got)
	}
}

func TestTplContains(t *testing.T) {
	if !tplContains("hello world", "hello") {
		t.Fatalf("contains should match substring")
	}
	if tplContains("hello world", "bye") {
		t.Fatalf("contains should not match missing substring")
	}
	if !tplContains([]string{"hello", "world"}, "hello") {
		t.Fatalf("contains should match element in []string")
	}
	if tplContains([]int{1, 2, 3}, 9) {
		t.Fatalf("contains should not match absent element")
	}
	if !tplContains([]int{1, 2, 3}, 2) {
		t.Fatalf("contains should match present element")
	}
}

func TestTplHasPrefixHasSuffix(t *testing.T) {
	if !tplHasPrefix("hello", "he") {
		t.Fatalf("hasPrefix should match")
	}
	if tplHasPrefix("hello", "ll") {
		t.Fatalf("hasPrefix should not match")
	}
	if !tplHasSuffix("hello", "lo") {
		t.Fatalf("hasSuffix should match")
	}
	if tplHasSuffix("hello", "he") {
		t.Fatalf("hasSuffix should not match")
	}
}

func TestTplDefaultAndCoalesce(t *testing.T) {
	if got := tplDefault("x", ""); got != "x" {
		t.Fatalf("default empty = %v", got)
	}
	if got := tplDefault("x", "y"); got != "y" {
		t.Fatalf("default non-empty = %v", got)
	}
	if got := tplCoalesce("", 0, "ok"); got != "ok" {
		t.Fatalf("coalesce = %v", got)
	}
	if got := tplCoalesce(nil, "", 0); got != "" {
		t.Fatalf("coalesce all empty = %v", got)
	}
}

func TestTplRequiredAndDig(t *testing.T) {
	if _, err := tplRequired("need value", ""); err == nil {
		t.Fatalf("required should fail on empty")
	}
	v, err := tplRequired("need value", "ok")
	if err != nil || v != "ok" {
		t.Fatalf("required unexpected result v=%v err=%v", v, err)
	}

	obj := map[string]any{"a": map[string]any{"b": "c"}}
	if got := tplDig(obj, "a", "b"); got != "c" {
		t.Fatalf("dig = %v", got)
	}
	if got := tplDig(obj, "a", "x"); got != nil {
		t.Fatalf("dig missing should be nil, got=%v", got)
	}
}

func TestTplToJSONURLQueryAndSHA256(t *testing.T) {
	j, err := tplToJSON(map[string]any{"a": 1})
	if err != nil {
		t.Fatalf("toJson err: %v", err)
	}
	if !strings.Contains(j, "\"a\":1") {
		t.Fatalf("toJson output: %s", j)
	}

	if got := tplURLQuery("a b&c"); got != "a+b%26c" {
		t.Fatalf("urlquery = %q", got)
	}
	if got := tplSHA256("abc"); got != "ba7816bf8f01cfea414140de5dae2223b00361a396177a9cb410ff61f20015ad" {
		t.Fatalf("sha256 = %q", got)
	}
}

func TestTplToJSONError(t *testing.T) {
	_, err := tplToJSON(map[string]any{"f": func() {}})
	if err == nil {
		t.Fatalf("expected marshal error")
	}
	if !errors.Is(err, err) {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestIsEmptyVariants(t *testing.T) {
	cases := []struct {
		v     any
		empty bool
	}{
		{nil, true},
		{"", true},
		{"x", false},
		{[]int{}, true},
		{[]int{1}, false},
		{map[string]any{}, true},
		{map[string]any{"a": 1}, false},
		{false, true},
		{true, false},
		{0, true},
		{1, false},
		{0.0, true},
		{0.1, false},
	}
	for _, c := range cases {
		if got := isEmpty(c.v); got != c.empty {
			t.Fatalf("isEmpty(%v [%s])=%v want %v", c.v, reflect.TypeOf(c.v), got, c.empty)
		}
	}

	var p *int
	if !isEmpty(p) {
		t.Fatalf("nil pointer should be empty")
	}
	n := 1
	p = &n
	if isEmpty(p) {
		t.Fatalf("non-nil pointer should not be empty")
	}
}
