package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

func TestCacheFilterSingleflightDedupesConcurrentMisses(t *testing.T) {
	var calls int32
	release := make(chan struct{})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		atomic.AddInt32(&calls, 1)
		<-release
		_, _ = w.Write([]byte("ok"))
	}))
	defer srv.Close()

	cfg := defaultConfig()
	cfg.Filters = []FilterConfig{
		{ID: "A", Type: "http", Params: map[string]any{"url": srv.URL}},
		{ID: "C", Type: "cache", Params: map[string]any{"filter": "A", "ttl": "60s", "key": "k"}},
	}
	idx := map[string]FilterConfig{"A": cfg.Filters[0], "C": cfg.Filters[1]}
	eng := NewEngine(cfg, idx, NewTTLCache(1000), nil)

	const n = 5
	var wg sync.WaitGroup
	wg.Add(n)
	for range n {
		go func() {
			defer wg.Done()
			if _, err := eng.Execute(context.Background(), "C", map[string]any{"req": map[string]any{}}); err != nil {
				t.Errorf("execute: %v", err)
			}
		}()
	}

	// Let all goroutines reach the (blocked) backend call before releasing it.
	time.Sleep(50 * time.Millisecond)
	close(release)
	wg.Wait()

	if got := atomic.LoadInt32(&calls); got != 1 {
		t.Fatalf("expected backend to be called exactly once, got %d", got)
	}
}
