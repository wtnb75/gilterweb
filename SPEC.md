# gilterweb — 仕様書

コーディング規約・ディレクトリ構造は [AGENTS.md](AGENTS.md) を参照。

## サブコマンド一覧

| サブコマンド | 説明 |
|---|---|
| `server` | **メイン機能**。HTTPサーバを起動する |
| `check` | HTTPサーバ未起動で、指定 method/path の評価結果を標準出力に表示する |
| `version` | バージョン情報を表示する |
| `validate` | 設定ファイルの構文・バリデーションを確認する |

### グローバルフラグ

```
  --config string   設定ファイルのパス (default "config.yaml")
```

### server サブコマンド

HTTPサーバを起動して接続を待ち受ける。シグナル（SIGINT / SIGTERM）を受け取ったらグレースフルシャットダウンする。SIGHUP を受け取ったら設定ファイルを再読み込みする。

```
gilterweb server [flags]
  --addr string   バインドアドレス（設定ファイルの server.addr を上書き）
```

### check サブコマンド

HTTPサーバを起動せず、指定した疑似リクエスト（method/path）で `paths` のルーティングとフィルタ実行を 1 回だけ行う。

```
gilterweb check [flags]
  --method string   リクエストメソッド (default "GET")
  --path string     リクエストパス (required)
  --header strings  リクエストヘッダ（`Key: Value`、複数指定可）
  --content-type string  リクエスト Content-Type
  --body string     リクエストボディ文字列
  --body-file string  リクエストボディファイル
```

使用例:
```
gilterweb check --method GET --path /foo
```

動作:
- 設定ファイル読み込み・バリデーション
- `paths` から method/path 一致エントリ選択（`server` と同一規則）
- 指定フラグから疑似リクエストコンテキスト（`.req`）を構築
- 対象 `filter` 実行
- 結果を標準出力へ表示（文字列はそのまま、オブジェクト/配列は JSON）

終了コード:
- `0`: 実行成功
- `1`: 設定不正、path 未一致、フィルタ実行失敗

### version サブコマンド

ビルド時に埋め込まれたバージョン文字列・コミットハッシュ・ビルド日時を標準出力に出力する。

```
gilterweb version
```

出力例:
```
gilterweb version v0.1.0 (commit: abc1234, built: 2026-04-16T00:00:00Z)
```

### validate サブコマンド

`--config` で指定したファイルを読み込み、構文エラーおよびバリデーションエラーを報告する。問題がなければ exit 0、エラーがあれば exit 1。

```
gilterweb validate [flags]
```

## コアコンセプト: フィルタパイプライン

HTTPリクエストが届いたとき、設定で定義した**フィルタ**を実行してデータを組み立て、レスポンスを返す。

```
incoming request
      ↓
 paths でフィルタを選択
      ↓
 フィルタ実行（他フィルタの結果を参照可）
      ↓
 フィルタ出力をレスポンスとして返す
```

### 設定例

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

`GET /foo` → フィルタ A で外部API呼び出し → フィルタ B でテンプレート展開 → `hello 203.0.113.1` を返す。

## 設定ファイル仕様 (YAML)

`example-config.yaml` をリポジトリに含める。設定ファイルのデフォルトパスは `config.yaml`。

```yaml
server:
  network: tcp            # tcp | unix（省略時: tcp）
  addr: ":8080"           # バインドアドレス（必須）
  unix_socket: ""          # network=unix のとき必須（例: /tmp/gilterweb.sock）
  unix_socket_mode: "0660" # ソケットファイル権限（省略時: 0660）
  read_timeout: 30s       # 読み込みタイムアウト（省略時: 30s）
  write_timeout: 30s      # 書き込みタイムアウト（省略時: 30s）
  shutdown_timeout: 10s   # グレースフルシャットダウン待機時間（省略時: 10s）

log:
  level: info   # debug | info | warn | error（省略時: info）
  format: json  # json | text（省略時: json）

filters:
  - id: string        # フィルタID（paths から参照）
    type: string      # フィルタ種別（後述）
    depends_on:       # 依存フィルタID（省略可）
      - string
    params: any       # 種別ごとのパラメータ

paths:
  - method: string              # HTTP メソッド（GET / POST / PUT / DELETE / * ）
    path: string                # パスパターン（Go の http.ServeMux 形式）
    filter: string              # 使用するフィルタ ID
    headers:                    # レスポンスヘッダ（省略可）
      Content-Type: text/plain  # 省略時は自動判定（後述）
```

