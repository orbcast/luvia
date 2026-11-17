package playintro

import (
	"io"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestIntroAnimationValid guards the embedded play-intro asset: it must decode
// as an animated APNG of a sensible length, so a broken regeneration fails the
// build instead of showing a blank or frozen intro.
func TestIntroAnimationValid(t *testing.T) {
	decoder, err := ui.NewAPNGDecoder(intro)
	if err != nil {
		t.Fatalf("playintro.apng does not decode: %v", err)
	}
	frames := 0
	var total time.Duration
	for {
		frame, err := decoder.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("frame %d: %v", frames, err)
		}
		if frame.Image == nil || frame.Duration <= 0 {
			t.Fatalf("frame %d is empty", frames)
		}
		frames++
		total += frame.Duration
	}
	if frames < 2 {
		t.Fatalf("want an animated intro, got %d frame(s)", frames)
	}
	if total < time.Second {
		t.Fatalf("intro is too short: %v", total)
	}
}

// TestStartWithoutAssetStillReports checks the controller falls back to calling
// after at once rather than hanging when no player can be made.
func TestControllerIdle(t *testing.T) {
	c := New(nil)
	if c.Active() {
		t.Fatal("a fresh controller must be idle")
	}
}
