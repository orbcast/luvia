package catalog

import (
	"context"
	"errors"
	"image"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// fakeSource answers List with a fixed page and remembers the last query so a
// test can prove a category change asked the source again.
type fakeSource struct {
	mu    sync.Mutex
	page  source.Page
	err   error
	calls int
	last  source.ListQuery
}

func (f *fakeSource) ID() string   { return "fake" }
func (f *fakeSource) Name() string { return "Fake" }

func (f *fakeSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, errors.New("unused")
}

func (f *fakeSource) List(_ context.Context, query source.ListQuery) (source.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = query
	return f.page, f.err
}

func (f *fakeSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, errors.New("unused")
}

func (f *fakeSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, errors.New("unused")
}

func (f *fakeSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, errors.New("unused")
}
func (f *fakeSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func (f *fakeSource) answer(page source.Page, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.page, f.err = page, err
}

func (f *fakeSource) state() (int, source.ListQuery) {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls, f.last
}

func sampleCards() []source.Card {
	return []source.Card{
		{Ref: source.Ref{Source: "fake", ID: "dune"}, Title: "Dune", Kind: source.Movie,
			Year: 2021, Tags: []string{"Sci-Fi", "Adventure"}, Remarks: "4K"},
		{Ref: source.Ref{Source: "fake", ID: "severance"}, Title: "Severance", Kind: source.Series,
			Year: 2022, Tags: []string{"Drama"}, Remarks: "updating"},
	}
}

// newPage builds a page whose main-thread callbacks are queued for the test to
// run on its own goroutine, mirroring the app's hand-off to the main thread so
// state is never written while a frame is building.
func newPage(t *testing.T, deps Deps) (*Page, chan func()) {
	t.Helper()
	queued := make(chan func(), 32)
	deps.Run = func(fn func()) { queued <- fn }
	if deps.Library == nil {
		store, err := library.Open("")
		if err != nil {
			t.Fatal(err)
		}
		deps.Library = store
	}
	page := New(deps)
	page.redraw = func() {}
	return page, queued
}

// wait runs the next queued main-thread callback, applying its state change.
func wait(t *testing.T, queued chan func()) {
	t.Helper()
	select {
	case fn := <-queued:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("the catalog request never finished")
	}
}

func load(t *testing.T, page *Page, queued chan func()) {
	t.Helper()
	page.Start()
	wait(t, queued)
}

// render builds a tester over the page, applying the design theme the AppShell
// would, so the page draws with its real palette in tests too.
func render(page *Page, width, height int, dark bool) *ui.Tester {
	tt := ui.NewTesterWith(func(c *ui.Context) {
		theme := design.Theme(c.Theme())
		c.SetTheme(&theme)
		page.View(c)
	}, width, height, ui.TesterOptions{Dark: dark})
	tt.SetLocale("en")
	return tt
}

func TestLibraryDrawsChromeAndPosters(t *testing.T) {
	src := &fakeSource{page: source.Page{Cards: sampleCards()}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()

	for _, want := range []string{
		"Library", "All", "Movies", "Series", "Anime", "Variety", "Shorts",
		"All titles", "Favorites", "All types", "All status", "Default order",
		"Dune", "Severance", "4K", "1080P", "Completed", "Updating",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if page.category != catAll || !page.grid {
		t.Fatalf("defaults are category %d grid %v, want all/grid", page.category, page.grid)
	}
}

func TestSelectingCategoryReloads(t *testing.T) {
	src := &fakeSource{page: source.Page{Cards: sampleCards()}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()

	before, _ := src.state()
	if err := tt.Click("Movies"); err != nil {
		t.Fatalf("click Movies: %v", err)
	}
	tt.Frame()
	wait(t, finished)
	tt.Frame()

	after, last := src.state()
	if after != before+1 {
		t.Fatalf("source listed %d times, want %d", after, before+1)
	}
	if last.Kind != source.Movie {
		t.Fatalf("query kind %q, want movie", last.Kind)
	}
	if page.category != catMovie {
		t.Fatalf("category %d, want catMovie", page.category)
	}
}

func TestShortCategoryFiltersMetadata(t *testing.T) {
	cards := append(sampleCards(), source.Card{
		Ref: source.Ref{Source: "fake", ID: "mini"}, Title: "Mini", Kind: source.Series,
		Tags: []string{"short"},
	})
	src := &fakeSource{page: source.Page{Cards: cards}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()

	if err := tt.Click("Shorts"); err != nil {
		t.Fatalf("click Shorts: %v", err)
	}
	tt.Frame()
	wait(t, finished)
	tt.Frame()

	if !tt.HasText("Mini") {
		t.Fatalf("short drama not listed: %v", tt.Texts())
	}
	if tt.HasText("Dune") || tt.HasText("Severance") {
		t.Fatalf("short tab kept non-short titles: %v", tt.Texts())
	}
}

func TestEmptyState(t *testing.T) {
	src := &fakeSource{page: source.Page{}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()
	if !tt.HasText("No titles here yet") {
		t.Fatalf("missing empty state in %v", tt.Texts())
	}
}

func TestErrorStateAndRetry(t *testing.T) {
	src := &fakeSource{err: errors.New("boom")}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()
	if !tt.HasText("This library could not load") || !tt.HasText("Retry") {
		t.Fatalf("missing error state in %v", tt.Texts())
	}

	src.answer(source.Page{Cards: sampleCards()}, nil)
	if err := tt.Click("Retry"); err != nil {
		t.Fatalf("click Retry: %v", err)
	}
	tt.Frame()
	wait(t, finished)
	tt.Frame()
	if page.err != nil {
		t.Fatalf("retry kept the error: %v", page.err)
	}
	if !tt.HasText("Dune") {
		t.Fatalf("retry did not load titles: %v", tt.Texts())
	}
}

func TestFavoritesFilterNarrowsToFollowed(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	kept := source.Ref{Source: "fake", ID: "dune"}
	if err := store.Follow(library.Follow{Media: kept, Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	src := &fakeSource{page: source.Page{Cards: sampleCards()}}
	page, finished := newPage(t, Deps{Source: src, Library: store})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()

	if err := tt.Click("Favorites"); err != nil {
		t.Fatalf("click Favorites: %v", err)
	}
	tt.Frame()

	if !tt.HasText("Dune") {
		t.Fatalf("followed title missing: %v", tt.Texts())
	}
	if tt.HasText("Severance") {
		t.Fatalf("favorite filter kept an unfollowed title: %v", tt.Texts())
	}
}

func TestListToggleSwitchesLayout(t *testing.T) {
	src := &fakeSource{page: source.Page{Cards: sampleCards()}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, false)
	tt.Frame()
	if !page.grid {
		t.Fatal("default layout is not grid")
	}
	if err := tt.Click("List view"); err != nil {
		t.Fatalf("click List view: %v", err)
	}
	tt.Frame()
	if page.grid {
		t.Fatal("list toggle did not switch the layout")
	}
	if !tt.HasText("Dune") {
		t.Fatalf("list kept titles: %v", tt.Texts())
	}
}

func TestDarkThemeDrawsTheLibrary(t *testing.T) {
	src := &fakeSource{page: source.Page{Cards: sampleCards()}}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	tt := render(page, 1100, 720, true)
	tt.Frame()
	if !tt.HasText("Library") || !tt.HasText("Dune") {
		t.Fatalf("dark render missing content in %v", tt.Texts())
	}
	img := tt.Image()
	if img == nil {
		t.Fatal("no image")
	}
	// The left margin is page background, which must be the dark near-black.
	if r, g, b := pixel(img, 6, 400); r+g+b > 90 {
		t.Fatalf("dark page background is (%d,%d,%d), want near black", r, g, b)
	}
}

func pixel(img *image.RGBA, x, y int) (uint8, uint8, uint8) {
	if x < 0 || y < 0 || x >= img.Bounds().Dx() || y >= img.Bounds().Dy() {
		return 0, 0, 0
	}
	c := img.RGBAAt(x, y)
	return c.R, c.G, c.B
}
