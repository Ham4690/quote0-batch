// Command batch は quote0-batch のエントリポイント。
// 合成ルート(composition root)として config をロードし adapter を生成して
// RunBatch へ注入する。
package main

import (
	"context"
	"log"

	"github.com/Ham4690/quote0-batch/internal/adapter/in"
	"github.com/Ham4690/quote0-batch/internal/adapter/out"
	"github.com/Ham4690/quote0-batch/internal/application"
	"github.com/Ham4690/quote0-batch/internal/config"
)

func main() {
	ctx := context.Background()

	cfg, err := config.Load() // env 検証、欠落で fail-fast
	if err != nil {
		log.Fatal(err)
	}

	src := in.NewHelloWorldSource()
	sink := out.NewQuote0Sink(cfg, nil)

	if err := application.RunBatch(ctx, src, sink); err != nil {
		log.Fatalf("batch 実行に失敗: %v", err) // 非ゼロ終了 → GitHub Actions が失敗表示
	}
	log.Printf("batch 成功: device=%s へ送信", cfg.MaskedSerial())
}
