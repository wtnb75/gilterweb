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
- `1`: config error (file not found, I/O error, parse error, or validation error such as cycle detection)
- `2`: runtime error (no matching route, filter execution failure, timeout, etc.)

Error messages are written to stderr with details sufficient to identify the cause (e.g., "config file not found: config.yaml", "cycle detected: A → B → A", "filter 'http_call' timeout after 5s").

### `version` Subcommand

Prints build version, commit hash, and build timestamp.

```text
gilterweb version
```

Example output:

```text
gilterweb version v0.1.0 (commit: abc1234, built: 2026-04-16T13:07:00+09:00)
```

Version information is embedded at build time using Go's `-ldflags`:
- Version: git tag or "dev"
- Commit: short commit hash
- Built: ISO 8601 timestamp with timezone

(See AGENTS.md for build task configuration.)

### `validate` Subcommand

Parses and validates config syntax and semantics (including cycle detection).

```text
gilterweb validate [flags]
```

Exit codes:

- `0`: config is valid
- `1`: config error (file not found, I/O error, parse error, or validation error)

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
  request_timeout: 30s      # max request processing time (default: 30s)
  max_body_size: 10485760  # bytes, default: 10MB
  max_filter_output_size: 104857600  # bytes, default: 100MB
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
- `server.request_timeout`: must be > 0; default 30s
- `server.read_timeout` / `write_timeout` / `shutdown_timeout`: must be > 0
- `server.max_body_size`: must be > 0; default 10485760 (10 MB)
- `server.max_filter_output_size`: must be > 0; default 104857600 (100 MB)
- `log.level`: one of `debug`, `info`, `warn`, `error`
- `log.format`: one of `json`, `text`
- `filters[].id`: unique and non-empty
- `filters[].type`: supported type only
- `filters[].depends_on`: all references must exist in filters
- **Cycle detection**: all filters must not have circular dependencies (including implicit dependencies inferred from template references)
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

`params` can be any YAML value (string/object/array).

Template expansion rules:

- If `params` is a **string**, render it with Go `text/template`
- If `params` is an **object or array**, recursively render all string values within using Go `text/template`
- All other types (numbers, booleans, nested non-string values) remain unchanged
- Nesting depth limit: 10 levels (maximum nested object/array depth during expansion)

Examples:

String:

```yaml
type: static
params: "hello {{.A.body.origin}}"
```

Object (all string values expanded):

```yaml
type: static
params:
  message: "hello {{.A.body.origin}}"
  status: ok
  count: 42
```

Result:

```json
{"message": "hello 203.0.113.1", "status": "ok", "count": 42}
```

Array (all string values expanded):

```yaml
type: static
params:
  - "user: {{.req.query.name}}"
  - fixed
  - 123
```

Nested object:

```yaml
type: static
params:
  metadata:
    user: "{{.req.body.username}}"
    timestamp: "{{.A.timestamp}}"
  items: []
```

Template context: uses Go `text/template` without HTML escaping. Embedded quotes require YAML escaping (e.g., `"...\"..."`)

Security note: Template expansion applies only to **config-defined string literals** in filter `params`. User input from `.req` (query, headers, body) is never re-evaluated as template code; it is treated as literal values and safely embedded in results.

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
.{id}.timeout  bool
```

Behavior:

- Executes command directly without shell expansion (no `$VAR`, `|`, `;` interpretation)
- Collects `stdout` and `stderr` into buffers
- On successful completion: `.code` is the exit code, `.timeout` is `false`
- On timeout: process is killed (SIGTERM), returns partial buffered output, `.code` is `-1`, `.timeout` is `true`

Example:

```yaml
filters:
  - id: data
    type: exec
    params:
      command: ["cat", "/data/file.json"]
      timeout: 10s
  - id: check
    type: static
    params: "Status: {{if .data.timeout}}TIMEOUT{{else}}OK ({{.data.code}}){{end}}"
```

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
  pattern: '^(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})$'
  op: find              # find | find_all | replace
  multiline: true       # default: false
  replace: "Date: $year-$month-$day"  # required when op=replace
```