### バリデーションルール

- `server.network`: `tcp` / `unix` のいずれか
- `server.addr`: `network=tcp` のとき空文字列不可
- `server.unix_socket`: `network=unix` のとき空文字列不可
- `server.unix_socket_mode`: 4桁8進数文字列（例: `0660`）
- `server.read_timeout` / `write_timeout` / `shutdown_timeout`: 0より大きい値
- `log.level`: `debug` / `info` / `warn` / `error` のいずれか
- `log.format`: `json` / `text` のいずれか
- `filters[].id`: 重複不可、空文字列不可
- `filters[].type`: 定義済み種別のいずれか
- `filters[].depends_on`: 定義済みフィルタ ID のみ、循環参照禁止
- `paths[].filter`: 定義済みフィルタ ID を参照すること
- `paths[].headers`: キー・値とも文字列（省略可）

## フィルタ種別

### `http` — 外部HTTPリクエスト

```yaml
type: http
params:
  method: GET                 # HTTP メソッド（必須）
  url: https://...            # リクエスト先URL（必須）
  unix_socket: ""             # UDS経由HTTP時に指定（省略時: TCP）
  headers:                    # 追加リクエストヘッダ（省略可）
    X-Foo: bar
  body: "..."                 # リクエストボディ（省略可）
```

`unix_socket` 指定時、HTTPクライアントは TCP ではなく Unix Domain Socket へ接続する。
このとき `url` は `http://localhost/...` のような HTTP URL を使う（パス・クエリ・Host ヘッダ生成に使用）。

UDS例:

```yaml
type: http
params:
  method: GET
  url: http://localhost/ip
  unix_socket: /var/run/httpbin.sock
```

### `static` — 値返却（テンプレート展開あり）

`params` は任意の YAML 値（文字列・オブジェクト・配列）。**すべての文字列値に対して Go の `text/template` が適用される**。

文字列:
```yaml
type: static
params: "hello {{.A.body.origin}}"
```

オブジェクト（値側にテンプレート可）:
```yaml
type: static
params:
  message: "hello {{.A.body.origin}}"
  status: ok
```

配列:
```yaml
type: static
params:
  - "{{.A.body.origin}}"
  - fixed
```

テンプレート不要な場合はそのまま書けばよい。`template` 種別は廃止し本種別に統合。

### `env` — 環境変数取得

```yaml
type: env
params:
  name: MY_SECRET_TOKEN   # 環境変数名（必須）
  default: ""             # 未設定時のデフォルト値（省略可）
```

結果は文字列。`.{id}` で直接参照できる。

### `exec` — 外部コマンド実行

```yaml
type: exec
params:
  command: ["jq", "-r", ".origin", "/tmp/data.json"]  # 引数配列（シェル展開なし）
  timeout: 5s              # タイムアウト（省略時: 5s）
  env:                     # 追加環境変数（省略可）
    FOO: bar
```

結果:

```
.{id}.stdout   string   標準出力
.{id}.stderr   string   標準エラー出力
.{id}.code     int      終了コード
```

**注意**: `command` はシェルを介さず直接 exec する。シェル展開（`$VAR`, `|`, `;`）は不可。

### `file` — ファイル内容読み込み

```yaml
type: file
params:
  path: /etc/data/config.json   # ファイルパス（必須）
  parse: json                   # json | text（省略時: text）
```

`parse: json` 指定時は JSON パースした結果を返す。

### `jq` — jq 式で JSON 変換

```yaml
type: jq
params:
  input: "{{.A.raw}}"    # 入力 JSON 文字列（テンプレート可）
  query: ".origin"        # jq 式（必須）
```

`github.com/itchyny/gojq` で実装。外部 `jq` コマンド不要。結果は jq の出力値（文字列・数値・オブジェクト等）。

### `base64` — Base64 エンコード/デコード

```yaml
type: base64
params:
  input: "{{.A.body.token}}"   # 入力文字列（テンプレート可）
  op: encode                    # encode | decode
```

