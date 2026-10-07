package weather

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	neturl "net/url"
	"strconv"
	"strings"
	"time"

	// tzdata をバイナリへ埋め込む。OS の tzdata に依存せず LoadLocation を成功させる。
	_ "time/tzdata"

	"github.com/Ham4690/quote0-batch/internal/config"
	"github.com/Ham4690/quote0-batch/internal/domain"
)

// jst は起動時に 1 度だけ解決する。runner の TZ 設定に依存せず確実に JST 化する。
var jst = mustLoadJST()

func mustLoadJST() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		panic("weather: Asia/Tokyo ロケーションのロードに失敗: " + err.Error())
	}
	return loc
}

// titleMaxRunes は quote/0 の title 表示幅に合わせた暫定上限(文字数)。
// 正確な上限は実機確認で確定する(Design Doc 0002 の Open Questions 参照)。
const titleMaxRunes = 11

// dateLabelToday は当日エントリを示す API 固定ラベル。
const dateLabelToday = "今日"

// weekdayJP は time.Weekday(日曜=0)を日本語一文字へ対応させる。
var weekdayJP = [...]string{"日", "月", "火", "水", "木", "金", "土"}

// defaultTimeout は天気 API 呼び出しの上限時間(out-adapter と同方針)。
const defaultTimeout = 30 * time.Second

// maxBodyBytes は decode するレスポンスボディの上限。天気 API のレスポンスは
// 数 KB 程度のため、時間(defaultTimeout)に加えバイト数でも防御する。
const maxBodyBytes = 1 << 20 // 1MiB

// WeatherSource は天気 API(weather.tsukumijima.net)を叩く ContentSource 実装。
type WeatherSource struct {
	cfg    config.Config
	client *http.Client     // DI: テストで httptest.Server の client を差し替える
	now    func() time.Time // DI: テストで実行時刻を固定する
}

// ContentSource 充足のコンパイル時表明。
var _ domain.ContentSource = (*WeatherSource)(nil)

// NewSource は WeatherSource を生成する。client が nil の場合はタイムアウト付き
// client を使う(http.DefaultClient は無制限のため使わない)。
func NewSource(cfg config.Config, client *http.Client) *WeatherSource {
	if client == nil {
		client = &http.Client{Timeout: defaultTimeout}
	}
	return &WeatherSource{cfg: cfg, client: client, now: time.Now}
}

