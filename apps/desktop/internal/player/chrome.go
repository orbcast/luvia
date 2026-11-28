package player

import (
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
)

// The stage's chrome hides itself while the picture plays. A viewer watching a
// film wants the picture, not a row of buttons over it, so the bars fade out
// after a few idle seconds and any pointer move or press brings them back. Three
// states keep them on no matter how long the viewer does nothing: a paused
// film, the pointer resting on the chrome (so a reader can aim at a control), and
// the danmaku settings box while it is unfolded — a box that vanished mid-drag
// would be worse than one that stays.

// showChrome is the switch itself: false while the bars are hidden. It runs
// once per frame that draws the stage, so the bars come and go on their own idle
// window rather than on the next unrelated redraw.
func (p *Page) showChrome(now time.Time) bool {
	// Only a moving picture earns a clean frame. A stage that is still opening,
	// waiting for its first frame, paused or carrying a notice keeps its bars:
	// seeking and retrying are the next things a viewer does, and a stage with
	// no picture and no controls is a dead end. The settings box keeps the
	// capsule it is anchored to for the same reason. The clock restarts with
	// them, so a picture that paused a while and plays on gets a whole idle
	// window rather than the remainder of the one it paused in.
	if p.chromeStays() {
		p.chromeAt = now
		if p.chromeHidden {
			p.revealChrome(now)
		}
		return true
	}
	if p.chromeHidden {
		if p.pointerMoved || p.pointerOverChrome {
			p.revealChrome(now)
		}
		return !p.chromeHidden
	}
	// The pointer resting on a bar keeps it: a reader aiming at a control is not
	// an idle viewer.
	if p.pointerOverChrome {
		p.chromeAt = now
		return true
	}
	// The window runs on the page's own clock, and only once the picture has
	// actually moved under it: a stream that has played nothing yet is still
	// opening, and opening is a wait, not a film being watched. The wait ends on
	// the frame the playhead leaves zero, and not a frame later: from then on
	// the clock runs, because a playhead that moves every frame while the
	// picture plays would otherwise restart the idle window every frame and the
	// bars would never leave.
	played := p.position()
	if !p.chromePlayedSet || (played > 0 && p.chromePlayed <= 0) {
		p.chromePlayed, p.chromePlayedSet = played, true
		p.chromeAt = now
		return true
	}
	p.chromePlayed = played
	if now.Sub(p.chromeAt) >= design.PlayerAutoHide {
		p.chromeHidden = true
		p.chromeAt = now
	}
	return !p.chromeHidden
}

// revealChrome brings the bars back and restarts the idle window.
func (p *Page) revealChrome(now time.Time) {
	p.chromeHidden = false
	p.chromeAt = now
	// The pointer belongs to the picture again: either it just moved, or it is
	// resting somewhere a reader will not be reaching for a control from.
	p.pointerMoved = false
	p.pointerOverChrome = false
}

// chromeStays reports the states in which the bars never hide: no picture yet,
// a notice covering the stage, a paused film, and the unfolded settings box.
// They are read from the player's state rather than from a timer, so a viewer
// who pauses or stalls always gets the controls back.
func (p *Page) chromeStays() bool {
	if !p.playing || p.danmakuOpen || p.notice != noticeNone {
		return true
	}
	return p.stalled()
}

// observePointer records where the pointer is on this frame, for the stage's
// bars: inside the stage at all, and resting on one of them. Hidden bars come
// back when the pointer is anywhere in the stage, and either bar stays while it
// covers the pointer. Only the bars read this — a bar that watched the pointer
// only while it is drawn means a hidden bar cannot wake itself.
func (p *Page) observePointer(_ time.Time, x, y, width, height float32) {
	// The stage's own coordinates, so a pointer over the panel beside the stage
	// is not a pointer over the stage. One set once at the start is a viewer who
	// has not touched the picture yet, not a viewer who moved: the bars are up,
	// and the idle window is already running for them.
	inside := x >= 0 && y >= 0 && x <= width && y <= height
	over := inside && (y <= stageTopHeight+8 || y >= height-bottomBarHeight)
	p.pointerMoved = inside && (x != p.pointerX || y != p.pointerY) && p.pointerSeen
	p.pointerOverChrome = over
	p.pointerX, p.pointerY, p.pointerSeen = x, y, inside
}

// chromeFrame is the frame's own wake-up call: while the bars are up and the
// film plays, a frame is due for the instant the idle window runs out, so the
// chrome leaves on time instead of on the next unrelated redraw.
func (p *Page) chromeFrame(c *ui.Context, now time.Time) {
	if p.chromeHidden || !p.showChrome(now) {
		return
	}
	c.After(time.Until(p.chromeAt.Add(design.PlayerAutoHide)) + time.Millisecond)
}

// stageShade is the strip under the stage's top line: black at the top, nothing
// by the time it reaches the picture's mass, so the way out, the episode marker
// and the pill read on a bright frame. The bottom bar draws fading glyphs over a
// capsule instead of a strip, because a strip there would sit on the picture
// itself — the stage fits the frame whole, and a band along its foot would be a
// band across the film.
func (p *Page) stageShade(c *ui.Context) {
	shade := design.PlayerShadeSpec()
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Height(shade.TopHeight).
		LinearGradient(ui.LinearGradient{
			Angle: 180,
			Stops: []ui.GradientStop{
				{Color: ui.Transparent, At: 0},
				{Color: shade.Top, At: shade.Mid},
				{Color: ui.Transparent, At: 1},
			},
		})
}
