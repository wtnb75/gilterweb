package main

import (
	"reflect"
	"testing"
)

func TestInferTemplateDepsAutoDetect(t *testing.T) {
	deps := inferTemplateDeps(map[string]any{
		"message": "hello {{.A.body.origin}} {{ default .B \"x\" }}",
		"nested":  []any{"{{ if .C }}ok{{ end }}"},
	})
	want := []string{"A", "B", "C"}
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("deps = %v, want %v", deps, want)
	}
}

func TestBuildDependencyGraphInferredWithoutDependsOn(t *testing.T) {
	filters := []FilterConfig{
		{ID: "A", Type: "static", Params: "hello"},
		{ID: "B", Type: "static", Params: "{{.A}} world"},
	}
	g, err := BuildDependencyGraph(filters)
	if err != nil {
		t.Fatalf("BuildDependencyGraph error: %v", err)
	}
	if !g["B"]["A"] {
		t.Fatalf("expected inferred dependency B -> A, got %#v", g["B"])
	}
}
