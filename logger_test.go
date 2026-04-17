package main

import (
	"context"
	"log/slog"
	"testing"
)

func TestParseLogLevel(t *testing.T) {
	cases := map[string]slog.Level{
		"debug": slog.LevelDebug,
		"warn":  slog.LevelWarn,
		"error": slog.LevelError,
		"info":  slog.LevelInfo,
		"other": slog.LevelInfo,
	}
	for in, want := range cases {
		if got := parseLogLevel(in); got != want {
			t.Fatalf("parseLogLevel(%q)=%v want %v", in, got, want)
		}
	}
}

func TestNewLoggerLevel(t *testing.T) {
	l := NewLogger(LogConfig{Level: "warn", Format: "text"})
	if l == nil {
		t.Fatalf("logger is nil")
	}
	if l.Enabled(context.Background(), slog.LevelInfo) {
		t.Fatalf("info should be disabled for warn level")
	}
	if !l.Enabled(context.Background(), slog.LevelError) {
		t.Fatalf("error should be enabled for warn level")
	}
}
