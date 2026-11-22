package tests

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// danmakuSettingsPage builds a player over one timed comment, with the layer's
// settings kept in path when path is not empty.
func danmakuSettingsPage(t *testing.T, path string) (*player.Page, chan func(), *ui.Tester) {
	t.Helper()
	base, _ := sampleTitle()
	src := &commentingSource{
		detail:   base.detail,
		episodes: base.episodes,
		comments: []source.Danmaku{{At: 0, Text: "Opening line"}},
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn }, SettingsPath: path,
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	apply(t, run)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	return page, run, tt
}

// TestDanmakuButtonUnfoldsTheSettings covers the button over the picture: a
// press with the layer on unfolds the settings, so the band, the opacity, the
// size, the speed, the rows and the kinds of comment are all there to change.
func TestDanmakuButtonUnfoldsTheSettings(t *testing.T) {
	page, _, tt := danmakuSettingsPage(t, "")
	if page.DanmakuOpen() {
		t.Fatal("the settings must not open by themselves")
	}
	if tt.HasText("Opacity") {
		t.Fatal("the settings are on screen before they were asked for")
	}

	if err := tt.Click("Danmaku"); err != nil {
		t.Fatalf("danmaku button: %v", err)
	}
	tt.Frame()
	if !page.DanmakuOpen() {
		t.Fatal("pressing the danmaku button did not unfold the settings")
	}
	for _, want := range []string{
		"Screen area", "Whole picture", "Top", "Middle", "Bottom",
		"Opacity", "Text size", "Speed", "Rows",
		"Hide scrolling", "Hide pinned above", "Hide pinned below", "Bold",
	} {
		if !tt.HasText(want) {
			t.Fatalf("the settings are missing %q: %v", want, tt.Texts())
		}
	}

	// A second press with the settings open turns the layer off, and a layer
	// that is off keeps its settings with it.
	if err := tt.Click("Danmaku"); err != nil {
		t.Fatalf("danmaku button: %v", err)
	}
	tt.Frame()
	if page.DanmakuOpen() {
		t.Fatal("turning the layer off left the settings open")
	}
	if tt.HasText("Opacity") {
		t.Fatal("turning the layer off left the settings on screen")
	}
}

// TestDanmakuAreaMovesTheBand covers the screen-area presets: the comments move
// out of the top of the picture when the band is set below it, and back when it
// is set to the whole picture again.
func TestDanmakuAreaMovesTheBand(t *testing.T) {
	page, run, tt := danmakuSettingsPage(t, "")
	showDanmaku(t, page, tt)
	if !pumpUntil(t, run, tt, "Opening line") {
		t.Fatalf("the comment never reached the picture: %v", tt.Texts())
	}
	full, ok := tt.Find("Opening line")
	if !ok {
		t.Fatalf("the comment has no place on the picture: %v", tt.Texts())
	}

	if err := tt.Click("Danmaku"); err != nil {
		t.Fatalf("danmaku button: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Bottom"); err != nil {
		t.Fatalf("bottom preset: %v", err)
	}
	pumpUntil(t, run, tt, "Opening line")
	low, ok := tt.Find("Opening line")
	if !ok {
		t.Fatalf("the comment left the picture after the preset: %v", tt.Texts())
	}
	if low.Y <= full.Y {
		t.Fatalf("the comment did not move down with the band: %v -> %v", full, low)
	}
	if page.DanmakuSettings().Area != 3 {
		t.Fatalf("the preset was not kept: area %d", page.DanmakuSettings().Area)
	}

	if err := tt.Click("Whole picture"); err != nil {
		t.Fatalf("full preset: %v", err)
	}
	pumpUntil(t, run, tt, "Opening line")
	again, ok := tt.Find("Opening line")
	if !ok {
		t.Fatalf("the comment left the picture after the preset: %v", tt.Texts())
	}
	if again.Y != full.Y {
		t.Fatalf("returning to the whole picture left the comment at %v, wanted %v", again, full)
	}
}

// TestDanmakuSettingsOutliveThePlayer covers the settings file: a change is
// written, and the next player opens with it instead of starting over.
func TestDanmakuSettingsOutliveThePlayer(t *testing.T) {
	path := filepath.Join(t.TempDir(), "player.json")
	page, _, tt := danmakuSettingsPage(t, path)

	if err := tt.Click("Danmaku"); err != nil {
		t.Fatalf("danmaku button: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Bottom"); err != nil {
		t.Fatalf("bottom preset: %v", err)
	}
	tt.Frame()

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("the settings were not written: %v", err)
	}
	var stored player.Settings
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("the settings file is not the player's own: %v", err)
	}
	if stored.Area != 3 {
		t.Fatalf("the written settings do not carry the area: %+v", stored)
	}
	if page.DanmakuSettings().Area != 3 {
		t.Fatalf("the page did not take the area: %+v", page.DanmakuSettings())
	}

	// A second player over the same file opens where the first one was left.
	again, _, _ := danmakuSettingsPage(t, path)
	if got := again.DanmakuSettings().Area; got != 3 {
		t.Fatalf("the next player started at area %d, wanted 3", got)
	}
}

// TestDanmakuSettingsFallBackToTheDefaults covers a file that cannot be used:
// the player draws comments with its own defaults rather than a zero value that
// would leave no rows at all.
func TestDanmakuSettingsFallBackToTheDefaults(t *testing.T) {
	path := filepath.Join(t.TempDir(), "player.json")
	if err := os.WriteFile(path, []byte("{not json"), 0600); err != nil {
		t.Fatal(err)
	}
	page, _, _ := danmakuSettingsPage(t, path)
	want := player.DefaultSettings()
	if got := page.DanmakuSettings(); got != want {
		t.Fatalf("a broken file gave %+v, wanted %+v", got, want)
	}

	// A file that parses but holds a half-written record is fitted back into
	// the range the controls offer rather than trusted.
	if err := os.WriteFile(path, []byte(`{"Size":200,"Lanes":40,"Speed":0.1,"Opacity":0}`), 0600); err != nil {
		t.Fatal(err)
	}
	page, _, _ = danmakuSettingsPage(t, path)
	got := page.DanmakuSettings()
	if got.Size != want.Size || got.Lanes != want.Lanes || got.Speed != want.Speed || got.Opacity != want.Opacity {
		t.Fatalf("an out-of-range record was not fitted back: %+v", got)
	}
}
