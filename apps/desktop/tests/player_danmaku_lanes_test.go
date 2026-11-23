package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// lanePage builds a player over a burst of comments that all arrive at once, so
// the layer has more lines to seat than rows to seat them in.
func lanePage(t *testing.T, lines int) (*player.Page, chan func(), *ui.Tester) {
	t.Helper()
	base, _ := sampleTitle()
	comments := make([]source.Danmaku, 0, lines)
	for i := 0; i < lines; i++ {
		comments = append(comments, source.Danmaku{At: time.Duration(i) * time.Millisecond, Text: fmt.Sprintf("burst %02d", i)})
	}
	src := &commentingSource{detail: base.detail, episodes: base.episodes, comments: comments}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 16)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	apply(t, run)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	return page, run, tt
}

// TestDanmakuScrollsNeverShareARow covers the busy case: a burst of comments
// arrives while the layer has fewer rows than lines, and no two scrolling lines
// may share a row. Two lines on one row is what makes a packed episode
// unreadable, and the layer drops what it cannot seat instead.
//
// Every fifth comment is pinned, and pinned lines hang in the middle of the band
// on purpose, so the check looks at the scrolling ones only: their rows must all
// differ.
func TestDanmakuScrollsNeverShareARow(t *testing.T) {
	const lines = 24
	page, run, tt := lanePage(t, lines)
	showDanmaku(t, page, tt)
	if !pumpUntil(t, run, tt, "burst 00") {
		t.Fatalf("the first line of the burst is not on the picture: %v", tt.Texts())
	}
	place := map[string]ui.Rect{}
	for _, item := range tt.AccessAll() {
		if len(item.Name) == 8 && item.Name[:5] == "burst" {
			place[item.Name] = item.Bounds
		}
	}
	if len(place) < 2 {
		t.Fatalf("the burst put %d lines on the picture: %v", len(place), place)
	}
	rowOf := map[float32]string{}
	scrolled := 0
	for i := 0; i < lines; i++ {
		if i%5 == 2 || i%5 == 4 {
			continue
		}
		name := fmt.Sprintf("burst %02d", i)
		box, ok := place[name]
		if !ok {
			continue
		}
		scrolled++
		if other, taken := rowOf[box.Y]; taken {
			t.Fatalf("%q and %q share the row at y=%.0f", other, name, box.Y)
		}
		rowOf[box.Y] = name
	}
	if scrolled < 2 {
		t.Fatalf("the burst put %d scrolling lines on the picture: %v", scrolled, place)
	}
}
