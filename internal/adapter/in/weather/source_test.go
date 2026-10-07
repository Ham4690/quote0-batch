package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"
	"time"

	"github.com/Ham4690/quote0-batch/internal/config"
	"github.com/Ham4690/quote0-batch/internal/domain"
)

// loadFixture は testdata の JSON を apiResponse へ decode するヘルパ。
func loadFixture(t *testing.T, name string) apiResponse {
	t.Helper()
	b, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("fixture 読み込みに失敗: %v", err)
	}
	var r apiResponse
	if err := json.Unmarshal(b, &r); err != nil {
		t.Fatalf("fixture の decode に失敗: %v", err)
	}
	return r
}

// canvasTexts は CanvasElement ツリーから span のテキストを出現順に集める。
// toCanvasPayload が組み立てた windowData の内容を検証するためのテストヘルパ。
func canvasTexts(e domain.CanvasElement) []string {
	if e.Type == "span" {
		if s, ok := e.Props.Children.(string); ok {
			return []string{s}
		}
		return nil
	}
	children, ok := e.Props.Children.([]domain.CanvasElement)
	if !ok {
		return nil
	}
	var out []string
	for _, c := range children {
		out = append(out, canvasTexts(c)...)
	}
	return out
}

func TestSelectToday(t *testing.T) {
	t.Run("selectToday は date が実行日と一致する要素を返す", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		f, err := selectToday(r, "2026-07-24")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if f.Telop != "雨のち曇" {
			t.Errorf("Telop = %q, want %q", f.Telop, "雨のち曇")
		}
	})

	t.Run("selectToday は date 不一致でも dateLabel が今日の要素へフォールバックする", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		// 実行日が fixture の date と異なる場合でも「今日」ラベルで拾う。
		f, err := selectToday(r, "2999-01-01")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if f.DateLabel != "今日" {
			t.Errorf("DateLabel = %q, want %q", f.DateLabel, "今日")
		}
	})

	t.Run("selectToday は当日エントリが存在しないとエラーを返す", func(t *testing.T) {
		r := apiResponse{Forecasts: []forecast{
			{Date: "2026-07-25", DateLabel: "明日", Telop: "晴れ"},
		}}
		if _, err := selectToday(r, "2026-07-24"); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("selectToday は telop が空だと構造異常としてエラーを返す", func(t *testing.T) {
		r := apiResponse{Forecasts: []forecast{
			{Date: "2026-07-24", DateLabel: "今日", Telop: ""},
		}}
		if _, err := selectToday(r, "2026-07-24"); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})
}

func TestToForecast(t *testing.T) {
	t.Run("toForecast は celsius 文字列を *int に変換し link を引き継ぐ", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "https://example.test/link")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if f.City != "東京" {
			t.Errorf("City = %q, want %q", f.City, "東京")
		}
		if f.TempMinC == nil || *f.TempMinC != 26 {
			t.Errorf("TempMinC = %v, want 26", f.TempMinC)
		}
		if f.TempMaxC == nil || *f.TempMaxC != 33 {
			t.Errorf("TempMaxC = %v, want 33", f.TempMaxC)
		}
		if f.ChanceOfRain.T1824 != "50%" {
			t.Errorf("T1824 = %q, want %q", f.ChanceOfRain.T1824, "50%")
		}
		if f.Link != "https://example.test/link" {
			t.Errorf("Link = %q", f.Link)
		}
		// date は JST として解釈する。
		if y, m, d := f.Date.Date(); y != 2026 || m != 7 || d != 24 {
			t.Errorf("Date = %v, want 2026-07-24", f.Date)
		}
		if name := f.Date.Location().String(); name != "Asia/Tokyo" {
			t.Errorf("Date location = %q, want Asia/Tokyo", name)
		}
	})

	t.Run("toForecast は celsius が null なら気温を nil にする", func(t *testing.T) {
		r := loadFixture(t, "tokyo_null_temp.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if f.TempMinC != nil {
			t.Errorf("TempMinC = %v, want nil", *f.TempMinC)
		}
		if f.TempMaxC != nil {
			t.Errorf("TempMaxC = %v, want nil", *f.TempMaxC)
		}
	})

	t.Run("toForecast は date が不正だとエラーを返す", func(t *testing.T) {
		if _, err := toForecast(forecast{Date: "not-a-date", Telop: "晴れ"}, "", ""); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("toForecast は非数値 celsius を欠損(nil)扱いし空白付き数値はパースする", func(t *testing.T) {
		bad := "abc"
		padded := " 33 "
		f, err := toForecast(forecast{
			Date:        "2026-07-24",
			Telop:       "晴れ",
			Temperature: temperature{Min: tempValue{Celsius: &bad}, Max: tempValue{Celsius: &padded}},
		}, "", "")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if f.TempMinC != nil {
			t.Errorf("TempMinC = %v, want nil(非数値は欠損)", *f.TempMinC)
		}
		if f.TempMaxC == nil || *f.TempMaxC != 33 {
			t.Errorf("TempMaxC = %v, want 33(前後空白は TrimSpace)", f.TempMaxC)
		}
	})
}

func TestToCanvasPayload(t *testing.T) {
	t.Run("toCanvasPayload は気温・降水確率を整形し windowData ツリーを組み立てる", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "https://example.test/link")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		p := toCanvasPayload(f, "2026年07月24日00:00")

		if !p.RefreshNow {
			t.Error("RefreshNow = false, want true")
		}
		if p.Link != "https://example.test/link" {
			t.Errorf("Link = %q", p.Link)
		}

		want := domain.CanvasPayload{
			WindowData: domain.WindowData{Default: []domain.CanvasElement{
				canvasDiv(cardTW,
					canvasDiv(headerTW,
						canvasSpan("東京 雨のち曇", titleStyle),
						canvasSpan("07/24(金)", dateStyle),
					),
					canvasDiv(bodyTW,
						canvasSpan("最低 26℃ / 最高 33℃", tempStyle),
						canvasSpan(rainLabel, rainLabelStyle),
						canvasDiv(rainRowTW,
							canvasRainCell("0-6時 10%"),
							canvasRainCell("6-12時 20%"),
							canvasRainCell("12-18時 40%"),
							canvasRainCell("18-24時 50%"),
						),
					),
					canvasSpan("2026年07月24日00:00", signatureStyle),
				),
			}},
			Link:       "https://example.test/link",
			RefreshNow: true,
		}
		if !reflect.DeepEqual(p, want) {
			t.Errorf("CanvasPayload =\n%+v\nwant\n%+v", p, want)
		}
	})

	t.Run("toCanvasPayload は欠損気温を -- で表示する", func(t *testing.T) {
		r := loadFixture(t, "tokyo_null_temp.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		p := toCanvasPayload(f, "sig")
		texts := canvasTexts(p.WindowData.Default[0])
		want := []string{
			"東京 晴時々曇",
			"07/24(金)",
			"最低 --℃ / 最高 --℃",
			rainLabel,
			"0-6時 --%",
			"6-12時 --%",
			"12-18時 --%",
			"18-24時 50%",
			"sig",
		}
		if !reflect.DeepEqual(texts, want) {
			t.Errorf("テキスト =\n%q\nwant\n%q", texts, want)
		}
	})

	t.Run("toCanvasPayload は地点名が空ならタイトルを telop のみにする", func(t *testing.T) {
		// City 欠損時のフォールバック。「地点名 + 天気概況」ではなく telop 単体になる。
		w := domain.Forecast{
			Date:  time.Date(2026, 7, 24, 0, 0, 0, 0, jst),
			City:  "", // 地点名なし
			Telop: "晴れ",
		}
		p := toCanvasPayload(w, "sig")
		texts := canvasTexts(p.WindowData.Default[0])
		if len(texts) == 0 || texts[0] != "晴れ" {
			t.Errorf("タイトル = %q, want %q(地点名なしは telop のみ)", texts, "晴れ")
		}
	})
}

