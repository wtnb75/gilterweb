package main

import (
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/google/uuid"
)

type App struct {
	mu          sync.RWMutex
	cfg         Config
	server      *http.Server
	engine      *Engine
	cache       *TTLCache
	filterIndex map[string]FilterConfig
	logger      *slog.Logger
}

type CheckRequest struct {
	Method      string
	Path        string
	Headers     []string
	ContentType string
	Body        string
	BodyFile    string
}

func NewApp(cfg Config) (*App, error) {
	if err := cfg.Validate(); err != nil {
		return nil, err
	}
	index := map[string]FilterConfig{}
	for _, f := range cfg.Filters {
		index[f.ID] = f
	}
	cache := NewTTLCache()
	logger := NewLogger(cfg.Log)
	eng := NewEngine(cfg, index, cache, logger)
	mux := http.NewServeMux()
	app := &App{cfg: cfg, engine: eng, cache: cache, filterIndex: index, logger: logger}
	mux.HandleFunc("/healthz", app.handleHealthz)
	mux.HandleFunc("/", app.handleRequest)
	app.server = &http.Server{
		Addr:         cfg.Server.Addr,
		Handler:      app.withAccessLog(app.withRecovery(mux)),
		ReadTimeout:  cfg.Server.ReadTimeout,
		WriteTimeout: cfg.Server.WriteTimeout,
	}
	return app, nil
}

func (a *App) Run(ctx context.Context) error {
	a.mu.RLock()
	cfg := a.cfg
	a.mu.RUnlock()
	logger := a.currentLogger()

	logger.Info("server starting",
		"network", cfg.Server.Network,
		"addr", cfg.Server.Addr,
		"unix_socket", cfg.Server.UnixSocket)
	go func() {
		<-ctx.Done()
	}()
	if cfg.Server.Network == "unix" {
		ln, err := prepareUnixListener(cfg.Server.UnixSocket)
		if err != nil {
			logger.Error("server start failed", "error", err)
			return err
		}
		if mode, parseErr := strconv.ParseUint(cfg.Server.UnixSocketMode, 8, 32); parseErr == nil {
			_ = os.Chmod(cfg.Server.UnixSocket, os.FileMode(mode))
		}
		defer func() {
			_ = os.Remove(cfg.Server.UnixSocket)
		}()
		err = a.server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			logger.Info("server stopped")
			return nil
		}
		logger.Error("server serve failed", "error", err)
		return err
	}
	err := a.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		logger.Info("server stopped")
		return nil
	}
	logger.Error("server listen failed", "error", err)
	return err
}

func prepareUnixListener(socketPath string) (net.Listener, error) {
	if st, err := os.Stat(socketPath); err == nil && st.Mode()&os.ModeSocket != 0 {
		conn, err := net.DialTimeout("unix", socketPath, 500*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return nil, fmt.Errorf("unix socket already in use: %s", socketPath)
		}
		if rmErr := os.Remove(socketPath); rmErr != nil {
			return nil, fmt.Errorf("remove stale unix socket: %w", rmErr)
		}
	}
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return nil, err
	}
	return ln, nil
}

func (a *App) Shutdown(ctx context.Context) error {
	a.currentLogger().Info("server shutting down")
	return a.server.Shutdown(ctx)
}

func (a *App) Reload(cfg Config) error {
	if err := cfg.Validate(); err != nil {
		return err
	}

	a.mu.Lock()
	defer a.mu.Unlock()

	if a.cfg.Server != cfg.Server {
		return fmt.Errorf("config reload failed: server settings are not hot-reloadable")
	}

	index := map[string]FilterConfig{}
	for _, f := range cfg.Filters {
		index[f.ID] = f
	}
	a.cfg = cfg
	a.filterIndex = index
	a.logger = NewLogger(cfg.Log)
	a.engine = NewEngine(cfg, index, a.cache, a.logger)
	return nil
}

func (a *App) currentLogger() *slog.Logger {
	a.mu.RLock()
	logger := a.logger
	a.mu.RUnlock()
	if logger == nil {
		return slog.Default()
	}
	return logger
}

