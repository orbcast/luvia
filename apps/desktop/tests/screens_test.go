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
	router := screens.New(screens.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		Base: func(c *ui.Context) ui.Element { return ui.Text(c, "NAV-HOME") },
	})
	tt := ui.NewTester(func(c *ui.Context) { router.View(c) }, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("NAV-HOME") {
		t.Fatalf("router did not start on the nav page: %v", tt.Texts())
	}

	// Opening a title shows the detail screen.
	router.OpenDetail(ref)
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Cyberpunk Edgerunners 2") {
		t.Fatalf("router did not show the detail page: %v", tt.Texts())
	}

	// Playing opens the player screen.
	if err := tt.Click("Play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if !tt.HasText("NOW PLAYING") {
		t.Fatalf("router did not show the player: %v", tt.Texts())
	}

	// Closing the player returns to the title page.
	if err := tt.Click("Close"); err != nil {
		t.Fatalf("close: %v", err)
	}
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
