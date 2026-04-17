package main

import (
	"net/http"
	"reflect"
	"testing"
)

func TestFlattenHeadersAndExtractQuery(t *testing.T) {
	h := http.Header{}
	h.Add("X-A", "1")
	h.Add("X-A", "2")
	h.Add("X-B", "x")
	m := flattenHeaders(h)
	if m["X-A"] != "1,2" || m["X-B"] != "x" {
		t.Fatalf("flattenHeaders = %#v", m)
	}

	q := extractQuery("a=1&a=2&b=x")
	if _, ok := q["a"].([]any); !ok {
		t.Fatalf("query[a] should be []any: %#v", q["a"])
	}
	if q["b"] != "x" {
		t.Fatalf("query[b] = %#v", q["b"])
	}
	if got := extractQuery("%"); got != nil {
		t.Fatalf("invalid query should be nil: %#v", got)
	}
}

func TestParseBodyVariants(t *testing.T) {
	j := parseBody("application/json", []byte(`{"n":1}`))
	jm, ok := j.(map[string]any)
	if !ok || jm["n"] != float64(1) {
		t.Fatalf("json parse = %#v", j)
	}

	form := parseBody("application/x-www-form-urlencoded", []byte("a=1&a=2"))
	fm, ok := form.(map[string][]string)
	if !ok {
		t.Fatalf("form parse type = %T", form)
	}
	want := []string{"1", "2"}
	if !reflect.DeepEqual(fm["a"], want) {
		t.Fatalf("form parse a=%#v", fm["a"])
	}

	if got := parseBody("text/plain", []byte("abc")); got != "abc" {
		t.Fatalf("plain parse = %#v", got)
	}
}

func TestMatchRoute(t *testing.T) {
	paths := []PathConfig{
		{Method: "POST", Path: "/x", Filter: "A"},
		{Method: "*", Path: "/y", Filter: "B"},
		{Method: "GET", Path: "/users/{id}", Filter: "U"},
		{Method: "GET", Path: "/users/me", Filter: "ME"},
	}
	if p := matchRoute(paths, "post", "/x"); p == nil || p.Filter != "A" {
		t.Fatalf("match exact method failed: %#v", p)
	}
	if p := matchRoute(paths, "GET", "/y"); p == nil || p.Filter != "B" {
		t.Fatalf("match wildcard failed: %#v", p)
	}
	if p := matchRoute(paths, "GET", "/z"); p != nil {
		t.Fatalf("unexpected match: %#v", p)
	}

	rm := matchRouteWithParams(paths, "GET", "/users/42")
	if rm == nil || rm.route.Filter != "U" || rm.pathParams["id"] != "42" {
		t.Fatalf("path param match failed: %#v", rm)
	}
	rm = matchRouteWithParams(paths, "GET", "/users/me")
	if rm == nil || rm.route.Filter != "ME" {
		t.Fatalf("static route should have precedence: %#v", rm)
	}
}

func TestBuildRequestContextPathParams(t *testing.T) {
	h := map[string]string{"Content-Type": "text/plain"}
	ctx := buildRequestContext("GET", "/users/42", "", "h", "r", map[string]string{"id": "42"}, h, []byte("ok"))
	req := ctx["req"].(map[string]any)
	pp, ok := req["path_params"].(map[string]any)
	if !ok {
		t.Fatalf("path_params type = %T", req["path_params"])
	}
	if pp["id"] != "42" {
		t.Fatalf("path_params.id = %#v", pp["id"])
	}
}
