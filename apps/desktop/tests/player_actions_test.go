package tests

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// tinyGIF is a one-frame 2×2 GIF, enough to build a cross-platform VideoPlayer
// without a system media framework.
func tinyGIF() []byte {
	g := &gif.GIF{Config: image.Config{Width: 2, Height: 2}}
	frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{color.RGBA{255, 0, 0, 255}})
	g.Image = []*image.Paletted{frame}
	g.Delay = []int{10}
	g.Disposal = []byte{gif.DisposalNone}
	var b bytes.Buffer
	_ = gif.EncodeAll(&b, g)
	return b.Bytes()
}

func newGIFPlayer(t *testing.T) *ui.VideoPlayer {
	t.Helper()
	decoder, err := ui.NewGIFDecoder(tinyGIF())
	if err != nil {
		t.Fatal(err)
	}
	pl, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	return pl
}

// volumeLingerForTest is longer than the player's idle time for the level
// track, so folding it is due without waiting in a test.
const volumeLingerForTest = 5 * time.Second

func TestPlayerControlsWithVideo(t *testing.T) {
	src, _ := sampleTitle()
	src.playback.URL = "mem://clip"
	store, _ := library.Open("")
	run := make(chan func(), 16)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	// The overlay carries the speaker, the danmaku toggle, the panel toggle and
	// the two ten-second seeks. The level track stays folded until asked for.
	for _, label := range []string{"Mute", "Danmaku", "Show panel", "Rewind 10 seconds", "Forward 10 seconds"} {
		if !tt.HasText(label) {
			t.Fatalf("overlay lost %q: %v", label, tt.Texts())
		}
	}
	if tt.HasText("Volume") {
		t.Fatalf("the level track must stay folded at first: %v", tt.Texts())
	}
	for _, label := range []string{"Rewind 10 seconds", "Forward 10 seconds"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
		tt.Frame()
	}
	if err := tt.Click("Show panel"); err != nil {
		t.Fatalf("revealing the panel: %v", err)
	}
	tt.Frame()
	if err := tt.Click("Hide panel"); err != nil {
		t.Fatalf("collapsing the panel: %v", err)
	}
	tt.Frame()

	// A player opens silent, and a press on the speaker unfolds the level track.
	if page.Audible() {
		t.Fatal("a fresh player must start muted")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click speaker: %v", err)
	}
	tt.Frame()
	if !page.VolumeOpen() {
		t.Fatal("the speaker did not unfold the level track")
	}
	if !tt.HasText("Volume") {
		t.Fatalf("the unfolded track is missing: %v", tt.Texts())
	}

	// A second press silences the video and folds the track away.
	page.ToggleMute()
	tt.Frame()
	if !page.Audible() {
		t.Fatal("unmuting kept the video silent")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click speaker again: %v", err)
	}
	tt.Frame()
	if page.VolumeOpen() {
		t.Fatal("the second press must fold the level track away")
	}
	if page.Audible() {
		t.Fatal("the second press must silence the video")
	}

	// The track folds back on its own once the viewer leaves it alone, from the
	// moment it unfolds, without waiting for a touch first.
	page.OpenVolume()
	tt.Frame()
	if !page.VolumeOpen() {
		t.Fatal("opening the level track failed")
	}
	page.FoldVolume(time.Now().Add(volumeLingerForTest + time.Second))
	tt.Frame()
	if page.VolumeOpen() {
		t.Fatal("the level track stayed out after its idle time")
	}

	// A level left at zero sounds nothing, and unmuting restores the sound
	// instead of leaving the video silent.
	page.SetVolume(0)
	tt.Frame()
	if page.Audible() {
		t.Fatal("zero volume should sound nothing")
	}
	page.ToggleMute()
	tt.Frame()
	if !page.Audible() {
		t.Fatalf("unmuting at zero volume kept the video silent: %v", tt.Texts())
	}

	page.TogglePlay()
	page.TogglePlay()
	page.ToggleSubtitle()
	page.CycleRate()
	for i := 0; i < 6; i++ {
		page.CycleRate()
	}
	page.SeekFraction(0.5)
	page.SeekFraction(-1)
	page.ToggleExpanded()
	tt.Frame()
	page.ToggleFullScreen()
	tt.Frame()
	page.ToggleFullScreen()
	page.SelectTab(1)
	tt.Frame()
	page.SelectTab(0)

	page.Next()
	apply(t, run)
	tt.Frame()
	page.Previous()
	apply(t, run)
	tt.Frame()

	// Selecting another line and reconciling it in the view resolves again.
	page.SelectLine("l2")
	apply(t, run)
	tt.Frame()
	applyOptional(t, run)
	tt.Frame()

	page.Back()
}

// applyOptional runs the next queued callback if one is already waiting.
func applyOptional(t *testing.T, run chan func()) {
	t.Helper()
	select {
	case fn := <-run:
		fn()
	default:
	}
}

// failingSource answers every operation with an error, to drive the pages' error
// and retry states.
type failingSource struct{}

func (failingSource) ID() string   { return "fail" }
func (failingSource) Name() string { return "Fail" }
func (failingSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, errors.New("boom")
}
func (failingSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, errors.New("boom")
}
func (failingSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, errors.New("boom")
}
func (failingSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, errors.New("boom")
}
func (failingSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, errors.New("boom")
}
func (failingSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}
