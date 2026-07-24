package config

import (
	"strings"
	"testing"
)

func TestLoad(t *testing.T) {
	tests := []struct {
		name       string
		env        map[string]string
		wantErr    bool
		wantBase   string
		wantSerial string
	}{
		{
			name:       "Load は必須変数が揃うと成功し BaseURL に既定値を設定する",
			env:        map[string]string{"DOT_API_KEY": "key123", "SERIAL_NUM": "SN000112345678"},
			wantErr:    false,
			wantBase:   defaultBaseURL,
			wantSerial: "SN000112345678",
		},
		{
			name:     "Load は DOT_BASE_URL 指定時にその値を BaseURL に使う",
			env:      map[string]string{"DOT_API_KEY": "key123", "SERIAL_NUM": "SN1", "DOT_BASE_URL": "https://example.test"},
			wantErr:  false,
			wantBase: "https://example.test",
		},
		{
			name:     "Load は DOT_BASE_URL 末尾のスラッシュを除去する",
			env:      map[string]string{"DOT_API_KEY": "key123", "SERIAL_NUM": "SN1", "DOT_BASE_URL": "https://example.test/"},
			wantErr:  false,
			wantBase: "https://example.test",
		},
		{
			name:    "Load は DOT_API_KEY が欠落すると起動に失敗する",
			env:     map[string]string{"SERIAL_NUM": "SN1"},
			wantErr: true,
		},
		{
			name:    "Load は SERIAL_NUM が欠落すると起動に失敗する",
			env:     map[string]string{"DOT_API_KEY": "key123"},
			wantErr: true,
		},
		{
			name:    "Load は必須変数が両方欠落すると起動に失敗する",
			env:     map[string]string{},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// 関連 env をクリアしてからテストケースの値を設定する。
			for _, k := range []string{"DOT_API_KEY", "SERIAL_NUM", "DOT_BASE_URL"} {
				t.Setenv(k, "")
			}
			for k, v := range tt.env {
				t.Setenv(k, v)
			}

			cfg, err := Load()
			if tt.wantErr {
				if err == nil {
					t.Fatalf("エラーを期待したが nil")
				}
				return
			}
			if err != nil {
				t.Fatalf("予期せぬエラー: %v", err)
			}
			if tt.wantBase != "" && cfg.BaseURL != tt.wantBase {
				t.Errorf("BaseURL = %q, want %q", cfg.BaseURL, tt.wantBase)
			}
			if tt.wantSerial != "" && cfg.SerialNum != tt.wantSerial {
				t.Errorf("SerialNum = %q, want %q", cfg.SerialNum, tt.wantSerial)
			}
		})
	}
}

func TestLoad_WeatherDefaults(t *testing.T) {
	weatherKeys := []string{"CITY_CODE", "WEATHER_BASE_URL", "WEATHER_LINK_URL"}

	t.Run("Load は天気系の env が未設定なら既定値を設定する", func(t *testing.T) {
		for _, k := range []string{"DOT_API_KEY", "SERIAL_NUM", "DOT_BASE_URL"} {
			t.Setenv(k, "")
		}
		for _, k := range weatherKeys {
			t.Setenv(k, "")
		}
		t.Setenv("DOT_API_KEY", "key123")
		t.Setenv("SERIAL_NUM", "SN1")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if cfg.CityCode != defaultCityCode {
			t.Errorf("CityCode = %q, want %q", cfg.CityCode, defaultCityCode)
		}
		if cfg.WeatherBaseURL != defaultWeatherBaseURL {
			t.Errorf("WeatherBaseURL = %q, want %q", cfg.WeatherBaseURL, defaultWeatherBaseURL)
		}
		if cfg.WeatherLinkURL != "" {
			t.Errorf("WeatherLinkURL = %q, want 空文字(未設定)", cfg.WeatherLinkURL)
		}
	})

	t.Run("Load は天気系の env 指定時にその値を使い WEATHER_BASE_URL 末尾スラッシュを除去する", func(t *testing.T) {
		for _, k := range []string{"DOT_API_KEY", "SERIAL_NUM", "DOT_BASE_URL"} {
			t.Setenv(k, "")
		}
		for _, k := range weatherKeys {
			t.Setenv(k, "")
		}
		t.Setenv("DOT_API_KEY", "key123")
		t.Setenv("SERIAL_NUM", "SN1")
		t.Setenv("CITY_CODE", "270000")
		t.Setenv("WEATHER_BASE_URL", "https://example.test/")
		t.Setenv("WEATHER_LINK_URL", "https://weather.yahoo.co.jp/weather/jp/13/4410.html")

		cfg, err := Load()
		if err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if cfg.CityCode != "270000" {
			t.Errorf("CityCode = %q, want %q", cfg.CityCode, "270000")
		}
		if cfg.WeatherBaseURL != "https://example.test" {
			t.Errorf("WeatherBaseURL = %q, want %q", cfg.WeatherBaseURL, "https://example.test")
		}
		if cfg.WeatherLinkURL != "https://weather.yahoo.co.jp/weather/jp/13/4410.html" {
			t.Errorf("WeatherLinkURL = %q", cfg.WeatherLinkURL)
		}
	})
}

func TestLoad_ErrorHidesSecretValues(t *testing.T) {
	t.Run("Load はエラーメッセージに秘匿値を含めない", func(t *testing.T) {
		for _, k := range []string{"DOT_API_KEY", "SERIAL_NUM", "DOT_BASE_URL"} {
			t.Setenv(k, "")
		}
		t.Setenv("DOT_API_KEY", "supersecretkey")

		_, err := Load()
		if err == nil {
			t.Fatal("エラーを期待したが nil")
		}
		if strings.Contains(err.Error(), "supersecretkey") {
			t.Errorf("エラーメッセージに秘匿値が含まれている: %v", err)
		}
	})
}

func TestMaskedSerial(t *testing.T) {
	t.Run("MaskedSerial は末尾4桁のみ残して他をマスクする", func(t *testing.T) {
		tests := []struct {
			serial string
			want   string
		}{
			{"SN000112345678", "**********5678"},
			{"12345", "*2345"},
			{"1234", "****"},
			{"12", "**"},
			{"", ""},
		}
		for _, tt := range tests {
			c := Config{SerialNum: tt.serial}
			if got := c.MaskedSerial(); got != tt.want {
				t.Errorf("MaskedSerial(%q) = %q, want %q", tt.serial, got, tt.want)
			}
		}
	})
}
