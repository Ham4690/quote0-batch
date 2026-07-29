package weather

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Ham4690/quote0-batch/internal/config"
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

func TestToTextPayload(t *testing.T) {
	t.Run("toTextPayload は気温・降水確率を整形し曜日付き日付を出す", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "https://example.test/link")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		p := toTextPayload(f, "2026年07月24日00:00")

		if p.Title != "東京 雨のち曇" {
			t.Errorf("Title = %q", p.Title)
		}
		if !p.RefreshNow {
			t.Error("RefreshNow = false, want true")
		}
		if p.Link != "https://example.test/link" {
			t.Errorf("Link = %q", p.Link)
		}
		if p.Signature != "2026年07月24日00:00" {
			t.Errorf("Signature = %q", p.Signature)
		}
		want := "07/24(金)\n最低 26℃ / 最高 33℃\n降水 0-6 10% / 6-12 20% / 12-18 40% / 18-24 50%"
		if p.Message != want {
			t.Errorf("Message =\n%q\nwant\n%q", p.Message, want)
		}
	})

	t.Run("toTextPayload は欠損気温を -- で表示する", func(t *testing.T) {
		r := loadFixture(t, "tokyo_null_temp.json")
		f, err := toForecast(r.Forecasts[0], r.Location.City, "")
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		p := toTextPayload(f, "sig")
		want := "07/24(金)\n最低 --℃ / 最高 --℃\n降水 0-6 --% / 6-12 --% / 12-18 --% / 18-24 50%"
		if p.Message != want {
			t.Errorf("Message =\n%q\nwant\n%q", p.Message, want)
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
		if p.Title != "東京 雨のち曇" {
			t.Errorf("Title = %q", p.Title)
		}
		if p.Signature != "2026年07月24日00:00" {
			t.Errorf("Signature = %q", p.Signature)
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
