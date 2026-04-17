package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
)

type App struct {
	cfg         Config
	server      *http.Server
	engine      *Engine
	cache       *TTLCache
	filterIndex map[string]FilterConfig
	logger      *slog.Logger
	reqSeq      uint64
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
	a.logger.Info("server starting",
		"network", a.cfg.Server.Network,
		"addr", a.cfg.Server.Addr,
		"unix_socket", a.cfg.Server.UnixSocket)
	go func() {
		<-ctx.Done()
	}()
	if a.cfg.Server.Network == "unix" {
		ln, err := prepareUnixListener(a.cfg.Server.UnixSocket)
		if err != nil {
			a.logger.Error("server start failed", "error", err)
			return err
		}
		if mode, parseErr := strconv.ParseUint(a.cfg.Server.UnixSocketMode, 8, 32); parseErr == nil {
			_ = os.Chmod(a.cfg.Server.UnixSocket, os.FileMode(mode))
		}
		defer func() {
			_ = os.Remove(a.cfg.Server.UnixSocket)
		}()
		err = a.server.Serve(ln)
		if errors.Is(err, http.ErrServerClosed) {
			a.logger.Info("server stopped")
			return nil
		}
		a.logger.Error("server serve failed", "error", err)
		return err
	}
	err := a.server.ListenAndServe()
	if errors.Is(err, http.ErrServerClosed) {
		a.logger.Info("server stopped")
		return nil
	}
	a.logger.Error("server listen failed", "error", err)
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
	a.logger.Info("server shutting down")
	return a.server.Shutdown(ctx)
}

func (a *App) Check(ctx context.Context, in CheckRequest) (any, error) {
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

	route := matchRoute(a.cfg.Paths, strings.ToUpper(in.Method), in.Path)
	if route == nil {
		a.logger.Warn("check route not found", "method", in.Method, "path", in.Path)
		return nil, errors.New("no matching route")
	}
	reqCtx := buildRequestContext(strings.ToUpper(in.Method), in.Path, "", "", "", headers, []byte(body))
	reqID := a.nextRequestID()
	ctx = context.WithValue(ctx, requestIDKey{}, reqID)
	ctx, cancel := context.WithTimeout(ctx, a.cfg.Server.RequestTimeout)
	defer cancel()
	res, err := a.engine.Execute(ctx, route.Filter, reqCtx)
	if err != nil {
		a.logger.Error("check execution failed",
			"request_id", reqID, "method", in.Method,
			"path", in.Path, "filter", route.Filter, "error", err)
		return nil, err
	}
	a.logger.Info("check execution succeeded",
		"request_id", reqID, "method", in.Method,
		"path", in.Path, "filter", route.Filter)
	return res, nil
}

func (a *App) handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	_, _ = io.WriteString(w, `{"status":"ok"}`)
}

func (a *App) handleRequest(w http.ResponseWriter, r *http.Request) {
	reqID := requestIDFromContext(r.Context())
	route := matchRoute(a.cfg.Paths, r.Method, r.URL.Path)
	if route == nil {
		writeError(w, http.StatusNotFound, "ROUTE_NOT_FOUND", reqID)
		return
	}

	body, tooLarge, err := readRequestBody(r, a.cfg.Server.MaxBodySize)
	if err != nil {
		a.logger.Error("request body read failed", "request_id", reqID, "error", err)
		writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
		return
	}
	if tooLarge {
		writeError(w, http.StatusRequestEntityTooLarge, "REQUEST_BODY_TOO_LARGE", reqID)
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), a.cfg.Server.RequestTimeout)
	defer cancel()
	reqCtxData := buildRequestContext(
		r.Method, r.URL.Path, r.URL.RawQuery,
		r.Host, r.RemoteAddr, flattenHeaders(r.Header), body,
	)
	res, err := a.engine.Execute(ctx, route.Filter, reqCtxData)
	if err != nil {
		if errors.Is(err, context.DeadlineExceeded) {
			a.logger.Warn("request timeout", "request_id", reqID, "filter", route.Filter)
			writeError(w, http.StatusInternalServerError, "REQUEST_TIMEOUT", reqID)
			return
		}
		a.logger.Error("request execution failed", "request_id", reqID, "filter", route.Filter, "error", err)
		writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
		return
	}

	for k, v := range route.Headers {
		w.Header().Set(k, v)
	}
	writeResult(w, res)
}

func writeResult(w http.ResponseWriter, v any) {
	switch x := v.(type) {
	case string:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		}
		_, _ = io.WriteString(w, x)
	default:
		if w.Header().Get("Content-Type") == "" {
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
		}
		enc := json.NewEncoder(w)
		_ = enc.Encode(v)
	}
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
		reqID := a.nextRequestID()
		w.Header().Set("X-Request-ID", reqID)
		r = r.WithContext(context.WithValue(r.Context(), requestIDKey{}, reqID))
		rw := &responseRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rw, r)
		a.logger.Info("access",
			"request_id", reqID,
			"method", r.Method,
			"path", r.URL.Path,
			"status", rw.status,
			"remote_addr", r.RemoteAddr,
			"latency_ms", time.Since(start).Milliseconds(),
		)
	})
}

func (a *App) withRecovery(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				reqID := requestIDFromContext(r.Context())
				a.logger.Error("panic recovered", "request_id", reqID, "panic", rec, "stack", string(debug.Stack()))
				writeError(w, http.StatusInternalServerError, "FILTER_EXECUTION_FAILED", reqID)
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func (a *App) nextRequestID() string {
	n := atomic.AddUint64(&a.reqSeq, 1)
	return fmt.Sprintf("req-%d", n)
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
