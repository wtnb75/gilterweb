# gilterweb — Specification

For coding conventions and directory policy, see [AGENTS.md](AGENTS.md).

## Subcommands

- `server`: main feature; starts the HTTP server
- `check`: evaluates one method/path request without starting the server
- `version`: prints build/version metadata
- `validate`: validates config syntax and semantic rules

### Global Flags

```text
--config string   Config file path (default "config.yaml")
```

### `server` Subcommand

Starts the HTTP server and waits for requests.

- `SIGINT`/`SIGTERM`: graceful shutdown
- `SIGHUP`: reload config from `--config`

```text
gilterweb server [flags]
  --addr string   Override server.addr from config
```

### `check` Subcommand

Runs route + filter evaluation once with a synthetic request context.

```text
gilterweb check [flags]
  --method string        Request method (default "GET")
  --path string          Request path (required)
  --header strings       Request headers (`Key: Value`, repeatable)
  --content-type string  Request Content-Type
  --body string          Request body string
  --body-file string     Request body file path
```

Example:

```text
gilterweb check --method GET --path /foo
```

Behavior:

- Load and validate config
- Resolve matching `paths` entry with the same match rules as server mode
- Build synthetic `.req` context from flags
- Execute target filter graph
- Print result to stdout (plain string as-is, object/array as JSON)

Exit codes:

- `0`: success
- `1`: config invalid, no route match, or filter execution failure

### `version` Subcommand

Prints embedded version, commit hash, and build timestamp.

```text
gilterweb version
```

Example output:

```text
gilterweb version v0.1.0 (commit: abc1234, built: 2026-04-16T00:00:00Z)
```

### `validate` Subcommand

Parses and validates config.

```text
gilterweb validate [flags]
```

## Core Concept: Filter Pipeline

Incoming HTTP request -> route selection -> filter execution -> response.

```text
incoming request
      ↓
 select filter via paths
      ↓
 execute filters (can reference previous results)
      ↓
 return filter output as response
```

### Example

```yaml
filters:
  - id: A
    type: http
    params:
      method: GET
      url: https://httpbin.org/ip
  - id: B
    type: static
    params: "hello {{.A.body.origin}}"

paths:
  - method: GET
    path: /foo
    filter: B
```

## Config File (YAML)

Default config path: `config.yaml`. Keep `example-config.yaml` in repository.

```yaml
server:
  network: tcp             # tcp | unix (default: tcp)
  addr: ":8080"            # required when network=tcp
  unix_socket: ""           # required when network=unix (e.g. /tmp/gilterweb.sock)
  unix_socket_mode: "0660"  # default: 0660
  read_timeout: 30s
  write_timeout: 30s
  shutdown_timeout: 10s

log:
  level: info              # debug | info | warn | error
  format: json             # json | text

filters:
  - id: string
    type: string
    depends_on:
      - string
    params: any

paths:
  - method: string
    path: string
    filter: string
    headers:
      Content-Type: text/plain
```

### Validation Rules

- `server.network`: one of `tcp`, `unix`
- `server.addr`: non-empty when `network=tcp`
- `server.unix_socket`: non-empty when `network=unix`
- `server.unix_socket_mode`: four-digit octal string (e.g. `0660`)
- `server.read_timeout` / `write_timeout` / `shutdown_timeout`: must be > 0
- `log.level`: one of `debug`, `info`, `warn`, `error`
- `log.format`: one of `json`, `text`
- `filters[].id`: unique and non-empty
- `filters[].type`: supported type only
- `filters[].depends_on`: all references must exist; cycles forbidden
- `paths[].filter`: must reference an existing filter id
- `paths[].headers`: keys and values must be strings

## Filter Types

### `http` — Outbound HTTP Request

```yaml
type: http
params:
  method: GET
  url: https://...
  unix_socket: ""    # optional; if set, connect via UDS instead of TCP
  headers:
    X-Foo: bar
  body: "..."
```

When `unix_socket` is set, the client dials UDS while still using an HTTP URL for path/query/host semantics.

UDS example:

```yaml
type: http
params:
  method: GET
  url: http://localhost/ip
  unix_socket: /var/run/httpbin.sock
```

