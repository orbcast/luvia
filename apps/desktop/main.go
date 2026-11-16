package main

import (
	"log"
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// item 是一条待办。界面状态全部放在 app 上，view 每次只负责把它画出来。
type item struct {
	id    int
	title string
	done  bool
}

// app 是窗口显示的全部状态。字段直接绑定给控件，Go 代码可以直接读写。
type app struct {
	items []item
	draft string
	next  int
	input ui.Handle
}

// add 把输入框里的文字加为待办，空白输入直接忽略。
func (a *app) add() {
	title := strings.TrimSpace(a.draft)
	if title == "" {
		return
	}
	a.next++
	a.items = append(a.items, item{id: a.next, title: title})
	a.draft = ""
}

// remove 按下标删除一条待办。
func (a *app) remove(i int) {
	a.items = append(a.items[:i], a.items[i+1:]...)
}

// clearDone 删除所有已完成的待办。
func (a *app) clearDone() {
	kept := a.items[:0]
	for _, it := range a.items {
		if !it.done {
			kept = append(kept, it)
		}
	}
	a.items = kept
}

// open 返回未完成的数量。
func (a *app) open() int {
	n := 0
	for _, it := range a.items {
		if !it.done {
			n++
		}
	}
	return n
}

// view 是 MyGo 每帧调用的界面函数：只读 a，构造控件，处理本帧的输入。
func (a *app) view(c *ui.Context) {
	t := c.Theme()
	ui.Column(c).Fill().Padding(24).Gap(12).Children(func() {
		ui.Text(c, "Luvia").FontSize(24).Bold()
		ui.Textf(c, "共 %d 项 · 未完成 %d", len(a.items), a.open()).
			FontSize(13).TextColor(t.TextMuted)

		ui.TextInput(c, &a.draft).
			Placeholder("写点什么，按 Enter 添加").
			Label("新任务").
			Bind(&a.input).
			OnSubmit(a.add)

		ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
			if ui.PrimaryButton(c, "添加").Clicked() {
				a.add()
			}
			if ui.Button(c, "清除已完成").Clicked() {
				a.clearDone()
			}
		})

		ui.Divider(c)

		if len(a.items) == 0 {
			ui.Text(c, "还没有任务。").
				FontSize(13).
				TextColor(t.TextMuted)
			return
		}
		// 删除要等这一帧构建完再做，否则遍历中的下标会错位。
		toRemove := -1
		for i := range a.items {
			it := &a.items[i]
			ui.Row(c).Key(it.id).Gap(10).AlignItems(ui.Center).Children(func() {
				ui.Checkbox(c, &it.done, it.title)
				ui.Spacer(c)
				if ui.Button(c, "删除").Clicked() {
					toRemove = i
				}
			})
		}
		if toRemove >= 0 {
			a.remove(toRemove)
		}
	})
}

func main() {
	a := &app{}
	mygo.App.WhenReady(func() {
		mygo.NewWindow(mygo.WindowOptions{
			Title:     "Luvia",
			Width:     560,
			Height:    520,
			MinWidth:  360,
			MinHeight: 320,
			// 记住上一次窗口位置。
			StateKey: "main",
			// 窗口显示 MyGo 自己绘制的原生界面，不加载网页。
			Content: ui.View(a.view),
		})
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
