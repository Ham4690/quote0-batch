// Package domain は外部依存を持たない純粋な型と port(interface)を定義する。
// ビジネス語彙の中心であり、adapter / application はここに依存する。
package domain

import "context"

// TextPayload は quote/0 Text API へ送信するテキスト表示内容を表す。
// json タグは Text API のリクエストボディに対応する。
type TextPayload struct {
	Title      string `json:"title"`
	Message    string `json:"message"`
	Signature  string `json:"signature,omitempty"`
	Link       string `json:"link,omitempty"`
	RefreshNow bool   `json:"refreshNow"` // omitempty 無し: false を明示送信できるようにする
}

// ContentSource は表示するコンテンツを組み立てる入力側の port。
// 例: HelloWorldSource(PoC) / 天気 API source(M3)。
type ContentSource interface {
	Build(ctx context.Context) (TextPayload, error)
}

// ContentSink は組み立てたコンテンツを表示先へ送信する出力側の port。
// 例: quote/0 Text API への送信。
type ContentSink interface {
	Send(ctx context.Context, p TextPayload) error
}
