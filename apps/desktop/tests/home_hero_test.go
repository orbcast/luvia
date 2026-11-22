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

// artworkTransport serves one still for every request.
type artworkTransport struct{ png []byte }

func (a artworkTransport) RoundTrip(*http.Request) (*http.Response, error) {
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(a.png)),
		Header:     http.Header{"Content-Type": []string{"image/png"}},
	}, nil
}

// byURLTransport serves a different still per URL, so a test can tell a
// title's poster from its backdrop.
type byURLTransport struct{ png map[string][]byte }

func (b byURLTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	body, ok := b.png[req.URL.String()]
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(bytes.NewReader(nil))}, nil
	}
	return &http.Response{
		StatusCode: http.StatusOK,
		Body:       io.NopCloser(bytes.NewReader(body)),
		Header:     http.Header{"Content-Type": []string{"image/png"}},
	}, nil
}

// solidArtwork serves a flat color, which a sample can recognize whatever
// scrims or scaling sit on top.
func solidArtwork(t *testing.T, c color.RGBA) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 64, 64))
	for x := 0; x < 64; x++ {
		for y := 0; y < 64; y++ {
			img.Set(x, y, c)
		}
	}
	var buf bytes.Buffer
	if err := png.Encode(&buf, img); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
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

			// The sections begin right below the banner, so the page under them
			// is the page background, and the banner's own fade must reach it
			// without a step: no row on the way down may jump in color.
			section, ok := tt.Find("Continue Watching")
			if !ok {
				t.Fatalf("no section below the banner in %v", tt.Texts())
			}
			// The left edge carries nothing but the banner's own gradient above
			// the sections, so it shows the fade without any glyphs in it.
			const x = 8
			bottom := int(section.Y) - 4
			below := sampleAt(t, canvas, x, bottom)
			if d := delta(below, tc.page); d > 8 {
				t.Fatalf("page above the sections is %v, want the page background %v", below, tc.page)
			}
			prev := sampleAt(t, canvas, x, bottom)
			for y := bottom; y > 0; y -= 4 {
				here := sampleAt(t, canvas, x, y)
				if d := delta(here, prev); d > 12 {
					t.Fatalf("the banner steps from %v to %v at y=%d", prev, here, y)
				}
				prev = here
			}

			// White copy must stay readable over a white still. The glyphs
			// themselves are white, so this checks the darkest sample of the
			// band: it catches scrims that stop painting at all, and a caption
			// area that is uniformly bright, not every single pixel.
			title, ok := tt.Find("Doctor X")
			if !ok {
				t.Fatalf("no hero title in %v", tt.Texts())
			}
			x0, x1 := int(title.X), int(title.X+title.W)
			y0, y1 := int(title.Y), int(title.Y+160)
			if luma := minLuma(canvas, x0, y0, x1, y1); luma > 110 {
				t.Fatalf("caption band has minimum luminance %d, too bright for white text", luma)
			}
		})
	}
}

// coverEdgeGap is how far a sampled pixel sits from the cover card's edge: a
// column on the border itself is the card's own hairline, not a gap.
const coverEdgeGap = 2

// TestHeroCoverLeadsWithThePoster checks the banner's two layers: the poster
// keeps its own colors in the cover card at the left of the copy, while the
// ground behind is the backdrop, not a sharp copy of that poster. Different
// flat colors tell the two apart whatever scrims sit on top.
func TestHeroCoverLeadsWithThePoster(t *testing.T) {
	const width, height = 1200, 800
	const posterURL, backdropURL = "https://cdn.example/doctor.jpg", "https://cdn.example/doctor-wide.jpg"
	posterInk := color.RGBA{R: 220, A: 255}
	backdropInk := color.RGBA{B: 220, A: 255}
	for _, tc := range []struct {
		name string
		dark bool
	}{
		{name: "dark", dark: true},
		{name: "light", dark: false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			images := media.New(t.TempDir(), media.Options{Client: &http.Client{Transport: byURLTransport{png: map[string][]byte{
				posterURL:   solidArtwork(t, posterInk),
				backdropURL: solidArtwork(t, backdropInk),
			}}}})
			images.SetRunner(func(fn func()) { fn() })
			page := newHeroPage(t, images)
			images.Prefetch(posterURL)
			images.Prefetch(backdropURL)
			for i := 0; i < 300; i++ {
				if images.Bitmap(posterURL) != nil && images.Bitmap(backdropURL) != nil {
					break
				}
				time.Sleep(10 * time.Millisecond)
			}

			tt := ui.NewTesterWith(func(c *ui.Context) {
				theme := design.Theme(c.Theme())
				c.SetTheme(&theme)
				page.View(c)
			}, width, height, ui.TesterOptions{Dark: tc.dark})
			tt.SetLocale("en")
			canvas := tt.Image()
			if canvas == nil {
				t.Fatal("no image")
			}

			// The banner is 55% of the window and the cover hangs from the chrome
			// line, so mid-height still sits in the part of the backdrop the mask
			// keeps opaque.
			inset := int(design.Inset(width))
			y := int(0.5 * 0.55 * height)
			cover := sampleAt(t, canvas, inset+24, y)
			if !(cover[0] > 140 && cover[2] < 110) {
				t.Fatalf("cover at x=%d is %v, want the poster's red", inset+24, cover)
			}
			// The cover's top edge shares the AppShell's own line, so its
			// colors are already there just below where the column starts.
			top := sampleAt(t, canvas, inset+24, int(design.ChromeTop(0))+4)
			if !(top[0] > 140 && top[2] < 110) {
				t.Fatalf("cover top is %v at the chrome line, want the poster's red", top)
			}
			ground := sampleAt(t, canvas, width-20, y)
			if ground[2] < 90 || ground[2] < ground[0]+60 {
				t.Fatalf("banner ground is %v, want the backdrop's blue", ground)
			}

			title, ok := tt.Find("Doctor X")
			if !ok {
				t.Fatalf("no hero title in %v", tt.Texts())
			}
			// The copy starts after the poster's own edge, never on top of it. A
			// run of poster colour counts only once it is wider than a glyph: the
			// title's antialiasing over the backdrop's blue throws single red
			// pixels well to its right, which a bare per-column test would call
			// poster.
			right := inset
			for x := inset; x < width/2; x++ {
				c := sampleAt(nil, canvas, x, y)
				if c[0] > 140 && c[2] < 110 {
					right = x
				}
				if run := x - right; run >= 16 {
					break
				}
			}
			if title.X < float32(right)+8 {
				t.Fatalf("title starts at %v, inside the cover that reaches %d", title.X, right)
			}

			// The cover card is a solid block while the copy beside it is not, so
			// every column between the card and the title stays clear of poster
			// colour. One scan stop cannot prove that on its own: a pixel of the
			// title's own antialiasing over the backdrop's blue passes the colour
			// test and would carry the reach forward by tens of pixels.
			for x := right + coverEdgeGap; x < int(title.X); x++ {
				if c := sampleAt(nil, canvas, x, y); c[0] > 140 && c[2] < 110 {
					t.Fatalf("column %d is %v, poster colour inside the gap before the copy at %v", x, c, title.X)
				}
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