func (a *App) Check(ctx context.Context, in CheckRequest) (any, error) {
	a.mu.RLock()
	cfg := a.cfg
	engine := a.engine
	a.mu.RUnlock()
	logger := a.currentLogger()

	body := in.Body
	if in.BodyFile != "" {
		b, err := os.ReadFile(in.BodyFile)
		if err != nil {
			return nil, fmt.Errorf("read body file: %w", err)
		}
		body = string(b)
	}

	headers := map[string]string{}
	for _, h := range in.Headers {
		parts := strings.SplitN(h, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid --header: %s", h)
		}
		headers[strings.TrimSpace(parts[0])] = strings.TrimSpace(parts[1])
	}
	if in.ContentType != "" {
		headers["Content-Type"] = in.ContentType
	}

	rm := matchRouteWithParams(cfg.Paths, strings.ToUpper(in.Method), in.Path)
	if rm == nil {
		logger.Warn("check route not found", "method", in.Method, "path", in.Path)
		return nil, errors.New("no matching route")
	}
	route := rm.route
	reqCtx := buildRequestContext(
		strings.ToUpper(in.Method), in.Path, "", "", "",
		rm.pathParams, headers, []byte(body),
	)
	reqID := a.nextRequestID()
	ctx = context.WithValue(ctx, requestIDKey{}, reqID)
	ctx, cancel := context.WithTimeout(ctx, cfg.Server.RequestTimeout)
	defer cancel()
	res, err := engine.Execute(ctx, route.Filter, reqCtx)
	if err != nil {
		logger.Error("check execution failed",
			"request_id", reqID, "method", in.Method,
			"path", in.Path, "filter", route.Filter, "error", err)
		return nil, err
	}
	logger.Info("check execution succeeded",
		"request_id", reqID, "method", in.Method,
		"path", in.Path, "filter", route.Filter)
	return res, nil
}

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}

func (a *App) handleRequest(w http.ResponseWriter, r *http.Request) {
	a.mu.RLock()
	cfg := a.cfg
	engine := a.engine
	a.mu.RUnlock()
	logger := a.currentLogger()

	reqID := requestIDFromContext(r.Context())
	rm := matchRouteWithParams(cfg.Paths, r.Method, r.URL.Path)
	if rm == nil {
		writeError(w, http.StatusNotFound, "ROUTE_NOT_FOUND", reqID)
		return
	}
	route := rm.route

	body, tooLarge, err := readRequestBody(r, cfg.Server.MaxBodySize)
	if err != nil {
		logger.Error("request body read failed", "request_id", reqID, "error", err)
		writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
		return
	}
	if tooLarge {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", reqID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), cfg.Server.RequestTimeout)
	defer cancel()
	reqCtxData := buildRequestContext(
		r.Method, r.URL.Path, r.URL.RawQuery,
		r.Host, r.RemoteAddr, rm.pathParams, flattenHeaders(r.Header), body,
	)
	res, err := engine.Execute(ctx, route.Filter, reqCtxData)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			logger.Warn("request timeout", "request_id", reqID, "filter", route.Filter)
			writeError(w, http.StatusInternalServerError, "REQUEST_TIMEOUT", reqID)
			return
		}
		if errors.Is(err, ErrFilterOutputTooLarge) {
			logger.Error("request execution failed", "request_id", reqID, "filter", route.Filter, "error", err)
			writeError(w, http.StatusInternalServerError, "FILTER_OUTPUT_TOO_LARGE", reqID)
			return
		}
		logger.Error("request execution failed", "request_id", reqID, "filter", route.Filter, "error", err)
		writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
		return
	}

	applyFilterResponseHeaders(w.Header(), res)

	for k, v := range route.Headers {
		w.Header().Set(k, v)
	}
	writeResultWithCompression(w, r, route, cfg.Compression, res)
}

func applyFilterResponseHeaders(h http.Header, v any) {
	m, ok := v.(map[string]any)
	if !ok {
		return
	}
	hv, ok := m["headers"]
	if !ok {
		return
	}
	switch x := hv.(type) {
	case map[string]string:
		for k, vv := range x {
			h.Set(k, vv)
		}
	case map[string]any:
		for k, vv := range x {
			h.Set(k, fmt.Sprint(vv))
		}
	}
}

func writeResult(w http.ResponseWriter, v any) {
	body, contentType, err := encodeResult(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", contentType)
	}
	_, _ = w.Write(body)
}

func writeResultWithCompression(
	w http.ResponseWriter,
	r *http.Request,
	route *PathConfig,
	cfg CompressionConfig,
	v any,
) {
	body, contentType, err := encodeResult(v)
	if err != nil {
		w.WriteHeader(http.StatusInternalServerError)
		return
	}
	if w.Header().Get("Content-Type") == "" {
		w.Header().Set("Content-Type", contentType)
	}
	if !shouldCompressResponse(r, route, cfg, w.Header().Get("Content-Type"), len(body), w.Header()) {
		_, _ = w.Write(body)
		return
	}
	appendVaryHeader(w.Header(), "Accept-Encoding")
	w.Header().Set("Content-Encoding", "gzip")
	w.Header().Del("Content-Length")
	zw, err := gzip.NewWriterLevel(w, cfg.Level)
	if err != nil {
		_, _ = w.Write(body)
		return
	}
	_, _ = zw.Write(body)
	_ = zw.Close()
}

func encodeResult(v any) ([]byte, string, error) {
	switch x := v.(type) {
	case string:
		return []byte(x), "text/plain; charset=utf-8", nil
	default:
		b, err := json.Marshal(x)
		if err != nil {
			return nil, "", err
		}
		return append(b, '\n'), "application/json; charset=utf-8", nil
	}
}