// Build は当日の天気予報を取得し、quote/0 表示用の CanvasPayload を組み立てる。
// 気温欠損(null)や "--%" は正常系として表示継続する。構造異常はエラーを返す。
func (s *WeatherSource) Build(ctx context.Context) (domain.CanvasPayload, error) {
	url := fmt.Sprintf("%s/api/forecast?city=%s", s.cfg.WeatherBaseURL, neturl.QueryEscape(s.cfg.CityCode))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.CanvasPayload{}, fmt.Errorf("weather: リクエスト生成に失敗: %w", err)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return domain.CanvasPayload{}, fmt.Errorf("weather: リクエスト送信に失敗: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.CanvasPayload{}, fmt.Errorf("weather: forecast API failed: status=%d", res.StatusCode)
	}

	var resp apiResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(&resp); err != nil {
		return domain.CanvasPayload{}, fmt.Errorf("weather: レスポンスの decode に失敗: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return domain.CanvasPayload{}, err
	}

	today := s.now().In(jst).Format("2006-01-02")
	entry, err := selectToday(resp, today)
	if err != nil {
		return domain.CanvasPayload{}, err
	}

	// link は env(Yahoo 天気)を優先し、未設定なら API の link(気象庁)へフォールバック。
	link := s.cfg.WeatherLinkURL
	if link == "" {
		link = resp.Link
	}

	w, err := toForecast(entry, resp.Location.City, link)
	if err != nil {
		return domain.CanvasPayload{}, err
	}

	sig := s.now().In(jst).Format("2006年01月02日15:04")
	return toCanvasPayload(w, sig), nil
}

// selectToday は forecasts から当日(実行日 JST)の要素を選ぶ。
//  1. date が実行日(today, "2006-01-02")と一致する要素
//  2. フォールバック: dateLabel == "今日" の要素
//  3. どちらも無い / telop 空 は構造異常としてエラー
func selectToday(r apiResponse, today string) (forecast, error) {
	var chosen *forecast
	for i := range r.Forecasts {
		if r.Forecasts[i].Date == today {
			chosen = &r.Forecasts[i]
			break
		}
	}
	if chosen == nil {
		for i := range r.Forecasts {
			if r.Forecasts[i].DateLabel == dateLabelToday {
				chosen = &r.Forecasts[i]
				break
			}
		}
	}
	if chosen == nil {
		return forecast{}, fmt.Errorf("weather: 当日(%s)エントリが見つからない", today)
	}
	if chosen.Telop == "" {
		return forecast{}, errors.New("weather: 当日エントリの telop が空")
	}
	return *chosen, nil
}

// toForecast は DTO の 1 日分を domain.Forecast へ変換する。
// date は JST 解釈、celsius 文字列は *int(空/null は nil)に変換する。
// city は API レスポンスの地点名(link と同様に Build 層から引き回す)。
func toForecast(f forecast, city, link string) (domain.Forecast, error) {
	d, err := time.ParseInLocation("2006-01-02", f.Date, jst)
	if err != nil {
		return domain.Forecast{}, fmt.Errorf("weather: date のパースに失敗 (%q): %w", f.Date, err)
	}
	return domain.Forecast{
		Date:     d,
		City:     city,
		Telop:    f.Telop,
		TempMinC: parseCelsius(f.Temperature.Min.Celsius),
		TempMaxC: parseCelsius(f.Temperature.Max.Celsius),
		ChanceOfRain: domain.RainChance{
			T0006: f.ChanceOfRain.T00_06,
			T0612: f.ChanceOfRain.T06_12,
			T1218: f.ChanceOfRain.T12_18,
			T1824: f.ChanceOfRain.T18_24,
		},
		Link: link,
	}, nil
}

// parseCelsius は celsius 文字列ポインタを *int へ変換する。
// nil / 空文字 / 数値でない場合は欠損として nil を返す(表示継続を優先)。
func parseCelsius(s *string) *int {
	if s == nil {
		return nil
	}
	v, err := strconv.Atoi(strings.TrimSpace(*s))
	if err != nil {
		return nil
	}
	return &v
}

// windowData レイアウト定数。画像・カスタムフォントは使わず div/span のみで構成する
// (実機の tw 構文が未検証のためリスクを最小化する。0005 Design Doc 参照)。
//
// 降水確率は "降水 0-6 10% / 6-12 20% / ..." の単一文字列で組んでいたが、
// 実機(296x152px)で p-4 適用後の幅 264px に収まらず折り返し、カード全体の必要高さが
// 152px を超えて署名行が見切れる不具合があった(0005 Design Doc 参照)。
// 見出し + 4 分割の等幅グリッド(各セル w-[25%])へ変更し、折り返し位置をブラウザ側の
// 自動改行に委ねない構造にして解消する。
//
// 上記修正で導入した 9-10px の通常フォント(ベクター)は、実機の低解像度 1bit(白黒)
// レンダリングでアンチエイリアスが効かず文字が潰れて判読できなくなった。公式ドキュメント
// 記載のピクセルフォント(低解像度ディスプレイ向けのビットマップフォント、tw クラス名
// "text-pixel-{size}[-variant]" で指定)に切り替え、小サイズ表示の視認性を改善する。
// サイズは pixel 値そのままではなくフォント側のバリアント単位(8/10/12-zpix/16-unifont 等)
// のため、style.fontSize は指定せず tw 側にサイズを委ねる。
const (
	cardTW     = "flex flex-col w-full h-full justify-between p-2"
	headerTW   = "flex flex-col"
	bodyTW     = "flex flex-col gap-1"
	rainRowTW  = "flex flex-row w-full"
	rainCellTW = "w-[25%] text-pixel-10"
	smallTW    = "text-pixel-10" // 降水確率見出し・署名など、潰れ対策でピクセルフォントへ切り替える小サイズ表示に使う

	rainLabel = "降水確率(時間帯)"
)

var (
	titleStyle     = map[string]any{"fontSize": 22, "fontWeight": 700}
	dateStyle      = map[string]any{"fontSize": 12}
	tempStyle      = map[string]any{"fontSize": 14}
	signatureStyle = map[string]any{"alignSelf": "flex-end"}
)

// canvasDiv は tw(レイアウト用ユーティリティ)と子要素を持つ div 要素を組み立てる。
func canvasDiv(tw string, children ...domain.CanvasElement) domain.CanvasElement {
	return domain.CanvasElement{Type: "div", Props: domain.CanvasElementProps{TW: tw, Children: children}}
}

// canvasSpan は style(pixel 値確定用)とテキストを持つ span 要素を組み立てる。
func canvasSpan(text string, style map[string]any) domain.CanvasElement {
	return canvasSpanFull("", style, text)
}

// canvasSpanTW は tw(カスタムフォント指定等のクラス)とテキストを持つ span 要素を組み立てる。
// style を使う canvasSpan と異なり、サイズ等をフォント側のバリアント(例: ピクセルフォント)に委ねる。
func canvasSpanTW(tw, text string) domain.CanvasElement {
	return canvasSpanFull(tw, nil, text)
}

// canvasSpanFull は tw と style を両方持つ span 要素を組み立てる(例: 幅固定 + ピクセルフォント指定)。
func canvasSpanFull(tw string, style map[string]any, text string) domain.CanvasElement {
	return domain.CanvasElement{Type: "span", Props: domain.CanvasElementProps{TW: tw, Style: style, Children: text}}
}

// canvasRainCell は降水確率グリッドの1セル(幅固定 w-[25%] + ピクセルフォント)を組み立てる。
// 幅を文字量に関わらず固定することで、セル内改行の有無をブラウザの自動折り返しに委ねない。
func canvasRainCell(text string) domain.CanvasElement {
	return canvasSpanTW(rainCellTW, text)
}

// toCanvasPayload は domain.Forecast を quote/0 Canvas API 表示用の CanvasPayload へ整形する。
// 欠損気温は "--" で表示し、title は表示幅超過時に rune 単位でクリップする。
func toCanvasPayload(w domain.Forecast, signature string) domain.CanvasPayload {
	// title は「地点名 + 天気概況」。地点名が空なら telop のみへフォールバックする。
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

// tempStr は欠損(nil)を "--"、それ以外を数値文字列にする。
func tempStr(c *int) string {
	if c == nil {
		return "--"
	}
	return strconv.Itoa(*c)
}

// clipRunes は rune 単位で s を max 文字に切り詰め、超過時は末尾に "…" を付す。
// byte 単位で切るとマルチバイト文字が壊れるため rune 単位で扱う。
func clipRunes(s string, max int) string {
	r := []rune(s)
	if len(r) <= max {
		return s
	}
	return string(r[:max]) + "…"
}
