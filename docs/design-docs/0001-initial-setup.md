# Design Doc: quote0-batch 初期構築

- **Author:** Arata Higashiguchi
- **Status:** Draft
- **Created:** 2026-07-11
- **Last Updated:** 2026-07-14
- **Reviewers:** TBD

---

## Context and Scope

quote/0 は e-ink スマートディスプレイ。REST API 経由でテキスト / 画像 / キャンバスを送信し、デバイス上に表示できる。

本リポジトリ `quote0-batch` は、外部コンテンツを定期的に取得・整形し、quote/0 の Text API へ連携する **バッチ処理基盤** を管理する。当面の連携対象は **天気情報のみ**。1 時間ごとに天気を取得し、デバイスへ表示する。

本 Design Doc のスコープは **初期構築のみ**:

1. 技術選定（実行ランタイム・スケジューラ・シークレット管理）
2. CI/CD 上で Text API を叩き `Hello World` を実機表示する疎通確認（PoC）

天気取得ロジックそのものは後続（M3）で扱うが、そこへ拡張できる構造を初期構築時に用意する。

### quote/0 Text API（前提知識）

| 項目 | 内容 |
| --- | --- |
| Method | `POST` |
| Endpoint | `https://dot.mindreset.tech/api/authV2/open/device/:serialNum/text` |
| Auth | `Authorization: Bearer {API_KEY}` |
| Body (主要) | `title`, `message`(`\n`/`\t`可), `signature`, `icon`, `link`, `refreshNow`, `taskKey`, `taskAlias`, `styles` |
| Response | `{"message": "Device {serialNum} text API content switched."}` |
| 主要エラー | 400 パラメータ不正 / 403 権限不足 / 404 デバイス未検出・Text API未登録 / 500 デバイス通信失敗 |

`API_KEY`（secret）と シリアルナンバー（`SERIAL_NUM`）は取得済み。両者とも秘匿情報として扱う。

---

## Goals

- **G1**: Text API に対し `Hello World` を送信し、実機表示を確認できる（PoC）。
- **G2**: API Key / シリアルナンバーを平文でコミットせず、安全に注入できる仕組みを持つ。
- **G3**: 1 時間ごとの定期実行（cron スケジュール）の土台を用意する。
- **G4**: ローカルでも CI 上でも同一コードで実行できる（環境差分は環境変数のみ）。
- **G5**: 後続で天気コンテンツを追加しやすいディレクトリ / モジュール構成にする。

## Non-Goals

- 天気データの取得・整形ロジック本体。M3（別 Design Doc）で扱う。
- 天気以外のコンテンツソース（カレンダー・RSS 等）。現時点で想定しない。
- 画像 / キャンバス API 連携。
- 複数デバイス・マルチテナント対応。
- 監視・アラートの本格構築（初期は失敗通知の最小限のみ検討）。
- Web UI / 管理画面。

---

## Actual Design

### システムコンテキスト

```
┌──────────────┐  cron(毎時)   ┌────────────────────┐   fetch    ┌──────────┐
│  Scheduler   │ ────────────▶ │   Batch Runtime     │ ─────────▶ │ 天気 API  │
│ (GH Actions) │               │   (Go binary)       │ ◀───────── │ (M3で追加)│
└──────────────┘               └─────────┬──────────┘            └──────────┘
       │ inject secrets                  │ POST /device/:serial/text
       │ (DOT_API_KEY, SERIAL_NUM)       ▼
       │                        ┌────────────────────┐
       └───────────────────────▶│  quote/0 Text API  │──▶ 実機表示
                                └────────────────────┘
```

Batch Runtime 内部は Ports & Adapters 構成: `in-adapter(取得) → RunBatch(usecase) → out-adapter(quote/0 送信)`。M1（PoC）では in-adapter が `HelloWorldSource`（天気 API 無し）、固定文言 `Hello World` を送信する。

### 選定案（結論）

