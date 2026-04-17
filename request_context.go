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
	headers map[string]string,
	body []byte,
) map[string]any {
	out := map[string]any{
		"method":       method,
		"path":         path,
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

func matchRoute(paths []PathConfig, method, path string) *PathConfig {
	for i := range paths {
		p := &paths[i]
		if p.Path != path {
			continue
		}
		if p.Method == "*" || strings.EqualFold(p.Method, method) {
			return p
		}
	}
	return nil
}
