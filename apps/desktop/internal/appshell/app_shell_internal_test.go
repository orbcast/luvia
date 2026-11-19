package appshell

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestWindowControlSquareFrames covers the maximize figure: the pair of
// squares a title bar draws while the window restores, one while it is
// maximized, both on the button's middle and inside its box.
func TestWindowControlSquareFrames(t *testing.T) {
	button := ui.Rect{X: 100, Y: 50, W: 38, H: 32}

	pair := windowControlSquareFrames(button, false)
	if len(pair) != 2 {
		t.Fatalf("a restoring window draws %d squares, not 2", len(pair))
	}
	if pair[0] == pair[1] {
		t.Fatal("the pair of squares sits on one box, not stepped apart")
	}
	if pair[0].X <= pair[1].X || pair[0].Y <= pair[1].Y {
		t.Fatalf("the far square %v does not step behind the near one %v", pair[0], pair[1])
	}
	for _, box := range pair {
		if box.W != box.H {
			t.Fatalf("the square %v is not square", box)
		}
		if box.X < button.X || box.Y < button.Y || box.X+box.W > button.X+button.W || box.Y+box.H > button.Y+button.H {
			t.Fatalf("the square %v leaves the button %v", box, button)
		}
	}

	single := windowControlSquareFrames(button, true)
	if len(single) != 1 {
		t.Fatalf("a maximized window draws %d squares, not 1", len(single))
	}
	if single[0].W != pair[0].W {
		t.Fatalf("the maximized square is %v wide, not the pair's %v", single[0].W, pair[0].W)
	}
	// It sits inside the pair's middle: the near square moved by half the
	// step the far one takes, so the figure stays where the pair centers it.
	if single[0].X < pair[1].X || single[0].X > pair[0].X || single[0].Y < pair[1].Y || single[0].Y > pair[0].Y {
		t.Fatalf("the maximized square %v does not sit between the pair's %v and %v", single[0], pair[1], pair[0])
	}
}
