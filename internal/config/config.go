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

// Config は起動時に確定する実行時設定。
type Config struct {
	APIKey    string // DOT_API_KEY: Bearer トークン(秘匿)
	SerialNum string // SERIAL_NUM: デバイスのシリアルナンバー(秘匿)
	BaseURL   string // DOT_BASE_URL: API ベース URL(非秘匿・既定値あり)
}

// Load は環境変数から Config を組み立てる。必須変数(DOT_API_KEY / SERIAL_NUM)が
// 欠落している場合は即エラーを返す(fail-fast)。エラーメッセージには秘匿値を含めない。
func Load() (Config, error) {
	cfg := Config{
		APIKey:    os.Getenv("DOT_API_KEY"),
		SerialNum: os.Getenv("SERIAL_NUM"),
		BaseURL:   os.Getenv("DOT_BASE_URL"),
	}
	if cfg.BaseURL == "" {
		cfg.BaseURL = defaultBaseURL
	}

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
