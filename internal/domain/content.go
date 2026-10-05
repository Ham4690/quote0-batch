// Package domain は外部依存を持たない純粋な型と port(interface)を定義する。
// ビジネス語彙の中心であり、adapter / application はここに依存する。
package domain

import "context"

// ContentSource は表示するコンテンツを組み立てる入力側の port。
// 例: HelloWorldSource(PoC) / 天気 API source(M3)。
type ContentSource interface {
	Build(ctx context.Context) (CanvasPayload, error)
}

// ContentSink は組み立てたコンテンツを表示先へ送信する出力側の port。
// 例: quote/0 Canvas API への送信。
type ContentSink interface {
	Send(ctx context.Context, p CanvasPayload) error
}
