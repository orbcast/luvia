package tests

import (
	"bytes"
	"context"
	"errors"
	"image"
	"image/color"
	"image/gif"
	"testing"

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

	// The overlay carries the level track beside the speaker, the danmaku
	// toggle, the panel toggle and the two ten-second seeks.
	for _, label := range []string{"Volume", "Mute", "Danmaku", "Show panel", "Rewind 10 seconds", "Forward 10 seconds"} {
		if !tt.HasText(label) {
			t.Fatalf("overlay lost %q: %v", label, tt.Texts())
		}
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

	// A player opens silent, with the level track already beside the speaker
	// as the system's player keeps it, not a slider that folds out on a press.
	if page.Audible() {
		t.Fatal("a fresh player must start muted")
	}
	if _, ok := tt.Find("Volume"); !ok {
		t.Fatalf("the level track is missing among %v", tt.Texts())
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click speaker: %v", err)
	}
	tt.Frame()
	if !page.Audible() {
		t.Fatal("the speaker did not unmute the video")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click speaker again: %v", err)
	}
	tt.Frame()
	if page.Audible() {
		t.Fatalf("the second press must silence the video: %v", tt.Texts())
	}

	// The track is the level: pressing it at a place sets the level there, and
	// dragging along it follows the hand, as the system's player does.
	track, ok := tt.Find("Volume")
	if !ok {
		t.Fatalf("the level track is missing among %v", tt.Texts())
	}
	const knob float32 = 12
	// The Tester starts with the pointer outside the window, and a press there
	// is no press, so the pointer comes onto the track first.
	tt.Move(track.X+track.W/2, track.Y+track.H/2)
	// drag travels along the track to x and lets go.
	drag := func(x float32) {
		t.Helper()
		tt.Frame()
		tt.Press(track.X+track.W/2, track.Y+track.H/2)
		tt.Frame()
		tt.Move(x, track.Y+track.H/2)
		tt.Frame()
		tt.Release(x, track.Y+track.H/2)
		tt.Frame()
	}
	// levelAt is the level the track reports for a place on it.
	levelAt := func(x float32) float64 {
		return float64((x - track.X) / (track.W - knob))
	}
	// held reports the level the track sits at, within a pixel or so of the
	// place the pointer left it.
	held := func(x float32) bool {
		want := levelAt(x)
		return page.Volume() >= want-0.02 && page.Volume() <= want+0.02
	}

	// Pressing the far end out of silence brings the sound back at that level.
	drag(track.X + track.W - knob)
	if !page.Audible() {
		t.Fatalf("raising the muted level kept the video silent: %v", tt.Texts())
	}
	if !held(track.X + track.W - knob) {
		t.Fatalf("the far end of the track set the level to %v, not %v", page.Volume(), levelAt(track.X+track.W-knob))
	}

	// A drag back along the track follows the pointer without silencing the
	// video on the way.
	drag(track.X + track.W/2)
	if !page.Audible() {
		t.Fatalf("a drag to a quieter level silenced the video: %v", tt.Texts())
	}
	if !held(track.X + track.W/2) {
		t.Fatalf("dragging along the track left the level at %v, not %v", page.Volume(), levelAt(track.X+track.W/2))
	}

	// A level dragged to the near end ends silent, as the system's own level
	// track does at its bottom.
	drag(track.X)
	if page.Audible() {
		t.Fatal("a level dragged to the near end should sound nothing")
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
