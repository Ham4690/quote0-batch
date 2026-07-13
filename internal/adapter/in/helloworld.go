// Package in は入力側 adapter(コンテンツ取得)を提供する。
// domain.ContentSource を実装する。
package in

import (
	"context"
	"time"

	// tzdata をバイナリへ埋め込む(約 +450KB)。OS の tzdata に依存しないため、
	// 将来 scratch/distroless コンテナへ移しても LoadLocation が失敗しない。
	_ "time/tzdata"

	"github.com/Ham4690/quote0-batch/internal/domain"
)

// jst は起動時に 1 度だけ解決する。Build 毎の LoadLocation を避ける。
// runner の TZ 設定に依存せず確実に JST 化するため、明示的にロケーションを取得する。
var jst = mustLoadJST()

func mustLoadJST() *time.Location {
	loc, err := time.LoadLocation("Asia/Tokyo")
	if err != nil {
		panic("in: Asia/Tokyo ロケーションのロードに失敗: " + err.Error())
	}
	return loc
}

// HelloWorldSource は PoC 用の ContentSource 実装。
// 天気 API を叩かず固定文言 "Hello World" を組み立てる。
type HelloWorldSource struct {
	now func() time.Time // DI: テストで時刻を固定する
}

// ContentSource 充足のコンパイル時表明。シグネチャがズレたらここでビルドが落ちる。
var _ domain.ContentSource = (*HelloWorldSource)(nil)

// NewHelloWorldSource は現在時刻に time.Now を用いる HelloWorldSource を生成する。
func NewHelloWorldSource() *HelloWorldSource {
	return &HelloWorldSource{now: time.Now}
}

// Build は固定の Hello World ペイロードを組み立てる。
// signature の日時は JST で生成する。
func (s *HelloWorldSource) Build(ctx context.Context) (domain.TextPayload, error) {
	sig := s.now().In(jst).Format("2006年01月02日15:04")
	return domain.TextPayload{
		RefreshNow: true,
		Title:      "Hello World",
		Message:    "Hello\nWorld",
		Signature:  sig,
		Link:       "https://www.yahoo.co.jp/",
	}, nil
}
