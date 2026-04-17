package main

import (
	"fmt"
	"reflect"
	"regexp"
	"sort"
	"text/template"
	"text/template/parse"
)

type DepGraph map[string]map[string]bool

var (
	templateActionRE = regexp.MustCompile(`\{\{([^}]*)\}\}`)
	topLevelDotRE    = regexp.MustCompile(`(?:^|[\s(])\.(\w+)`)
)

func BuildDependencyGraph(filters []FilterConfig) (DepGraph, error) {
	index := map[string]FilterConfig{}
	g := DepGraph{}
	for _, f := range filters {
		index[f.ID] = f
		g[f.ID] = map[string]bool{}
	}
	for _, f := range filters {
		for _, d := range f.DependsOn {
			g[f.ID][d] = true
		}
		for _, d := range inferTemplateDeps(f.Params) {
			if d == "req" {
				continue
			}
			if _, ok := index[d]; ok {
				g[f.ID][d] = true
			}
		}
	}
	if err := detectCycle(g); err != nil {
		return nil, err
	}
	return g, nil
}

func inferTemplateDeps(v any) []string {
	out := map[string]bool{}
	walkTemplateRefs(v, out)
	res := make([]string, 0, len(out))
	for k := range out {
		res = append(res, k)
	}
	sort.Strings(res)
	return res
}

func walkTemplateRefs(v any, out map[string]bool) {
	switch x := v.(type) {
	case string:
		walkTemplateStringRefs(x, out)
	case map[string]any:
		for _, vv := range x {
			walkTemplateRefs(vv, out)
		}
	case []any:
		for _, vv := range x {
			walkTemplateRefs(vv, out)
		}
	default:
		rv := reflect.ValueOf(v)
		if !rv.IsValid() {
			return
		}
		switch rv.Kind() {
		case reflect.Map:
			for _, k := range rv.MapKeys() {
				walkTemplateRefs(rv.MapIndex(k).Interface(), out)
			}
		case reflect.Array, reflect.Slice:
			for i := 0; i < rv.Len(); i++ {
				walkTemplateRefs(rv.Index(i).Interface(), out)
			}
		}
	}
}

func walkTemplateStringRefs(s string, out map[string]bool) {
	t, err := template.New("dep").Parse(s)
	if err == nil && t.Tree != nil && t.Tree.Root != nil { //nolint:staticcheck
		collectNodeRefs(t.Tree.Root, out) //nolint:staticcheck
		return
	}
	// Fallback path for templates that include unknown custom functions.
	for _, mm := range templateActionRE.FindAllStringSubmatch(s, -1) {
		if len(mm) < 2 {
			continue
		}
		for _, dm := range topLevelDotRE.FindAllStringSubmatch(mm[1], -1) {
			if len(dm) > 1 {
				out[dm[1]] = true
			}
		}
	}
}

func collectNodeRefs(node parse.Node, out map[string]bool) {
	if node == nil {
		return
	}
	rv := reflect.ValueOf(node)
	if rv.Kind() == reflect.Pointer && rv.IsNil() {
		return
	}
	switch n := node.(type) {
	case *parse.ListNode:
		for _, nn := range n.Nodes {
			collectNodeRefs(nn, out)
		}
	case *parse.ActionNode:
		collectNodeRefs(n.Pipe, out)
	case *parse.PipeNode:
		for _, cmd := range n.Cmds {
			collectNodeRefs(cmd, out)
		}
	case *parse.CommandNode:
		for _, arg := range n.Args {
			collectNodeRefs(arg, out)
		}
	case *parse.FieldNode:
		if len(n.Ident) > 0 {
			out[n.Ident[0]] = true
		}
	case *parse.ChainNode:
		collectNodeRefs(n.Node, out)
		if _, ok := n.Node.(*parse.DotNode); ok && len(n.Field) > 0 {
			out[n.Field[0]] = true
		}
	case *parse.IfNode:
		collectNodeRefs(n.Pipe, out)
		collectNodeRefs(n.List, out)
		collectNodeRefs(n.ElseList, out)
	case *parse.RangeNode:
		collectNodeRefs(n.Pipe, out)
		collectNodeRefs(n.List, out)
		collectNodeRefs(n.ElseList, out)
	case *parse.WithNode:
		collectNodeRefs(n.Pipe, out)
		collectNodeRefs(n.List, out)
		collectNodeRefs(n.ElseList, out)
	case *parse.TemplateNode:
		collectNodeRefs(n.Pipe, out)
	}
}

func detectCycle(g DepGraph) error {
	const (
		white = 0
		gray  = 1
		black = 2
	)
	color := map[string]int{}
	stack := []string{}

	var dfs func(n string) error
	dfs = func(n string) error {
		color[n] = gray
		stack = append(stack, n)
		for d := range g[n] {
			if color[d] == gray {
				return fmt.Errorf("cycle detected: %s -> %s", n, d)
			}
			if color[d] == white {
				if err := dfs(d); err != nil {
					return err
				}
			}
		}
		stack = stack[:len(stack)-1]
		color[n] = black
		_ = stack
		return nil
	}

	for n := range g {
		if color[n] == white {
			if err := dfs(n); err != nil {
				return err
			}
		}
	}
	return nil
}
