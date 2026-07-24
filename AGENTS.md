# AGENTS.md

LLM / 自動化エージェントがこのリポジトリで作業する際のガイド。人間の開発者にも適用する。
本ファイルは規約の索引を兼ねる。詳細は [`README.md`](README.md) / [`CONTRIBUTING.md`](CONTRIBUTING.md) /
`docs/design_docs/` を参照する。

## プロジェクト概要

[quote/0](https://dot.mindreset.tech)(e-ink スマートディスプレイ)へ外部コンテンツを定期整形して
表示するバッチ処理基盤。Go 1.25 / 標準ライブラリ中心。現在は M1(PoC 疎通)段階。

## アーキテクチャ

Ports & Adapters(ヘキサゴナル)構成。依存方向は常に **外側 → 内側**。

```
cmd/batch/main.go       合成ルート(DI 配線 → RunBatch 実行)
internal/
  domain/               TextPayload 型 + ContentSource/ContentSink port(外部依存なし)
  application/          RunBatch ユースケース(source→sink を協調)
  adapter/in/           入力側 adapter(helloworld / M3 で weather)
  adapter/out/          出力側 adapter(quote/0 Text API 送信)
  config/               環境変数ロード / バリデーション
```

- `domain` は他パッケージに依存しない。外部 I/O は port(interface)越しに扱う。
- 新しい入力源・出力先は port を実装する adapter として追加し、既存ユースケースを変更しない。

## 開発フロー

- **Design Doc が先、実装が後**。実装は必ず対応する `docs/design_docs/` セクションに紐づく。
- **main 直 push 禁止**。全変更は作業ブランチを切り PR 経由。CI green を merge 条件とする。
- **1 PR = 1 関心事**。ついで修正は別 PR に分ける。
- 詳細な PR 分割方針・レビュー観点は [`CONTRIBUTING.md`](CONTRIBUTING.md) を参照。

## ビルド / テスト / 静的解析

作業完了前に以下がすべて green であることを確認する。

```sh
go build ./...
go test ./...     # httptest でモック。ネットワーク非依存・secrets 不要
go vet ./...
gofmt -l .        # 出力が空なら整形済み。差分があれば gofmt -w で整形
golangci-lint run # standard プリセット(errcheck/govet/ineffassign/staticcheck/unused)
```

## テスト命名規則

テスト関数名と、実行時に表示されるケース名を二層で管理する。

### 関数名(機械識別子)

- `Test<対象型/関数>_<振る舞い>` の英語・アンダースコア区切り。
  例: `TestLoad_ErrorHidesSecretValues`, `TestQuote0Sink_Send_Non2xxReturnsError`。
- `go test -run` で選択する識別子。英語のまま維持する。

### ケース名(`t.Run` / テーブル `name`) — 日本語完結文

**テンプレート(推奨):** `<対象>は<条件>のとき<期待動作>する`

- 全テストは本体を `t.Run("<完結文>", func(t *testing.T){ ... })` で1段ラップし、日本語の完結文で
  振る舞いを1行の仕様として表す。テーブル駆動テストは `name` フィールドに同形式で記述する。
- 主語(対象=型.メソッド / 関数)を明示し、英語関数名と対応が取れるようにする。
- 文中の技術用語(`DOT_BASE_URL`, `payload`, `BaseURL`, `sink` 等)は英語のまま混ぜてよい。
- 体言止め・断片(`DOT_API_KEY 欠落で fail-fast`)は使わず、述語で締めた完結文にする。

```go
func TestLoad_ErrorHidesSecretValues(t *testing.T) {
	t.Run("Load はエラーメッセージに秘匿値を含めない", func(t *testing.T) {
		// ...
	})
}
```

良い例: `Load は DOT_API_KEY が欠落すると起動に失敗する` /
`Quote0Sink.Send はエラー時に API Key とシリアル全体を漏らさない`

### アサーション / コメント

- 失敗メッセージ(`t.Errorf` / `t.Fatalf`)・コメントも日本語で書く。実際値と期待値を含める。
  例: `t.Errorf("BaseURL = %q, want %q", got, want)`。

## 実装規則

- **標準ライブラリ優先**。テストは Go 標準 `testing`(テーブル駆動 + `t.Run`)。testify 等は導入しない。
- **整形は gofmt / goimports に従う**。手動整形しない。
- **エラーは握り潰さない**。文脈を付けて `fmt.Errorf("...: %w", err)` でラップし、`errors.Is/As` で判定する。
- **外部境界は interface(port)経由**。HTTP クライアントはテスト用に差し替え可能な形で受け取る
  (例: `NewQuote0Sink(cfg, client)`、nil 時は Timeout 付き既定クライアント)。
- **時刻はテスト可能に**。`time.Now` を直接呼ばず注入する(例: `WeatherSource{now: func() time.Time}`)。
  表示はJSTへ明示変換する。
- コメント・ドキュメントは日本語。

## secret / 環境変数

- **秘匿情報(`DOT_API_KEY` / `SERIAL_NUM`)をコード・ログ・エラー・PR 差分に平文で出さない**。
  シリアルは `MaskedSerial`(末尾4桁のみ)でマスクする。
- ローカルは `.env`(gitignore 対象)、CI は GitHub Secrets(Environment `production`)。
  必要キーは `.env.example`(値は空)で共有する。
- secret を扱わない CI(`ci.yml`)と実送信(`batch.yml`)を分離する方針を崩さない。

## コミット / PR

- コミット・PR タイトルは **Conventional Commits**(`feat:` / `fix:` / `docs:` / `chore:` …)。
- **PR タイトル・本文は日本語**。本文に以下を必ず含める:
  - 対応 Design Doc セクションへのリンク
  - スコープ(何を含み、何を含まないか)
  - 動作確認方法(テスト / 手動手順)
- スコープ外の変更を混ぜない。