func TestWeatherSource_Build(t *testing.T) {
	// 2026-07-24 00:00 JST を固定実行時刻とする。
	fixedNow := func() time.Time { return time.Date(2026, 7, 24, 0, 0, 0, 0, jst) }

	t.Run("WeatherSource.Build は city 指定で GET し当日の payload を組み立てる", func(t *testing.T) {
		var gotPath, gotQuery string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			gotQuery = r.URL.RawQuery
			b, _ := os.ReadFile(filepath.Join("testdata", "tokyo.json"))
			_, _ = w.Write(b)
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		p, err := src.Build(context.Background())
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if gotPath != "/api/forecast" {
			t.Errorf("path = %q, want /api/forecast", gotPath)
		}
		if gotQuery != "city=130010" {
			t.Errorf("query = %q, want city=130010", gotQuery)
		}
		texts := canvasTexts(p.WindowData.Default[0])
		if len(texts) == 0 || texts[0] != "東京 雨のち曇" {
			t.Errorf("タイトル = %q, want %q", texts, "東京 雨のち曇")
		}
		if last := texts[len(texts)-1]; last != "2026年07月24日00:00" {
			t.Errorf("署名 = %q, want %q", last, "2026年07月24日00:00")
		}
	})

	t.Run("WeatherSource.Build は location 欠損なら title を telop のみにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// location キーを含まないレスポンス。地点名は取得できない。
			_, _ = w.Write([]byte(`{"forecasts":[{"date":"2026-07-24","dateLabel":"今日","telop":"晴れ"}],"link":"x"}`))
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		p, err := src.Build(context.Background())
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		texts := canvasTexts(p.WindowData.Default[0])
		if len(texts) == 0 || texts[0] != "晴れ" {
			t.Errorf("タイトル = %q, want %q(location 欠損は telop のみ)", texts, "晴れ")
		}
	})

	t.Run("WeatherSource.Build は WEATHER_LINK_URL 未設定なら API の link を使う", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := os.ReadFile(filepath.Join("testdata", "tokyo.json"))
			_, _ = w.Write(b)
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL} // WeatherLinkURL 空
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		p, err := src.Build(context.Background())
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if p.Link != "https://www.jma.go.jp/bosai/forecast/#area_code=130000" {
			t.Errorf("Link = %q, want API の link", p.Link)
		}
	})

	t.Run("WeatherSource.Build は WEATHER_LINK_URL 設定時にそれを優先する", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			b, _ := os.ReadFile(filepath.Join("testdata", "tokyo.json"))
			_, _ = w.Write(b)
		}))
		defer srv.Close()

		want := "https://weather.yahoo.co.jp/weather/jp/13/4410.html"
		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL, WeatherLinkURL: want}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		p, err := src.Build(context.Background())
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if p.Link != want {
			t.Errorf("Link = %q, want %q", p.Link, want)
		}
	})

	t.Run("WeatherSource.Build は API が非2xxを返すとエラーにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		if _, err := src.Build(context.Background()); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("WeatherSource.Build は forecasts 空レスポンスでエラーにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"forecasts":[],"link":"x"}`))
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		if _, err := src.Build(context.Background()); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("WeatherSource.Build は不正 JSON ボディで decode エラーにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"forecasts":`)) // 途中で切れた壊れた JSON
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		if _, err := src.Build(context.Background()); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("WeatherSource.Build は当日エントリ不在でエラーにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// date も dateLabel も当日(2026-07-24)に一致しない。
			_, _ = w.Write([]byte(`{"forecasts":[{"date":"2026-07-25","dateLabel":"明日","telop":"晴れ"}],"link":"x"}`))
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		if _, err := src.Build(context.Background()); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})

	t.Run("WeatherSource.Build は当日 telop 空でエラーにする", func(t *testing.T) {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(`{"forecasts":[{"date":"2026-07-24","dateLabel":"今日","telop":""}],"link":"x"}`))
		}))
		defer srv.Close()

		cfg := config.Config{CityCode: "130010", WeatherBaseURL: srv.URL}
		src := &WeatherSource{cfg: cfg, client: srv.Client(), now: fixedNow}

		if _, err := src.Build(context.Background()); err == nil {
			t.Fatal("エラーを期待したが nil")
		}
	})
}

func TestNewSource(t *testing.T) {
	t.Run("NewSource は client が nil でもタイムアウト付き client を用意する", func(t *testing.T) {
		src := NewSource(config.Config{CityCode: "130010", WeatherBaseURL: "https://example.test"}, nil)
		if src.client == nil {
			t.Fatal("client = nil, want 既定 client")
		}
		if src.now == nil {
			t.Fatal("now = nil, want time.Now")
		}
	})
}

func TestClipRunes(t *testing.T) {
	tests := []struct {
		name string
		in   string
		max  int
		want string
	}{
		{"clipRunes は上限以下ならそのまま返す", "雨のち曇", 8, "雨のち曇"},
		{"clipRunes は上限ちょうどならそのまま返す", "雨のち曇", 4, "雨のち曇"},
		{"clipRunes は超過分を rune 単位で切り末尾に … を付す", "雨時々曇一時雷", 4, "雨時々曇…"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := clipRunes(tt.in, tt.max); got != tt.want {
				t.Errorf("clipRunes(%q, %d) = %q, want %q", tt.in, tt.max, got, tt.want)
			}
		})
	}
}
