package tests

import (
	"image"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
)

// The stage's top line is one row: the way out at the left and the controls'
// capsule at the right share a top and a height, so the two read as the ends of
// one line. Before that they were two independent numbers — a 34px round button
// at one top, a capsule whose height followed its contents at another — which
// left the corner a couple of pixels low and half the height of the cluster.
//
// The case reads the row off the pixels: the corner's capsule is the patch of
// glass at the stage's left margin, the cluster is the patch at its right. Text
// cannot tell them apart — the fallback notice also says "Close" — and the
// question here is the shape of the row, not which control is which.
func TestPlayerTopLineIsOneRow(t *testing.T) {
	frame := waitingStageShot(t, 1280, 900)

	// Scan down a column through each end of the row and take the first and
	// last lit rows: that is the capsule's own top and bottom, which is what
	// "one row" has to mean. Sampling only the middle would let one end slide
	// up or down and still pass.
	// Only the top of the stage is the top line: the column through each end
	// runs on down to the controls along the bottom, which are lit too.
	leftTop, leftBottom, ok := litRows(frame, 20+21, 0, 120)
	if !ok {
		t.Fatal("the corner carries no glass: the way out is not on the stage")
	}
	rightTop, rightBottom, ok := litRows(frame, 1280-20-21, 0, 120)
	if !ok {
		t.Fatal("the cluster carries no glass: the row does not reach it")
	}

	if leftTop != rightTop {
		t.Fatalf("the two ends of the row start at y=%d and y=%d: one line, two tops", leftTop, rightTop)
	}
	if leftBottom != rightBottom {
		t.Fatalf("the two ends of the row end at y=%d and y=%d: one line, two bottoms", leftBottom, rightBottom)
	}
	// And the row is one capsule tall, not a thin bar: stageTopBox is 42.
	if got := leftBottom - leftTop + 1; got != 42 {
		t.Fatalf("the row is %d high, want 42: the ends are not the same capsule", got)
	}
}

// The corner's capsule is as tall as the row it sits on, not a smaller square
// floating in it: a 34px circle at the top edge of a 42px row reads as off by a
// couple of pixels even when the two tops agree. It is also a circle, so the
// lit rows narrow towards its top and bottom rather than staying full width.
func TestPlayerWayOutIsAsTallAsTheCluster(t *testing.T) {
	frame := waitingStageShot(t, 1280, 900)
	const top, box = 24, 34 + 2*4

	for _, x := range []int{20, 1280 - 20 - box} {
		middle := x + box/2
		if !isGlass(frame, middle, top) {
			t.Fatalf("the capsule at x=%d is not glass at its top edge y=%d", x, top)
		}
		if !isGlass(frame, middle, top+box-2) {
			t.Fatalf("the capsule at x=%d stops before its bottom edge y=%d", x, top+box)
		}
		if isGlass(frame, middle, top-3) {
			t.Fatalf("the capsule at x=%d reaches above its top edge y=%d", x, top)
		}
	}
}

// litRows returns the first and last row of the column x that is lit brighter
// than the stage: a capsule's glass sits on the stage and lets the picture
// under it through, while the stage round it does not. The threshold sits
// between the two, so the case does not need the material's exact colour.
func litRows(frame image.Image, x, from, to int) (top, bottom int, ok bool) {
	top, bottom = -1, -1
	if to > frame.Bounds().Dy() {
		to = frame.Bounds().Dy()
	}
	for y := from; y < to; y++ {
		if !isGlass(frame, x, y) {
			continue
		}
		if top < 0 {
			top = y
		}
		bottom = y
	}
	return top, bottom, top >= 0
}

// isGlass reports whether a point is lit brighter than the stage's near-black.
func isGlass(frame image.Image, x, y int) bool {
	if x < 0 || y < 0 || x >= frame.Bounds().Dx() || y >= frame.Bounds().Dy() {
		return false
	}
	r, g, b, _ := frame.At(x, y).RGBA()
	luma := (299*int(r>>8) + 587*int(g>>8) + 114*int(b>>8)) / 1000
	return luma > 34
}

// waitingStageShot draws the player on a stream that never opens, so the top
// line is the frame a viewer sees while the stage waits — the frame they have
// to be able to leave from.
func waitingStageShot(t *testing.T, width, height int) image.Image {
	t.Helper()
	src, _ := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	page := player.New(player.Deps{
		Source: src, Library: store,
		Run:       func(fn func()) { fn() },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { select {} },
	})
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, width, height)
	tt.SetLocale("en")
	page.Open(src.detail, src.episodes, 0)
	tt.Frame()
	frame := tt.Image()
	if frame == nil {
		t.Fatal("no image")
	}
	return frame
}
