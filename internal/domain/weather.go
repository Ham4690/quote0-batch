package domain

import "time"

// Forecast は 1 日分の天気予報を表す API 非依存のドメインモデル。
// 欠損値(気温未発表)は nil で表現する。
type Forecast struct {
	Date         time.Time  // 対象日 0:00 JST
	Telop        string     // 天気概況(例「雨のち曇」)
	TempMinC     *int       // 最低気温(℃)。nil = 欠損
	TempMaxC     *int       // 最高気温(℃)。nil = 欠損
	ChanceOfRain RainChance // 4 時間帯の降水確率
	Link         string     // タップ遷移先 URL
}

// RainChance は 4 時間帯固定の降水確率。未確定帯は "--%" 等がそのまま入る。
type RainChance struct {
	T0006 string // 0-6 時
	T0612 string // 6-12 時
	T1218 string // 12-18 時
	T1824 string // 18-24 時
}
