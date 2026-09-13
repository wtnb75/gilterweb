package main

import (
	"testing"
	"time"
)

func TestTTLCacheLRUEviction(t *testing.T) {
	c := NewTTLCache(2)
	c.Set("a", time.Minute, "va")
	c.Set("b", time.Minute, "vb")

	// touching "a" makes "b" the least recently used entry.
	if v, ok := c.Get("a"); !ok || v != "va" {
		t.Fatalf("get a failed: v=%v ok=%v", v, ok)
	}

	// inserting a third entry over capacity should evict "b", not "a".
	c.Set("c", time.Minute, "vc")

	if _, ok := c.Get("b"); ok {
		t.Fatalf("expected b to be evicted")
	}
	if v, ok := c.Get("a"); !ok || v != "va" {
		t.Fatalf("expected a to survive: v=%v ok=%v", v, ok)
	}
	if v, ok := c.Get("c"); !ok || v != "vc" {
		t.Fatalf("expected c present: v=%v ok=%v", v, ok)
	}
}
