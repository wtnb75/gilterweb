package main

import (
	"bytes"
	"encoding/json"
	"io"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

func readRequestBody(r *http.Request, maxBytes int64) ([]byte, bool, error) {
	if r.Body == nil {
		return nil, false, nil
	}
	defer func() { _ = r.Body.Close() }()
	buf := bytes.NewBuffer(nil)
	limited := io.LimitReader(r.Body, maxBytes+1)
	_, err := io.Copy(buf, limited)
	if err != nil {
		return nil, false, err
	}
	if int64(buf.Len()) > maxBytes {
		return nil, true, nil
	}
	return buf.Bytes(), false, nil
}

func flattenHeaders(h http.Header) map[string]string {
	m := map[string]string{}
	for k, v := range h {
		m[k] = strings.Join(v, ",")
	}
	return m
}

func buildRequestContext(
	method, path, rawQuery, host, remoteAddr string,
	pathParams map[string]string,
	headers map[string]string,
	body []byte,
) map[string]any {
	pp := map[string]any{}
	for k, v := range pathParams {
		pp[k] = v
	}
	out := map[string]any{
		"method":       method,
		"path":         path,
		"path_params":  pp,
		"host":         host,
		"remote_addr":  remoteAddr,
		"query":        map[string]any{},
		"headers":      headers,
		"content_type": headers["Content-Type"],
		"body_raw":     string(body),
		"body_text":    string(body),
		"body":         parseBody(headers["Content-Type"], body),
	}
	if q := extractQuery(rawQuery); len(q) > 0 {
		out["query"] = q
	}
	return map[string]any{"req": out}
}

func extractQuery(rawQuery string) map[string]any {
	if rawQuery == "" {
		return nil
	}
	vals, err := url.ParseQuery(rawQuery)
	if err != nil {
		return nil
	}
	res := map[string]any{}
	for k, v := range vals {
		if len(v) == 1 {
			res[k] = v[0]
		} else {
			arr := make([]any, 0, len(v))
			for _, e := range v {
				arr = append(arr, e)
			}
			res[k] = arr
		}
	}
	return res
}

func parseBody(contentType string, body []byte) any {
	ct, _, _ := mime.ParseMediaType(contentType)
	switch ct {
	case "application/json":
		var v any
		if err := json.Unmarshal(body, &v); err == nil {
			return v
		}
	case "application/x-www-form-urlencoded":
		vals, err := url.ParseQuery(string(body))
		if err == nil {
			res := map[string][]string{}
			for k, v := range vals {
				res[k] = append([]string(nil), v...)
			}
			return res
		}
	}
	return string(body)
}

type routeMatch struct {
	route      *PathConfig
	pathParams map[string]string
}

func methodMatches(routeMethod, reqMethod string) bool {
	return routeMethod == "*" || strings.EqualFold(routeMethod, reqMethod)
}

func hasPathParamPattern(path string) bool {
	return strings.Contains(path, "{") && strings.Contains(path, "}")
}

func isPathParamSegment(seg string) bool {
	return len(seg) > 2 && strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}")
}

func matchPathPattern(pattern, actual string) (map[string]string, bool) {
	pp := strings.Split(pattern, "/")
	ap := strings.Split(actual, "/")
	if len(pp) != len(ap) {
		return nil, false
	}
	out := map[string]string{}
	for i := range pp {
		if isPathParamSegment(pp[i]) {
			name := strings.TrimSuffix(strings.TrimPrefix(pp[i], "{"), "}")
			if name == "" {
				return nil, false
			}
			out[name] = ap[i]
			continue
		}
		if pp[i] != ap[i] {
			return nil, false
		}
	}
	return out, true
}

func matchRouteWithParams(paths []PathConfig, method, path string) *routeMatch {
	for i := range paths {
		p := &paths[i]
		if !methodMatches(p.Method, method) {
			continue
		}
		if hasPathParamPattern(p.Path) {
			continue
		}
		if p.Path == path {
			return &routeMatch{route: p, pathParams: map[string]string{}}
		}
	}
	for i := range paths {
		p := &paths[i]
		if !methodMatches(p.Method, method) {
			continue
		}
		if !hasPathParamPattern(p.Path) {
			continue
		}
		params, ok := matchPathPattern(p.Path, path)
		if ok {
			return &routeMatch{route: p, pathParams: params}
		}
	}
	return nil
}

func matchRoute(paths []PathConfig, method, path string) *PathConfig {
	m := matchRouteWithParams(paths, method, path)
	if m == nil {
		return nil
	}
	return m.route
}
