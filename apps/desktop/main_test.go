package main

import (
	"testing"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/locales"
)

// en says the app's messages in the source language, which tests read
// without a window or a system language.
var en = locales.In("en")

// view 不需要窗口就能跑：Tester 会像用户一样点击、输入、按键。
func newTester(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := &app{}
	return a, ui.NewTester(a.view, 560, 520)
}

func TestAddByEnter(t *testing.T) {
	a, tt := newTester(t)
	if err := tt.Click(en.NewTaskLabel()); err != nil {
		t.Fatal(err)
	}
	tt.Type("Write the template docs")
	tt.Key(0, ui.KeyEnter)
	if len(a.items) != 1 || a.items[0].title != "Write the template docs" {
		t.Fatalf("items %#v after Enter", a.items)
	}
	if a.draft != "" {
		t.Errorf("draft %q after submit, want empty", a.draft)
	}
	if !tt.HasText("Write the template docs") || !tt.HasText(en.Summary(1, 1)) {
		t.Errorf("texts %q after adding", tt.Texts())
	}
}

func TestAddIgnoresBlankInput(t *testing.T) {
	a, tt := newTester(t)
	tt.Type("   ")
	if err := tt.Click(en.Add()); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 0 {
		t.Fatalf("items %#v after blank input", a.items)
	}
	if !tt.HasText(en.Empty()) {
		t.Errorf("texts %q without items", tt.Texts())
	}
}

func TestCheckAndClearDone(t *testing.T) {
	a, tt := newTester(t)
	a.items = []item{{id: 1, title: "Do this first"}, {id: 2, title: "Then that"}}
	a.next = 2
	tt.Frame()
	if err := tt.Click("Do this first"); err != nil {
		t.Fatal(err)
	}
	if !a.items[0].done || a.items[1].done {
		t.Fatalf("items %#v after checking the first", a.items)
	}
	if !tt.HasText(en.Summary(2, 1)) {
		t.Errorf("texts %q after checking one item", tt.Texts())
	}
	if err := tt.Click(en.ClearDone()); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 1 || a.items[0].title != "Then that" {
		t.Fatalf("items %#v after clearing", a.items)
	}
}

func TestRemoveKeepsTheRestInOrder(t *testing.T) {
	a, tt := newTester(t)
	a.items = []item{{id: 1, title: "First"}, {id: 2, title: "Second"}, {id: 3, title: "Third"}}
	a.next = 3
	tt.Frame()
	if err := tt.Click(en.Delete()); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 2 || a.items[0].title != "Second" || a.items[1].title != "Third" {
		t.Fatalf("items %#v after removing the first", a.items)
	}
	if !tt.HasText(en.Summary(2, 2)) {
		t.Errorf("texts %q after removing", tt.Texts())
	}
}

// TestMessagesFollowTheLanguage 验证界面文案取自当前语言，而不是写死的字面量。
func TestMessagesFollowTheLanguage(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocaleFunc(nil) })
	i18n.SetLocaleFunc(func() string { return "zh-Hans" })

	a, tt := newTester(t)
	a.items = []item{{id: 1, title: "Buy milk"}}
	a.next = 1
	tt.Frame()

	zh := locales.In("zh-Hans")
	for _, want := range []string{zh.Add(), zh.ClearDone(), zh.Summary(1, 1), zh.Delete(), zh.NewTaskLabel()} {
		if !tt.HasText(want) {
			t.Errorf("texts %q missing %q", tt.Texts(), want)
		}
	}
	if zh.Add() == en.Add() {
		t.Errorf("zh-Hans and en say the same %q; the translation is missing", zh.Add())
	}
}
