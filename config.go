package main

import (
	"compress/gzip"
	"errors"
	"fmt"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

type Config struct {
	Server      ServerConfig      `yaml:"server"`
	Log         LogConfig         `yaml:"log"`
	Compression CompressionConfig `yaml:"compression"`
	Filters     []FilterConfig    `yaml:"filters"`
	Paths       []PathConfig      `yaml:"paths"`
}

type ServerConfig struct {
	Network             string        `yaml:"network"`
	Addr                string        `yaml:"addr"`
	UnixSocket          string        `yaml:"unix_socket"`
	UnixSocketMode      string        `yaml:"unix_socket_mode"`
	RequestTimeout      time.Duration `yaml:"request_timeout"`
	ReadTimeout         time.Duration `yaml:"read_timeout"`
	WriteTimeout        time.Duration `yaml:"write_timeout"`
	ShutdownTimeout     time.Duration `yaml:"shutdown_timeout"`
	MaxBodySize         int64         `yaml:"max_body_size"`
	MaxFilterOutputSize int64         `yaml:"max_filter_output_size"`
}

type LogConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

type CompressionConfig struct {
	Enabled    bool     `yaml:"enabled"`
	MinSize    int      `yaml:"min_size"`
	Level      int      `yaml:"level"`
	Types      []string `yaml:"types"`
	Algorithms []string `yaml:"algorithms"`
}

type FilterConfig struct {
	ID        string   `yaml:"id"`
	Type      string   `yaml:"type"`
	DependsOn []string `yaml:"depends_on"`
	Params    any      `yaml:"params"`
}

type PathConfig struct {
	Method      string                 `yaml:"method"`
	Path        string                 `yaml:"path"`
	Filter      string                 `yaml:"filter"`
	Headers     map[string]string      `yaml:"headers"`
	Compression *PathCompressionConfig `yaml:"compression"`
}

type PathCompressionConfig struct {
	Enabled *bool `yaml:"enabled"`
}

var supportedFilterTypes = map[string]bool{
	"http":   true,
	"static": true,
	"env":    true,
	"exec":   true,
	"file":   true,
	"jq":     true,
	"base64": true,
	"regex":  true,
	"cache":  true,
}

func LoadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	b, err := os.ReadFile(path)
	if err != nil {
		return Config{}, fmt.Errorf("read config: %w", err)
	}
	if err := yaml.Unmarshal(b, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config: %w", err)
	}
	if err := cfg.Validate(); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

func defaultConfig() Config {
	return Config{
		Server: ServerConfig{
			Network:             "tcp",
			Addr:                ":8080",
			UnixSocketMode:      "0660",
			RequestTimeout:      30 * time.Second,
			ReadTimeout:         30 * time.Second,
			WriteTimeout:        30 * time.Second,
			ShutdownTimeout:     10 * time.Second,
			MaxBodySize:         10 * 1024 * 1024,
			MaxFilterOutputSize: 100 * 1024 * 1024,
		},
		Log: LogConfig{Level: "info", Format: "json"},
		Compression: CompressionConfig{
			Enabled:    false,
			MinSize:    1024,
			Level:      5,
			Types:      []string{"application/json", "text/plain", "text/html"},
			Algorithms: []string{"gzip"},
		},
	}
}

func (c Config) Validate() error {
	if c.Server.Network != "tcp" && c.Server.Network != "unix" {
		return fmt.Errorf("server.network must be tcp or unix")
	}
	if c.Server.Network == "tcp" && strings.TrimSpace(c.Server.Addr) == "" {
		return fmt.Errorf("server.addr is required when network=tcp")
	}
	if c.Server.Network == "unix" && strings.TrimSpace(c.Server.UnixSocket) == "" {
		return fmt.Errorf("server.unix_socket is required when network=unix")
	}
	if ok, _ := regexp.MatchString(`^[0-7]{4}$`, c.Server.UnixSocketMode); !ok {
		return fmt.Errorf("server.unix_socket_mode must be 4-digit octal")
	}
	if c.Server.RequestTimeout <= 0 || c.Server.ReadTimeout <= 0 ||
		c.Server.WriteTimeout <= 0 || c.Server.ShutdownTimeout <= 0 {
		return fmt.Errorf("server timeouts must be > 0")
	}
	if c.Server.MaxBodySize <= 0 {
		return fmt.Errorf("server.max_body_size must be > 0")
	}
	if c.Server.MaxFilterOutputSize <= 0 {
		return fmt.Errorf("server.max_filter_output_size must be > 0")
	}
	if !inSet(c.Log.Level, "debug", "info", "warn", "error") {
		return fmt.Errorf("log.level must be debug|info|warn|error")
	}
	if !inSet(c.Log.Format, "json", "text") {
		return fmt.Errorf("log.format must be json|text")
	}
	if c.Compression.MinSize < 0 {
		return fmt.Errorf("compression.min_size must be >= 0")
	}
	if c.Compression.Level < gzip.HuffmanOnly || c.Compression.Level > gzip.BestCompression {
		return fmt.Errorf("compression.level must be between %d and %d", gzip.HuffmanOnly, gzip.BestCompression)
	}
	if c.Compression.Enabled && len(c.Compression.Types) == 0 {
		return fmt.Errorf("compression.types must not be empty when compression.enabled=true")
	}
	if c.Compression.Enabled && !inSet("gzip", c.Compression.Algorithms...) {
		return fmt.Errorf("compression.algorithms must include gzip when compression.enabled=true")
	}

	ids := map[string]bool{}
	for _, f := range c.Filters {
		if strings.TrimSpace(f.ID) == "" {
			return errors.New("filters[].id must be non-empty")
		}
		if ids[f.ID] {
			return fmt.Errorf("duplicate filter id: %s", f.ID)
		}
		ids[f.ID] = true
		if !supportedFilterTypes[f.Type] {
			return fmt.Errorf("unsupported filter type: %s", f.Type)
		}
	}
	for _, f := range c.Filters {
		for _, dep := range f.DependsOn {
			if !ids[dep] {
				return fmt.Errorf("filter '%s' depends_on undefined filter: %s", f.ID, dep)
			}
		}
	}
	for _, p := range c.Paths {
		if !ids[p.Filter] {
			return fmt.Errorf("path filter undefined: %s", p.Filter)
		}
	}
	if _, err := BuildDependencyGraph(c.Filters); err != nil {
		return err
	}
	return nil
}

func inSet(v string, allowed ...string) bool {
	return slices.Contains(allowed, v)
}
