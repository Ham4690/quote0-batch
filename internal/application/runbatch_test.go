package application

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/Ham4690/quote0-batch/internal/domain"
)

// fakeSource は ContentSource のテスト用フェイク。
type fakeSource struct {
	payload domain.CanvasPayload
	err     error
	called  bool
}

func (f *fakeSource) Build(ctx context.Context) (domain.CanvasPayload, error) {
	f.called = true
	return f.payload, f.err
}

// fakeSink は ContentSink のテスト用フェイク。受け取った payload を記録する。
type fakeSink struct {
	got    domain.CanvasPayload
	called bool
	err    error
}

func (f *fakeSink) Send(ctx context.Context, p domain.CanvasPayload) error {
	f.called = true
	f.got = p
	return f.err
}

func TestRunBatch_PassesPayloadFromSourceToSink(t *testing.T) {
	t.Run("RunBatch は source が生成した payload を sink に渡す", func(t *testing.T) {
		want := domain.CanvasPayload{RefreshNow: true}
		src := &fakeSource{payload: want}
		sink := &fakeSink{}

		if err := RunBatch(context.Background(), src, sink); err != nil {
			t.Fatalf("予期せぬエラー: %v", err)
		}
		if !src.called {
			t.Error("source.Build が呼ばれていない")
		}
		if !sink.called {
			t.Error("sink.Send が呼ばれていない")
		}
		if !reflect.DeepEqual(sink.got, want) {
			t.Errorf("sink が受け取った payload = %+v, want %+v", sink.got, want)
		}
	})
}

func TestRunBatch_SourceErrorStopsBeforeSink(t *testing.T) {
	t.Run("RunBatch は source がエラーのとき sink を呼ばずに中断する", func(t *testing.T) {
		srcErr := errors.New("build failed")
		src := &fakeSource{err: srcErr}
		sink := &fakeSink{}

		err := RunBatch(context.Background(), src, sink)
		if !errors.Is(err, srcErr) {
			t.Errorf("source のエラーが伝播していない: %v", err)
		}
		if sink.called {
			t.Error("source が失敗したのに sink.Send が呼ばれた")
		}
	})
}

func TestRunBatch_SinkErrorPropagates(t *testing.T) {
	t.Run("RunBatch は sink のエラーを呼び出し元へ伝播する", func(t *testing.T) {
		sinkErr := errors.New("send failed")
		src := &fakeSource{payload: domain.CanvasPayload{}}
		sink := &fakeSink{err: sinkErr}

		err := RunBatch(context.Background(), src, sink)
		if !errors.Is(err, sinkErr) {
			t.Errorf("sink のエラーが伝播していない: %v", err)
		}
	})
}
