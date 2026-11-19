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
	for _, text := range []string{"Mute", "Danmaku", "Show panel"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}
	if tt.HasText("Hide panel") {
		t.Fatal("the picture must not offer to collapse a panel that is already away")
	}

	// The floating cluster hangs level with the episode marker instead of
	// dropping below it by the marker's whole height.
	boxes := make(map[string]ui.Rect, 4)
	for _, node := range tt.AccessAll() {
		if node.Role == ui.RoleButton || node.Role == ui.RoleText {
			boxes[node.Name] = node.Bounds
		}
	}
	mute, ok := boxes["Mute"]
	if !ok {
		t.Fatalf("the speaker is missing among %v", boxes)
	}
	marker, ok := boxes["Episode 1"]
	if !ok {
		t.Fatalf("the episode marker is missing among %v", boxes)
	}
	// The label's box carries the line height of its 13-point face, so the
	// centers sit within a few units rather than on one number.
	if mid, want := mute.Y+mute.H/2, marker.Y+marker.H/2; mid > want+5 || mid < want-5 {
		t.Fatalf("the player controls sit at %v, not level with the episode marker at %v", mid, want)
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

	// The close button rides on the expanded panel: it comes back with it and
	// leaves the player.
	if err := tt.Click("Show panel"); err != nil {
		t.Fatalf("reveal panel again: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Close"); err != nil {
		t.Fatalf("close: %v", err)
	}
	if !backed {
		t.Fatal("close did not call OnBack")
	}
}
