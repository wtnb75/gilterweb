package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"net"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"text/template"
	"time"

	"github.com/itchyny/gojq"
)

type Engine struct {
	cfg         Config
	filters     map[string]FilterConfig
	graph       DepGraph
	cache       *TTLCache
	renderFuncs template.FuncMap
	logger      *slog.Logger
}

var ErrFilterOutputTooLarge = errors.New("filter output too large")

func NewEngine(cfg Config, filters map[string]FilterConfig, cache *TTLCache, logger *slog.Logger) *Engine {
	g, _ := BuildDependencyGraph(cfg.Filters)
	if logger == nil {
		logger = slog.Default()
	}
	return &Engine{cfg: cfg, filters: filters, graph: g, cache: cache, renderFuncs: templateFuncMap(), logger: logger}
}

func (e *Engine) Execute(ctx context.Context, target string, base map[string]any) (any, error) {
	if _, ok := e.filters[target]; !ok {
		return nil, fmt.Errorf("filter not found: %s", target)
	}
	order, err := topoForTarget(e.graph, target)
	if err != nil {
		return nil, err
	}
	resultMap := map[string]any{}
	maps.Copy(resultMap, base)
	for _, id := range order {
		f := e.filters[id]
		start := time.Now()
		out, err := e.execFilter(ctx, f, resultMap)
		if err != nil {
			e.logger.Error("filter execution failed",
				"request_id", requestIDFromContext(ctx),
				"filter_id", id,
				"filter_type", f.Type,
				"duration_us", time.Since(start).Microseconds(),
				"error", err,
			)
			return nil, fmt.Errorf("filter '%s': %w", id, err)
		}
		if tooLarge(out, e.cfg.Server.MaxFilterOutputSize) {
			e.logger.Error("filter output too large",
				"request_id", requestIDFromContext(ctx),
				"filter_id", id,
				"filter_type", f.Type,
				"duration_us", time.Since(start).Microseconds(),
			)
			return nil, fmt.Errorf("%w: %s", ErrFilterOutputTooLarge, id)
		}
		e.logger.Info("filter executed",
			"request_id", requestIDFromContext(ctx),
			"filter_id", id,
			"filter_type", f.Type,
			"duration_us", time.Since(start).Microseconds(),
		)
		resultMap[id] = out
	}
	return resultMap[target], nil
}

func topoForTarget(g DepGraph, root string) ([]string, error) {
	seen := map[string]bool{}
	visiting := map[string]bool{}
	out := []string{}
	var dfs func(string) error
	dfs = func(n string) error {
		if visiting[n] {
			return fmt.Errorf("cycle detected at %s", n)
		}
		if seen[n] {
			return nil
		}
		visiting[n] = true
		for d := range g[n] {
			if err := dfs(d); err != nil {
				return err
			}
		}
		visiting[n] = false
		seen[n] = true
		out = append(out, n)
		return nil
	}
	if err := dfs(root); err != nil {
		return nil, err
	}
	return out, nil
}

func (e *Engine) execFilter(ctx context.Context, f FilterConfig, data map[string]any) (any, error) {
	switch f.Type {
	case "static":
		return e.expandStatic(f.Params, data, 0)
	case "env":
		return execEnvFilter(f)
	case "http":
		return e.execHTTPFilter(ctx, f, data)
	case "exec":
		return e.execExecFilter(ctx, f, data)
	case "file":
		return e.execFileFilter(f)
	case "jq":
		return e.execJQFilter(f, data)
	case "base64":
		return e.execBase64Filter(f, data)
	case "regex":
		return e.execRegexFilter(f, data)
	case "cache":
		return e.execCacheFilter(ctx, f, data)
	default:
		return nil, fmt.Errorf("filter type not implemented yet: %s", f.Type)
	}
}

func (e *Engine) expandStatic(v any, data map[string]any, depth int) (any, error) {
	if depth > 10 {
		return nil, fmt.Errorf("static params nesting depth exceeded")
	}
	switch x := v.(type) {
	case string:
		return renderTemplate(x, data, e.renderFuncs)
	case map[string]any:
		out := map[string]any{}
		for k, vv := range x {
			r, err := e.expandStatic(vv, data, depth+1)
			if err != nil {
				return nil, err
			}
			out[k] = r
		}
		return out, nil
	case []any:
		out := make([]any, 0, len(x))
		for _, vv := range x {
			r, err := e.expandStatic(vv, data, depth+1)
			if err != nil {
				return nil, err
			}
			out = append(out, r)
		}
		return out, nil
	default:
		return v, nil
	}
}

