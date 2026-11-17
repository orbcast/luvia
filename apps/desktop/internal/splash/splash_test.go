package splash

import (
	"io"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

// TestOpeningAnimationValid guards the embedded opening asset: it must decode
// as an animated APNG of a sensible length, so a broken regeneration fails the
// build instead of showing a blank or frozen splash.
func TestOpeningAnimationValid(t *testing.T) {
	decoder, err := ui.NewAPNGDecoder(opening)
	if err != nil {
		t.Fatalf("opening.apng does not decode: %v", err)
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
		t.Fatalf("want an animated opening, got %d frame(s)", frames)
	}
	if total < time.Second {
		t.Fatalf("opening is too short: %v", total)
	}
}
