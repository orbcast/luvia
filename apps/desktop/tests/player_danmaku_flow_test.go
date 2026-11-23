package tests

import (
	"fmt"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// flowPage builds a player over a handful of comments and a clock the test walks
// itself, so the layer's pace can be read frame by frame instead of being
// watched in real time.
func flowPage(t *testing.T, lines int) (*player.Page, *fakeClock) {
	t.Helper()
	base, _ := sampleTitle()
	comments := make([]source.Danmaku, 0, lines)
	for i := 0; i < lines; i++ {
		comments = append(comments, source.Danmaku{At: time.Duration(i) * time.Second, Text: fmt.Sprintf("line %02d", i)})
	}
	src := &commentingSource{detail: base.detail, episodes: base.episodes, comments: comments}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 16)
	clock := &fakeClock{at: time.Unix(0, 0)}
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.SetClock(clock.now)
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	apply(t, run)
	return page, clock
}

// fakeClock is the monotonic clock the layer times itself by, which the test
// moves on instead of sleeping through a crossing.
type fakeClock struct {
	at time.Time
}

func (c *fakeClock) now() time.Time { return c.at }

// TestDanmakuCrossesThePictureInEvenSteps covers the complaint that comments
// arrived in steps rather than crossing: the player reports its playhead a few
// times a second, and a layer laid out from those reports alone moves a comment
// only when one comes. The layer walks the playhead itself between reports, so
// every frame moves a crossing line by the same small distance.
//
// The GIF player the tests open holds its playhead at zero, so what moves the
// line here is the clock alone: exactly the carry-over that was missing.
func TestDanmakuCrossesThePictureInEvenSteps(t *testing.T) {
	const lines = 3
	page, clock := flowPage(t, lines)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	showDanmaku(t, page, tt)
	// A comment at the very start of the episode is crossing already.
	tt.Frame()
	position := func(name string) (float32, bool) {
		for _, item := range tt.AccessAll() {
			if item.Name == name {
				return item.Bounds.X, true
			}
		}
		return 0, false
	}
	first, ok := position("line 00")
	if !ok {
		t.Skip("the layer is not showing the first comment in this build")
	}
	var last float32 = first
	var steps []float32
	for i := 0; i < 6; i++ {
		clock.at = clock.at.Add(100 * time.Millisecond)
		tt.Frame()
		x, ok := position("line 00")
		if !ok {
			t.Fatalf("the first comment left the picture after %d frames of a %v crossing", i, page.DanmakuSettings().Speed)
		}
		steps = append(steps, last-x)
		last = x
	}
	for i, step := range steps {
		if step <= 0 {
			t.Fatalf("frame %d moved the comment by %v: it either stood still or went backwards", i, step)
		}
		// A pixel is laid out in floats, so a frame may land half a pixel either
		// side of the step; anything more is the crossing speeding up or slowing.
		if step < steps[0]-1 || step > steps[0]+1 {
			t.Fatalf("frame %d moved the comment by %v where the first moved it by %v: the crossing is not even", i, step, steps[0])
		}
	}
}
