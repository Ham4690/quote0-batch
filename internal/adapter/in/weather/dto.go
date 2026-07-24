// Package weather は天気 API(weather.tsukumijima.net / livedoor 互換)からの
// 入力側 adapter を提供する。domain.ContentSource を実装する。
package weather

import "errors"

// apiResponse は天気 API の生レスポンスを素直に写した External DTO。
// null / "--%" 等の欠損を受けられるよう気温は文字列ポインタで受ける。
type apiResponse struct {
	Forecasts []forecast `json:"forecasts"`
	Link      string     `json:"link"`
}

type forecast struct {
	Date         string       `json:"date"`      // "2026-07-24"
	DateLabel    string       `json:"dateLabel"` // "今日"
	Telop        string       `json:"telop"`     // "雨のち曇"
	Temperature  temperature  `json:"temperature"`
	ChanceOfRain chanceOfRain `json:"chanceOfRain"`
}

type temperature struct {
	Min tempValue `json:"min"`
	Max tempValue `json:"max"`
}

// tempValue の celsius は文字列(例 "26")または null。
type tempValue struct {
	Celsius *string `json:"celsius"`
}

// chanceOfRain は 4 時間帯固定キー。未確定帯は "--%"。
type chanceOfRain struct {
	T00_06 string `json:"T00_06"`
	T06_12 string `json:"T06_12"`
	T12_18 string `json:"T12_18"`
	T18_24 string `json:"T18_24"`
}

// Validate は decode 後の構造異常を早期検出する。
// 当日エントリの存在・telop 非空は選択ロジック側で確認する。
func (r apiResponse) Validate() error {
	if len(r.Forecasts) == 0 {
		return errors.New("weather: forecasts が空")
	}
	return nil
}
