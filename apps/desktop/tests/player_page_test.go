package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
)

func TestPlayerRendersSwitchesEpisodeAndCloses(t *testing.T) {
	src, _ := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	detail := src.detail
	episodes := src.episodes
	run := make(chan func(), 8)
	backed := false
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnBack: func() { backed = true },
		OpenVideo: func(string) (*ui.VideoPlayer, error) {
			t.Fatal("an empty address must not open a player")
			return nil, nil
		},
	})
	page.Open(detail, episodes, 0)
	apply(t, run)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	// A player opens on the picture alone; the panel waits to be asked for.
	if tt.HasText("NOW PLAYING") {
		t.Fatal("the side panel must stay hidden when the player opens")
	}
	for _, text := range []string{"Show panel", "Mute", "Danmaku"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
	if tt.HasText("Hide panel") {
		t.Fatal("the picture must not offer to collapse a panel that is already away")
	}

	// The reveal control brings the side panel back beside the picture.
	if err := tt.Click("Show panel"); err != nil {
		t.Fatalf("reveal panel: %v", err)
	}
	tt.Frame()
	for _, text := range []string{"NOW PLAYING", "Cyberpunk Edgerunners 2", "Episodes", "Line 1"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	// Choosing another episode resolves it again.
	before := src.resolves.Load()
	if err := tt.Click("Episode 3"); err != nil {
		t.Fatalf("episode switch: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if src.resolves.Load() <= before {
		t.Fatalf("switching episode did not resolve again: %d -> %d", before, src.resolves.Load())
	}

	// Collapsing puts the panel away again and leaves the reveal control.
	if err := tt.Click("Hide panel"); err != nil {
		t.Fatalf("collapse panel: %v", err)
	}
	tt.Frame()
	if tt.HasText("NOW PLAYING") {
		t.Fatal("panel should be hidden after collapsing")
	}
	if !tt.HasText("Show panel") {
		t.Fatal("the picture should offer to show the panel again")
	}

	// Closing the panel leaves the player.
	if err := tt.Click("Close"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !backed {
		t.Fatal("close did not call OnBack")
	}
}
