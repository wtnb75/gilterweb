# gilterweb — 仕様書

コーディング規約・ディレクトリ構造は [AGENTS.md](AGENTS.md) を参照。

## サブコマンド一覧

| サブコマンド | 説明 |
|---|---|
| `server` | **メイン機能**。HTTPサーバを起動する |
| `version` | バージョン情報を表示する |
| `config check` | 設定ファイルの構文・バリデーションを確認する |

### グローバルフラグ

```
  --config string   設定ファイルのパス (default "config.yaml")
```

### server サブコマンド

HTTPサーバを起動して接続を待ち受ける。シグナル（SIGINT / SIGTERM）を受け取ったらグレースフルシャットダウンする。

```
gilterweb server [flags]
  --addr string   バインドアドレス（設定ファイルの server.addr を上書き）
```

### version サブコマンド

ビルド時に埋め込まれたバージョン文字列・コミットハッシュ・ビルド日時を標準出力に出力する。

```
gilterweb version
```

出力例:
```
gilterweb version v0.1.0 (commit: abc1234, built: 2026-04-16T00:00:00Z)
```

### config check サブコマンド

`--config` で指定したファイルを読み込み、構文エラーおよびバリデーションエラーを報告する。問題がなければ exit 0、エラーがあれば exit 1。

```
gilterweb config check [flags]
```

## 設定ファイル仕様 (YAML)

`example-config.yaml` をリポジトリに含める。設定ファイルのデフォルトパスは `config.yaml`。

```yaml
server:
  addr: ":8080"           # バインドアドレス（必須）
  read_timeout: 30s       # 読み込みタイムアウト（省略時: 30s）
  write_timeout: 30s      # 書き込みタイムアウト（省略時: 30s）
  shutdown_timeout: 10s   # グレースフルシャットダウン待機時間（省略時: 10s）

log:
  level: info   # debug | info | warn | error（省略時: info）
  format: json  # json | text（省略時: json）
```

### バリデーションルール

| フィールド | ルール |
|---|---|
| `server.addr` | 空文字列不可 |
| `server.read_timeout` | 0より大きい値 |
| `server.write_timeout` | 0より大きい値 |
| `server.shutdown_timeout` | 0より大きい値 |
| `log.level` | `debug` / `info` / `warn` / `error` のいずれか |
| `log.format` | `json` / `text` のいずれか |

## 設定構造体

```go
type Config struct {
    Server ServerConfig `yaml:"server"`
    Log    LogConfig    `yaml:"log"`
}

type ServerConfig struct {
    Addr            string        `yaml:"addr"`
    ReadTimeout     time.Duration `yaml:"read_timeout"`
    WriteTimeout    time.Duration `yaml:"write_timeout"`
    ShutdownTimeout time.Duration `yaml:"shutdown_timeout"`
}

type LogConfig struct {
    Level  string `yaml:"level"`
    Format string `yaml:"format"`
}
```

デフォルト値は構造体初期化時に設定する（`yaml.Unmarshal` 前に埋め込む）。

## HTTPサーバ仕様

- ルーター: `net/http` 標準の `http.ServeMux`
- ミドルウェア: 関数チェーン `func(http.Handler) http.Handler`
- 必須ミドルウェア:
  - アクセスログ（slog でリクエストメソッド・パス・ステータスコード・レイテンシを記録）
  - リカバリ（パニックを捕捉して 500 を返す）

### エンドポイント

| メソッド | パス | 説明 |
|---|---|---|
| GET | `/healthz` | ヘルスチェック。`{"status":"ok"}` を返す |
| GET | `/` | ルートハンドラ（実装内容は拡張ポイント） |

### レスポンス形式

JSON レスポンスの `Content-Type` は `application/json; charset=utf-8`。

エラーレスポンス例:
```json
{"error": "not found"}
```

## グレースフルシャットダウン

`server` サブコマンドは SIGINT / SIGTERM を受け取ったら以下の手順でシャットダウンする。

1. 新規リクエストの受付を停止
2. `server.shutdown_timeout` 内に進行中リクエストの完了を待つ
3. タイムアウトした場合は強制終了してエラーを返す
