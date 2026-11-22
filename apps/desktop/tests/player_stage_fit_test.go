package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/player"
)

// wideFrame is a one-frame 32×15 GIF, near the 2.11:1 a short drama ships. In a
// 1280×900 stage Contain fits it whole, 1280×600, which leaves a band above and
// below it: the room the backdrop has to fill.
func wideFrame(t *testing.T) *ui.VideoPlayer {
	t.Helper()
	g := &gif.GIF{Config: image.Config{Width: 32, Height: 15}}
	frame := image.NewPaletted(image.Rect(0, 0, 32, 15), color.Palette{color.RGBA{R: 200, G: 60, B: 40, A: 255}})
	g.Image = []*image.Paletted{frame}
	g.Delay = []int{10}
	g.Disposal = []byte{gif.DisposalNone}
	var b bytes.Buffer
	if err := gif.EncodeAll(&b, g); err != nil {
		t.Fatal(err)
	}
	decoder, err := ui.NewGIFDecoder(b.Bytes())
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

// stageShot draws the player stage of one episode once, with the poster the
// detail carries (none for "") decoded and ready, and returns the frame. The
// frame's bands are the room beside and above the picture; what they show says
// whether the backdrop fills them.
func stageShot(t *testing.T, poster string) image.Image {
	t.Helper()
	src, _ := sampleTitle()
	src.detail.PosterURL = poster
	src.detail.BackdropURL = ""
	src.playback.URL = "mem://clip"
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	images := media.New(t.TempDir(), media.Options{
		Client: &http.Client{Transport: artworkTransport{whiteArtwork(t, 64, 36)}},
	})
	images.SetRunner(func(fn func()) { fn() })
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Images: images,
		Run:       func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return wideFrame(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	if poster != "" {
		images.Prefetch(poster)
		for i := 0; i < 200 && images.Bitmap(poster) == nil; i++ {
			time.Sleep(10 * time.Millisecond)
		}
		if images.Bitmap(poster) == nil {
			t.Fatalf("the poster %q never decoded", poster)
		}
	}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	frame := tt.Image()
	if frame == nil {
		t.Fatal("no image")
	}
	return frame
}

// TestPlayerStageBackdropFillsTheFrameBands covers the room a whole frame
// leaves: a stage that shows only the near-black behind the picture reads as a
// picture that did not fill it, so the cover is blurred over all of the stage
// and the bands take its colours.
func TestPlayerStageBackdropFillsTheFrameBands(t *testing.T) {
	theme := design.Theme(ui.DarkTheme())
	stage := design.PlayerStage(&theme)
	bare := [3]uint8{stage.R, stage.G, stage.B}

	// 1280×900 stage, 1280×600 frame: the picture sits from y 150 to 750, so
	// these points are the bands above and below it, clear of the controls
	// that hang at the top line and along the bottom.
	points := [][2]int{{640, 60}, {200, 60}, {1000, 60}, {640, 775}}

	filled := stageShot(t, "https://cdn.example/poster.png")
	empty := stageShot(t, "")

	for _, p := range points {
		got := sampleAt(t, filled, p[0], p[1])
		if luma := (299*int(got[0]) + 587*int(got[1]) + 114*int(got[2])) / 1000; luma < 90 {
			t.Fatalf("stage at %v is %v (luma %d): the backdrop does not reach it", p, got, luma)
		}
		if d := delta(got, bare); d < 60 {
			t.Fatalf("stage at %v is %v, only %d from the bare stage %v", p, got, d, bare)
		}
		// The band is the backdrop's, not something the stage drew anyway: with
		// no poster to blur, the same point stays the stage's own colour.
		none := sampleAt(t, empty, p[0], p[1])
		if luma := (299*int(none[0]) + 587*int(none[1]) + 114*int(none[2])) / 1000; luma > 40 {
			t.Fatalf("stage at %v without a poster is %v (luma %d), too bright to be the stage", p, none, luma)
		}
	}
}
