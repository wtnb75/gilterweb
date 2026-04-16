# gilterweb — Agent Instructions

Go製のWebサーバプロジェクト。プログラムの仕様は [SPEC.md](SPEC.md) を参照。

## プロジェクト概要

- 言語: Go
- 主機能: `server` サブコマンドで起動するHTTPサーバ
- 設定: YAMLファイル
- Linter: golangci-lint
- テストカバレッジ目標: 90%以上

## ディレクトリ構造

```
gilterweb/
├── AGENTS.md
├── SPEC.md
├── Taskfile.yml
├── go.mod
├── go.sum
├── .golangci.yml
├── .gitignore
├── config.go          # 設定構造体・読み込みロジック
├── config_test.go
├── handler.go         # HTTPハンドラ
├── handler_test.go
├── server.go          # サーバ起動ロジック
├── server_test.go
├── cmd/
│   └── gilterweb/
│       └── main.go    # エントリーポイント（サブコマンド定義）
├── example-config.yaml
└── cover.out
```

## コーディング規約

- パッケージ名: `gilterweb`（main パッケージは `cmd/gilterweb/`）
- エラーハンドリング: `fmt.Errorf("...: %w", err)` でラップ
- ログ: `log/slog` を使用（構造化ログ）
- CLIフレームワーク: `github.com/spf13/cobra` を使用
- 設定読み込み: `gopkg.in/yaml.v3` を使用
- HTTPハンドラは `http.Handler` インタフェースを実装した構造体として定義
- ミドルウェアは関数チェーン `func(http.Handler) http.Handler` で実装
- `server` サブコマンドは SIGHUP を監視し、設定ホットリロード（成功時のみ反映・失敗時は旧設定維持）を実装

## Linter 設定 (.golangci.yml)

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

## テスト規約

- カバレッジ目標: **90%以上**
- `go test -v -cover -coverprofile=cover.out ./...` で計測
- テストファイルは実装ファイルと同じパッケージに置く（`_test` サフィックスパッケージは統合テストのみ）
- HTTPハンドラのテストは `net/http/httptest` を使用
- 設定ファイル読み込みのテストはテンポラリファイルを使用（`t.TempDir()`）

## Taskfile.yml タスク

| タスク | 説明 |
|---|---|
| `task test` | テスト実行・カバレッジ計測・HTMLレポート生成 |
| `task lint` | `go fix` + `go fmt` + `golangci-lint run` |
| `task build` | `go build ./cmd/gilterweb/` |
| `task run` | `go run ./cmd/gilterweb/ server` |
| `task cover` | カバレッジが90%未満なら失敗 |

`cover` タスクの実装例:

```yaml
cover:
  desc: check coverage >= 90%
  cmds:
    - go test -coverprofile=cover.out ./...
    - go tool cover -func=cover.out | awk '/^total:/{if ($3+0 < 90) {print "Coverage "$3" < 90%"; exit 1}}'
```

## 品質基準

- `golangci-lint run` がエラーなしで通ること
- `go vet ./...` がエラーなしで通ること
- カバレッジ 90% 以上
- `go build ./...` がエラーなしで通ること
- `example-config.yaml` が `validate` サブコマンドでバリデーション通過すること

## 開発フロー

1. `SPEC.md` を確認して実装対象の仕様を把握する
2. 設定構造体・読み込みロジック (`config.go`) を実装しテストを書く
3. HTTPハンドラ (`handler.go`) を実装しテストを書く
4. サーバ起動ロジック (`server.go`) を実装しテストを書く
5. `cmd/gilterweb/main.go` でサブコマンドを組み立てる
6. `task lint` と `task cover` を通してから PR を出す
