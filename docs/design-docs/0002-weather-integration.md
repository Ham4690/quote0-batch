# Design Doc: M3 天気連携

- **Author:** Arata Higashiguchi
- **Status:** Draft
- **Created:** 2026-07-24
- **Last Updated:** 2026-07-24
- **Reviewers:** TBD
- **関連:** [0001 初期構築 Design Doc](./0001-initial-setup.md)

---

## Context and Scope

[0001](./0001-initial-setup.md) で構築した Ports & Adapters 基盤（`in-adapter → RunBatch → out-adapter`）に、実コンテンツソースとして**天気連携（M3）**を追加する。M1 の `HelloWorldSource`（固定文言）を、天気予報を組み立てる `WeatherSource` に差し替える。

本 Design Doc のスコープ:

1. 天気 API の選定と取得仕様
2. レスポンスの型設計（External DTO → Domain → TextPayload の 3 層）と境界検証
3. quote/0 への表示レイアウト（当日の天気）
4. 実行頻度を **毎日 0 時（JST）** に変更

`domain.ContentSource` / `ContentSink` / `RunBatch` / `Quote0Sink` / `config` は既存のまま。追加は `adapter/in/weather/` と `domain/weather.go`、および `cmd/batch` の配線差し替えに閉じる。

### 要件

