package main

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// view 不需要窗口就能跑：Tester 会像用户一样点击、输入、按键。
func newTester(t *testing.T) (*app, *ui.Tester) {
	t.Helper()
	a := &app{}
	return a, ui.NewTester(a.view, 560, 520)
}

func TestAddByEnter(t *testing.T) {
	a, tt := newTester(t)
	if err := tt.Click("新任务"); err != nil {
		t.Fatal(err)
	}
	tt.Type("写模板文档")
	tt.Key(0, ui.KeyEnter)
	if len(a.items) != 1 || a.items[0].title != "写模板文档" {
		t.Fatalf("items %#v after Enter", a.items)
	}
	if a.draft != "" {
		t.Errorf("draft %q after submit, want empty", a.draft)
	}
	if !tt.HasText("写模板文档") || !tt.HasText("共 1 项 · 未完成 1") {
		t.Errorf("texts %q after adding", tt.Texts())
	}
}

func TestAddIgnoresBlankInput(t *testing.T) {
	a, tt := newTester(t)
	tt.Type("   ")
	if err := tt.Click("添加"); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 0 {
		t.Fatalf("items %#v after blank input", a.items)
	}
	if !tt.HasText("还没有任务。") {
		t.Errorf("texts %q without items", tt.Texts())
	}
}

func TestCheckAndClearDone(t *testing.T) {
	a, tt := newTester(t)
	a.items = []item{{id: 1, title: "先做这个"}, {id: 2, title: "再做那个"}}
	a.next = 2
	tt.Frame()
	if err := tt.Click("先做这个"); err != nil {
		t.Fatal(err)
	}
	if !a.items[0].done || a.items[1].done {
		t.Fatalf("items %#v after checking the first", a.items)
	}
	if !tt.HasText("未完成 1") {
		t.Errorf("texts %q after checking one item", tt.Texts())
	}
	if err := tt.Click("清除已完成"); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 1 || a.items[0].title != "再做那个" {
		t.Fatalf("items %#v after clearing", a.items)
	}
}

func TestRemoveKeepsTheRestInOrder(t *testing.T) {
	a, tt := newTester(t)
	a.items = []item{{id: 1, title: "甲"}, {id: 2, title: "乙"}, {id: 3, title: "丙"}}
	a.next = 3
	tt.Frame()
	if err := tt.Click("删除"); err != nil {
		t.Fatal(err)
	}
	if len(a.items) != 2 || a.items[0].title != "乙" || a.items[1].title != "丙" {
		t.Fatalf("items %#v after removing the first", a.items)
	}
	if !tt.HasText("共 2 项 · 未完成 2") {
		t.Errorf("texts %q after removing", tt.Texts())
	}
}
