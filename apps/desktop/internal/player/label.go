package player

import "time"

// LengthLabel is the total-time reading the controls show: a dash while the
// engine has not learned the length, otherwise the media time.
func LengthLabel(total time.Duration) string { return lengthLabel(total) }

// WaitLabel is how long a wait has run, as m:ss, which the stage shows under
// the message of the wait.
func WaitLabel(wait time.Duration) string { return waitedLine(wait) }

// BackdateWait moves the clock of the stream's waits back by age, for a test
// that needs a stream to have been waited on for longer than an opening may
// take. A wait that has not started stays unstarted.
func (p *Page) BackdateWait(age time.Duration) {
	if age <= 0 {
		return
	}
	for _, clock := range []*time.Time{&p.buffered, &p.opened, &p.stallAt} {
		if !clock.IsZero() {
			*clock = clock.Add(-age)
		}
	}
}

// BackdateStall moves the clock of a picture that stood still back by age, for a
// test that needs a stream to have been stuck for longer than a wait may take.
func (p *Page) BackdateStall(age time.Duration) {
	if age <= 0 || p.stallAt.IsZero() {
		return
	}
	p.stallAt = p.stallAt.Add(-age)
}

// SpeedLabel is the speed of a transfer, which the stage appends to a wait while
// bytes are arriving. The unit is the one a viewer reads at a glance: MB/s for a
// fast line, KB/s below a megabyte a second.
func SpeedLabel(rate float64) (value, unit string) { return formatSpeed(rate) }
