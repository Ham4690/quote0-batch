// Package config は環境変数から実行時設定をロードし検証する。
// シリアルナンバー / API Key はハードコードせず、必ず環境変数から取得する。
package config

import (
	"fmt"
	"os"
	"strings"
)

// 既定の quote/0 API ベース URL。DOT_BASE_URL 未設定時に使う。
const defaultBaseURL = "https://dot.mindreset.tech"

// 天気連携(M3)の既定値。いずれも非秘匿で GitHub Actions Variables から注入する。
const (
	defaultCityCode       = "130010"                          // 東京
	defaultWeatherBaseURL = "https://weather.tsukumijima.net" // livedoor 天気互換
)

// Config は起動時に確定する実行時設定。
type Config struct {
	APIKey    string // DOT_API_KEY: Bearer トークン(秘匿)
	SerialNum string // SERIAL_NUM: デバイスのシリアルナンバー(秘匿)
	BaseURL   string // DOT_BASE_URL: API ベース URL(非秘匿・既定値あり)

	// 天気連携(M3)。すべて非秘匿・既定値あり。
	CityCode       string // CITY_CODE: 天気 API の地点コード(既定 130010=東京)
	WeatherBaseURL string // WEATHER_BASE_URL: 天気 API ベース URL(末尾スラッシュ正規化)
	WeatherLinkURL string // WEATHER_LINK_URL: 表示の link 遷移先(未設定なら API の link へフォールバック)
}

// Load は環境変数から Config を組み立てる。必須変数(DOT_API_KEY / SERIAL_NUM)が
// 欠落している場合は即エラーを返す(fail-fast)。エラーメッセージには秘匿値を含めない。
func Load() (Config, error) {
	cfg := Config{
		APIKey:    os.Getenv("DOT_API_KEY"),
		SerialNum: os.Getenv("SERIAL_NUM"),
		// 末尾スラッシュを除去して正規化(URL 連結時の "//" を防止)。
		BaseURL: strings.TrimSuffix(os.Getenv("DOT_BASE_URL"), "/"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

	cfg.CityCode = os.Getenv("CITY_CODE")
	if cfg.CityCode == "" {
		cfg.CityCode = defaultCityCode
	}
	// 末尾スラッシュを除去して正規化(URL 連結時の "//" を防止)。
	cfg.WeatherBaseURL = strings.TrimSuffix(os.Getenv("WEATHER_BASE_URL"), "/")
	if cfg.WeatherBaseURL == "" {
		cfg.WeatherBaseURL = defaultWeatherBaseURL
	}
	// 未設定は空のまま。source 側で API レスポンスの link へフォールバックする。
	cfg.WeatherLinkURL = os.Getenv("WEATHER_LINK_URL")

	var missing []string
	if cfg.APIKey == "" {
		missing = append(missing, "DOT_API_KEY")
	}
	if cfg.SerialNum == "" {
		missing = append(missing, "SERIAL_NUM")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("config: 必須環境変数が未設定です: %s", strings.Join(missing, ", "))
	}
	return cfg, nil
}

// MaskedSerial はログ / エラー出力用にシリアルナンバーを末尾 4 桁のみ残してマスクする。
// シリアルの漏洩は第三者による実機書き込みに繋がるため、平文で出力しない。
func (c Config) MaskedSerial() string {
	return maskTail(c.SerialNum)
}

func maskTail(s string) string {
	const visible = 4
	if len(s) <= visible {
		return strings.Repeat("*", len(s))
	}
	return strings.Repeat("*", len(s)-visible) + s[len(s)-visible:]
}
