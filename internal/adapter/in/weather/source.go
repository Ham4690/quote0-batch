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

// Build は当日の天気予報を取得し、quote/0 表示用の TextPayload を組み立てる。
// 気温欠損(null)や "--%" は正常系として表示継続する。構造異常はエラーを返す。
func (s *WeatherSource) Build(ctx context.Context) (domain.TextPayload, error) {
	url := fmt.Sprintf("%s/api/forecast?city=%s", s.cfg.WeatherBaseURL, neturl.QueryEscape(s.cfg.CityCode))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return domain.TextPayload{}, fmt.Errorf("weather: リクエスト生成に失敗: %w", err)
	}

	res, err := s.client.Do(req)
	if err != nil {
		return domain.TextPayload{}, fmt.Errorf("weather: リクエスト送信に失敗: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		return domain.TextPayload{}, fmt.Errorf("weather: forecast API failed: status=%d", res.StatusCode)
	}

	var resp apiResponse
	if err := json.NewDecoder(io.LimitReader(res.Body, maxBodyBytes)).Decode(&resp); err != nil {
		return domain.TextPayload{}, fmt.Errorf("weather: レスポンスの decode に失敗: %w", err)
	}
	if err := resp.Validate(); err != nil {
		return domain.TextPayload{}, err
	}

	today := s.now().In(jst).Format("2006-01-02")
	entry, err := selectToday(resp, today)
	if err != nil {
		return domain.TextPayload{}, err
	}

	// link は env(Yahoo 天気)を優先し、未設定なら API の link(気象庁)へフォールバック。
	link := s.cfg.WeatherLinkURL
	if link == "" {
		link = resp.Link
	}

	w, err := toForecast(entry, resp.Location.City, link)
	if err != nil {
		return domain.TextPayload{}, err
	}

	sig := s.now().In(jst).Format("2006年01月02日15:04")
	return toTextPayload(w, sig), nil
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

// toTextPayload は domain.Forecast を quote/0 表示用の TextPayload へ整形する。
// 欠損気温は "--" で表示し、title は表示幅超過時に rune 単位でクリップする。
func toTextPayload(w domain.Forecast, signature string) domain.TextPayload {
	dateLine := fmt.Sprintf("%s(%s)", w.Date.Format("01/02"), weekdayJP[w.Date.Weekday()])
	tempLine := fmt.Sprintf("最低 %s℃ / 最高 %s℃", tempStr(w.TempMinC), tempStr(w.TempMaxC))
	rainLine := fmt.Sprintf("降水 0-6 %s / 6-12 %s / 12-18 %s / 18-24 %s",
		w.ChanceOfRain.T0006, w.ChanceOfRain.T0612, w.ChanceOfRain.T1218, w.ChanceOfRain.T1824)

	// title は「地点名 + 天気概況」。地点名が空なら telop のみへフォールバックする。
	title := w.Telop
	if w.City != "" {
		title = w.City + " " + w.Telop
	}

	return domain.TextPayload{
		Title:      clipRunes(title, titleMaxRunes),
		Message:    strings.Join([]string{dateLine, tempLine, rainLine}, "\n"),
		Signature:  signature,
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