### `static` — Return Value with Template Expansion

`params` can be any YAML value (string/object/array). All string values are rendered with Go `text/template`.

String:

```yaml
type: static
params: "hello {{.A.body.origin}}"
```

Object:

```yaml
type: static
params:
  message: "hello {{.A.body.origin}}"
  status: ok
```

Array:

```yaml
type: static
params:
  - "{{.A.body.origin}}"
  - fixed
```

### `env` — Read Environment Variable

```yaml
type: env
params:
  name: MY_SECRET_TOKEN
  default: ""
```

Returns a string at `.{id}`.

### `exec` — Execute External Command

```yaml
type: exec
params:
  command: ["jq", "-r", ".origin", "/tmp/data.json"]
  timeout: 5s
  env:
    FOO: bar
```

Result fields:

```text
.{id}.stdout   string
.{id}.stderr   string
.{id}.code     int
```

Security requirement: execute directly without shell expansion (`$VAR`, `|`, `;` are not interpreted).

### `file` — Read File Content

```yaml
type: file
params:
  path: /etc/data/config.json
  parse: json   # json | text (default: text)
```

### `jq` — Transform JSON by jq Expression

```yaml
type: jq
params:
  input: "{{.A.raw}}"
  query: ".origin"
```

Implementation uses `github.com/itchyny/gojq` (no external jq binary needed).

### `base64` — Base64 Encode/Decode

```yaml
type: base64
params:
  input: "{{.A.body.token}}"
  op: encode   # encode | decode
```

### `regex` — Regex Match/Replace

```yaml
type: regex
params:
  input: "{{.A.stdout}}"
  pattern: '^-[rwx-]{9}\\s+\\d+\\s+\\S+\\s+\\S+\\s+\\d+\\s+\\S+\\s+\\d+\\s+\\d{2}:\\d{2}\\s+(.+)$'
  op: find_all              # find | find_all | replace
  multiline: true           # default: false
  replace: "***"            # required when op=replace
```

Operation behavior:

- `find`: first match result
- `find_all`: all match results
- `replace`: replace all matches

`multiline: true` enables `(?m)` semantics for `^` and `$` per line.

Capture-group behavior:

- `find`: one group -> string; multiple groups -> `[]string`
- `find_all`: one group -> `[]string`; multiple groups -> `[][]string`
- `replace`: `$1`, `$2`, ... are supported in replacement text

`ls -la` example:

```yaml
filters:
  - id: ls
    type: exec
    params:
      command: ["ls", "-la"]
  - id: files
    type: regex
    params:
      input: "{{.ls.stdout}}"
      pattern: '^-[rwx-]{9}\\s+\\d+\\s+\\S+\\s+\\S+\\s+\\d+\\s+\\S+\\s+\\d+\\s+[\\d:]+\\s+(.+)$'
      op: find_all
      multiline: true
```

Flatten one-group results via `static`:

```yaml
- id: filenames
  type: static
  params: "{{range .files}}{{index . 0}}\n{{end}}"
```

### `cache` — Cache Filter Results

```yaml
type: cache
params:
  filter: A
  ttl: 60s
  key: "{{.req.query.user}}"
```

Within TTL, returns cached result without re-running the target filter. Scope: in-process memory.

## Filter Selection and Execution Strategy

Goal: execute only required filters, avoid duplicate heavy operations.

Rules:

- Per request, root target is exactly one filter from `paths[].filter`
- Execute only filters reachable from the root
- Reachability sources:
  - explicit `depends_on`
  - template references (e.g. `{{.A.stdout}}`)
- Unreachable filters must not execute
- A filter ID executes at most once per request (memoized)

Execution flow:

1. Resolve matching route root
2. Build dependency graph (`depends_on` + template references)
3. Detect cycles (error on cycle)
4. Evaluate in topological order
5. Store each result in `resultMap[id]`
6. Serve repeated references from `resultMap`

A/B/C example:

- A: `type=exec`
- B: `type=static`, `params: "out={{.A.stdout}} err={{.A.stderr}}"`
- C: unrelated filter
- route target: `paths[].filter = B`

Expected:

- A executes once
- B executes
- C does not execute

## Filter Result Data Model