func renderTemplate(tpl string, data map[string]any, fn template.FuncMap) (string, error) {
	t, err := template.New("tpl").Funcs(fn).Parse(tpl)
	if err != nil {
		return "", err
	}
	buf := bytes.NewBuffer(nil)
	if err := t.Execute(buf, data); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func execEnvFilter(f FilterConfig) (any, error) {
	params, ok := f.Params.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("env params must be object")
	}
	name, _ := params["name"].(string)
	def, _ := params["default"].(string)
	if name == "" {
		return nil, fmt.Errorf("env.name required")
	}
	v, ok := lookupEnv(name)
	if !ok {
		return def, nil
	}
	return v, nil
}

var lookupEnv = func(k string) (string, bool) {
	return os.LookupEnv(k)
}

func (e *Engine) execHTTPFilter(ctx context.Context, f FilterConfig, data map[string]any) (any, error) {
	params, ok := asMap(f.Params)
	if !ok {
		return nil, fmt.Errorf("http params must be object")
	}
	unixSocket := toString(params["unix_socket"])
	method := toString(params["method"])
	if method == "" {
		method = http.MethodGet
	}
	url := toString(params["url"])
	if url == "" {
		return nil, fmt.Errorf("http.url required")
	}
	bodyTpl := toString(params["body"])
	body, err := renderTemplate(bodyTpl, data, e.renderFuncs)
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		return nil, err
	}
	for k, v := range toStringMap(params["headers"]) {
		req.Header.Set(k, v)
	}
	client := http.DefaultClient
	if unixSocket != "" {
		transport := &http.Transport{
			DialContext: func(ctx context.Context, _, _ string) (net.Conn, error) {
				var d net.Dialer
				return d.DialContext(ctx, "unix", unixSocket)
			},
		}
		client = &http.Client{Transport: transport}
	}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	raw, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	out := map[string]any{
		"status":  resp.StatusCode,
		"headers": flattenHeaders(resp.Header),
		"raw":     string(raw),
		"body":    parseBody(resp.Header.Get("Content-Type"), raw),
	}
	return out, nil
}

func (e *Engine) execExecFilter(ctx context.Context, f FilterConfig, data map[string]any) (any, error) {
	params, ok := asMap(f.Params)
	if !ok {
		return nil, fmt.Errorf("exec params must be object")
	}
	rawCmd, ok := params["command"].([]any)
	if !ok || len(rawCmd) == 0 {
		return nil, fmt.Errorf("exec.command required")
	}
	cmdArgs := make([]string, 0, len(rawCmd))
	for _, v := range rawCmd {
		argTpl := toString(v)
		arg, err := renderTemplate(argTpl, data, e.renderFuncs)
		if err != nil {
			return nil, err
		}
		cmdArgs = append(cmdArgs, arg)
	}

	timeout := 5 * time.Second
	if t := toString(params["timeout"]); t != "" {
		d, err := time.ParseDuration(t)
		if err != nil {
			return nil, err
		}
		timeout = d
	}
	execCtx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(execCtx, cmdArgs[0], cmdArgs[1:]...)
	for k, v := range toStringMap(params["env"]) {
		cmd.Env = append(cmd.Env, k+"="+v)
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	timedOut := execCtx.Err() == context.DeadlineExceeded
	code := 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			code = exitErr.ExitCode()
		} else if timedOut {
			code = -1
		} else {
			return nil, err
		}
	}
	return map[string]any{
		"stdout":  stdout.String(),
		"stderr":  stderr.String(),
		"code":    code,
		"timeout": timedOut,
	}, nil
}

func (e *Engine) execFileFilter(f FilterConfig) (any, error) {
	params, ok := asMap(f.Params)
	if !ok {
		return nil, fmt.Errorf("file params must be object")
	}
	path := toString(params["path"])
	if path == "" {
		return nil, fmt.Errorf("file.path required")
	}
	mode := toString(params["parse"])
	if mode == "" {
		mode = "text"
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	if mode == "json" {
		var out any
		if err := json.Unmarshal(b, &out); err != nil {
			return nil, err
		}
		return out, nil
	}
	return string(b), nil
}

func (e *Engine) execJQFilter(f FilterConfig, data map[string]any) (any, error) {
	params, ok := asMap(f.Params)
	if !ok {
		return nil, fmt.Errorf("jq params must be object")
	}
	inputTpl := toString(params["input"])
	query := toString(params["query"])
	if query == "" {
		return nil, fmt.Errorf("jq.query required")
	}
	input, err := renderTemplate(inputTpl, data, e.renderFuncs)
	if err != nil {
		return nil, err
	}
	var in any
	if err := json.Unmarshal([]byte(input), &in); err != nil {
		return nil, err
	}
	q, err := gojq.Parse(query)
	if err != nil {
		return nil, err
	}
	it := q.Run(in)
	v, ok := it.Next()
	if !ok {
		return nil, nil
	}
	if err, ok := v.(error); ok {
		return nil, err
	}
	return v, nil
}

func (e *Engine) execBase64Filter(f FilterConfig, data map[string]any) (any, error) {
	params, ok := f.Params.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("base64 params must be object")
	}
	inputTpl, _ := params["input"].(string)
	op, _ := params["op"].(string)
	in, err := renderTemplate(inputTpl, data, e.renderFuncs)
	if err != nil {
		return nil, err
	}
	switch op {
	case "encode":
		return base64.StdEncoding.EncodeToString([]byte(in)), nil
	case "decode":
		b, err := base64.StdEncoding.DecodeString(in)
		if err != nil {
			return nil, err
		}
		return string(b), nil
	default:
		return nil, fmt.Errorf("base64 op must be encode|decode")
	}
}

