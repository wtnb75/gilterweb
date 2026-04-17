package main

import (
	"bytes"
	"net/http"
	"testing"
)

func TestReadRequestBodyTooLarge(t *testing.T) {
	r, err := http.NewRequest(http.MethodPost, "/x", bytes.NewBufferString("123456"))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	_, tooLarge, err := readRequestBody(r, 3)
	if err != nil {
		t.Fatalf("read body: %v", err)
	}
	if !tooLarge {
		t.Fatalf("expected tooLarge=true")
	}
}

func TestBuildRequestContextJSON(t *testing.T) {
	h := map[string]string{"Content-Type": "application/json"}
	ctx := buildRequestContext("POST", "/x", "a=1", "h", "r", map[string]string{}, h, []byte(`{"name":"alice"}`))
	req := ctx["req"].(map[string]any)
	body := req["body"].(map[string]any)
	if body["name"] != "alice" {
		t.Fatalf("unexpected body: %#v", body)
	}
	q := req["query"].(map[string]any)
	if q["a"] != "1" {
		t.Fatalf("unexpected query: %#v", q)
	}
}
