package main

import (
	"context"
	"log/slog"
	"net/http"
	"strings"
	"testing"
)

func TestRunErrorBranchAndCheckExecuteError(t *testing.T) {
	app := &App{
		cfg:    Config{Server: ServerConfig{Network: "tcp", Addr: "bad-addr"}},
		logger: slog.New(slog.DiscardHandler),
	}
	app.server = &http.Server{Addr: app.cfg.Server.Addr}
	if err := app.Run(context.Background()); err == nil {
		t.Fatalf("expected Run listen error")
	}

	cfg := testAppConfig()
	app2, err := NewApp(cfg)
	if err != nil {
		t.Fatalf("NewApp err: %v", err)
	}
	app2.cfg.Paths = []PathConfig{{Method: "GET", Path: "/x", Filter: "NOPE"}}
	_, err = app2.Check(context.Background(), CheckRequest{Method: "GET", Path: "/x"})
	if err == nil || !strings.Contains(err.Error(), "filter not found") {
		t.Fatalf("expected execute error, got: %v", err)
	}
}
