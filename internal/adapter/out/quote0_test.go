package out

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"reflect"
	"strings"
	"testing"

	"github.com/Ham4690/quote0-batch/internal/config"
	"github.com/Ham4690/quote0-batch/internal/domain"
)

// roundTripFunc は http.RoundTripper を関数で実装するためのアダプタ。
type roundTripFunc func(req *http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) { return f(req) }

func newResponse(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Body:       io.NopCloser(strings.NewReader(body)),
		Header:     make(http.Header),
	}
}

func testConfig() config.Config {
	return config.Config{
		APIKey:    "secret-api-key",
		SerialNum: "SN000112345678",
		BaseURL:   "https://dot.mindreset.tech",
	}
}

func TestQuote0Sink_Send_Success(t *testing.T) {
	t.Run("Quote0Sink.Send は正しい URL・ヘッダ・body で POST する", func(t *testing.T) {
		var captured *http.Request
		var capturedBody []byte

		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			captured = req
			capturedBody, _ = io.ReadAll(req.Body)
			return newResponse(http.StatusOK, `{"message":"Device SN000112345678 Canvas API content switched."}`), nil
		})}

		sink := NewQuote0Sink(testConfig(), client)
		payload := domain.CanvasPayload{
			WindowData: domain.WindowData{Default: []domain.CanvasElement{
				{Type: "div", Props: domain.CanvasElementProps{
					TW: "flex flex-col w-full h-full justify-between p-4",
					Children: []domain.CanvasElement{
						{Type: "span", Props: domain.CanvasElementProps{
							Style:    map[string]any{"fontSize": float64(28)},
							Children: "Hello World",
						}},
					},
				}},
			}},
			Link:       "https://www.yahoo.co.jp/",
			RefreshNow: true,
		}

		if err := sink.Send(context.Background(), payload); err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}

		// メソッドと URL を検証する。
		if captured.Method != http.MethodPost {
			t.Errorf("Method = %q, want POST", captured.Method)
		}
		wantURL := "https://dot.mindreset.tech/api/authV2/open/device/SN000112345678/canvas"
		if got := captured.URL.String(); got != wantURL {
			t.Errorf("URL = %q, want %q", got, wantURL)
		}

		// Bearer ヘッダを検証する。
		if got := captured.Header.Get("Authorization"); got != "Bearer secret-api-key" {
			t.Errorf("Authorization = %q, want %q", got, "Bearer secret-api-key")
		}
		if got := captured.Header.Get("Content-Type"); got != "application/json" {
			t.Errorf("Content-Type = %q", got)
		}

		// body が payload と一致するか検証する。
		// CanvasElementProps.Children は any のため、型付き struct へ decode すると
		// []domain.CanvasElement には戻らない([]interface{} になる)。
		// 両辺を map[string]any に decode してから比較する。
		wantJSON, err := json.Marshal(payload)
		if err != nil {
			t.Fatalf("payload の marshal に失敗: %v", err)
		}
		var want, got map[string]any
		if err := json.Unmarshal(wantJSON, &want); err != nil {
			t.Fatalf("want の unmarshal に失敗: %v", err)
		}
		if err := json.Unmarshal(capturedBody, &got); err != nil {
			t.Fatalf("送信 body の unmarshal に失敗: %v", err)
		}
		if !reflect.DeepEqual(got, want) {
			t.Errorf("送信 body = %+v, want %+v", got, want)
		}
		// refreshNow が明示的に含まれるか(false でも送れる設計)。
		if !strings.Contains(string(capturedBody), `"refreshNow"`) {
			t.Errorf("body に refreshNow が含まれない: %s", capturedBody)
		}
	})
}

func TestQuote0Sink_Send_Non2xxReturnsError(t *testing.T) {
	t.Run("Quote0Sink.Send は非2xx応答をエラーとして返す", func(t *testing.T) {
		statuses := []int{http.StatusBadRequest, http.StatusForbidden, http.StatusNotFound, http.StatusInternalServerError}
		for _, status := range statuses {
			client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
				return newResponse(status, `{"error":"boom"}`), nil
			})}
			sink := NewQuote0Sink(testConfig(), client)

			err := sink.Send(context.Background(), domain.CanvasPayload{})
			if err == nil {
				t.Errorf("status=%d でエラーを期待したが nil", status)
			}
		}
	})
}

func TestQuote0Sink_Send_ErrorDoesNotLeakSecrets(t *testing.T) {
	t.Run("Quote0Sink.Send はエラー時に API Key とシリアル全体を漏らさない", func(t *testing.T) {
		client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
			return newResponse(http.StatusForbidden, ``), nil
		})}
		sink := NewQuote0Sink(testConfig(), client)

		err := sink.Send(context.Background(), domain.CanvasPayload{})
		if err == nil {
			t.Fatal("エラーを期待したが nil")
		}
		msg := err.Error()
		if strings.Contains(msg, "secret-api-key") {
			t.Errorf("エラーに API Key が漏れている: %v", err)
		}
		if strings.Contains(msg, "SN000112345678") {
			t.Errorf("エラーにシリアル全体が漏れている: %v", err)
		}
		// マスクされた末尾 4 桁のみ許容する。
		if !strings.Contains(msg, "5678") {
			t.Errorf("エラーにマスク済みシリアル末尾が含まれない: %v", err)
		}
	})
}

func TestNewQuote0Sink_NilClientHasTimeout(t *testing.T) {
	t.Run("NewQuote0Sink は client が nil のとき Timeout 付きクライアントを使う", func(t *testing.T) {
		sink := NewQuote0Sink(testConfig(), nil)
		if sink.client == http.DefaultClient {
			t.Error("nil client 時に http.DefaultClient(Timeout 無制限)が使われている")
		}
		if sink.client.Timeout <= 0 {
			t.Errorf("nil client 時の Timeout = %v, 正の値を期待(ハング防止)", sink.client.Timeout)
		}
	})
}
