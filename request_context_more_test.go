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
	ctx := buildRequestContext("GET", "/users/42", "", "h", "r",
		map[string]string{"id": "42"}, h, map[string]string{}, []byte("ok"))
	req := ctx["req"].(map[string]any)
	pp, ok := req["path_params"].(map[string]any)
	if !ok {
		t.Fatalf("path_params type = %T", req["path_params"])
	}
	if pp["id"] != "42" {
		t.Fatalf("path_params.id = %#v", pp["id"])
	}
}

func TestBuildRequestContextCookies(t *testing.T) {
	h := map[string]string{"Content-Type": "text/plain"}
	cookies := map[string]string{"session": "abc123", "theme": "dark"}
	ctx := buildRequestContext("GET", "/", "", "h", "r", map[string]string{}, h, cookies, []byte(""))
	req := ctx["req"].(map[string]any)
	c, ok := req["cookies"].(map[string]any)
	if !ok {
		t.Fatalf("cookies type = %T", req["cookies"])
	}
	if c["session"] != "abc123" {
		t.Fatalf("cookies.session = %#v", c["session"])
	}
	if c["theme"] != "dark" {
		t.Fatalf("cookies.theme = %#v", c["theme"])
	}
}

func TestFlattenCookies(t *testing.T) {
	cookies := []*http.Cookie{
		{Name: "session", Value: "abc123"},
		{Name: "theme", Value: "dark"},
	}
	m := flattenCookies(cookies)
	if m["session"] != "abc123" || m["theme"] != "dark" {
		t.Fatalf("flattenCookies = %#v", m)
	}
	if got := flattenCookies(nil); len(got) != 0 {
		t.Fatalf("empty cookies should return empty map: %#v", got)
	}
}

func TestNormalizeHeaders(t *testing.T) {
	h := map[string]string{
		"Accept-Encoding": "gzip",
		"Content-Type":    "application/json",
		"X-Request-Id":    "abc",
	}
	m := normalizeHeaders(h)
	if m["Accept-Encoding"] != "gzip" {
		t.Fatalf("original key preserved: %#v", m["Accept-Encoding"])
	}
	if m["accept_encoding"] != "gzip" {
		t.Fatalf("underscore key: %#v", m["accept_encoding"])
	}
	if m["content_type"] != "application/json" {
		t.Fatalf("content_type: %#v", m["content_type"])
	}
	if m["x_request_id"] != "abc" {
		t.Fatalf("x_request_id: %#v", m["x_request_id"])
	}
}

func TestBuildRequestContextHeadersNormalized(t *testing.T) {
	h := map[string]string{"Accept-Encoding": "gzip", "Content-Type": "text/plain"}
	ctx := buildRequestContext("GET", "/", "", "h", "r", map[string]string{}, h, map[string]string{}, []byte(""))
	req := ctx["req"].(map[string]any)
	headers, ok := req["headers"].(map[string]any)
	if !ok {
		t.Fatalf("headers type = %T", req["headers"])
	}
	if headers["Accept-Encoding"] != "gzip" {
		t.Fatalf("original key: %#v", headers["Accept-Encoding"])
	}
	if headers["accept_encoding"] != "gzip" {
		t.Fatalf("underscore key: %#v", headers["accept_encoding"])
	}
}