Each filter result is available in template context by filter ID.

HTTP filter result:

```text
.{id}.status   int
.{id}.headers  map[string]string
.{id}.body     any
.{id}.raw      string
```

Template examples:

```text
{{.A.status}}
{{.A.body.origin}}
{{index .A.headers "Content-Type"}}
```

Request context is available as `.req`:

```text
.req.method
.req.path
.req.host
.req.remote_addr
.req.query.{key}
.req.headers.{key}
.req.content_type
.req.body_raw
.req.body_text
.req.body
```

`req.body` parse rules:

- `application/json` -> parsed JSON value
- `application/x-www-form-urlencoded` -> `map[string][]string`
- otherwise (or parse failure) -> same as `req.body_text`

Notes:

- Request body must be read once and reused from shared cache
- For header keys containing `-`, use `index`
  - `{{index .req.headers "User-Agent"}}`

POST JSON example:

```yaml
filters:
  - id: echo
    type: static
    params:
      message: "hello {{.req.body.name}} ua={{index .req.headers \"User-Agent\"}}"
```

## Config Structs

```go
type Config struct {
    Server  ServerConfig   `yaml:"server"`
    Log     LogConfig      `yaml:"log"`
    Filters []FilterConfig `yaml:"filters"`
    Paths   []PathConfig   `yaml:"paths"`
}

type ServerConfig struct {
    Network         string        `yaml:"network"`
    Addr            string        `yaml:"addr"`
    UnixSocket      string        `yaml:"unix_socket"`
    UnixSocketMode  string        `yaml:"unix_socket_mode"`
    ReadTimeout     time.Duration `yaml:"read_timeout"`
    WriteTimeout    time.Duration `yaml:"write_timeout"`
    ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type LogConfig struct {
    Level  string `yaml:"level"`
    Format string `yaml:"format"`
}

type FilterConfig struct {
    ID        string   `yaml:"id"`
    Type      string   `yaml:"type"`
    DependsOn []string `yaml:"depends_on"`
    Params    any      `yaml:"params"`
}

type PathConfig struct {
    Method  string            `yaml:"method"`
    Path    string            `yaml:"path"`
    Filter  string            `yaml:"filter"`
    Headers map[string]string `yaml:"headers"`
}
```

Defaults should be set before `yaml.Unmarshal`.

## HTTP Server Behavior

- Router: standard `net/http` `ServeMux`
- Listener mode:
  - `server.network=tcp` -> listen on `server.addr`
  - `server.network=unix` -> listen on `server.unix_socket` (safely replace stale socket file on startup)
- Middleware chain: `func(http.Handler) http.Handler`
- Required middleware:
  - access logging (`slog`: method/path/status/latency)
  - panic recovery (return 500)

### Built-in Endpoint

- `GET /healthz` -> `{"status":"ok"}`

### Dynamic Endpoints

Register from `paths` at startup. `method: "*"` matches all methods.

Match policy:

- evaluate top to bottom
- method exact match or `*`
- path match
- first match wins

`check` uses the same match policy.

### Response Formatting

Apply `paths[].headers` to response. If `Content-Type` is specified there, use it. Otherwise auto-detect:

- string -> `text/plain; charset=utf-8`
- object/array -> `application/json; charset=utf-8`

Error response:

```json
{"error": "filter execution failed: ..."}
```

## Config Hot Reload (SIGHUP)

When `server` receives SIGHUP, reload from `--config`.

Rules:

- parse + validate every reload attempt
- atomically apply only on success
- keep old config and continue serving on failure

Hot-reloadable fields:

- `filters`
- `paths`
- `paths[].headers`
- `log.level`
- `log.format`

Not hot-reloadable (restart required):

- `server.network`
- `server.addr`
- `server.unix_socket`
- `server.unix_socket_mode`
- `server.read_timeout`
- `server.write_timeout`
- `server.shutdown_timeout`

Required logs:

- success: `config reload succeeded`
- failure: `config reload failed` (with reason)

## Graceful Shutdown

On `SIGINT`/`SIGTERM` in `server` mode:

1. Stop accepting new requests
2. Wait for in-flight requests within `server.shutdown_timeout`
3. Force stop and return error on timeout
