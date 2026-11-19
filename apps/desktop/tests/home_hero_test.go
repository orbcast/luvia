package tests

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/png"
	"io"
	"net/http"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/home"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
)

// whiteArtwork serves a plain white still, the worst case for banner text.
func whiteArtwork(t *testing.T, width, height int) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, width, height))
	for x := 0; x < width; x++ {
		for y := 0; y < height; y++ {
			img.Set(x, y, color.White)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

type artworkTransport struct{ png []byte }

func (a artworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(a.png)),
		Header:     http.Header{"Content-Type": []string{"image/png"}},
	}, nil
}

func newHeroPage(t *testing.T, images *media.Loader) *home.Page {
	t.Helper()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	card := source.Card{Ref: source.Ref{Source: "fake", ID: "doctor"}, Title: "Doctor X", Kind: source.Series,
		Year: 2026, Tags: []string{"Drama"}, Overview: "A surgeon who cannot lose.",
		PosterURL: "https://cdn.example/doctor.jpg", BackdropURL: "https://cdn.example/doctor-wide.jpg"}
	other := source.Card{Ref: source.Ref{Source: "fake", ID: "dune"}, Title: "Dune", Kind: source.Movie,
		PosterURL: "https://cdn.example/dune.jpg", BackdropURL: "https://cdn.example/dune-wide.jpg"}
	src := &homeSource{home: source.Home{Sections: []source.Section{
		{ID: "featured", Title: "Featured", Cards: []source.Card{card, other}},
		{ID: "latest", Title: "Latest", Cards: []source.Card{other}},
	}}, done: make(chan struct{})}
	settled := make(chan struct{}, 8)
	page := home.New(home.Deps{
		Source:  src,
		Library: store,
		Images:  images,
		Run: func(fn func()) {
			fn()
			settled <- struct{}{}
		},
		Now: func() time.Time { return time.Date(2026, 11, 16, 20, 0, 0, 0, time.Local) },
	})
	// The page loads its source on the first frame; finish that answer before
	// the test observes the window, as the app's main thread would.
	page.Start()
	select {
	case <-settled:
	case <-time.After(5 * time.Second):
		t.Fatal("home request never finished")
	}
	return page
}

func TestHeroBlendsIntoThePage(t *testing.T) {
	const width, height = 1200, 800
	// The hero is clamped to 62% of the window between 380 and 620 points.
	heroHeight := int(0.62 * height)
	for _, tc := range []struct {
		name string
		dark bool
		page [3]uint8
	}{
		{name: "dark", dark: true, page: [3]uint8{0x0B, 0x0A, 0x0F}},
		{name: "light", dark: false, page: [3]uint8{0xF3, 0xEE, 0xF6}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			images := media.New(t.TempDir(), media.Options{Client: &http.Client{Transport: artworkTransport{whiteArtwork(t, 64, 36)}}})
			images.SetRunner(func(fn func()) { fn() })
			page := newHeroPage(t, images)
			images.Prefetch("https://cdn.example/doctor-wide.jpg")

			tt := ui.NewTesterWith(func(c *ui.Context) {
				theme := design.Theme(c.Theme())
				c.SetTheme(&theme)
				page.View(c)
			}, width, height, ui.TesterOptions{Dark: tc.dark})
			for i := 0; i < 200; i++ {
				if images.Bitmap("https://cdn.example/doctor-wide.jpg") != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}
			canvas := tt.Image()
			if canvas == nil {
				t.Fatal("no image")
			}

			below := sampleAt(t, canvas, width/2, heroHeight+8)
			if d := delta(below, tc.page); d > 8 {
				t.Fatalf("page below the banner is %v, want the page background %v", below, tc.page)
			}
			edge := sampleAt(t, canvas, width/2, heroHeight-2)
			if d := delta(edge, below); d > 12 {
				t.Fatalf("banner bottom %v does not meet the page %v (delta %d)", edge, below, d)
			}
			// White copy must stay readable over a white still. The glyphs
			// themselves are white, so this checks the darkest sample of the
			// band: it catches scrims that stop painting at all, and a caption
			// area that is uniformly bright, not every single pixel.
			if luma := minLuma(canvas, 110, heroHeight-260, 700, heroHeight-120); luma > 110 {
				t.Fatalf("caption band has minimum luminance %d, too bright for white text", luma)
			}
		})
	}
}

func minLuma(img image.Image, x0, y0, x1, y1 int) int {
	best := 255
	for y := y0; y <= y1; y += 10 {
		for x := x0; x <= x1; x += 10 {
			c := sampleAt(nil, img, x, y)
			luma := (299*int(c[0]) + 587*int(c[1]) + 114*int(c[2])) / 1000
			if luma < best {
				best = luma
			}
		}
	}
	return best
}

func sampleAt(t *testing.T, img image.Image, x, y int) [3]uint8 {
	if t != nil {
		t.Helper()
	}
	r, g, b, _ := img.At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b >> 8)}
}

func delta(a, b [3]uint8) int {
	d := 0
	for i := range a {
		v := int(a[i]) - int(b[i])
		if v < 0 {
			v = -v
		}
		if v > d {
			d = v
		}
	}
	return d
}

var _ = context.Background
