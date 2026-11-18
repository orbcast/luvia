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

	// The overlay chrome: volume track, speaker, danmaku toggle and the two
	// ten-second seeks all take a click without leaving the screen.
	for _, label := range []string{"Volume", "Danmaku", "Rewind 10 seconds", "Forward 10 seconds"} {
		if err := tt.Click(label); err != nil {
			t.Fatalf("click %q: %v", label, err)
		}
		tt.Frame()
	}

	// The speaker toggles sound on its own: the first press silences the video,
	// the second restores it.
	if !page.Audible() {
		t.Fatal("a fresh player must sound audible")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click mute: %v", err)
	}
	tt.Frame()
	if page.Audible() {
		t.Fatal("the speaker did not mute the video")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click mute again: %v", err)
	}
	tt.Frame()
	if !page.Audible() {
		t.Fatal("the speaker did not restore sound")
	}

	// A slider left at zero sounds nothing, and the speaker brings the level
	// back instead of leaving the video silent.
	page.SetVolume(0)
	tt.Frame()
	if page.Audible() {
		t.Fatal("zero volume should sound nothing")
	}
	if err := tt.Click("Mute"); err != nil {
		t.Fatalf("click mute at zero: %v", err)
	}
	tt.Frame()
	if !page.Audible() {
		t.Fatalf("unmuting at zero volume kept the video silent: %v", tt.Texts())
	}

	page.TogglePlay()
	page.TogglePlay()
	page.ToggleMute()
	page.ToggleSubtitle()
	page.CycleRate()
	for i := 0; i < 6; i++ {
		page.CycleRate()
	}
	page.SeekFraction(0.5)
	page.SeekFraction(-1)
	page.ToggleExpanded()
	tt.Frame()
	page.ToggleCinema()
	tt.Frame()
	page.ToggleCinema()
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
