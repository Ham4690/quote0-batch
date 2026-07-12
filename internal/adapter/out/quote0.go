// Package out は出力側 adapter を提供する。
// quote/0 Text API へコンテンツを送信し、domain.ContentSink を実装する。
package out

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/Ham4690/quote0-batch/internal/config"
	"github.com/Ham4690/quote0-batch/internal/domain"
)

// Quote0Sink は quote/0 Text API への ContentSink 実装。
type Quote0Sink struct {
	cfg    config.Config
	client *http.Client // DI: テストで Transport(RoundTripper)を差し替える
}

// NewQuote0Sink は Quote0Sink を生成する。client が nil の場合は http.DefaultClient を使う。
func NewQuote0Sink(cfg config.Config, client *http.Client) *Quote0Sink {
	if client == nil {
		client = http.DefaultClient
	}
	return &Quote0Sink{cfg: cfg, client: client}
}

// Send は payload を JSON 化し Text API へ POST する。
// 非 2xx はエラーとして返す。エラー文言に API Key / シリアルの平文を含めない。
func (s *Quote0Sink) Send(ctx context.Context, p domain.TextPayload) error {
	body, err := json.Marshal(p)
	if err != nil {
		return fmt.Errorf("quote0: payload の marshal に失敗: %w", err)
	}

	url := fmt.Sprintf("%s/api/authV2/open/device/%s/text", s.cfg.BaseURL, s.cfg.SerialNum)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("quote0: リクエスト生成に失敗: %w", err)
	}
	req.Header.Set("Authorization", "Bearer "+s.cfg.APIKey)
	req.Header.Set("Content-Type", "application/json")

	res, err := s.client.Do(req)
	if err != nil {
		return fmt.Errorf("quote0: リクエスト送信に失敗: %w", err)
	}
	defer func() { _ = res.Body.Close() }()

	if res.StatusCode < 200 || res.StatusCode >= 300 {
		// serialNum / apiKey はエラーに含めずマスク済みのシリアル末尾のみ添える。
		return fmt.Errorf("quote0: Text API failed: status=%d serial=%s", res.StatusCode, s.cfg.MaskedSerial())
	}
	return nil
}
