package tests

import (
	"bytes"
	"image"
	"image/color"
	"image/gif"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/player"
)

// The picture keeps to its own box: the bands round a portrait frame stay the
// stage's colour right up to the frame, and not one pixel of the frame — nor of
// the window an engine draws it in — reaches them.
//
// This is the case the running app failed. A stage-wide video fitted to Contain
// paints inside a window of its own that is a few DIPs larger than the picture
// and fades the frame's edge into that window; over the stage, the window's apron
// laid a grey smudge under the picture (measured at #262731 in the running app,
// eleven DIPs deep and as wide as the frame) and a soft edge along the picture's
// rim. The frame of this case has white rows at its own top and bottom so the
// fade has something loud to leak: before the picture was clipped to its box,
// those rows tinted the apron grey.
func TestPlayerBandsStayClearOfThePicture(t *testing.T) {
	// 1280×900 stage, a 9:16 frame in it: the picture is 506×900 through the
	// middle, so x 387 to 893 is picture and everything beside it is band.
	for _, dark := range []bool{true, false} {
		src, _ := sampleTitle()
		src.detail.PosterURL = ""
		src.detail.BackdropURL = ""
		src.playback.URL = "mem://clip"
		store, err := library.Open("")
		if err != nil {
			t.Fatal(err)
		}
		images := media.New(t.TempDir(), media.Options{})
		images.SetRunner(func(fn func()) { fn() })
		page := player.New(player.Deps{
			Source: src, Library: store, Images: images,
			Run:       func(fn func()) { fn() },
			OpenVideo: func(string) (*ui.VideoPlayer, error) { return portraitFrame(t), nil },
		})
		page.Open(src.detail, src.episodes, 0)
		tt := ui.NewTester(func(c *ui.Context) {
			theme := design.Theme(ui.DarkTheme())
			if !dark {
				theme = design.Theme(ui.LightTheme())
			}
			c.SetTheme(&theme)
			page.View(c)
		}, 1280, 900)
		tt.SetLocale("en")
		tt.Frame()
		shot := tt.Image()
		if shot == nil {
			t.Fatal("no image")
		}
		darkTheme := design.Theme(ui.DarkTheme())
		stage := design.PlayerStage(&darkTheme)
		bare := [3]uint8{stage.R, stage.G, stage.B}
		if !dark {
			light := design.Theme(ui.LightTheme())
			stage = design.PlayerStage(&light)
			bare = [3]uint8{stage.R, stage.G, stage.B}
		}
		// Beside the picture at several heights, and under and over it where the
		// apron used to show: none of them carries anything but the stage.
		for _, y := range []int{120, 300, 450, 600, 770} {
			for _, x := range []int{120, 380, 392, 700, 888, 900, 1160} {
				if x > 387 && x < 893 && y > 0 && y < 900 {
					continue // inside the picture
				}
				got := sampleAt(t, shot, x, y)
				if d := delta(got, bare); d > 16 {
					t.Fatalf("dark=%v: band at %d,%d is %v, %d from the stage's %v: the picture reaches past its box", dark, x, y, got, d, bare)
				}
			}
		}
		// The picture's own bottom edge is picture, not band: a white frame row
		// laid through it has to stay white at the picture's last row.
		if got := sampleAt(t, shot, 640, 898); lumaOf(got) < 200 {
			t.Fatalf("dark=%v: the frame's last row reads %v (luma %d): the picture was cut short", dark, got, lumaOf(got))
		}
	}
}

// portraitFrame is a one-frame 9:16 GIF with white rows along its top and
// bottom edge, as a frame that fades out to white does.
func portraitFrame(t *testing.T) *ui.VideoPlayer {
	t.Helper()
	const w, h = 36, 64
	g := &gif.GIF{Config: image.Config{Width: w, Height: h}}
	frame := image.NewPaletted(image.Rect(0, 0, w, h), color.Palette{
		color.RGBA{R: 200, G: 60, B: 40, A: 255},
		color.RGBA{R: 255, G: 255, B: 255, A: 255},
	})
	for y := 0; y < h; y++ {
		for x := 0; x < w; x++ {
			if y < 2 || y >= h-2 {
				frame.SetColorIndex(x, y, 1)
			}
		}
	}
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