### `regex` — 正規表現で抽出・置換

```yaml
type: regex
params:
  input: "{{.A.stdout}}"   # 入力文字列（テンプレート可）
  pattern: '^-[rwx-]{9}\s+\d+\s+\S+\s+\S+\s+\d+\s+\S+\s+\d+\s+\d{2}:\d{2}\s+(.+)$'
  op: find_all              # find | find_all | replace
  multiline: true           # 行ごとにマッチ（省略時: false）
  replace: "***"            # op: replace 時のみ必須
```

#### op の動作

- `find` — 入力全体から最初のマッチを返す。結果は文字列
- `find_all` — 全マッチを返す。結果は配列
- `replace` — 全マッチを `replace` 文字列で置換した結果を返す。結果は文字列

#### multiline

`true` のとき `(?m)` フラグを有効化。`^` / `$` が各行頭・行末にマッチするようになる。複数行テキスト（`exec` の stdout など）を行単位で処理する場合に使用。

#### キャプチャグループ

`pattern` にキャプチャグループ `(...)` が含まれる場合:

- `find` — グループ数が 1 つ: グループ 1 の文字列を返す。複数グループ: グループ文字列の配列 `["g1", "g2", ...]` を返す
- `find_all` — 各マッチについてグループを展開した配列の配列 `[["g1a", "g2a"], ["g1b", "g2b"]]` を返す
- `replace` — `replace` 文字列内で `$1`, `$2` によるグループ参照が使える

`ls -la` 出力からファイル名一覧を取り出す例:

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
      pattern: '^-[rwx-]{9}\s+\d+\s+\S+\s+\S+\s+\d+\s+\S+\s+\d+\s+[\d:]+\s+(.+)$'
      op: find_all
      multiline: true
```

結果 (`.files`):
```json
[["AGENTS.md"], ["SPEC.md"]]
```

グループが 1 つなので `static` でフラット化可:
```yaml
  - id: filenames
    type: static
    params: "{{range .files}}{{index . 0}}\n{{end}}"
```

#### 結果の型まとめ

| op | グループなし | グループ 1 つ | グループ複数 |
|---|---|---|---|
| `find` | `string` | `string` | `[]string` |
| `find_all` | `[]string` | `[]string` | `[][]string` |
| `replace` | `string` | `string` | `string` |

### `cache` — フィルタ結果キャッシュ

```yaml
type: cache
params:
  filter: A      # キャッシュ対象のフィルタ ID
  ttl: 60s       # キャッシュ有効期間（必須）
  key: "{{.req.query.user}}"  # キャッシュキー（省略時: フィルタ ID のみ）
```

TTL 内は対象フィルタを再実行せず前回結果を返す。キャッシュはプロセス内メモリ。

## フィルタ選択と実行戦略

目的: 必要フィルタのみ実行。重い処理（`exec` など）重複実行 回避。

ルール:

- リクエストごとに実行対象 root は `paths[].filter` の 1 つ
- 実行対象は root から到達可能な依存フィルタ集合のみ
  - 依存元: `depends_on`
  - 依存元: `static` などのテンプレート参照（例: `{{.A.stdout}}`）
- 到達不能フィルタは実行しない
- 同一リクエスト内で同一 filter ID は最大 1 回実行（メモ化）

実行手順:

1. `paths` から一致ルート取得
2. 依存グラフ構築（`depends_on` + テンプレート参照）
3. 循環検出（検出時エラー）
4. トポロジカル順で評価
5. 各フィルタ結果を `resultMap[id]` へ保存
6. 参照時は `resultMap` 再利用

例（A/B/C）:

- A: `type=exec`
- B: `type=static`, `params: "out={{.A.stdout}} err={{.A.stderr}}"`
- C: 無関係
- `paths[].filter = B`

このとき:

- A は 1 回だけ実行（stdout/stderr 同一結果オブジェクトから参照）
- B 実行
- C 未実行

### フィルタ結果データモデル

各フィルタの実行結果はフィルタ ID をキーとしてコンテキストに格納され、後続フィルタのテンプレートから参照できる。

`http` フィルタの結果:

```
.{id}.status   int              HTTPステータスコード
.{id}.headers  map[string]string レスポンスヘッダ
.{id}.body     any              JSONパース済みボディ（パース失敗時は文字列）
.{id}.raw      string           生レスポンスボディ
```

テンプレート内での参照例:

```
{{.A.status}}           → 200
{{.A.body.origin}}      → "203.0.113.1"
{{.A.headers.Content-Type}} → "application/json"
```

加えて、受信リクエスト自体も `.req` でアクセス可:

```
.req.method             → "GET"
.req.path               → "/foo"
.req.host               → Host ヘッダ値
.req.remote_addr        → クライアントアドレス
.req.query.{key}        → クエリパラメータ
.req.headers.{key}      → リクエストヘッダ
.req.content_type       → Content-Type
.req.body_raw           → 生ボディ（bytes）
.req.body_text          → UTF-8文字列ボディ
.req.body               → パース済みボディ
```

`req.body` のパース規則:

- `application/json` → JSON パース結果（object/array/number/string/bool/null）
- `application/x-www-form-urlencoded` → `map[string][]string`
- その他、またはパース失敗 → `body_text` と同じ文字列

注意:

- リクエストボディは 1 回だけ読み取り、以降は共有キャッシュを使う
- テンプレート参照ではヘッダキーに `-` を含む場合 `index` を使う
  - 例: `{{index .req.headers "User-Agent"}}`

`static` フィルタ例（POST JSON 参照）:

```yaml
filters:
  - id: echo
    type: static
    params:
      message: "hello {{.req.body.name}} ua={{index .req.headers \"User-Agent\"}}"
