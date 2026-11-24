package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
)

// TestPlayerStageBackdropWashesTheBandWithoutFillingIt pins the stage behind a
// picture that does not match its shape.
//
// The picture is the only thing on the stage that carries information, so the
// room beside it stays the stage's near-black, as every desktop player leaves
// it: a cover blurred into the bands reads as a second picture competing with
// the first, and on a portrait drama it is a third of the stage wide on each
// side. The cover still warms or cools the whole stage at a strength that
// cannot be mistaken for content — one image over all of it, so there is no
// seam where the picture ends.
//
// The case therefore asserts both halves: the band is never the bare stage
// (the cover reaches it), and never bright enough to be a picture of its own.
func TestPlayerStageBackdropWashesTheBandWithoutFillingIt(t *testing.T) {
	theme := design.Theme(ui.DarkTheme())
	stage := design.PlayerStage(&theme)
	bare := [3]uint8{stage.R, stage.G, stage.B}

	// 1280×900 stage, 1280×600 frame: the picture sits from y 150 to 750, so
	// these points are the bands above and below it, clear of the controls that
	// hang at the top line and along the bottom. The top line is a capsule 42
	// high from y 24 to y 66, so the band above the picture is sampled at y 110
	// — still inside the band (the picture starts at 150) and clear of the pill.
	points := [][2]int{{640, 110}, {200, 110}, {1000, 110}, {640, 775}}

	filled := stageShot(t, "https://cdn.example/poster.png")
	empty := stageShot(t, "")

	for _, p := range points {
		got := sampleAt(t, filled, p[0], p[1])
		// The wash is faint by design: the white artwork behind it lands the
		// band around a tenth of the way off the stage's black. What matters is
		// that it is more than nothing, and far below a picture's brightness.
		if luma := lumaOf(got); luma > 70 {
			t.Fatalf("stage at %v is %v (luma %d): the wash is bright enough to read as a picture", p, got, luma)
		}
		if d := delta(got, bare); d < 4 {
			t.Fatalf("stage at %v is %v, only %d from the bare stage %v: the wash does not reach it", p, got, d, bare)
		}
		// The wash is the cover's, not something the stage drew anyway: with no
		// poster to blur, the same point is the stage's own colour.
		none := sampleAt(t, empty, p[0], p[1])
		if luma := lumaOf(none); luma > 40 {
			t.Fatalf("stage at %v without a poster is %v (luma %d), too bright to be the stage", p, none, luma)
		}
	}
}

// TestPlayerStageKeepsOneWashUnderTheWholeStage guards the seam: the wash is a
// single image over all of the stage, not one per band cut to the picture's
// edges, so a point inside the picture's column and a point beside it are lit
// by the same layer rather than by two images that meet.
func TestPlayerStageKeepsOneWashUnderTheWholeStage(t *testing.T) {
	// A portrait frame in a wide stage leaves a band on each side; both sides
	// sit on the same stage, so the same wash has to reach both.
	shot := stageShot(t, "https://cdn.example/poster.png")
	left := sampleAt(t, shot, 60, 450)
	right := sampleAt(t, shot, 1220, 450)
	if delta(left, right) > 24 {
		t.Fatalf("the two sides of the stage read %v and %v: the wash is cut to the picture, not laid under all of it", left, right)
	}
}

// lumaOf is the perceived brightness of a colour in thousandths.
func lumaOf(c [3]uint8) int {
	return (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
}
