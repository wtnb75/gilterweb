# gilterweb — Agent Instructions

Go-based web server project. See [SPEC.md](SPEC.md) for runtime behavior and feature semantics.

## Project Summary

- Language: Go
- Main feature: HTTP server started by the `server` subcommand
- Config format: YAML
- Linter: golangci-lint
- Unit test coverage target: 90%+

## Directory Layout

```text
gilterweb/
├── AGENTS.md
├── SPEC.md
├── Taskfile.yml
├── go.mod
├── go.sum
├── .golangci.yml
├── .gitignore
├── config.go
├── config_test.go
├── handler.go
├── handler_test.go
├── server.go
├── server_test.go
├── cmd/
│   └── gilterweb/
│       └── main.go
├── example-config.yaml
└── cover.out
```

## Coding Rules

- Package naming: use `gilterweb` (main package under `cmd/gilterweb/`)
- Error wrapping: use `fmt.Errorf("...: %w", err)`
- Logging: use structured logging with `log/slog`
- CLI framework: use `github.com/spf13/cobra`
- YAML parsing/loading: use `gopkg.in/yaml.v3`
- `server` supports TCP and Unix Domain Socket listeners
- `http` filter supports outbound HTTP over both TCP and UDS
- HTTP handlers should be explicit `http.Handler` implementations
- Middleware shape: `func(http.Handler) http.Handler`
- `server` must handle SIGHUP for config hot reload (apply only on successful validation, keep old config on failure)

## Linter Baseline (`.golangci.yml`)

```yaml
version: "2"
linters:
  enable:
    - staticcheck
    - errcheck
    - govet
    - copyloopvar
    - misspell
    - durationcheck
    - predeclared
    - sloglint
    - lll
formatters:
  enable:
    - gofmt
```

## Testing Rules

- Coverage target: **90%+**
- Coverage command: `go test -v -cover -coverprofile=cover.out ./...`
- Keep tests in the same package as implementation (`_test` external package only when needed)
- HTTP handler tests should use `net/http/httptest`
- Config loading tests should use temporary files from `t.TempDir()`

## Taskfile Tasks

- `task test`: run tests + coverage + HTML report
- `task lint`: `go fix` + `go fmt` + `golangci-lint run`
- `task build`: `go build ./cmd/gilterweb/`
- `task run`: `go run ./cmd/gilterweb/ server`
- `task cover`: fail when total coverage is below 90%

`cover` task example:

```yaml
cover:
  desc: check coverage >= 90%
  cmds:
    - go test -coverprofile=cover.out ./...
    - go tool cover -func=cover.out | awk '/^total:/{if ($3+0 < 90) {print "Coverage "$3" < 90%"; exit 1}}'
```

## Quality Gates

- `golangci-lint run` passes with zero errors
- `go vet ./...` passes with zero errors
- Coverage is >= 90%
- `go build ./...` succeeds
- `example-config.yaml` passes the `validate` subcommand

## Development Flow

1. Read `SPEC.md` first
2. Implement and test config model/loading in `config.go`
3. Implement and test HTTP handlers in `handler.go`
4. Implement and test server runtime in `server.go`
5. Wire subcommands in `cmd/gilterweb/main.go`
6. Run `task lint` and `task cover` before opening a PR
