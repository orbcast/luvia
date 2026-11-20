package player

import "time"

// LengthLabel is the total-time reading the controls show: a dash while the
// engine has not learned the length, otherwise the media time.
func LengthLabel(total time.Duration) string { return lengthLabel(total) }

// WaitLabel is how long a wait has run, as m:ss, which the stage shows under
// the message of the wait.
func WaitLabel(wait time.Duration) string { return waitedLine(wait) }

// SpeedLabel is the speed of a transfer, which the stage appends to a wait while
// bytes are arriving. The unit is the one a viewer reads at a glance: MB/s for a
// fast line, KB/s below a megabyte a second.
func SpeedLabel(rate float64) (value, unit string) { return formatSpeed(rate) }
