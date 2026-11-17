// Package splash plays the brand opening animation in a short-lived,
// frameless window before the main window appears.
package splash

import (
	_ "embed"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
)

// opening is the generated Luvia opening animation, produced from
// assets/icons/mark.webp by scripts/generate-opening.py (make brand). It is
// embedded so the splash never depends on runtime files or the network.
//
//go:embed opening.apng
var opening []byte

const (
	// windowSize matches the animation's square canvas so it shows 1:1.
	windowSize = 512
	// revealGrace lets the final frame rest before the main window opens.
	revealGrace = 400 * time.Millisecond
)

// Show plays the opening animation and calls reveal when it ends, then closes
// its window. If the animation cannot be decoded, it calls reveal at once so a
// broken asset never blocks startup. Show runs on the main thread, as
// App.WhenReady does.
func Show(reveal func()) {
	decoder, err := ui.NewAPNGDecoder(opening)
	if err != nil {
		reveal()
		return
	}
	player, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		reveal()
		return
	}
	player.SetLoop(false).SetMuted(true)

	win := mygo.NewWindow(mygo.WindowOptions{
		Title:           "Luvia",
		Width:           windowSize,
		Height:          windowSize,
		Frameless:       true,
		AlwaysOnTop:     true,
		AutoHideMenuBar: true,
		DisableResize:   true,
		DisableMove:     true,
		DisableMinimize: true,
		DisableMaximize: true,
		DisableClose:    true,
		SkipTaskbar:     true,
		Hidden:          true,
		BackgroundColor: design.SplashBackground,
		Content: ui.View(func(c *ui.Context) {
			ui.Video(c, player).Fill().Fit(ui.Contain)
		}),
	})
	win.Show()

	delay := player.Duration() + revealGrace
	if delay <= revealGrace {
		delay = revealGrace
	}
	time.AfterFunc(delay, func() {
		win.Update(func() {
			reveal()
			win.Close()
			player.Close()
		})
	})
}
