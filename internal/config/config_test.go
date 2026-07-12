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
			name:       "必須変数が揃えば成功し BaseURL は既定値",
			env:        map[string]string{"DOT_API_KEY": "key123", "SERIAL_NUM": "SN000112345678"},
			wantErr:    false,
			wantBase:   defaultBaseURL,
			wantSerial: "SN000112345678",
		},
		{
			name:     "DOT_BASE_URL 指定時はそれを使う",
			env:      map[string]string{"DOT_API_KEY": "key123", "SERIAL_NUM": "SN1", "DOT_BASE_URL": "https://example.test"},
			wantErr:  false,
			wantBase: "https://example.test",
		},
		{
			name:    "DOT_API_KEY 欠落で fail-fast",
			env:     map[string]string{"SERIAL_NUM": "SN1"},
			wantErr: true,
		},
		{
			name:    "SERIAL_NUM 欠落で fail-fast",
			env:     map[string]string{"DOT_API_KEY": "key123"},
			wantErr: true,
		},
		{
			name:    "両方欠落で fail-fast",
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

func TestLoad_ErrorHidesSecretValues(t *testing.T) {
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
}

func TestMaskedSerial(t *testing.T) {
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
}
