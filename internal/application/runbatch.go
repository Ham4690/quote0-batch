// Package application はユースケースを提供する。port(interface)経由で
// 入力(ContentSource)から出力(ContentSink)へコンテンツを受け渡すだけで、
// 具体的な adapter 実装を知らない。
package application

import (
	"context"

	"github.com/Ham4690/quote0-batch/internal/domain"
)

// RunBatch は source でコンテンツを組み立て、sink で表示先へ送信する。
// バッチ 1 回分の実行に相当する。
func RunBatch(ctx context.Context, src domain.ContentSource, sink domain.ContentSink) error {
	p, err := src.Build(ctx)
	if err != nil {
		return err
	}
	return sink.Send(ctx, p)
}