- **API**: [`weather.tsukumijima.net`](https://weather.tsukumijima.net/)（livedoor 天気互換・データ元は気象庁）
- **地点**: 東京（`city=130010`）を既定とし、**地点コードは env 化**（GitHub Actions Variables に登録）。
- **取得**: `GET https://weather.tsukumijima.net/api/forecast?city={CITY_CODE}`（認証不要）
- **頻度**: 毎日 0 時（JST）に取得し、**リクエスト日（当日）の予報**を表示
- **表示項目**:
  - 対象日付 `MM/DD(曜日)`
  - 最低気温 / 最高気温
  - 降水確率（0-6 / 6-12 / 12-18 / 18-24 の 4 時間帯すべて）
  - 天気（`telop`: 例「晴れ」「雨のち曇」）
  - タップ遷移先は **Yahoo 天気**（`link` も env 化。後述）

### API レスポンス構造（前提知識）

`forecasts` は要素 3 件の配列（`[0]`=今日 / `[1]`=明日 / `[2]`=明後日）。当日は `[0]`（`dateLabel: "今日"`）。

```jsonc
{
  "forecasts": [
    {
      "date": "2026-07-24",            // 対象日（曜日は含まれない → 自前で導出）
      "dateLabel": "今日",
      "telop": "雨のち曇",              // 天気概況（日本語・そのまま利用）
      "temperature": {
        "min": { "celsius": null,  "fahrenheit": null },   // ← 当日は null になり得る
        "max": { "celsius": null,  "fahrenheit": null }
      },
      "chanceOfRain": {
        "T00_06": "--%",  "T06_12": "--%",                 // ← 未確定は "--%"
        "T12_18": "--%",  "T18_24": "50%"
      }
    }
    // ... [1] 明日, [2] 明後日
  ],
  "location": { "prefecture": "東京都", "city": "東京" },
  "link": "https://www.jma.go.jp/bosai/forecast/#area_code=130000"
}
```

**重要な特性（設計の前提）**:

- `temperature.min/max.celsius` は**文字列（例 `"26"`）または `null`**。当日分は発表タイミングにより `null` になり得る。
- `chanceOfRain` は 4 時間帯固定キー。未確定帯は `"--%"`。
- 曜日は API に含まれない → `date` から導出する。

---

## Goals

- **G1**: 毎日 0 時（JST）に東京の当日天気予報を取得し、quote/0 に表示する。
- **G2**: 日付 `MM/DD(曜日)` / 最低・最高気温 / 降水確率（4 帯）/ 天気概況 を表示する。
- **G3**: 外部レスポンスを DTO → Domain → TextPayload の 3 層で型付けし、境界で `Validate()` する（[0001 の型設計方針](./0001-initial-setup.md) を踏襲）。
- **G4**: `null` 気温・`"--%"` 等の欠損値でも落ちず、代替表記（`--`）で表示を継続する。
- **G5**: 既存の `out-adapter` / `RunBatch` / `config` を無変更に保ち、`ContentSource` 差し替えのみで拡張する。

## Non-Goals

- 複数地点の同時対応（地点コードは env で切替可能だが、運用は単一地点＝既定・東京）。
- 明日以降（週間）予報の表示。**当日のみ**。
- 天気アイコン / 画像（`image.url`）連携。`telop` テキストのみ。
- リトライ・失敗通知の本格化（M2 の範囲）。
- 気象警報・注意報（`description` の本文）連携。

---

## Actual Design

### システムコンテキスト（M1 からの差分）

`in-adapter` を `HelloWorldSource` → `WeatherSource`（天気 API 実接続）へ差し替える。それ以外の経路は不変。

```
┌──────────────┐ cron(毎日0時) ┌────────────────────┐  GET forecast  ┌───────────────────────┐
│  Scheduler   │ ────────────▶ │   Batch Runtime     │ ─────────────▶ │ weather.tsukumijima.net │
│ (GH Actions) │               │   WeatherSource     │ ◀───────────── │  ?city=130010           │
└──────────────┘               └─────────┬──────────┘                └───────────────────────┘
                                         │ POST /device/:serial/text
                                         ▼
                                ┌────────────────────┐
                                │  quote/0 Text API  │──▶ 実機表示
                                └────────────────────┘
```

### 追加ディレクトリ

[0001](./0001-initial-setup.md) で予約済みの `adapter/in/weather/` を実装する。

```
internal/
├── domain/
│   └── weather.go          # 追加: 内部 Forecast モデル（API 非依存）
└── adapter/in/weather/
    ├── dto.go              # 追加: API 生レスポンス + Validate()
    ├── source.go           # 追加: WeatherSource(ContentSource 実装) + mapper
    ├── source_test.go
    └── testdata/
        ├── tokyo.json      # 実レスポンス fixture（正常）
        └── tokyo_null_temp.json  # 当日気温 null の fixture
```

### 取得仕様

- **エンドポイント**: `GET {WEATHER_BASE_URL}/api/forecast?city={CITY_CODE}`
- **認証**: 不要（**新たな secret は増えない**）。
- **タイムアウト**: 既存の `http.Client{Timeout: 30s}` を流用（out-adapter と同方針）。

#### 設定（env）

天気連携で追加する env は**すべて非秘匿**。GitHub Actions では **Variables**（Secrets ではない）に登録する。`config.Config` に読み込み口を追加し、未設定は既定値。

| 変数 | 内容 | 既定値 | 秘匿 |
| --- | --- | --- | --- |
| `CITY_CODE` | 天気 API の地点コード | `130010`（東京） | No |
| `WEATHER_BASE_URL` | 天気 API ベース URL（末尾スラッシュ正規化） | `https://weather.tsukumijima.net` | No |
| `WEATHER_LINK_URL` | 表示の `link` 遷移先（Yahoo 天気ページ） | API レスポンスの `link`（気象庁） | No |

- 非秘匿のため Secrets ではなく **Actions Variables**（`vars.CITY_CODE` 等）で注入する。
- `CITY_CODE` を変えたら、対応する `WEATHER_LINK_URL`（Yahoo の該当地点ページ）も併せて更新する運用とする（[link の方針](#link-遷移先yahoo-天気)参照）。

#### link 遷移先（Yahoo 天気）

表示のタップ遷移先は **Yahoo 天気**とする。ただし **Yahoo 天気は独自の地域コード体系**で、天気 API の JMA 地点コード（`130010`）とは対応しない。

例: 東京地方 = `https://weather.yahoo.co.jp/weather/jp/13/4410.html`（`13`=東京都 / `4410`=東京地方）。

「JMA コード → Yahoo URL」の対応表を持つのは保守コストになるため、**link は `WEATHER_LINK_URL` env として明示指定**する（`CITY_CODE` とセットで運用者が設定）。未設定時は API レスポンスの `link`（気象庁ページ）へフォールバック。

### 当日エントリの選択ロジック

`forecasts` から当日を選ぶ。0 時実行時、当日予報は前日発表分に含まれる想定だが、防御的に二段で判定する。

1. 第一候補: `date` が**実行日（JST）**と一致する要素。
2. フォールバック: `dateLabel == "今日"` の要素。
3. どちらも無ければ `Validate()` エラー（想定外レスポンス）。

### 型設計（3 層）

**1. External DTO**（`adapter/in/weather/dto.go`）— API を素直に写す。`null`/`"--%"` を受けるため気温は文字列ポインタ等で受ける。

```go
package weather

type apiResponse struct {
	Forecasts []forecast `json:"forecasts"`
	Link      string     `json:"link"`
}
type forecast struct {
	Date         string       `json:"date"`      // "2026-07-24"
	DateLabel    string       `json:"dateLabel"` // "今日"
	Telop        string       `json:"telop"`     // "雨のち曇"
	Temperature  temperature  `json:"temperature"`
	ChanceOfRain chanceOfRain `json:"chanceOfRain"`
}
type temperature struct {
	Min tempValue `json:"min"`
	Max tempValue `json:"max"`
}
type tempValue struct {
	Celsius *string `json:"celsius"` // "26" | null
}
type chanceOfRain struct {
	T00_06 string `json:"T00_06"` // "50%" | "--%"
	T06_12 string `json:"T06_12"`
	T12_18 string `json:"T12_18"`
	T18_24 string `json:"T18_24"`
}

// decode 後の明示検証（想定外レスポンスの早期検出）
func (r apiResponse) Validate() error {
	if len(r.Forecasts) == 0 {
		return errors.New("weather: forecasts が空")
	}
	// 当日エントリの存在・telop 非空は選択ロジック側で確認
	return nil
}
```

**2. Domain model**（`domain/weather.go`）— API 非依存のクリーンな型。欠損は `nil` で表現。

```go
package domain

type Forecast struct {
	Date         time.Time // 当日 0:00 JST
	Telop        string    // 天気概況
	TempMinC     *int      // nil = 欠損
	TempMaxC     *int      // nil = 欠損
	ChanceOfRain RainChance
	Link         string
}
type RainChance struct{ T0006, T0612, T1218, T1824 string } // "50%" / "--%"
```

**3. Mapper / Presenter**（`source.go`・純粋関数）

- `toForecast(f forecast) (domain.Forecast, error)`: DTO → Domain。`date` を JST でパースし曜日算出、`celsius` 文字列 → `*int`（空/`null` は `nil`）。
- `toTextPayload(w domain.Forecast) domain.TextPayload`: Domain → 表示。

### 表示レイアウト（TextPayload）

| フィールド | 内容 | 例 |
| --- | --- | --- |
| `title` | 地点名 + 天気概況（`地点名 + telop`）。上限超過は rune 単位でスライス。地点名の併記は [0003](./0003-weather-location-title.md) で追加（未取得時は `telop` のみ） | `東京 雨のち曇` |
| `message` | 日付・気温・降水確率（複数行） | 下記 |
| `signature` | 生成日時（JST） | `2026年07月24日00:00` |
| `link` | `WEATHER_LINK_URL`（Yahoo 天気）／未設定時は API の `link` | `https://weather.yahoo.co.jp/weather/jp/13/4410.html` |
| `refreshNow` | 即時表示 | `true` |

`message` 例（欠損は `--`）:

```
07/24(金)
最低 --℃ / 最高 --℃
降水 0-6 --% / 6-12 --% / 12-18 --% / 18-24 50%
```

- **日付**: `date` を `time.Parse("2006-01-02", ...)` で JST 解釈 → `01/02` 整形 + 曜日を日本語 map（`[]string{"日","月","火","水","木","金","土"}[t.Weekday()]`）。
- **気温**: `*int` が `nil` なら `--`、それ以外は数値 + `℃`。
- **降水確率**: 4 帯を `"--%"` 含めそのまま並べる。

#### `title`（telop）の長さ対策

quote/0 の `title` 表示幅を超える `telop`（例「雨時々曇一時雷を伴い…」）は、e-ink 上で見切れる。一旦 **rune 単位でスライス**し、超過時は末尾に省略記号を付す（例 `fugafugafugafu…`）。

> 注: [0003](./0003-weather-location-title.md) 以降、`title` は「地点名 + 天気概況」（例「東京 雨のち曇」）となる。地点名は API レスポンスの `location.city` から取得し、`地点名 + " " + telop` を組み立てた上で、本節のスライス（`clipRunes`）を最後に適用する。地点名が未取得（空）の場合は従来どおり `telop` のみを表示する。

```go
func clipRunes(s string, max int) string {
	r := []rune(s)          // マルチバイト安全に文字数で切る（byte 単位は禁止）
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
```

- **上限値**は quote/0 の `title` 仕様（最大表示文字数）に合わせて定数化する。**正確な上限は実機で確認**し、確定するまで暫定値を置く（[Open Questions](#open-questions) に残す）。
- スライスは表示専用。ログ・エラーには影響させない。

### 欠損値ハンドリング方針

外部データの欠損で**バッチを失敗させない**（表示継続を優先）。

- 気温 `celsius` が `null` / 空文字 → `nil` → `--` 表示。
- `chanceOfRain` が `"--%"` → そのまま表示。
- ただし `forecasts` 空 / 当日エントリ不在 / `telop` 空 は**構造異常**として `Validate()`・選択ロジックでエラー（非ゼロ終了 → Actions 失敗表示）。

### 配線差し替え（`cmd/batch/main.go`）

```go
// src := in.NewHelloWorldSource()            // M1
src := weather.NewSource(cfg, nil)            // M3: 天気 API
sink := out.NewQuote0Sink(cfg, nil)
application.RunBatch(ctx, src, sink)          // RunBatch は無変更
```

### 実行頻度の変更（`batch.yml`）

要件により**毎日 0 時（JST）**へ変更する（M2 の「毎時」より低頻度）。JST 0 時 = UTC 前日 15 時。

```yaml
on:
  workflow_dispatch:
  schedule:
    - cron: "0 15 * * *"   # UTC15:00 = JST 00:00（毎日）
```

GitHub Actions の cron は UTC 基準のため `15` を指定。JST は `time.LoadLocation("Asia/Tokyo")` で明示取得（[0001](./0001-initial-setup.md) 同方針）。

---

## Alternatives Considered

### A. 天気 API の選定

| 案 | 長所 | 短所 | 判定 |
| --- | --- | --- | --- |
| **tsukumijima.net（livedoor 互換）** | 認証不要（secret 増えない）、日本語 `telop`・降水確率・気温をそのまま取得、データ元が気象庁、地点コード指定が容易 | min/max 気温が当日 `null` になり得る、非公式・可用性保証なし | ✅ 採用 |
| Open-Meteo | 無料・高機能・信頼性高 | `weather_code`（数値）を自前で日本語天気にマッピング必要、降水確率の粒度・単位変換が要る | 次点 |
| 気象庁 生 JSON | 一次情報・無料 | レスポンス構造が複雑、telop 相当が無く整形コスト大 | 却下 |
| OpenWeather | 高機能・実績 | **API キー必須**（secret 管理が増える）、日本語天気は要変換 | 却下 |

> 要件（東京・当日・日本語天気・降水確率）に対し、tsukumijima は**認証不要かつ日本語 telop 即利用**で最小実装。可用性は非公式のため、失敗は Actions のジョブ失敗として検知（M2 のリトライ導入で緩和予定）。

### B. 対象日

当日 / 明日 の 2 案。**要件により当日（`forecasts[0]`）を採用**。0 時実行のため当日発表が揃う想定だが、気温 `null` に対しては欠損表示で対応。

### C. 降水確率の表現

4 時間帯すべて / 最大値のみ / 昼夜 2 区分。**4 帯すべて採用**（情報量を最大化。e-ink の 1 行に収まる範囲）。

### D. 気温欠損時の挙動

エラーで停止 / `--` で表示継続。**表示継続を採用**（天気概況・降水確率は取得できており、気温欠損だけで無表示にする価値が低い）。

---

## Cross-Cutting Concerns

### セキュリティ

- 天気 API は**認証不要 → 新たな secret を追加しない**。`CITY_CODE` / `WEATHER_BASE_URL` / `WEATHER_LINK_URL` は非秘匿 → Secrets ではなく Actions **Variables** で注入。
- 既存の `DOT_API_KEY` / `SERIAL_NUM` の扱いは [0001](./0001-initial-setup.md) のまま。

### 可観測性 / 失敗時挙動

- 天気 API の 4xx/5xx・タイムアウト → error → 非ゼロ終了で Actions 失敗表示。
- 欠損値（気温 `null`・`"--%"`）は**正常系**として `--` 表示で継続（失敗にしない）。
- タイムアウトは既存 `http.Client{Timeout: 30s}` を流用。

### テスト / CI 品質

- **fixture 駆動**: 実レスポンスを `testdata/tokyo.json`（正常）と `tokyo_null_temp.json`（当日気温 `null`）に固定し、mapper を純粋関数としてテーブル駆動テスト。
- **mapper**: `toForecast` が `date` → 曜日、`celsius` 文字列 → `*int`、`null` → `nil` を正しく変換するか。`toTextPayload` が欠損時に `--` を出すか。
- **WeatherSource**: `httptest.Server` で API をモックし、正しい URL（`?city=130010`）で GET し当日を選択するか。**実 API は叩かない**（secrets 不要・fork PR でも走る）。
- **異常系**: `forecasts` 空 / 当日不在 で `Validate()`・選択がエラーを返すか。
- CI（`ci.yml`）は既存のまま（ネットワーク非依存）。

### コスト

- 実行は 1 日 1 回（毎時 → 毎日に低頻度化）。API 負荷・Actions 消費とも極小。

---

## Milestones（M3 内訳 / PR 分割案）

1. `domain/weather.go`（`Forecast` / `RainChance`）+ `dto.go`（`apiResponse` + `Validate`）。
2. mapper（`toForecast` / `toTextPayload`）+ 純粋関数テスト（fixture 2 種）。
3. `WeatherSource`（`ContentSource` 実装）+ `httptest` 統合テスト。
4. `cmd/batch` 配線差し替え（`HelloWorldSource` → `WeatherSource`）。
5. `batch.yml` cron を毎日 0 時（`0 15 * * *`）へ変更。

---

## Resolved Decisions

- **地点コード**: `CITY_CODE` を **env 化**（既定 `130010`）。GitHub Actions **Variables** に登録して注入する。
- **`link` 遷移先**: **Yahoo 天気**。Yahoo は独自地域コードで JMA コードと非対応のため、`WEATHER_LINK_URL` env で明示指定（`CITY_CODE` とセット運用）。未設定時は API の `link`（気象庁）へフォールバック。
- **当日気温 `null`**: 現状のまま `--` 表示で対応。「明日」予報へのフォールバックは行わない（頻発時に再検討）。
- **長い `telop`**: `title` を rune 単位でスライスし、超過時は末尾 `…`。なお `title` の書式は [0003](./0003-weather-location-title.md) で「地点名 + telop」（例「東京 雨のち曇」）へ拡張済み。

## Open Questions

- quote/0 `title`（および `message`）の最大表示文字数 = スライス上限値。**実機で確認**して定数化する。
- 当日気温 `null` の実発生頻度（0 時実行での実測）。頻発するなら「明日」フォールバックを再検討。