```

## 設定構造体

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

デフォルト値は構造体初期化時に設定する（`yaml.Unmarshal` 前に埋め込む）。

## HTTPサーバ仕様

- ルーター: `net/http` 標準の `http.ServeMux`
- リスナー:
  - `server.network=tcp` → `server.addr` で待受
  - `server.network=unix` → `server.unix_socket` で待受（起動時に既存ソケットを安全に置換）
- ミドルウェア: 関数チェーン `func(http.Handler) http.Handler`
- 必須ミドルウェア:
  - アクセスログ（slog でリクエストメソッド・パス・ステータスコード・レイテンシを記録）
  - リカバリ（パニックを捕捉して 500 を返す）

### 組み込みエンドポイント（設定不要）

- `GET /healthz` — ヘルスチェック。`{"status":"ok"}` を返す

### 動的エンドポイント

`paths` セクションの定義に従い起動時に登録。`method: "*"` は全メソッドにマッチ。

マッチ規則:
- 上から順に評価
- `method` 完全一致、または `*`
- `path` 一致
- 最初に一致した 1 件を採用

`check` サブコマンドも同一規則を使用。

### レスポンス形式

`paths[].headers` に指定したヘッダをレスポンスに付与する。`Content-Type` を指定した場合はその値を使用する。省略時は自動判定:

- フィルタ出力が文字列 → `text/plain; charset=utf-8` でそのまま返す
- フィルタ出力がオブジェクト/配列 → `application/json; charset=utf-8` で JSON シリアライズして返す

エラーレスポンス:
```json
{"error": "filter execution failed: ..."}
```

## 設定ホットリロード（SIGHUP）

`server` サブコマンド実行中、SIGHUP 受信で `--config` の設定ファイルを再読み込みする。

適用ルール:
- 再読み込み時、構文チェック + バリデーション実行
- 成功時のみ新設定をアトミックに反映
- 失敗時は旧設定を維持し、エラーログ出力して継続動作

反映対象:
- `filters`
- `paths`
- `paths[].headers`
- `log.level` / `log.format`

非反映対象（再起動必要）:
- `server.network`
- `server.addr`
- `server.unix_socket`
- `server.unix_socket_mode`
- `server.read_timeout`
- `server.write_timeout`
- `server.shutdown_timeout`

ログ要件:
- 成功: `config reload succeeded`
- 失敗: `config reload failed`（原因付き）

## グレースフルシャットダウン

`server` サブコマンドは SIGINT / SIGTERM を受け取ったら以下の手順でシャットダウンする。

1. 新規リクエストの受付を停止
2. `server.shutdown_timeout` 内に進行中リクエストの完了を待つ
3. タイムアウトした場合は強制終了してエラーを返す
