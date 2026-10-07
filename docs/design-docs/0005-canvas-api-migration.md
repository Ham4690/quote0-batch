# Design Doc: Canvas API 移行

- **Author:** Arata Higashiguchi
- **Status:** Draft
- **Created:** 2026-10-06
- **Last Updated:** 2026-10-07（ピクセルフォント切替反映）
- **Reviewers:** TBD
- **関連:** [0001 初期構築 Design Doc](./0001-initial-setup.md) / [0002 M3 天気連携 Design Doc](./0002-weather-integration.md)

---

## Context and Scope

[0001](./0001-initial-setup.md) は quote/0 **Text API**（`/api/authV2/open/device/:serial/text`）を前提に Ports & Adapters 基盤を構築し、「画像 / キャンバス API 連携」を明示的に Non-Goal とした。[0002](./0002-weather-integration.md) も同様に「天気アイコン / 画像連携」を Non-Goal とし、`telop` テキストのみの表示に留めている。

本 Design Doc は、その方針を転換し、quote/0 **[Canvas API](https://dot.mindreset.tech/docs/service/open/canvas_api)**（`/api/authV2/open/device/:serial/canvas`）への**完全移行**を定義する。0001/0002 における Text API 関連の表示レイアウト記述は、本 doc の内容で置き換わる。

**スコープ**:

1. 既存の天気表示（[0002](./0002-weather-integration.md) で構築）を Text API → Canvas API に完全移行する。Text API 関連コードは削除し、並存モードは設けない。
2. 表示レイアウトは新規デザインとする。既存レイアウトとの画面一致（pixel parity）は要求しない。
3. `domain.ContentSource` / `ContentSink` という port の形は維持し、ペイロード型のみ差し替える。`config` / secrets / CI 構成・取得対象（天気データそのもの）には変更を加えない。

本 doc は設計と実装計画を定義する。**実装自体は別 PR（ブランチ）で行う**（1 PR = 1 関心事の原則に従い、本 PR は Design Doc のみを含む）。

---

## Goals

- **G1**: quote/0 への送信を Text API 呼び出しから Canvas API 呼び出しへ全面的に置き換える。
- **G2**: 現在 Text API で表示している情報（地点名 + 天気概況 / 日付 + 曜日 / 最低最高気温 / 4 時間帯の降水確率 / JST 署名タイムスタンプ / タップ遷移リンク / `refreshNow`）を、新レイアウトでも等価に維持する。
- **G3**: `config` / secrets（`DOT_API_KEY` / `SERIAL_NUM`）/ CI 構成に変更を加えない。新規の環境変数は追加しない。
- **G4**: `RunBatch` ユースケースおよび `ContentSource` / `ContentSink` の port 構成（Ports & Adapters）を変えず、変更を adapter 層（`domain` の型定義含む）に閉じる。

## Non-Goals

- 画像 (`img` 要素) / dithering (`img-dither-*`) / kernel (`img-kernel-*`) / levels (`img-levels-*`) の利用。
- Canvas API のデバイス側テンプレート機能（`data` フィールド、`{{get inputData ...}}`、`$for` / `$ifAny` 等）。値はこれまで同様 Go 側で事前計算し、プレーンな文字列として埋め込む。
- `taskAlias` / `border` / `layoutFull` フィールドの利用。
- 複数デバイス・複数メッセージへのバッチ化（現状どおり単一デバイス・単一ペイロードの単発実行）。
- 実機の pixel 寸法・有効な `tw` 構文/フォント一覧の完全な検証（[Open Questions](#open-questions) に委ねる）。

---

## Actual Design

### Canvas API 仕様（前提知識）

[公式ドキュメント](https://dot.mindreset.tech/docs/service/open/canvas_api) に基づく。

- **エンドポイント**: `POST /api/authV2/open/device/:deviceId/canvas`（`deviceId` = 既存 `SerialNum`、URL パスに埋める点は Text API と同じ）
- **認証**: 既存同様 `Authorization: Bearer {DOT_API_KEY}` ヘッダ

| フィールド | 型 | 必須 | 内容 |
| --- | --- | --- | --- |
| `refreshNow` | boolean | No | 即時画面更新 |
| `taskAlias` | string | No | デバイス側タスク一覧用ラベル（**本実装では未使用**） |
| `data` | object | No | デバイス側テンプレートが参照する動的コンテンツ（**本実装では未使用**） |
| `windowData` | object | **Yes** | 画面要素構造 |
| `layoutFull` | object | No | デバイス FULL レイアウトの間隔上書き（**本実装では未使用**） |
| `link` | string | No | タップ遷移先 URL |
| `border` | integer | No | 画面ボーダー色（**本実装では未使用**） |

`windowData` は `default` レイヤー配列を持ち、各要素は `{ "type": "div"|"span"|"img", "props": { "tw": "...", "style": {...}, "children": ... } }`。`props.tw` は Tailwind 風構文 + 独自拡張、`props.style` は確定的な pixel 値を指定する inline CSS。成功レスポンスは `{ "message": "Device [ID] Canvas API content switched." }`。

### 新 domain 型

`internal/domain/content.go` の `TextPayload` を削除し、新規ファイル `internal/domain/canvas.go` に以下を追加する。`taskAlias` / `data` / `layoutFull` / `border` は Non-Goal のため型に含めない（YAGNI）。

```go
// internal/domain/canvas.go

package domain

// CanvasPayload は quote/0 Canvas API へ送信する表示内容を表す。
// json タグは Canvas API のリクエストボディに対応する。
type CanvasPayload struct {
	WindowData WindowData `json:"windowData"`
	Link       string     `json:"link,omitempty"`
	RefreshNow bool       `json:"refreshNow"` // omitempty 無し: false を明示送信できるようにする
}

// WindowData は Canvas API の画面要素構造。現状は default レイヤーのみ使う。
type WindowData struct {
	Default []CanvasElement `json:"default"`
}

// CanvasElement は windowData.default 配下の1要素。
// Type は "div" | "span" | "img" のいずれか(本実装では img は未使用)。
type CanvasElement struct {
	Type  string             `json:"type"`
	Props CanvasElementProps `json:"props"`
}

// CanvasElementProps は要素の見た目・子要素。
// Children は string(テキスト) または []CanvasElement(入れ子要素)を any で持つ
// (Canvas API 仕様上 children は動的型のため)。
type CanvasElementProps struct {
	TW       string         `json:"tw,omitempty"`
	Style    map[string]any `json:"style,omitempty"`
	Children any            `json:"children,omitempty"`
}
```

`internal/domain/content.go` は port 定義のみを残し、ペイロード型を差し替える:

```go
package domain

import "context"

// ContentSource は表示するコンテンツを組み立てる入力側の port。
type ContentSource interface {
	Build(ctx context.Context) (CanvasPayload, error)
}

// ContentSink は組み立てたコンテンツを表示先へ送信する出力側の port。
type ContentSink interface {
	Send(ctx context.Context, p CanvasPayload) error
}
```

### windowData レイアウト（天気カード）

画像・カスタムフォントは使わず `div` / `span` のみで構成する。実機で有効な `tw` 構文の全体像が未検証（[Open Questions](#open-questions)）のため、`tw` はレイアウト用の基本的な flex/spacing ユーティリティに限定し、フォントサイズ・太字・位置寄せなど pixel 値を要する指定は `props.style`（公式ドキュメントが「確定的な pixel 値」用途として明示する手段）で行い、リスクを最小化する。

構成: 縦方向カード。ヘッダ（タイトル + 日付）／本文（気温 + 降水確率見出し + 4 分割グリッド）／右下寄せの署名、という 3 ブロック構成で [0002](./0002-weather-integration.md) の表示項目を維持する。

> **初版（見切れ不具合）からの変更:** 初版は降水確率を `"降水 0-6 10% / 6-12 20% / ..."` という単一の連結文字列で組んでいた。ブラウザでの近似レンダリング確認（[Verification](#verification実装-pr-側のチェックリスト)参照）の結果、画面幅 296px・`p-4` 適用後の実効幅 264px に対しこの文字列が1行に収まらず折り返し、カード全体の必要高さが 174px となって 152px の枠を 22px超過、署名行が画面外に押し出されることが判明した。対策として、(1) `p-4` → `p-2` で余白を削減、(2) 主要フォントサイズを縮小（title 28→22 / date 16→12 / temp 18→14）、(3) 降水確率を「降水確率(時間帯)」の見出し span + 4 分割の等幅グリッド（各セル `w-[25%]` 固定）に変更し、セル内改行の有無をブラウザの自動折り返しに委ねない構造にした。ブラウザ近似レンダリングでの実測ではオーバーフロー 0px（安全マージン 17px 超）。
>
> **実機確認後の追加変更（文字潰れ対策）:** 上記修正で 9-10px まで縮小した通常フォント（ベクター）を実機（降水確率グリッド・署名）で表示したところ、実機の低解像度 1bit（白黒）レンダリングでアンチエイリアスが効かず文字が潰れて判読できないとユーザー報告があった。公式ドキュメントに記載のピクセルフォント（低解像度ディスプレイ向けビットマップフォント、`tw` クラス `text-pixel-{size}[-variant]` で指定、`text-pixel-8`/`text-pixel-10`/`text-pixel-12-zpix`/`text-pixel-16-unifont` 等のバリアントのみ存在）に切り替えた。サイズはフォント側のバリアント単位で決まるため `style.fontSize` は指定せず `tw` 側にサイズを委ねる（降水確率グリッド・見出し・署名は `text-pixel-10` に統一）。ブラウザはこのカスタムフォントをレンダリングできないため見た目の事前確認はできず、効果は実機でのみ検証可能（[Open Questions](#open-questions)）。

`testdata/tokyo.json`（東京・雨のち曇・最低26/最高33・降水10/20/40/50・link=JMA・署名=2026年07月24日00:00）を例にした実際の送信 JSON:

```json
{
  "windowData": {
    "default": [
      {
        "type": "div",
        "props": {
          "tw": "flex flex-col w-full h-full justify-between p-2",
          "children": [
            {
              "type": "div",
              "props": {
                "tw": "flex flex-col",
                "children": [
                  { "type": "span", "props": { "style": { "fontSize": 22, "fontWeight": 700 }, "children": "東京 雨のち曇" } },
                  { "type": "span", "props": { "style": { "fontSize": 12 }, "children": "07/24(金)" } }
                ]
              }
            },
            {
              "type": "div",
              "props": {
                "tw": "flex flex-col gap-1",
                "children": [
                  { "type": "span", "props": { "style": { "fontSize": 14 }, "children": "最低 26℃ / 最高 33℃" } },
                  { "type": "span", "props": { "tw": "text-pixel-10", "children": "降水確率(時間帯)" } },
                  {
                    "type": "div",
                    "props": {
                      "tw": "flex flex-row w-full",
                      "children": [
                        { "type": "span", "props": { "tw": "w-[25%] text-pixel-10", "children": "0-6時 10%" } },
                        { "type": "span", "props": { "tw": "w-[25%] text-pixel-10", "children": "6-12時 20%" } },
                        { "type": "span", "props": { "tw": "w-[25%] text-pixel-10", "children": "12-18時 40%" } },
                        { "type": "span", "props": { "tw": "w-[25%] text-pixel-10", "children": "18-24時 50%" } }
                      ]
                    }
                  }
                ]
              }
            },
            { "type": "span", "props": { "tw": "text-pixel-10", "style": { "alignSelf": "flex-end" }, "children": "2026年07月24日00:00" } }
          ]
        }
      }
    ]
  },
  "link": "https://www.jma.go.jp/bosai/forecast/#area_code=130000",
  "refreshNow": true
}
```

### `adapter/in/weather/source.go` の変更

`toTextPayload` を `toCanvasPayload` に置き換える。日付 / 気温 / 降水確率の文字列整形ロジック（`dateLine` / `tempLine` / `rainLine` / `clipRunes` によるタイトル整形）は不変で、出力先を `Message` 文字列結合から span ツリーへ変更するだけに留める。

```go
const (
	cardTW     = "flex flex-col w-full h-full justify-between p-2"
	headerTW   = "flex flex-col"
	bodyTW     = "flex flex-col gap-1"
	rainRowTW  = "flex flex-row w-full"
	rainCellTW = "w-[25%] text-pixel-10"
	smallTW    = "text-pixel-10" // 文字潰れ対策で低解像度向けピクセルフォントへ切り替える小サイズ表示に使う

	rainLabel = "降水確率(時間帯)"
)

var (
	titleStyle     = map[string]any{"fontSize": 22, "fontWeight": 700}
	dateStyle      = map[string]any{"fontSize": 12}
	tempStyle      = map[string]any{"fontSize": 14}
	signatureStyle = map[string]any{"alignSelf": "flex-end"}
)

func canvasDiv(tw string, children ...domain.CanvasElement) domain.CanvasElement {
	return domain.CanvasElement{Type: "div", Props: domain.CanvasElementProps{TW: tw, Children: children}}
}

func canvasSpan(text string, style map[string]any) domain.CanvasElement {
	return canvasSpanFull("", style, text)
}

// canvasSpanTW は tw(カスタムフォント指定等)とテキストを持つ span を組み立てる。
func canvasSpanTW(tw, text string) domain.CanvasElement {
	return canvasSpanFull(tw, nil, text)
}

// canvasSpanFull は tw と style を両方持つ span を組み立てる(例: 幅固定 + ピクセルフォント指定)。
func canvasSpanFull(tw string, style map[string]any, text string) domain.CanvasElement {
	return domain.CanvasElement{Type: "span", Props: domain.CanvasElementProps{TW: tw, Style: style, Children: text}}
}

// canvasRainCell は降水確率グリッドの1セル(幅固定 w-[25%] + ピクセルフォント)を組み立てる。
func canvasRainCell(text string) domain.CanvasElement {
	return canvasSpanTW(rainCellTW, text)
}

// toCanvasPayload は domain.Forecast を Canvas API 表示用の CanvasPayload へ整形する。
func toCanvasPayload(w domain.Forecast, signature string) domain.CanvasPayload {
	title := w.Telop
	if w.City != "" {
		title = w.City + " " + w.Telop
	}
	title = clipRunes(title, titleMaxRunes)

	dateLine := fmt.Sprintf("%s(%s)", w.Date.Format("01/02"), weekdayJP[w.Date.Weekday()])
	tempLine := fmt.Sprintf("最低 %s℃ / 最高 %s℃", tempStr(w.TempMinC), tempStr(w.TempMaxC))

	return domain.CanvasPayload{
		WindowData: domain.WindowData{Default: []domain.CanvasElement{
			canvasDiv(cardTW,
				canvasDiv(headerTW, canvasSpan(title, titleStyle), canvasSpan(dateLine, dateStyle)),
				canvasDiv(bodyTW,
					canvasSpan(tempLine, tempStyle),
					canvasSpanTW(smallTW, rainLabel),
					canvasDiv(rainRowTW,
						canvasRainCell("0-6時 "+w.ChanceOfRain.T0006),
						canvasRainCell("6-12時 "+w.ChanceOfRain.T0612),
						canvasRainCell("12-18時 "+w.ChanceOfRain.T1218),
						canvasRainCell("18-24時 "+w.ChanceOfRain.T1824),
					),
				),
				canvasSpanFull(smallTW, signatureStyle, signature),
			),
		}},
		Link:       w.Link,
		RefreshNow: true,
	}
}
```

`Build` 内の `domain.TextPayload{}` ゼロ値返却はすべて `domain.CanvasPayload{}` に、最終行は `return toCanvasPayload(w, sig), nil` に置き換える。

### `adapter/out/quote0.go` の変更

- URL: `.../device/%s/text` → `.../device/%s/canvas`。
- `Send` のシグネチャ: `p domain.TextPayload` → `p domain.CanvasPayload`。
- コメント・エラー文言の "Text API" 表記を "Canvas API" に置き換える（例: `"quote0: Canvas API failed: status=%d serial=%s"`）。
- marshal / POST / ステータス判定 / masked serial によるエラー文言生成は、ペイロードの型に依存しないため変更不要。

### `cmd/batch/main.go`

`weather.NewSource` / `out.NewQuote0Sink` / `application.RunBatch` の呼び出し形は変わらない。"Text API" と書かれているコメント・ログ文言のみ "Canvas API" に更新する。

### テスト方針（実装 PR への申し送り）

既存テストは以下 2 点の構造的な注意が必要になる。

1. **`==`/`!=` 比較が使えなくなる**: `CanvasPayload` はスライス（`WindowData.Default`）を含むため、Go の構造体比較演算子が使えない。すべて `reflect.DeepEqual` に置き換える。
2. **`Children any` の JSON ラウンドトリップは型を保持しない**: `CanvasElementProps.Children` は `any` のため、`json.Unmarshal` で `any` へ戻すと `[]domain.CanvasElement` ではなく汎用の `[]interface{}` / `map[string]interface{}` になる。ワイヤーレベルでの送信 body 検証（`quote0_test.go`）は、送信側・期待値側の両方を一度 `map[string]any` へ decode してから `reflect.DeepEqual` で比較する（型付き構造体への decode と比較しない）。

対象テスト: `internal/adapter/out/quote0_test.go`（URL・body アサーション）、`internal/adapter/in/weather/source_test.go`（`toTextPayload` → `toCanvasPayload` のテスト名・アサーション変更）、`internal/application/runbatch_test.go`（fake source/sink の型変更）。`internal/config/config_test.go` はペイロード型と無関係のため変更不要。

---

## Alternatives Considered

### A. Text API と Canvas API の並存（フラグ切替）

既存 Text API 実装を残し、環境変数等で Canvas API と切替可能にする案。却下。ユーザーが完全置換を明示的に決定しており、2 つのペイロード構築ロジックを並存させる運用上のメリットがない。

### B. デバイス側テンプレート機能の利用

Canvas API の `data` フィールドと `{{get inputData ...}}` / `$for` 等のテンプレート構文を使い、値の埋め込みをデバイス側に委ねる案。却下。本バッチは実行ごとに全ての表示値を Go 側で確定済みで取得しているため、テンプレート言語をもう一層導入する理由がない。事前レンダリングした文字列を送る方が構成が単純で、既存のテスト容易性（純粋関数によるマッパー）を維持できる。

### C. 既存レイアウトの pixel 再現

Text API での表示（`title`/`message`/`signature`/`link`）をそのまま視覚的に再現する案。却下。ユーザーがデザイン一任を明示しており、かつ実機で有効な `tw` 構文の全体像が未検証なため、画像やカスタムフォントを使わないシンプルな `div`/`span` カード構成に留め、リスクを最小化する。

---

## Cross-Cutting Concerns

### セキュリティ

秘匿情報（`DOT_API_KEY` / `SERIAL_NUM`）の扱いは変更なし。`MaskedSerial()` によるログ・エラーのマスキング方針も [0001](./0001-initial-setup.md) のまま維持する。

### 可観測性 / 失敗時挙動

非 2xx レスポンス時はマスク済みシリアルのみを含むエラーでラップして返す、という既存方針を維持する。Canvas API のエラーレスポンス（body の JSON 形式）は現時点で未確認のため、body は decode せずステータスコードのみで判定する（[Open Questions](#open-questions)）。

### テスト / CI 品質

既存のテーブル駆動 + `t.Run` 日本語ケース名規約・fixture 駆動（`testdata/`）パターンを維持する。上記「テスト方針」節の `reflect.DeepEqual` / `any` ラウンドトリップに関する注意を実装時に踏襲する。CI（`ci.yml`）はネットワーク非依存のまま変更不要。

### コスト

実行頻度（1 日 1 回）に変更はない。

---

## Milestones（実装 PR 側の分割案）

1. `domain/canvas.go` 新規追加 + `domain/content.go` の port 更新（`TextPayload` 削除）。
2. `adapter/out/quote0.go`: エンドポイント `/canvas` 化 + テスト更新。
3. `adapter/in/weather/source.go`: `toCanvasPayload` 実装 + テスト更新。
4. `application/runbatch_test.go` の fake source/sink 型更新。
5. `cmd/batch/main.go` / `README.md` / `AGENTS.md` / `.github/workflows/batch.yml` のコメント・文言更新（"Text API" → "Canvas API"）。
6. 実機への手動確認（下記 Verification）と、その結果の Resolved Decisions への反映。

## Verification（実装 PR 側のチェックリスト）

1. `go build ./...`
2. `go test ./...`
3. `go vet ./...`
4. `gofmt -l .`（出力が空であること）
5. `golangci-lint run`
6. 実機への手動確認（ドキュメントのみでは検証不可なため必須）: 本 doc の JSON 例と同形のペイロードを組み、
   ```sh
   curl -X POST "https://dot.mindreset.tech/api/authV2/open/device/${SERIAL_NUM}/canvas" \
     -H "Authorization: Bearer ${DOT_API_KEY}" \
     -H "Content-Type: application/json" \
     -d @payload.json
   ```
   で 200 応答と実機での表示崩れ（画面からの溢れ・フォントサイズの過不足・`text-pixel-10` 切替後の文字の判読可否）がないことを確認する。結果は本 doc の Resolved Decisions に追記する。

---

## Resolved Decisions

- **移行方針**: Text API を完全に置換する。並存モードは設けない（ユーザー決定）。
- **レイアウト方針**: 既存表示の pixel 再現は要求せず、新規デザインとする（ユーザー決定）。画像・カスタムフォントは使わず `div`/`span` のみで構成し、`tw` は基本レイアウト用途に限定、pixel 値指定は `style` に委ねる。
- **降水確率レイアウト（見切れ修正）**: 初版の単一連結文字列は実機幅で折り返し、カード下部（署名）が見切れる不具合があった。ブラウザでの近似レンダリング確認により原因特定し、「降水確率(時間帯)」見出し + 4 分割等幅グリッド（セル幅固定 `w-[25%]`）へ変更して解消（実測オーバーフロー 0px）。あわせて `p-4`→`p-2`、主要フォントサイズを縮小。
- **小サイズ表示のフォント（文字潰れ対策）**: 見切れ修正で 9-10px まで縮小した通常フォントを実機で表示したところ、文字が潰れて判読できないとユーザー報告があった（ユーザー決定: 低解像度向けピクセルフォントへの切替を優先検証）。降水確率グリッド・見出し・署名を `text-pixel-10`（`tw` クラス指定、`style.fontSize` は使わない）に変更。効果は実機未検証（[Open Questions](#open-questions)）。
- **デバイス側テンプレート機能**: 使用しない。値は全て Go 側で事前計算した文字列として埋め込む。

## Open Questions

- 実機の正確な pixel 寸法。Image API ドキュメント記載値（296×152px）をブラウザでの近似レンダリング確認に転用しているが、Canvas API 自体のドキュメントには寸法記載がなく、実機確定ではない。
- 実機で有効な `tw` 構文・フォント名の全体像（公式ドキュメントに記載のカスタムフォント `text-{size}-{font}` / `text-pixel-{size}` が実機でどこまで利用可能か）。ブラウザ近似レンダリングは `flex`/`gap`/`padding`/`justify`/`w-[...]` 等の主要クラスのみ簡易解釈しており、実機の `tw` パーサ・フォントレンダリングとの一致は未検証。
- **`text-pixel-10` 切替の実機効果**: 文字潰れ対策として降水確率グリッド・見出し・署名を `text-pixel-10` に変更したが、ブラウザはこのカスタムフォントをレンダリングできないため事前確認ができていない。実機で (1) 判読可能な太さ・輪郭になるか、(2) `w-[25%]` セル幅内で文字列が折り返さず収まるか（ピクセルフォントは等幅ビットマップの可能性があり、ブラウザでの近似幅と実際の文字幅が異なりうる）を確認する必要がある。
- Canvas API のエラーレスポンスの JSON 形式（不明な `type`/`props` 等を送った場合のエラー構造）。

いずれも実機への手動確認（[Verification](#verification実装-pr-側のチェックリスト)）で解消し、結果を本 doc に追記する。
