package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/locales"
)

// leftPlayer is the page with a way out that counts the presses, so a case can
// say the corner took it out rather than only that a button exists.
func leftPlayer(t *testing.T, open func() (*ui.VideoPlayer, error)) (page *player.Page, tt *ui.Tester, left *int) {
	t.Helper()
	src, _ := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 16)
	count := 0
	page = player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return open() },
		OnBack:    func() { count++ },
	})
	tt = ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	return page, tt, &count
}

// The player's corner carries its own way out: a viewer needs to leave from the
// first frame, before the picture and the controls under it have arrived, and
// the top-left corner is where they look for one.
func TestPlayerOffersAWayOutInTheCorner(t *testing.T) {
	page, tt, left := leftPlayer(t, func() (*ui.VideoPlayer, error) {
		// An open that never returns: what the corner has to offer while the
		// stage is still waiting for a stream is the whole point.
		select {}
	})

	src, _ := sampleTitle()
	page.Open(src.detail, src.episodes, 0)
	tt.Frame()

	m := locales.In("en")
	if !tt.HasText(m.ClosePlayer()) {
		t.Fatalf("the waiting stage offers no way out: %v", tt.Texts())
	}
	if err := tt.Click(m.ClosePlayer()); err != nil {
		t.Fatalf("click the corner's way out: %v", err)
	}
	if *left != 1 {
		t.Fatalf("the corner's press reached the player %d times, want 1", *left)
	}
}

// The corner's button sits at the top-left, to the left of the episode marker
// rather than on the controls at the bottom: a viewer who leaves looks there.
func TestPlayerLeavesFromTheTopLeft(t *testing.T) {
	page, tt, left := leftPlayer(t, func() (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil })

	src, _ := sampleTitle()
	page.Open(src.detail, src.episodes, 0)
	tt.Frame()
	tt.Frame()

	m := locales.In("en")
	bounds, ok := tt.Find(m.ClosePlayer())
	if !ok {
		t.Fatalf("the corner has no way out among %v", tt.Texts())
	}
	if bounds.X > 120 || bounds.Y > 120 {
		t.Fatalf("the way out is at %v, not in the top-left corner", bounds)
	}
	if err := tt.Click(m.ClosePlayer()); err != nil {
		t.Fatalf("click the corner's way out: %v", err)
	}
	if *left != 1 {
		t.Fatalf("the corner's press reached the player %d times, want 1", *left)
	}
}