func (e *Engine) execRegexFilter(f FilterConfig, data map[string]any) (any, error) {
	params, ok := f.Params.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("regex params must be object")
	}
	inputTpl, _ := params["input"].(string)
	pattern, _ := params["pattern"].(string)
	op, _ := params["op"].(string)
	replace, _ := params["replace"].(string)
	multiline, _ := params["multiline"].(bool)
	if multiline {
		pattern = "(?m)" + pattern
	}
	re, err := regexp.Compile(pattern)
	if err != nil {
		return nil, err
	}
	input, err := renderTemplate(inputTpl, data, e.renderFuncs)
	if err != nil {
		return nil, err
	}
	switch op {
	case "find":
		return buildRegexMap(re, input), nil
	case "find_all":
		ms := re.FindAllStringSubmatch(input, -1)
		if len(ms) == 0 {
			return []any{}, nil
		}
		res := make([]any, 0, len(ms))
		for _, sub := range ms {
			res = append(res, mapWithNames(re, sub))
		}
		return res, nil
	case "replace":
		if replace == "" {
			return nil, fmt.Errorf("replace required when op=replace")
		}
		return expandRegexReplace(re, input, replace), nil
	default:
		return nil, fmt.Errorf("regex op must be find|find_all|replace")
	}
}

func buildRegexMap(re *regexp.Regexp, input string) map[string]any {
	s := re.FindStringSubmatch(input)
	if len(s) == 0 {
		return map[string]any{}
	}
	return mapWithNames(re, s)
}

func mapWithNames(re *regexp.Regexp, groups []string) map[string]any {
	out := map[string]any{}
	names := re.SubexpNames()
	for i, g := range groups {
		out[strconv.Itoa(i)] = g
		if i < len(names) && names[i] != "" {
			out[names[i]] = g
		}
	}
	return out
}

func expandRegexReplace(re *regexp.Regexp, input, replace string) string {
	out := input
	names := re.SubexpNames()
	for i, n := range names {
		if i == 0 || n == "" {
			continue
		}
		replace = strings.ReplaceAll(replace, "$"+n, "${"+n+"}")
	}
	return re.ReplaceAllString(out, replace)
}

func (e *Engine) execCacheFilter(ctx context.Context, f FilterConfig, data map[string]any) (any, error) {
	params, ok := f.Params.(map[string]any)
	if !ok {
		return nil, fmt.Errorf("cache params must be object")
	}
	target, _ := params["filter"].(string)
	if target == "" {
		return nil, fmt.Errorf("cache.filter required")
	}
	ttlStr, _ := params["ttl"].(string)
	if ttlStr == "" {
		ttlStr = "60s"
	}
	ttl, err := time.ParseDuration(ttlStr)
	if err != nil {
		return nil, err
	}
	keyTpl, _ := params["key"].(string)
	key, err := renderTemplate(keyTpl, data, e.renderFuncs)
	if err != nil {
		return nil, err
	}
	cacheKey := target + ":" + key
	if v, ok := e.cache.Get(cacheKey); ok {
		e.logger.Info("cache hit",
			"request_id", requestIDFromContext(ctx),
			"filter_id", f.ID,
			"target_filter", target,
			"cache_key", cacheKey,
		)
		return v, nil
	}
	e.logger.Info("cache miss",
		"request_id", requestIDFromContext(ctx),
		"filter_id", f.ID,
		"target_filter", target,
		"cache_key", cacheKey,
	)
	res, err := e.execFilter(ctx, e.filters[target], data)
	if err != nil {
		return nil, err
	}
	e.cache.Set(cacheKey, ttl, res)
	return res, nil
}

func tooLarge(v any, limit int64) bool {
	if limit <= 0 {
		return false
	}
	b, err := json.Marshal(v)
	if err != nil {
		return false
	}
	return int64(len(b)) > limit
}

func asMap(v any) (map[string]any, bool) {
	m, ok := v.(map[string]any)
	return m, ok
}

func toString(v any) string {
	s, _ := v.(string)
	return s
}

func toStringMap(v any) map[string]string {
	out := map[string]string{}
	m, ok := v.(map[string]any)
	if !ok {
		return out
	}
	for k, vv := range m {
		out[k] = fmt.Sprint(vv)
	}
	return out
}
