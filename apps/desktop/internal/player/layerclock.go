package player

import "time"

// sampleTolerance is how far a report may lie from where the run expects the
// playhead before the layer takes it for a jump rather than for playback. A
// player reports its position a few times a second, so a run of time moves it by
// tens of milliseconds; a seek moves it by seconds at once, and the two are told
// apart by size rather than by watching for the seek itself.
const sampleTolerance = 400 * time.Millisecond

// layerClock is where the picture's playhead stands as far as the layer over it
// is concerned. The player reports its position now and then, and a layer that
// laid its comments out from those reports alone would move them in steps each
// time one came: a comment would sit, then jump, a few times a second as it
// crossed. This clock walks the playhead between reports at the pace being
// played, which is what makes a comment glide.
type layerClock struct {
	// at is where the clock stands and sampled when it stood there. last is the
	// report the clock last read, at told, so a seek is told from a player that
	// samples slowly by what one report changed against what the gap explains.
	at      time.Duration
	sampled time.Time
	last    time.Duration
	told    time.Time
	// rate is the pace the run is walked at; a report at another pace is a new
	// run rather than drift.
	rate float64
	ok   bool
}

// read walks the clock to now, given what the player currently reports.
func (c *layerClock) read(sample time.Duration, now time.Time, rate float64) time.Duration {
	if !c.ok || rate != c.rate {
		// Nothing to walk from yet, or another pace than the run was walked at:
		// this report begins the run.
		c.start(sample, now, rate)
		return sample
	}
	// A report that lands somewhere other than walking can account for is a seek
	// or a stall the player did not announce, and that begins a new run from what
	// the player says. A report that comes in behind, because the player samples
	// slowly, is the ordinary case: the clock is not pulled back to it, since
	// that is what would step a comment on the frame the report arrived.
	if drift := c.drift(sample, now); drift < -sampleTolerance || drift > sampleTolerance {
		c.start(sample, now, rate)
		return sample
	}
	// The report is where the run expects it. A picture that is not playing does
	// not carry the layer forward, so the clock stops where it walks to rather
	// than keeping on walking from a still picture.
	walked := c.walk(now)
	// A picture that is not playing does not carry the layer forward: the clock
	// stops where it walked to and reads the next report from there.
	c.at, c.sampled = walked, now
	c.last, c.told = sample, now
	return walked
}

// drift is how far a report lies from where the run expects the playhead: what
// the report says the playhead did since the last one, less what the pace says
// the gap between them should have carried it in the same stretch of watching.
func (c *layerClock) drift(sample time.Duration, now time.Time) time.Duration {
	pace := c.rate
	if pace <= 0 {
		pace = 1
	}
	gap := now.Sub(c.told)
	if gap < 0 {
		gap = 0
	}
	return (sample - c.last) - time.Duration(float64(gap)*pace)
}

// start begins a run at a report.
func (c *layerClock) start(sample time.Duration, now time.Time, rate float64) {
	c.at, c.sampled = sample, now
	c.last, c.told = sample, now
	c.rate, c.ok = rate, true
}

// walk is where the layer stands now: where it stood plus the time since, at the
// pace the run is being walked.
func (c *layerClock) walk(now time.Time) time.Duration {
	since := now.Sub(c.sampled)
	if since <= 0 {
		return c.at
	}
	pace := c.rate
	if pace <= 0 {
		pace = 1
	}
	return c.at + time.Duration(float64(since)*pace)
}

// layerPosition is where the layer draws its comments from: the playhead as the
// player reports it, carried across the frames between reports by the clock of
// the screen. Paging a comment list the source gave no times for walks a moment
// of its own and is left as it was, since there is no playhead behind it.
func (p *Page) layerPosition() time.Duration {
	if !p.started || !p.danmaku || !timedComments(p.comments) {
		p.layer.ok = false
		return p.position()
	}
	return p.layer.read(p.position(), p.now(), p.rate)
}