Operation behavior:

- `find`: first match result (returns a map of capture groups)
- `find_all`: all match results (returns an array of maps)
- `replace`: replace all matches with substitution pattern

Capture group behavior:

- Named groups use Go syntax: `(?P<name>...)`
- Result is always a **map** (object):
  - Keys are group names (if named) **and** group numbers (as strings: `"1"`, `"2"`, etc.)
  - Example: `{"year": "2026", "1": "2026", "month": "04", "2": "04"}`
- `find` with no groups returns the full match: `{"0": "..."}`
- `find_all` returns an array of maps: `[{...}, {...}, ...]`
- Replacement text supports both named (`$name`) and numbered (`$1`, `$2`, ...) substitution

`multiline: true` enables `(?m)` semantics for `^` and `$` per line.

Example with named groups:

```yaml
filters:
  - id: parse_date
    type: regex
    params:
      input: "2026-04-17"
      pattern: '^(?P<year>\d{4})-(?P<month>\d{2})-(?P<day>\d{2})$'
      op: find
  - id: formatted
    type: static
    params: "Date: {{.parse_date.year}}-{{.parse_date.month}}-{{.parse_date.day}}"
```

Example with numbered groups:

```yaml
filters:
  - id: extract
    type: regex
    params:
      input: "{{.A.stdout}}"
      pattern: '(\d+)\s+(\w+)'
      op: find_all
  - id: process
    type: static
    params:
      - "{{index .extract 0 \"1\"}}"
      - "{{index .extract 0 \"2\"}}"
```

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
      pattern: '^-[rwx-]{9}\s+\d+\s+\S+\s+\S+\s+\d+\s+\S+\s+\d+\s+[\d:]+\s+(?P<filename>.+)$'
      op: find_all
      multiline: true
```

Access named group in static filter:

```yaml
- id: filenames
  type: static
  params: "{{range .files}}{{.filename}}\n{{end}}"
```

### `cache` — Cache Filter Results

```yaml
type: cache
params:
  filter: A
  ttl: 60s
  key: "{{.req.query.user}}"
```

Caching behavior:

- **Scope**: global in-process memory (shared across all requests)
- **TTL**: time-to-live in seconds; expired entries are automatically cleaned up
- **Key**: template-expanded string; each unique key holds one cached result
- **Execution**: if a cache hit occurs (key exists and TTL not expired), the target filter is skipped and the cached result is returned
- **Multi-instance**: each process maintains its own cache; no cross-instance synchronization (deploy with load balancer for distributed caching)

Example:

```yaml
filters:
  - id: expensive_api
    type: http
    params:
      method: GET
      url: https://api.example.com/data
  - id: cached_result
    type: cache
    params:
      filter: expensive_api
      ttl: 300s
      key: "api:data:{{.req.query.version}}"