func shouldCompressResponse(
	r *http.Request,
	route *PathConfig,
	cfg CompressionConfig,
	contentType string,
	bodySize int,
	h http.Header,
) bool {
	if !isCompressionEnabledForRoute(cfg.Enabled, route) {
		return false
	}
	if bodySize < cfg.MinSize {
		return false
	}
	if h.Get("Content-Encoding") != "" {
		return false
	}
	if r.Header.Get("Range") != "" {
		return false
	}
	if !acceptsGzip(r.Header.Get("Accept-Encoding")) {
		return false
	}
	if !inSet("gzip", cfg.Algorithms...) {
		return false
	}
	if !matchesCompressionType(contentType, cfg.Types) {
		return false
	}
	return true
}

func isCompressionEnabledForRoute(globalEnabled bool, route *PathConfig) bool {
	enabled := globalEnabled
	if route != nil && route.Compression != nil && route.Compression.Enabled != nil {
		enabled = *route.Compression.Enabled
	}
	return enabled
}

func acceptsGzip(v string) bool {
	if strings.TrimSpace(v) == "" {
		return false
	}
	for raw := range strings.SplitSeq(v, ",") {
		part := strings.TrimSpace(raw)
		if part == "" {
			continue
		}
		name := part
		params := ""
		if before, after, ok := strings.Cut(part, ";"); ok {
			name = strings.TrimSpace(before)
			params = strings.ToLower(after)
		}
		if !strings.EqualFold(name, "gzip") && name != "*" {
			continue
		}
		if strings.Contains(params, "q=0") || strings.Contains(params, "q=0.") {
			continue
		}
		return true
	}
	return false
}

func matchesCompressionType(contentType string, allow []string) bool {
	if len(allow) == 0 {
		return true
	}
	mediaType, _, err := mime.ParseMediaType(contentType)
	if err != nil || mediaType == "" {
		mediaType = strings.TrimSpace(strings.Split(contentType, ";")[0])
	}
	mediaType = strings.ToLower(strings.TrimSpace(mediaType))
	for _, a := range allow {
		p := strings.ToLower(strings.TrimSpace(a))
		if p == mediaType {
			return true
		}
		if strings.HasSuffix(p, "/*") {
			prefix := strings.TrimSuffix(p, "*")
			if strings.HasPrefix(mediaType, prefix) {
				return true
			}
		}
	}
	return false
}

func appendVaryHeader(h http.Header, v string) {
	cur := h.Values("Vary")
	for _, line := range cur {
		for token := range strings.SplitSeq(line, ",") {
			if strings.EqualFold(strings.TrimSpace(token), v) {
				return
			}
		}
	}
	h.Add("Vary", v)
}

func writeError(w http.ResponseWriter, status int, code, requestID string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	if requestID != "" {
		w.Header().Set("X-Request-ID", requestID)
	}
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]any{
		"error":      http.StatusText(status),
		"code":       code,
		"request_id": requestID,
	})
}

type responseRecorder struct {
	http.ResponseWriter
	status int
}

func (r *responseRecorder) WriteHeader(code int) {
	r.status = code
	r.ResponseWriter.WriteHeader(code)
}

type requestIDKey struct{}

func (a *App) withAccessLog(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		reqID := strings.TrimSpace(r.Header.Get("X-Request-ID"))
		if reqID == "" {
			reqID = a.nextRequestID()
		}
		w.Header().Set("X-Request-ID", reqID)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, reqID))
		rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		a.currentLogger().Info("access",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"remote_addr", r.RemoteAddr,
			"latency_us", time.Since(start).Microseconds(),
		)
	})
}

func (a *App) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := requestIDFromContext(r.Context())
				a.currentLogger().Error("panic recovered", "request_id", reqID, "panic", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) nextRequestID() string {
	return uuid.NewString()
}

func requestIDFromContext(ctx context.Context) string {
	v := ctx.Value(requestIDKey{})
	s, _ := v.(string)
	return s
}

func RenderResult(v any) string {
	switch x := v.(type) {
	case string:
		return x
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

type TTLCache struct {
	mu      sync.RWMutex
	entries map[string]ttlEntry
}

type ttlEntry struct {
	expires time.Time
	value   any
}

func NewTTLCache() *TTLCache {
	return &TTLCache{entries: map[string]ttlEntry{}}
}

func (c *TTLCache) Get(key string) (any, bool) {
	c.mu.RLock()
	ent, ok := c.entries[key]
	c.mu.RUnlock()
	if !ok {
		return nil, false
	}
	if time.Now().After(ent.expires) {
		c.mu.Lock()
		delete(c.entries, key)
		c.mu.Unlock()
		return nil, false
	}
	return ent.value, true
}

func (c *TTLCache) Set(key string, ttl time.Duration, value any) {
	c.mu.Lock()
	c.entries[key] = ttlEntry{expires: time.Now().Add(ttl), value: value}
	c.mu.Unlock()
}
