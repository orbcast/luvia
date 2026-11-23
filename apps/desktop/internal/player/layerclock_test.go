package player

import (
	"testing"
	"time"
)

// A player that reports its playhead once a second still has to show comments
// moving every frame: the clock carries the sample across the frames between.
func TestLayerClockWalksBetweenSamples(t *testing.T) {
	var c layerClock
	start := time.Unix(0, 0)
	if got := c.read(0, start, 1); got != 0 {
		t.Fatalf("first sample = %v, want the playhead it was given", got)
	}
	// A quarter of a second later the player is still reporting zero, as one
	// that samples on a slow clock does.
	if got, want := c.read(0, start.Add(250*time.Millisecond), 1), 250*time.Millisecond; got != want {
		t.Fatalf("a quarter in = %v, want %v", got, want)
	}
	if got, want := c.read(0, start.Add(500*time.Millisecond), 1), 500*time.Millisecond; got != want {
		t.Fatalf("half in = %v, want %v", got, want)
	}
	// The next sample lands where the clock already walked to, so no frame jumps.
	if got, want := c.read(970*time.Millisecond, start.Add(970*time.Millisecond), 1), 970*time.Millisecond; got != want {
		t.Fatalf("at the sample = %v, want %v", got, want)
	}
}

// A player that reports its playhead a little behind where the clock walked is
// not a jump: reports arrive late, which is not the same as the playhead going
// back.
func TestLayerClockTakesAShortLateReportForTheSameRun(t *testing.T) {
	var c layerClock
	start := time.Unix(0, 0)
	c.read(0, start, 1)
	got := c.read(800*time.Millisecond, start.Add(time.Second), 1)
	if want := time.Second; got != want {
		t.Fatalf("a late sample = %v, want the run's own %v", got, want)
	}
}

// A seek is the playhead landing somewhere the clock could not have walked to,
// and the clock starts again from what the player says rather than snapping the
// comments back over the time they were already given.
func TestLayerClockStartsAgainAfterASeek(t *testing.T) {
	var c layerClock
	start := time.Unix(0, 0)
	c.read(0, start, 1)
	c.read(0, start.Add(2*time.Second), 1)
	at := start.Add(2 * time.Second)
	if got, want := c.read(5*time.Minute, at, 1), 5*time.Minute; got != want {
		t.Fatalf("after a seek = %v, want %v", got, want)
	}
	if got, want := c.read(5*time.Minute, at.Add(100*time.Millisecond), 1), 5*time.Minute+100*time.Millisecond; got != want {
		t.Fatalf("a frame after the seek = %v, want %v", got, want)
	}
}

// At double speed a second of watching carries the layer two seconds of
// picture, and the first report at the new pace starts a run of its own.
func TestLayerClockWalksAtThePlaybackRate(t *testing.T) {
	var c layerClock
	start := time.Unix(0, 0)
	c.read(0, start, 1)
	// The report at the new pace starts the run again from the playhead it
	// gives: the old run was walked at another pace, and carrying it on would
	// move the comments over a stretch the viewer never played.
	if got, want := c.read(0, start.Add(500*time.Millisecond), 2), time.Duration(0); got != want {
		t.Fatalf("the first report at double speed = %v, want %v", got, want)
	}
	// Half a second of watching from there is a second of picture: the run
	// started at the report, which stood at zero.
	if got, want := c.read(time.Second, start.Add(time.Second), 2), time.Second; got != want {
		t.Fatalf("half a second of watching at double speed = %v, want %v", got, want)
	}
	// The report that has caught up with the clock leaves it walking: a slow
	// sampler is not a seek, so nothing steps.
	if got, want := c.read(2*time.Second, start.Add(1500*time.Millisecond), 2), 2*time.Second; got != want {
		t.Fatalf("three quarters of a second of watching = %v, want %v", got, want)
	}
}

// A picture that is not playing holds the layer at the playhead it stopped at:
// a comment waits with the picture rather than sliding on over it.
func TestLayerClockHoldsStillOnAStandingReport(t *testing.T) {
	var c layerClock
	start := time.Unix(0, 0)
	c.read(3*time.Second, start, 1)
	if got := c.read(3*time.Second, start.Add(time.Second), 1); got != 3*time.Second {
		t.Fatalf("a paused second = %v, want the playhead it stopped at", got)
	}
	if got, want := c.read(3*time.Second, start.Add(2*time.Second), 1), 3*time.Second; got != want {
		t.Fatalf("resumed = %v, want %v", got, want)
	}
}
