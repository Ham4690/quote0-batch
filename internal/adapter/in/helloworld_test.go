package in

import (
	"context"
	"testing"
	"time"
)

func TestHelloWorldSource_Build(t *testing.T) {
	// 2026-07-12 09:00 JST を固定時刻として与える。
	fixed := time.Date(2026, 7, 12, 9, 0, 0, 0, jst)
	src := &HelloWorldSource{now: func() time.Time { return fixed }}

	p, err := src.Build(context.Background())
	if err != nil {
		t.Fatalf("予期せぬエラー: %v", err)
	}

	if p.Title != "Hello World" {
		t.Errorf("Title = %q, want %q", p.Title, "Hello World")
	}
	if p.Message != "Hello\nWorld" {
		t.Errorf("Message = %q, want %q", p.Message, "Hello\nWorld")
	}
	if !p.RefreshNow {
		t.Error("RefreshNow = false, want true")
	}
	if want := "2026年07月12日09:00"; p.Signature != want {
		t.Errorf("Signature = %q, want %q", p.Signature, want)
	}
	if p.Link != "https://www.yahoo.co.jp/" {
		t.Errorf("Link = %q", p.Link)
	}
}

func TestHelloWorldSource_BuildConvertsToJST(t *testing.T) {
	// UTC で 2026-07-12 00:00 は JST では同日 09:00。
	utcMidnight := time.Date(2026, 7, 12, 0, 0, 0, 0, time.UTC)
	src := &HelloWorldSource{now: func() time.Time { return utcMidnight }}

	p, err := src.Build(context.Background())
	if err != nil {
		t.Fatalf("予期せぬエラー: %v", err)
	}
	if want := "2026年07月12日09:00"; p.Signature != want {
		t.Errorf("Signature = %q, want %q(JST 変換されていない)", p.Signature, want)
	}
}
