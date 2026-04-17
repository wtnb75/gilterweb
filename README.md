# gilterweb

Filter-driven HTTP server written in Go.

`gilterweb` loads a YAML config, matches an incoming request to a path rule, executes the target filter graph, and returns the result as an HTTP response.

## Features

- YAML-based routing and filter pipeline
- Multiple filter types: `static`, `http`, `exec`, `file`, `env`, `regex`, `base64`, `jq`, `cache`
- Automatic dependency resolution from `depends_on` and template references
- Dynamic endpoints with path parameters (`/users/{id}`)
- Request context in templates (`.req.*` including `.req.path_params.*`)
- Structured logging with configurable level and format
- Request ID support (`X-Request-Id` accepted from client or generated as UUID)
- Response compression (gzip)
- Validation command with per-route execution plan

## Requirements

- Go 1.22+

## Quick Start

1. Clone and move into repository root.
2. Run validation:

```bash
go run . validate --config config.yaml
```

3. Start server:

```bash
go run . server --config config.yaml
```

4. Test endpoint:

```bash
curl -sS http://localhost:8888/foo
```

## CLI

```bash
gilterweb [--config FILE] [--log-level debug|info|warn|error] <command>
```

Commands:

- `server`: Start HTTP server
- `check`: Evaluate one request from CLI
- `validate`: Validate config and print execution plan
- `version`: Print version info

Examples:

```bash
# validate config
go run . validate --config config.sample.full.yaml

# start server
go run . server --config config.sample.full.yaml --addr :8888

# dry-run one request through filter graph
go run . check --config config.sample.full.yaml --method GET --path /demo/local
```

## Path Parameters

Path patterns can include named segments using `{name}`.

```yaml
paths:
  - method: GET
    path: /demo/users/{id}
    filter: PATH_PARAM_VIEW
```

Inside templates, read captured values from request context:

```text
{{ index .req.path_params "id" }}
```

Matching behavior:

- Exact static routes are evaluated before parameter routes.
- Matching is case-sensitive.
- Within the same category, first match wins.

## Config Examples

- Minimal config: `config.yaml`
- Full feature demo: `config.sample.full.yaml`

## Development

Task shortcuts:

```bash
task test    # tests + cover.html
task lint    # go fix/fmt + golangci-lint
task cover   # enforce total coverage >= 90%
task build   # build binary with version metadata
```

## License

MIT. See `LICENSE`.
