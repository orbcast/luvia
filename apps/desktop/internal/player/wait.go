package player

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/locales"
)

// A wait is redrawn on its own clock: often enough for a live ring and a clock
// that counts, slow enough that a wait costs nothing.
const waitTick = 200 * time.Millisecond

// stageOpening covers the picture while the episode's line is resolved and its
// engine opened. A resolution that takes long says so, with the time it has
// taken: a mark on its own left a slow line looking like a broken one.
func (p *Page) stageOpening(c *ui.Context, m locales.Messages) {
	if p.opened.IsZero() {
		p.opened = time.Now()
	}
	c.After(waitTick)
	ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Justify(ui.Center).AlignItems(ui.Center).Gap(14).Children(func() {
		ui.Icon(c, icons.Parse("full-screen-linear")).Size(30, 30).TextColor(mediaMuted)
		ui.Text(c, m.Preparing()).FontSize(14).TextColor(mediaInk)
		ui.Text(c, p.waitText(m, time.Since(p.opened))).FontSize(12).TextColor(mediaMuted)
	})
}

// waitText is the line under a wait's message: how long it has run, and how fast
// the bytes are coming when the cache is the one fetching them.
func (p *Page) waitText(m locales.Messages, wait time.Duration) string {
	line := m.Waited(waitedLine(wait))
	if speed := p.speedText(m); speed != "" {
		line += "  " + speed
	}
	return line
}

// speedText is the cache's speed right now, or "" when nothing is arriving: a
// transfer that stalled has no rate worth showing, and leaving the number out
// says that better than a zero would.
func (p *Page) speedText(m locales.Messages) string {
	if p.deps.Cache == nil {
		return ""
	}
	transfer := p.deps.Cache.Transfer()
	if transfer.Rate <= 0 {
		return ""
	}
	value, unit := formatSpeed(transfer.Rate)
	if unit == kilobytes {
		return m.LoadSpeedKB(value)
	}
	return m.LoadSpeed(value)
}

// The units a speed is written in: a line slower than a megabyte a second reads
// as kilobytes, because "0.03 MB/s" tells a viewer nothing.
const (
	kilobytes = "KB"
	megabytes = "MB"
)

// formatSpeed writes bytes per second in the unit a viewer reads at a glance,
// rounded rather than truncated so a slow but steady transfer never reads as
// zero.
func formatSpeed(rate float64) (value, unit string) {
	if rate < 1000*1000 {
		// A trickle keeps a decimal: rounding it to whole kilobytes would show a
		// line that is still moving as nothing.
		speed := rate / 1000
		if speed < 10 {
			return strconv.FormatFloat(speed, 'f', 1, 64), kilobytes
		}
		return strconv.FormatFloat(speed, 'f', 0, 64), kilobytes
	}
	speed := rate / (1000 * 1000)
	if speed < 10 {
		return strconv.FormatFloat(speed, 'f', 2, 64), megabytes
	}
	return strconv.FormatFloat(speed, 'f', 1, 64), megabytes
}

// waitedLine is the clock a wait shows, as m:ss: a wait that runs into hours has
// gone wrong in another way, and a viewer reads the start of it as the story.
func waitedLine(wait time.Duration) string {
	if wait < 0 {
		wait = 0
	}
	return strconv.Itoa(int(wait/time.Minute)) + ":" + twoDigits(int(wait/time.Second)%60)
}

// twoDigits pads a second below ten, because the clock a viewer reads is a clock.
func twoDigits(n int) string {
	if n < 10 {
		return "0" + strconv.Itoa(n)
	}
	return strconv.Itoa(n)
}
