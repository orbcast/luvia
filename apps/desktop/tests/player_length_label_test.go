package tests

import (
	"testing"
	"time"

	"luvia/apps/desktop/internal/player"
)

// While the engine has not learned the stream's length the total reads as a
// dash. Showing 00:00 there is what made a working stream look like a film
// that cannot start.
func TestLengthLabelMarksAnUnknownLength(t *testing.T) {
	for _, tc := range []struct {
		name  string
		total time.Duration
		want  string
	}{
		{"unknown", 0, "--:--"},
		{"negative", -time.Second, "--:--"},
		{"seconds", 7 * time.Second, "00:07"},
		{"minutes", 48*time.Minute + time.Second, "48:01"},
		{"hours", time.Hour + 2*time.Minute + 3*time.Second, "1:02:03"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := player.LengthLabel(tc.total); got != tc.want {
				t.Fatalf("lengthLabel(%v) = %q, want %q", tc.total, got, tc.want)
			}
		})
	}
}