```

If two requests arrive with `?version=latest` within 300 seconds, only the first executes `expensive_api`; the second uses the cached result.

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
- Cycle detection occurs at three stages:
  1. **Server startup**: validate all filters in config (detect cycles early)
  2. **SIGHUP reload**: re-validate new config before applying (keep old config on failure)
  3. **Per-request**: detect cycles in the target filter's dependency graph (return 500 on cycle)

Cycle detection algorithm:

- Build dependency graph: `depends_on` fields + automatic extraction from template references in `params`
- Use DFS (depth-first search) to detect cycles
- Cycles in unreachable filters are still reported as config errors (fail validation)

Execution flow:

1. Resolve matching route root
2. Build dependency graph for the target filter (`depends_on` + template reference extraction)
3. Detect cycles in the graph (return 500 if cycle found)
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
- **Security**: All `.req` fields (query, headers, body, etc.) are treated as **literal string values**. Template syntax in user input (e.g., `?name={{.A.stdout}}`) is not interpreted as template code; it is stored as a literal string. This prevents Server-Side Template Injection (SSTI) attacks.

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
    RequestTimeout  time.Duration `yaml:"request_timeout"`
    ReadTimeout     time.Duration `yaml:"read_timeout"`
    WriteTimeout    time.Duration `yaml:"write_timeout"`
    ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
    MaxBodySize     int64         `yaml:"max_body_size"`
    MaxFilterOutputSize int64     `yaml:"max_filter_output_size"`
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
  - `server.network=unix` -> listen on `server.unix_socket`
- Unix Domain Socket startup:
  - If socket file exists: attempt to dial it
  - If dial succeeds: a process is still using it; startup fails with error
  - If dial fails (timeout or connection refused): socket is stale; delete it and create new listener
- Request timeout strategy:
  - Request processing runs under `server.request_timeout` (default: 30s)
  - If timeout is exceeded: cancel request context, stop remaining filter execution, return 500 error
  - Individual filter timeouts (e.g. `exec.params.timeout`) are still applied and can fail earlier
- Request body size limit:
  - Maximum allowed: `server.max_body_size` (default: 10 MB)
  - If body exceeds limit: return `413 Payload Too Large` with error message
  - Request body is read once and cached in memory; all template references use the cached value
- Filter output size limit:
  - Maximum output per filter: `server.max_filter_output_size` (default: 100 MB)
  - If any filter output exceeds limit: filter execution fails, return 500 error
  - Limit checked during filter execution (exec, http, file, etc. check size as they buffer output)
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
- path exact string match (no wildcards, no regex, no path parameters)
- first match wins

Path matching is case-sensitive and requires an exact string match. For example, `/foo` matches only `/foo`, not `/foo/`, `/foobar`, or `/foo/bar`.

`check` uses the same match policy.

### Response Formatting

HTTP status codes:

- `200 OK`: successful filter execution
- `500 Internal Server Error`: filter execution fails, config invalid, or no route match

Response headers:

- Apply `paths[].headers` to the response (takes precedence over filter result headers)
- If the target filter returns an object with a `.headers` field, merge them with `paths[].headers` applied last (overriding any conflicting keys)
- If `Content-Type` is not specified in `paths[].headers`, auto-detect based on the response body:
  - string -> `text/plain; charset=utf-8`
  - object/array -> `application/json; charset=utf-8`

Response body: filter result (string/object/array as-is).

Error response body:

```json
{"error": "internal server error", "code": "FILTER_EXECUTION_FAILED", "request_id": "<id>"}
```

Error detail and information disclosure policy:

- HTTP responses must not include internal details (file paths, command lines, stack traces, dependency graph internals, raw upstream errors)
- HTTP error payload must be stable and minimal: `error`, `code`, `request_id`
- Detailed diagnostics are written only to server logs with the same `request_id` for correlation
- In development mode, diagnostics are still logged, but HTTP payload format remains unchanged
- `check` subcommand is an operator-facing CLI and may print detailed errors to stderr

## Config Hot Reload (SIGHUP)

When `server` receives SIGHUP, reload from `--config`.

Rules:

- parse + validate every reload attempt (including cycle detection)
- atomically apply only on success
- on validation failure: keep old config, continue serving, and log the reason
- cache is preserved across reload (old cache entries remain valid for existing filters)
- validation uses same rules as server startup: `depends_on`, template reference extraction, and cycle detection

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
- `server.request_timeout`
- `server.read_timeout`
- `server.write_timeout`
- `server.shutdown_timeout`
- `server.max_body_size`
- `server.max_filter_output_size`

Required logs:

- **Success**: `config reload succeeded` (at info level)
- **Failure**: `config reload failed: <detailed reason>` (at error level)

Log safety rules:

- Logs may include detailed technical reasons, but must redact secrets (authorization headers, tokens, passwords, private keys)
- Do not log full request bodies by default; log size and content type instead

Failure reasons must include details such as:
- `file not found: /path/to/config.yaml`
- `parse error at line X: ...`
- `validation error: filter 'id' referenced but not defined`
- `cycle detected: A → B → C → A`

## Graceful Shutdown

On `SIGINT`/`SIGTERM` in `server` mode:

1. Stop accepting new requests
2. Wait for in-flight requests within `server.shutdown_timeout`
3. Force stop and return error on timeout
