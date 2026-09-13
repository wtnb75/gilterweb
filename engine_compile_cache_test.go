package main

import "testing"

func TestEngineCachesCompiledTemplates(t *testing.T) {
	e := newTestEngine()
	t1, err := e.compileTemplate("{{.req.x}}")
	if err != nil {
		t.Fatalf("compileTemplate: %v", err)
	}
	t2, err := e.compileTemplate("{{.req.x}}")
	if err != nil {
		t.Fatalf("compileTemplate: %v", err)
	}
	if t1 != t2 {
		t.Fatalf("expected the same compiled template instance to be reused")
	}
}

func TestEngineCachesCompiledTemplateParseErrors(t *testing.T) {
	e := newTestEngine()
	_, err1 := e.compileTemplate("{{")
	if err1 == nil {
		t.Fatalf("expected parse error")
	}
	_, err2 := e.compileTemplate("{{")
	if err2 == nil || err2.Error() != err1.Error() {
		t.Fatalf("expected the same cached parse error, got %v vs %v", err1, err2)
	}
}

func TestEngineCachesCompiledRegex(t *testing.T) {
	e := newTestEngine()
	re1, err := e.compileRegex("^a+$")
	if err != nil {
		t.Fatalf("compileRegex: %v", err)
	}
	re2, err := e.compileRegex("^a+$")
	if err != nil {
		t.Fatalf("compileRegex: %v", err)
	}
	if re1 != re2 {
		t.Fatalf("expected the same compiled regex instance to be reused")
	}
}

func TestEngineCachesCompiledJQ(t *testing.T) {
	e := newTestEngine()
	q1, err := e.compileJQ(".a")
	if err != nil {
		t.Fatalf("compileJQ: %v", err)
	}
	q2, err := e.compileJQ(".a")
	if err != nil {
		t.Fatalf("compileJQ: %v", err)
	}
	if q1 != q2 {
		t.Fatalf("expected the same compiled jq query instance to be reused")
	}
}
