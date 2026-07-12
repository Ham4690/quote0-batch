# quote0-batch

[quote/0](https://dot.mindreset.tech)(e-ink スマートディスプレイ)へ、外部コンテンツを
定期的に整形して表示するバッチ処理基盤。当面の連携対象は天気情報のみで、1 時間ごとに
取得してデバイスへ表示する。

設計の詳細は [`docs/design_docs/0001-initial-setup.md`](docs/design_docs/0001-initial-setup.md) を参照。
開発フロー・PR 分割方針は [`CONTRIBUTING.md`](CONTRIBUTING.md) を参照。

> 現在は **M1(PoC 疎通)** の段階。固定文言 `Hello World` を Text API へ送信し、実機表示を確認する。
> 天気連携は M3 で追加する。

## アーキテクチャ

Ports & Adapters(ヘキサゴナル)構成。入力(コンテンツ取得)と出力(quote/0 送信)を
port(interface)で抽象化し、実装を adapter として差し替え可能にする。

```
cmd/batch/main.go       合成ルート(DI 配線 → RunBatch 実行)
internal/
  domain/               TextPayload 型 + ContentSource/ContentSink port(外部依存なし)
  application/          RunBatch ユースケース(source→sink を協調)
  adapter/in/           入力側 adapter(helloworld / M3 で weather)
  adapter/out/          出力側 adapter(quote/0 Text API 送信)
  config/               環境変数ロード / バリデーション
```

## セットアップ

必要なもの: Go 1.23 以上。

```sh
cp .env.example .env    # 値を埋める(.env はコミット禁止)
```

| 変数 | 内容 | 秘匿 | 既定値 |
| --- | --- | --- | --- |
| `DOT_API_KEY` | quote/0 Text API の Bearer トークン | Yes | なし(必須) |
| `SERIAL_NUM` | デバイスのシリアルナンバー | Yes | なし(必須) |
| `DOT_BASE_URL` | API ベース URL | No | `https://dot.mindreset.tech` |

`DOT_API_KEY` / `SERIAL_NUM` はどちらも秘匿情報。シリアルも漏洩すると第三者が実機へ
書き込み可能になるため秘匿扱いとし、ログ・エラー出力ではマスクする。

## ローカル実行

```sh
set -a; source .env; set +a   # .env を環境変数へ読み込む
go run ./cmd/batch            # Hello World を実機へ送信
```

必須環境変数が欠落している場合は起動時に fail-fast する。

## テスト

```sh
go test ./...   # httptest でモック(ネットワーク非依存・secrets 不要)
go vet ./...
gofmt -l .      # 出力が空なら整形済み
```

## CI / CD(GitHub Actions)

- **`ci.yml`**: PR / `main` への push で golangci-lint(go vet 内包)+ `go test ./...` を実行。
  外部 API を叩かないため secrets 不要 → fork PR でも安全に走る。
- **`batch.yml`**: quote/0 へ実送信するワークフロー。secrets(`DOT_API_KEY` / `SERIAL_NUM`)を
  使うため `workflow_dispatch`(手動)/ `schedule`(cron)のみで起動。M1 は手動実行で疎通確認し、
  M2 で毎時 cron(`0 * * * *`)を有効化する。

secret は GitHub Actions Secrets(Environment `production`)で管理する。public リポジトリでも
Actions はログを自動マスクし fork PR には secrets を注入しないため安全に運用できる。

## API Key ローテーション(漏洩時)

1. quote/0 の管理画面で API Key を再発行する。
2. GitHub の Settings → Environments → `production` の `DOT_API_KEY` を更新する。
3. ローカルの `.env` を更新する。
4. 旧 Key を失効させる。

## 参考

- [quote/0(dot.mindreset.tech)](https://dot.mindreset.tech)
- Text API エンドポイント: `POST /api/authV2/open/device/:serialNum/text`
- 設計判断の背景: [`docs/design_docs/0001-initial-setup.md`](docs/design_docs/0001-initial-setup.md)
