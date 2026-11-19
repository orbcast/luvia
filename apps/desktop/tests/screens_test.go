package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/screens"
)

func TestRouterOpensDetailAndPlayer(t *testing.T) {
	src, ref := sampleTitle()
	store, _ := library.Open("")
	run := make(chan func(), 16)
	// Only the player covers the window without controls of its own in the
	// title bar's corner, so it reports the window's buttons hidden; the
	// navigation pages and the title page report them back.
	var hidden []bool
	router := screens.New(screens.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		WindowButtons: func(h bool) { hidden = append(hidden, h) },
		Base:          func(c *ui.Context) ui.Element { return ui.Text(c, "NAV-HOME") },
	})
	lastHidden := func() bool {
		if len(hidden) == 0 {
			return true
		}
		return hidden[len(hidden)-1]
	}
	tt := ui.NewTester(func(c *ui.Context) { router.View(c) }, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("NAV-HOME") {
		t.Fatalf("router did not start on the nav page: %v", tt.Texts())
	}
	if lastHidden() {
		t.Fatal("the nav pages must keep the window buttons")
	}

	// Opening a title shows the detail screen.
	router.OpenDetail(ref)
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Cyberpunk Edgerunners 2") {
		t.Fatalf("router did not show the detail page: %v", tt.Texts())
	}
	if lastHidden() {
		t.Fatal("the title page must keep the window buttons")
	}

	// Playing opens the player screen, which starts on the picture alone and
	// keeps the side panel behind its reveal control.
	if err := tt.Click("Play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Show panel") {
		t.Fatalf("router did not show the player: %v", tt.Texts())
	}
	if tt.HasText("NOW PLAYING") {
		t.Fatal("the player must open with its side panel away")
	}
	if !lastHidden() {
		t.Fatal("the player must hide the window buttons")
	}

	// The panel's close button returns to the title page.
	if err := tt.Click("Show panel"); err != nil {
		t.Fatalf("reveal panel: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Close"); err != nil {
		t.Fatalf("close: %v", err)
	}
	tt.Frame()
	if lastHidden() {
		t.Fatal("leaving the player must bring the window buttons back")
	}
	// The title page draws once the player it replaced is gone, so its text
	// follows in the next frame.
	tt.Frame()
	if !tt.HasText("Episodes") {
		t.Fatalf("router did not return to the detail page: %v", tt.Texts())
	}

	// Back from the title page returns to navigation.
	if err := tt.Click("Back"); err != nil {
		t.Fatalf("back: %v", err)
	}
	tt.Frame()
	if !tt.HasText("NAV-HOME") {
		t.Fatalf("router did not return to navigation: %v", tt.Texts())
	}

	// Reset also returns to navigation from an open title.
	router.OpenDetail(ref)
	apply(t, run)
	router.Reset()
	tt.Frame()
	if !tt.HasText("NAV-HOME") {
		t.Fatalf("reset did not return to navigation: %v", tt.Texts())
	}
}
