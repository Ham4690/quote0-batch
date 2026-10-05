package domain

// CanvasPayload は quote/0 Canvas API へ送信する表示内容を表す。
// json タグは Canvas API のリクエストボディに対応する。
// taskAlias / data / layoutFull / border は現時点で使わないため型に含めない(YAGNI)。
type CanvasPayload struct {
	WindowData WindowData `json:"windowData"`
	Link       string     `json:"link,omitempty"`
	RefreshNow bool       `json:"refreshNow"` // omitempty 無し: false を明示送信できるようにする
}

// WindowData は Canvas API の画面要素構造。現状は default レイヤーのみ使う。
type WindowData struct {
	Default []CanvasElement `json:"default"`
}

// CanvasElement は windowData.default 配下の1要素。
// Type は "div" | "span" | "img" のいずれか(本実装では img は未使用)。
type CanvasElement struct {
	Type  string             `json:"type"`
	Props CanvasElementProps `json:"props"`
}

// CanvasElementProps は要素の見た目・子要素。
// Children は string(テキスト) または []CanvasElement(入れ子要素)を any で持つ
// (Canvas API 仕様上 children は動的型のため)。
type CanvasElementProps struct {
	TW       string         `json:"tw,omitempty"`
	Style    map[string]any `json:"style,omitempty"`
	Children any            `json:"children,omitempty"`
}