- **ランタイム: Go (1.25)**。Go には LTS が無く、[サポートは最新 2 系列のみ](https://go.dev/doc/devel/release)（メジャーは約 6 ヶ月毎、1.x のパッチは 1.x+2 リリースで停止）。CI のツールチェーンはサポート内バージョンを使い、新系列リリース時に追随する。`go.mod` の `go` directive は「最小要求バージョン」であり、CI ツールチェーンと同じ値に揃える。
- **スケジューラ / CI: GitHub Actions（`schedule` cron + `workflow_dispatch`）**
- **シークレット管理: GitHub Actions Secrets（Repository / Environment secrets）**
- **リポジトリ公開設定: public**
- **品質ツール: `go test`（テスト）+ golangci-lint（lint）+ `go vet`（静的解析）+ `gofmt`/`goimports`（整形）。外部データ境界検証は M3 で `encoding/json` decode + 明示バリデーション（必要なら go-playground/validator）**

各選定の比較は [Alternatives Considered](#alternatives-considered) 参照。

### ディレクトリ構成

**Ports & Adapters（ヘキサゴナル / DDD-lite）** を採用する。入力（コンテンツ取得）と出力（quote/0 送信）を port（interface）で抽象化し、実装を adapter として差し替え可能にする。比較は [Alternatives Considered D](#d-ディレクトリ構成) 参照。

```
quote0-batch/
├── docs/design_docs/               # 本 Design Doc 群
├── cmd/
│   └── batch/
│       └── main.go                 # 合成ルート（DI 配線 → RunBatch 実行）
├── internal/
│   ├── domain/
│   │   ├── content.go              # TextPayload 型 + ContentSource/ContentSink interface
│   │   └── weather.go              # M3: 内部 Weather モデル（API 非依存）
│   ├── application/
│   │   ├── runbatch.go             # ユースケース: source→sink を協調
│   │   └── runbatch_test.go
│   ├── adapter/
│   │   ├── in/                     # 入力側 adapter（コンテンツ取得）
│   │   │   ├── helloworld.go
│   │   │   ├── helloworld_test.go
│   │   │   └── weather/            # M3: 天気 API → Content
│   │   │       ├── dto.go          #   外部 API 生レスポンス + 検証
│   │   │       ├── source.go       #   ContentSource 実装 + mapper
│   │   │       └── source_test.go
│   │   └── out/                    # 出力側 adapter
│   │       ├── quote0.go           # Content → Text API 送信
│   │       └── quote0_test.go
│   └── config/
│       ├── config.go               # 環境変数ロード / バリデーション
│       └── config_test.go
├── .github/workflows/
│   ├── ci.yml                      # PR/push: lint(govet含) + test
│   └── batch.yml                   # schedule + workflow_dispatch（実送信）
├── .env.example                    # 必要な env のキーのみ（値は空）
├── .golangci.yml                   # golangci-lint v2 設定（standard preset + gofmt/goimports）
├── go.mod
├── go.sum                          # 外部依存の追加時に生成（初期は標準ライブラリのみで存在しない）
└── README.md
```

（テストは実装ファイルと同パッケージで co-locate。`*_test.go` を各実装ファイル隣に置く。）

- **domain**: 外部依存の無い純粋な型と port（interface）。ビジネス語彙の中心。
- **application**: ユースケース。port 経由で source→sink を協調するだけで、具体実装を知らない。
- **adapter/in**: 入力側の具体実装（コンテンツ取得）。`ContentSource` を実装。
- **adapter/out**: 出力側の具体実装（quote/0 Text API）。`ContentSink` を実装。
- **cmd/batch/main.go**: 合成ルート（composition root）。config を読み、adapter を生成して `RunBatch` へ注入。

### 表示レイアウト方針

初期の Text ペイロードは以下の形とする（`styles` / `icon` は指定しない）。

| フィールド | 内容 | 例 |
| --- | --- | --- |
| `title` | 見出し | `"Hello World"`（M3 では天気概況） |
| `message` | 本文（`\n` 区切り複数行） | 天気詳細（気温・降水等）。M1 はダミー文言 |
| `signature` | 生成日時 | `2026年07月12日09:00` |
| `link` | タップ遷移先 | 天気詳細ページ URL |
| `refreshNow` | 即時表示 | `true` |

`signature` の日時は **JST** で生成する。Go では `time.LoadLocation("Asia/Tokyo")` で明示的にロケーションを取得し `t.In(jst)` で変換する → runner の TZ 設定に依存せず確実に JST 化（`TZ` 環境変数への依存を排除）。

参考（ユーザー提示の想定 curl 形式）:

```bash
export DOT_API_KEY="..."   # secret
export SERIAL_NUM="..."    # secret
CURRENT_DATE=$(TZ=Asia/Tokyo date '+%Y年%m月%d日%H:%M')

curl -X POST \
  --url "https://dot.mindreset.tech/api/authV2/open/device/${SERIAL_NUM}/text" \
  -H "Authorization: Bearer ${DOT_API_KEY}" \
  -H 'Content-Type: application/json' \
  -d '{
    "refreshNow": true,
    "title": "Hello World",
    "message": "性別: 男\n年齢: 10\n趣味: 読書\n職業: エンジニア\n",
    "signature": "'"${CURRENT_DATE}"'",
    "link": "https://www.yahoo.co.jp/"
  }'
```

Go 実装はこの curl と等価な POST を発行する。

### 設定 / シークレットの流れ

コードは シリアルナンバー / API Key を **環境変数からのみ** 取得する。ハードコード禁止。変数名はユーザー想定に合わせる。

| 変数 | 内容 | 秘匿 | ローカル | CI |
| --- | --- | --- | --- | --- |
| `DOT_API_KEY` | Bearer トークン | Yes | `.env`（gitignore） | GH Secrets |
| `SERIAL_NUM` | シリアルナンバー | Yes（推測・列挙対策） | `.env` | GH Secrets |
| `DOT_BASE_URL` | API ベース URL | No | 既定値 | 既定値 |

- `.env` は `.gitignore` 対象。リポジトリには `.env.example`（キーのみ）を置く。
- `config.go` の `config.Load()` が起動時に必須変数の存在を検証し、欠落時は即 fail（fail-fast）。`DOT_BASE_URL` は末尾スラッシュを除去して正規化（URL 連結時の `//` を防止）。
- シリアルナンバーも秘匿扱い: 漏洩すると第三者が実機へ書き込み可能なため（API Key と組で悪用）。ログ・エラー出力にマスク。

### 疑似コード（PoC）

```go
// internal/domain/content.go — port 定義（外部依存なし）
package domain

type TextPayload struct {
	Title      string `json:"title"`
	Message    string `json:"message"`
	Signature  string `json:"signature,omitempty"`
	Link       string `json:"link,omitempty"`
	RefreshNow bool   `json:"refreshNow"` // omitempty 無し: false を明示送信できるように
}

type ContentSource interface {
	Build(ctx context.Context) (TextPayload, error) // 入力: コンテンツを組み立てる
}
type ContentSink interface {
	Send(ctx context.Context, p TextPayload) error // 出力: 表示先へ送信
}

// internal/adapter/out/quote0.go — ContentSink 実装
package out

type Quote0Sink struct {
	cfg    config.Config
	client *http.Client // DI: テストで Transport(RoundTripper) を差し替え
}

func NewQuote0Sink(cfg config.Config, client *http.Client) *Quote0Sink {
	if client == nil {
		// http.DefaultClient は Timeout ゼロ（無制限）のため使わない。
		// API ハング時に scheduler のジョブ上限まで待ち続けるのを防ぐ。
		client = &http.Client{Timeout: 30 * time.Second}
	}
	return &Quote0Sink{cfg: cfg, client: client}
}

func (s *Quote0Sink) Send(ctx context.Context, p domain.TextPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return err
	}
	url := fmt.Sprintf("%s/api/authV2/open/device/%s/text", s.cfg.BaseURL, s.cfg.SerialNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// serialNum/apiKey はエラーに含めずマスク
		return fmt.Errorf("text API failed: status=%d", res.StatusCode)
	}
	return nil
}

// internal/adapter/in/helloworld.go — ContentSource 実装（PoC）
package in

// time/tzdata を blank import し tzdata をバイナリへ埋め込む（約 +450KB）。
// OS の tzdata に依存しないため、将来 scratch/distroless コンテナへ移しても
// LoadLocation が失敗しない。
import _ "time/tzdata"

// JST は起動時に 1 度だけ解決（Build 毎の LoadLocation を回避）
var jst = must(time.LoadLocation("Asia/Tokyo"))

type HelloWorldSource struct {
	now func() time.Time // DI: テストで時刻固定
}

func NewHelloWorldSource() *HelloWorldSource {
	return &HelloWorldSource{now: time.Now}
}

func (s *HelloWorldSource) Build(ctx context.Context) (domain.TextPayload, error) {
	sig := s.now().In(jst).Format("2006年01月02日15:04")
	return domain.TextPayload{
		RefreshNow: true,
		Title:      "Hello World",
		Message:    "Hello\nWorld",
		Signature:  sig,
		Link:       "https://www.yahoo.co.jp/",
	}, nil
}

// internal/application/runbatch.go — ユースケース（具体実装を知らない）
package application

func RunBatch(ctx context.Context, src domain.ContentSource, sink domain.ContentSink) error {
	p, err := src.Build(ctx)
	if err != nil {
		return err
	}
	return sink.Send(ctx, p)
}

// cmd/batch/main.go — 合成ルート（composition root）
func main() {
	ctx := context.Background()
	cfg, err := config.Load() // env 検証、欠落で fail-fast
	if err != nil {
		log.Fatal(err)
	}
	if err := application.RunBatch(ctx, in.NewHelloWorldSource(), out.NewQuote0Sink(cfg, nil)); err != nil {
		log.Fatal(err) // 非ゼロ終了 → GitHub Actions が失敗表示
	}
}
```

### 天気データの型設計方針（M3 プレビュー）

天気連携（M3）は「外部 API の生データ」と「アプリ内部の型」を **明確に分離** して丁寧に型付けする。詳細は M3 の別 Design Doc で確定するが、初期構築の型レイヤ方針を先に定める。

3 層に分ける:

1. **External DTO**（`internal/adapter/in/weather/dto.go`）: 天気 API のレスポンスを **そのまま写した struct**（`json` タグ付き）。API ごとに定義。`json.Unmarshal` で decode 後、`Validate()` で **明示バリデーション**（必須・範囲）→ 想定外レスポンスを早期検出。zod のような decode+検証一体型は無いため検証は手書きだが、天気 1 ソースなら軽量。
2. **Domain model**（`internal/domain/weather.go`）: アプリ内部で扱うクリーンな型（例: `Weather{ TempC float64; Condition WeatherCondition; ObservedAt time.Time }`）。API 依存の命名・単位を排除。
3. **Mapper / Presenter**: DTO → Domain（`ToWeather(dto)`）、Domain → `TextPayload`（`ToTextPayload(w)`）。各々 **純粋関数** としてテーブル駆動テスト。

```go
// internal/adapter/in/weather/dto.go — 外部 API の生レスポンス
package weather

type APIResponse struct {
	Current struct {
		Temperature2m float64 `json:"temperature_2m"`
		WeatherCode   int     `json:"weather_code"`
		Time          string  `json:"time"`
	} `json:"current"`
}

// decode 後の明示検証（zod 相当は無いので手書き）
func (r APIResponse) Validate() error {
	if r.Current.Time == "" {
		return errors.New("weather: missing current.time")
	}
	// ...API 仕様に応じて範囲チェック等を追加
	return nil
}

// internal/domain/weather.go — 内部モデル（API 非依存）
package domain

type WeatherCondition string

const (
	Sunny   WeatherCondition = "sunny"
	Cloudy  WeatherCondition = "cloudy"
	Rain    WeatherCondition = "rain"
	Snow    WeatherCondition = "snow"
	Unknown WeatherCondition = "unknown"
)

type Weather struct {
	TempC      float64
	Condition  WeatherCondition
	ObservedAt time.Time
}

// internal/adapter/in/weather/source.go — mapper（純粋関数・テスト対象）
// domain は外部依存ゼロが原則 → APIResponse を参照する mapper は adapter 側に置く
package weather

func ToWeather(r APIResponse) domain.Weather           { /* ... */ }
func ToTextPayload(w domain.Weather) domain.TextPayload { /* title/message/... */ }
```

利点: 天気 API を差し替えても影響は DTO と mapper に閉じ、`domain/weather.go` 以降は無変更。境界での `Validate()` で、外部データ起因のバグを decode + 検証の両段で防ぐ。

### GitHub Actions ワークフロー（骨子）

```yaml
name: batch
on:
  workflow_dispatch:          # 手動実行（初期疎通確認用）
  # M1 は workflow_dispatch のみで疎通確認。実機表示 OK 後（M2）に有効化する。
  # schedule:
  #   - cron: "0 * * * *"     # 毎時 0 分（UTC 基準だが 1 時間間隔なので時刻ずれは無影響）
permissions:
  contents: read              # 最小権限
jobs:
  run:
    runs-on: ubuntu-latest
    environment: production    # Environment secrets + 承認ゲート活用
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - run: go run ./cmd/batch
        env:
          DOT_API_KEY: ${{ secrets.DOT_API_KEY }}
          SERIAL_NUM: ${{ secrets.SERIAL_NUM }}
```

JST は `time.LoadLocation("Asia/Tokyo")` で明示取得するため `TZ` 環境変数は不要。初期は `schedule` をコメントアウトし `workflow_dispatch` のみで疎通確認 → OK 後に cron 有効化。

### CI ワークフロー（テスト）骨子

`batch.yml`（実送信）とは別に、PR / push でコード品質を検証する `ci.yml` を用意する。secrets 不要（外部 API を叩かずモックでテスト）→ fork PR でも安全に走る。

```yaml
name: ci
on:
  pull_request:
  push:
    branches: [main]
permissions:
  contents: read
jobs:
  test:
    runs-on: ubuntu-latest
    steps:
      - uses: actions/checkout@v4
      - uses: actions/setup-go@v5
        with:
          go-version: "1.25"
          cache: true
      - uses: golangci/golangci-lint-action@v8   # govet 内包
        with:
          version: v2.12.2     # golangci-lint v2 系。.golangci.yml(version: "2") と揃える
      - run: go test ./...     # httptest でモック（ネットワーク非依存・secrets 不要）
```

---

## Alternatives Considered

### A. 実行ランタイム

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **Go** | 単一静的バイナリ（依存ゼロ・CI コールドスタート高速）、標準ライブラリで完結（`net/http` / `encoding/json` / `time`）、静的型、Ports & Adapters も interface で自然に表現、BE 主力言語で習得価値大 | 境界検証が zod ほど一体型でない（`encoding/json` decode + 明示バリデーション）、JSON 整形が struct tag ベースでやや冗長 | ✅ 採用 |
| Node.js + TS | 型安全、fetch 標準内蔵(v18+)、天気 API の JSON 整形が容易、[zod](https://zod.dev) で境界を decode+validate 一体検証 | ビルド構成が必要、ランタイム同梱で単一バイナリにならない | 次点 |
| Python | 記述簡潔、データ処理ライブラリ豊富 | 型は任意 | 次点 |
| Bash + curl | 依存ゼロ・最速で疎通可（ユーザー提示形式そのまま） | JSON parse に jq 依存、天気データ整形・エラー処理・テストに弱い | PoC のみ可、本採用は不可 |

> M1 の疎通だけなら Bash + curl で最速。ただし M3 で天気 API レスポンスの parse・気温/天気コードの整形が入る → JSON 処理と型安全でコンパイル言語を選択。
> Go / Node+TS どちらも本件に十分適す。決め手は (1) 単一静的バイナリで依存ゼロ・CI 起動が軽い、(2) BE エンジニアの主力言語として習得価値が高く、本件は小規模・低リスクで学習台に最適、の 2 点で **Go を採用**。zod 相当の decode+検証一体型は失うが、天気 1 ソースの境界検証は `encoding/json` decode 後の `Validate()` 明示チェックで十分カバーできる（[天気データの型設計方針](#天気データの型設計方針m3-プレビュー)参照）。M1 も同じランタイムで通し、二重実装を避ける。

### B. スケジューラ / 実行基盤

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **GitHub Actions** | 追加インフラ不要、コードと同居、secrets 管理内蔵、cron 対応、public は無料枠無制限 | cron 起動遅延あり(数分〜)、実行環境が汎用、長時間/高頻度はコスト・制約 | ✅ 採用 |
| Cloud Run Jobs + Cloud Scheduler | 起動時刻精度高、スケール・実行時間自由、リージョン選択 | GCP セットアップ・課金・IAM 管理コスト | 将来、要件超過時に移行候補 |
| セルフホスト cron (VPS/RasPi) | 常時制御・低レイテンシ | 保守・可用性・秘密管理を自前 | ✗ 運用負荷過大 |
| AWS Lambda + EventBridge | サーバレス・安価 | GH との連携に追加設定、初期学習コスト | 次点 |

> 初期要件（毎時・軽量・単一デバイス・天気 1 ソース）では GitHub Actions が最小コスト。cron 精度が問題化 or 実行が重くなれば Cloud Run Jobs へ移行する（コードは env のみ差分なので移行容易＝G4 の効用）。

### C. シークレット管理

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **GH Actions Secrets (Environment)** | 標準・無料、承認ゲート/ブランチ保護と連携、ログ自動マスク | GH 外実行では使えない、ローテーション手動 | ✅ 採用 |
| GH Repository Secrets | 設定簡単 | 環境分離不可、全 workflow から参照可 | Environment secrets を優先 |
| クラウド Secret Manager (GCP/AWS) | ローテーション・監査・IAM 細粒度 | 認証の初期設定・課金 | 将来基盤移行時に併せて採用 |
| `.env` を暗号化コミット (SOPS/age) | GH 非依存・可搬 | 復号鍵の管理問題が残る | 現状不要 |

> **public リポジトリ**のため、secret 露出対策が特に重要。GitHub Actions Secrets はログ自動マスク・fork PR への非注入が標準で効く → public でも安全に運用可能。基盤をクラウドへ移す際に Secret Manager へ移行。

### D. ディレクトリ構成

| 案 | 概要 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- | --- |
| A. 役割/レイヤー flat | `client/` `content/` `config` `main` | 単純・認知負荷低 | I/O 抽象が無くテスト時に fetch を差し替えにくい | 却下（小規模だが下記 C の利点を優先） |
| B. Vertical Slice | `features/<source>/` 単位 | ソース追加で凝集 | 天気 1 ソースの現状は凝集メリット薄い | 次点 |
| **C. Ports & Adapters** | `domain`(port) / `application`(usecase) / `adapter`(in・out) | I/O を interface 抽象化 → **テスト容易・差し替え耐性**、天気 API や出力先の交換に強い | 層が増え小規模には型/ファイルがやや多い | ✅ 採用 |
| フル DDD | entities/value-objects/repositories/domain-services… | 複雑ドメインに強い | 本件はドメイン不変条件が無く boilerplate 過剰 | ✗ 却下（YAGNI） |

> 採用理由: 本バッチの価値は「入力(コンテンツ) → 出力(quote/0)」の連携そのもの。ここを port で抽象化すると、(1) `ContentSink` をモックして API を叩かず単体テスト可能、(2) 天気 API の変更や出力先追加が adapter 追加で済む、(3) 実行基盤移行(GH Actions→Cloud Run)でも domain/application は無変更。フル DDD はドメインの不変条件・集約が無いため語彙だけが空回りし、boilerplate が増える → 却下。B は 2 ソース目が現れた時点で C の adapter/in 配下を feature 別に再編する余地を残す。

---

## Cross-Cutting Concerns

### セキュリティ（public リポジトリ前提）

- **秘匿情報**: `DOT_API_KEY` と `SERIAL_NUM` の両方を secret 扱い。コミット・ログ・エラー・PR 出力に平文を残さない。エラー時は シリアルナンバーをマスク（例: 末尾4桁のみ）。
- **fork PR**: public のため外部から PR が来る前提。GitHub Actions は fork からの PR に secrets を注入しない標準挙動 → 漏洩を防止。加えて Environment に承認ゲートを設定し、意図しない secret 利用を抑止。
- **`.gitignore`**: `.env`, `*.local` を除外。`git-secrets` 等の push 前スキャンを将来検討。
- **最小権限**: workflow の `permissions:` を `contents: read` に絞る。
- **キーローテーション**: API Key 漏洩時の再発行手順を README に記載（手動運用として許容）。

### 可観測性 / 失敗時挙動

- API 4xx/5xx は非ゼロ終了で fail → GitHub Actions が失敗表示。
- **HTTP タイムアウト（M1 から必須）**: `http.Client{Timeout: 30s}` を注入。`http.DefaultClient` は Timeout 無制限のため、API ハング時にジョブ上限（6 時間）まで待ち続けるリスクがある。リトライ導入（M2）以前の最低限の防御。
- 失敗通知は初期最小限（GH の workflow 失敗メール）。将来 Slack 通知等を検討。
- リトライ: 500（デバイス通信失敗）は一時的な可能性 → 指数バックオフで数回リトライを将来追加（初期 PoC は単発）。

### コスト

- public リポジトリのため GitHub Actions は **無料枠無制限**。毎時 cron でもコスト懸念なし。

### テスト / CI 品質

Ports & Adapters 採用の主目的の一つがテスト容易性。ネットワーク・secrets 非依存で回せる構成にする。

- **テストランナー**: 標準 `go test`（テーブル駆動 + `net/http/httptest`）。
- **CI**: `ci.yml` で PR / push 時に golangci-lint（govet 内包 + lint）+ `go test`（test）を実行。実送信ワークフロー `batch.yml` とは分離し、CI は secrets 不要 → fork PR でも走る。
- **out-adapter（`Quote0Sink`）**: `http.Client` の `Transport`（`RoundTripper`）を差し替え or `httptest.Server` で、(1) 正常時に正しい URL / Bearer ヘッダ / body を送るか、(2) 非 2xx で error を返し、エラー文言に API Key/シリアルが漏れないか、を検証。実 API は叩かない。
- **in-adapter（`HelloWorldSource` / M3 weather）**: `Build()` が期待する `TextPayload` を返すか（時刻は `now` を DI して固定）。M3 は天気 API レスポンス（後述の型）を fixture 化し、mapper を純粋関数としてテスト。
- **application（`RunBatch`）**: `ContentSource` / `ContentSink` をフェイク実装で注入し、source→sink の受け渡し順序・payload を検証。
- **config**: 必須 env 欠落時に fail-fast するか。
- カバレッジ目標は初期は設定せず（PoC 後に閾値検討）。

---

## 開発フロー

本リポジトリは Design Doc 駆動。実装は Design Doc のセクション / マイルストーン単位で
PR に分割し、main 直 push は禁止・CI green を merge 条件とする。PR 分割方針・PR 本文の
必須項目・レビュー観点・secret 取扱いの詳細は [`CONTRIBUTING.md`](../../CONTRIBUTING.md) を参照。

---

## Milestones

1. **M1 – PoC 疎通**: Go 雛形（`cmd/batch` + Ports & Adapters）+ `Quote0Sink.Send` + `workflow_dispatch` で `Hello World` 実機表示。（本 Design Doc の主対象）
2. **M2 – 定期実行**: `cron: 0 * * * *`（毎時）有効化、失敗通知、リトライ。
3. **M3 – 天気連携**: 天気 API 取得 + `internal/adapter/in/weather/`。DTO / domain / mapper の 3 層で型を丁寧に設計（decode + `Validate()` で境界検証）、各層を単体テスト（別 Design Doc）。

---

## Resolved Decisions

- **実行ランタイム**: Go（1.25）。単一静的バイナリ・標準ライブラリ完結・BE 主力言語の習得価値を重視。Node.js/TS は次点（zod の境界検証は失うが天気 1 ソースは明示検証で代替可）。Go に LTS は無いため、サポート対象（最新 2 系列）内のバージョンへ随時追随する。
- **実行頻度**: 1 時間ごと（`cron: "0 * * * *"`）。当面の連携対象は天気のみ。
- **表示レイアウト**: `title` / `message`(複数行) / `signature`(JST 日時) / `link` を使用。`styles` / `icon` は指定しない。
- **コンテンツソース**: 天気のみ（カレンダー・RSS 等は現時点で想定せず）。
- **リポジトリ公開設定**: public（Actions 無料枠無制限、secret はログマスク＋fork PR 非注入で保護）。
- **ディレクトリ構成**: Ports & Adapters（ヘキサゴナル）。`domain`(port) / `application`(usecase) / `adapter`(in・out) の 3 層。フル DDD は不採用（YAGNI）。

## Open Questions

- 天気 API はどのサービスを使うか（Open-Meteo / 気象庁 / OpenWeather 等）。M3 で決定。
- `message` に載せる天気項目（気温・降水確率・風・週間 等）の粒度。M3 で決定。
- `link` の遷移先（天気詳細ページの URL）。M3 で決定。
