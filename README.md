# quote0-batch

[quote/0](https://dot.mindreset.tech)(e-ink スマートディスプレイ)へ、外部コンテンツを
定期的に整形して表示するバッチ処理基盤。当面の連携対象は天気情報のみで、毎日 0 時(JST)に
当日の天気予報を取得してデバイスへ表示する。

設計の詳細は [`docs/design-docs/`](docs/design-docs/) を参照(0001 初期構成 / 0002 天気連携 / 0003 地点名タイトル /
0004 CI の setup-go リトライ / 0005 Canvas API 移行。0005 が 0001・0002 の Text API 記述を置き換える)。
開発フロー・PR 分割方針は [`CONTRIBUTING.md`](CONTRIBUTING.md) を参照。

> 現在は **M3(天気連携)** まで実装済み。[天気 API(tsukumijima, livedoor 互換)](https://weather.tsukumijima.net)から
> 当日の予報を取得し、Canvas API でピクセルフォント描画して送信する。

## 表示内容

地点名 / 天気概況 / 日付 / 最低・最高気温(未発表の場合は null を許容)/ 4 時間帯の降水確率 / JST の署名を
Canvas(`windowData`)で描画する。時刻は JST で表示し、tzdata はバイナリに埋め込む。

## アーキテクチャ

Ports & Adapters(ヘキサゴナル)構成。入力(コンテンツ取得)と出力(quote/0 送信)を
port(interface)で抽象化し、実装を adapter として差し替え可能にする。

```
cmd/batch/main.go       合成ルート(DI 配線 → RunBatch 実行)
internal/
  domain/               canvas.go(CanvasPayload)/ content.go(ContentSource/ContentSink port)/
                        weather.go(Forecast, RainChance)。外部依存なし
  application/          RunBatch ユースケース(source→sink を協調)
  adapter/in/weather/   天気 API → Forecast → Canvas windowData 組み立て(source.go / dto.go / testdata/)
  adapter/out/          出力側 adapter(quote0.go: Quote0Sink が Canvas API へ送信)
  config/               環境変数ロード / バリデーション
```

## セットアップ

必要なもの: Go 1.25 以上(標準ライブラリのみ・外部依存なし)。

```sh
cp .env.example .env    # 値を埋める(.env はコミット禁止)
```

| 変数 | 内容 | 秘匿 | 既定値 |
| --- | --- | --- | --- |
| `DOT_API_KEY` | quote/0 Canvas API の Bearer トークン | Yes | なし(必須) |
| `SERIAL_NUM` | デバイスのシリアルナンバー | Yes | なし(必須) |
| `DOT_BASE_URL` | API ベース URL | No | `https://dot.mindreset.tech` |
| `CITY_CODE` | 天気 API の地点コード | No | `130010`(東京) |
| `WEATHER_BASE_URL` | 天気 API ベース URL | No | `https://weather.tsukumijima.net` |
| `WEATHER_LINK_URL` | タップ遷移先 URL | No | 未設定なら API の link(気象庁) |

`DOT_API_KEY` / `SERIAL_NUM` はどちらも秘匿情報。シリアルも漏洩すると第三者が実機へ
書き込み可能になるため秘匿扱いとし、ログ・エラー出力ではマスクする。

## ローカル実行

```sh
set -a; source .env; set +a   # .env を環境変数へ読み込む
go run ./cmd/batch            # 天気予報を実機へ送信
```

必須環境変数が欠落している場合は起動時に fail-fast する。

## テスト

```sh
go build ./...
go test ./...   # httptest でモック(ネットワーク非依存・secrets 不要)
go vet ./...
gofmt -l .      # 出力が空なら整形済み
golangci-lint run   # v2 / standard + gofmt + goimports
```

## CI / CD(GitHub Actions)

- **`ci.yml`**: PR / `main` への push で golangci-lint(go vet 内包)+ `go test ./...` を実行。
  外部 API を叩かないため secrets 不要 → fork PR でも安全に走る。
- **`batch.yml`**: quote/0 へ実送信するワークフロー。secrets(`DOT_API_KEY` / `SERIAL_NUM`)を
  使うため `workflow_dispatch`(手動)/ `schedule`(毎日 JST 0:00 = `0 15 * * *` UTC)のみで起動。
  地点などの非秘匿設定は Actions Variables(`CITY_CODE` / `WEATHER_BASE_URL` / `WEATHER_LINK_URL`)で注入する。
  setup-go 失敗時のリトライあり(0004)。
- **`claude.yml`**: issue / PR で `@claude` をメンションすると起動(secret `CLAUDE_CODE_OAUTH_TOKEN`)。
- **Dependabot**: SHA ピン留めした Actions を週次で更新する。

secret は GitHub Actions Secrets(Environment `production`)、非秘匿の天気設定は Variables で管理する。public リポジトリでも
Actions はログを自動マスクし fork PR には secrets を注入しないため安全に運用できる。

## API Key ローテーション(漏洩時)

1. quote/0 の管理画面で API Key を再発行する。
2. GitHub の Settings → Environments → `production` の `DOT_API_KEY` を更新する。
3. ローカルの `.env` を更新する。
4. 旧 Key を失効させる。

## 参考

- [quote/0(dot.mindreset.tech)](https://dot.mindreset.tech)
- Canvas API エンドポイント: `POST /api/authV2/open/device/:serialNum/canvas`
- 設計判断の背景: [`docs/design-docs/`](docs/design-docs/)(0001〜0005。Canvas API 移行は [0005](docs/design-docs/0005-canvas-api-migration.md))
