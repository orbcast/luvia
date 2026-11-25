package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
)

// The room round a picture is the stage's own near-black, as every desktop
// player leaves it: the picture is the only thing on the stage that carries
// information, so anything lit beside it competes with it.
//
// Two attempts put a blurred cover there and both read wrong. At 45% it was a
// second picture, a third of the stage wide on each side of a portrait drama; at
// 10% it was a flat grey wash that made a pale frame look badly lit — measured
// in the running app at luma 74–183 beside a portrait picture, which is not a
// letterbox by any reading.
//
// The case pins the bands to the stage's colour and nothing else, with a poster
// present (the case that used to light them) and loud whites in it.
func TestPlayerBandStaysTheStagesOwnBlack(t *testing.T) {
	theme := design.Theme(ui.DarkTheme())
	stage := design.PlayerStage(&theme)
	bare := [3]uint8{stage.R, stage.G, stage.B}

	// 1280×900 stage, 1280×600 frame: the picture sits from y 150 to 750, so the
	// points below are bands, clear of the capsules at the top line (y 24–66)
	// and of the control bar along the bottom.
	points := [][2]int{{640, 110}, {200, 110}, {1000, 110}, {640, 775}}

	for _, dark := range []bool{true, false} {
		for _, name := range []string{"", "https://cdn.example/poster.png"} {
			shot := stageShotIn(t, name, dark)
			for _, p := range points {
				got := sampleAt(t, shot, p[0], p[1])
				if luma := lumaOf(got); luma > 40 {
					t.Fatalf("band at %v with poster %q in dark=%v is %v (luma %d): a picture beside the picture is not a letterbox", p, name, dark, got, luma)
				}
				if d := delta(got, bare); d > 24 {
					t.Fatalf("band at %v with poster %q in dark=%v is %v, %d from the stage's %v: something is painted over the stage", p, name, dark, got, d, bare)
				}
			}
		}
	}
}

// A portrait picture in a wide stage leaves a band on each side. Both bands must
// read the same as each other: one wash cut to the picture's edges lit them from
// two images that met at a seam, which is what made the picture look cut off.
func TestPlayerBandsOnBothSidesMatch(t *testing.T) {
	for _, dark := range []bool{true, false} {
		shot := stageShotIn(t, "https://cdn.example/poster.png", dark)
		left := sampleAt(t, shot, 60, 450)
		right := sampleAt(t, shot, 1220, 450)
		if delta(left, right) > 12 {
			t.Fatalf("in dark=%v the two bands read %v and %v: they are not the same stage", dark, left, right)
		}
		for _, c := range [][3]uint8{left, right} {
			if luma := lumaOf(c); luma > 40 {
				t.Fatalf("in dark=%v band %v (luma %d) is lit: a letterbox is the stage's own colour", dark, c, luma)
			}
		}
	}
}

// lumaOf is the perceived brightness of a colour in thousandths.
func lumaOf(c [3]uint8) int {
	return (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
}
