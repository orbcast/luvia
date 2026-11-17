// Package playintro plays the brand animation that covers the window when
// playback starts and reveals the window again once it ends. It is a
// play-start intro (启播动画), not a launch splash: nothing plays it at startup.
package playintro

import (
	_ "embed"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
)

// intro is the generated Luvia play-intro animation, produced from
// assets/icons/mark.webp by scripts/generate-opening.py (make brand). It is
// embedded so the intro never depends on runtime files or the network.
//
//go:embed playintro.apng
var intro []byte

// holdAfter lets the final frame rest before the window is revealed again.
const holdAfter = 250 * time.Millisecond

// Controller owns at most one running intro player. Everything here runs on
// the main thread, like the view and the play actions that call it.
type Controller struct {
	invalidate func()
	player     *ui.VideoPlayer
	active     bool
	done       func()
}

// New returns a controller. invalidate redraws the window when the intro
// starts or ends; it may be nil in tests.
func New(invalidate func()) *Controller { return &Controller{invalidate: invalidate} }

// Active reports whether the intro is showing.
func (c *Controller) Active() bool { return c.active }

// Start plays the intro once from its beginning and calls after when it ends,
// or at once if the animation cannot be decoded. It replaces any intro already
// running. Start runs on the main thread, as the play action does.
func (c *Controller) Start(after func()) {
	c.stop()
	decoder, err := ui.NewAPNGDecoder(intro)
	if err != nil {
		if after != nil {
			after()
		}
		return
	}
	player, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		if after != nil {
			after()
		}
		return
	}
	player.SetLoop(false).SetMuted(true)
	c.player, c.active, c.done = player, true, after
	if c.invalidate != nil {
		c.invalidate()
	}
	delay := player.Duration() + holdAfter
	if delay <= holdAfter {
		delay = holdAfter
	}
	time.AfterFunc(delay, func() {
		mygo.RunOnMain(func() {
			if c.player != player {
				return // a newer intro replaced this one
			}
			c.finish()
		})
	})
}

// finish clears the intro and reports that it ended, on the main thread.
func (c *Controller) finish() {
	done := c.done
	c.stop()
	if c.invalidate != nil {
		c.invalidate()
	}
	if done != nil {
		done()
	}
}

// stop closes the running player, if any.
func (c *Controller) stop() {
	if c.player != nil {
		c.player.Close()
	}
	c.player, c.active, c.done = nil, false, nil
}

// View draws the intro over the whole window while it plays, and nothing
// otherwise. Call it from the window's content view, after the page.
func (c *Controller) View(ctx *ui.Context) {
	if !c.active || c.player == nil {
		return
	}
	player := c.player
	ui.Overlay(ctx, func() {
		ui.Column(ctx).Absolute().Left(0).Top(0).Right(0).Bottom(0).
			Background(design.PlayIntroBackground()).Center().Padding(72).
			Children(func() {
				ui.Video(ctx, player).Fill().Fit(ui.Contain)
			})
	})
}
