package weather

import (
	"encoding/json"
	"testing"
)

func TestApiResponseLocation(t *testing.T) {
	t.Run("apiResponse は location.city を decode する", func(t *testing.T) {
		r := loadFixture(t, "tokyo.json")
		if r.Location.City != "東京" {
			t.Errorf("Location.City = %q, want %q", r.Location.City, "東京")
		}
	})

	t.Run("location 欠損レスポンスは City が空になる", func(t *testing.T) {
		var r apiResponse
		// location キーを含まない最小レスポンス。
		body := `{"forecasts":[{"date":"2026-07-24","dateLabel":"今日","telop":"晴れ"}],"link":"x"}`
		if err := json.Unmarshal([]byte(body), &r); err != nil {
			t.Fatalf("decode に失敗: %v", err)
		}
		if r.Location.City != "" {
			t.Errorf("Location.City = %q, want 空文字", r.Location.City)
		}
	})
}
