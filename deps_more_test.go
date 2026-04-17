package main

import (
	"reflect"
	"slices"
	"testing"
)

func TestWalkTemplateRefsReflectAndComplexTemplate(t *testing.T) {
	in := map[string]string{
		"a": "{{.A.value}}",
		"b": "{{if .B}}x{{end}}",
	}
	deps := inferTemplateDeps(in)
	want := []string{"A", "B"}
	if !reflect.DeepEqual(deps, want) {
		t.Fatalf("deps=%v want=%v", deps, want)
	}

	complexTpl := `{{with .C}}{{.name}}{{end}}{{range .D}}{{.}}{{end}}{{if .E}}{{template "x" .F}}{{end}}`
	deps = inferTemplateDeps([]string{complexTpl})
	// TemplateNode/WithNode/RangeNode/IfNode 経由で少なくとも C/D/E/F を拾う。
	for _, k := range []string{"C", "D", "E", "F"} {
		found := slices.Contains(deps, k)
		if !found {
			t.Fatalf("missing dep %q in %v", k, deps)
		}
	}
}
