package tests

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/home"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

// homeSource answers Home with a small document and nothing else.
type homeSource struct {
	home source.Home
	done chan struct{}
}

func (h *homeSource) ID() string   { return "fake" }
func (h *homeSource) Name() string { return "Fake" }
func (h *homeSource) Home(context.Context) (source.Home, error) {
	defer close(h.done)
	return h.home, nil
}
func (h *homeSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "list"}
}
func (h *homeSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "detail"}
}
func (h *homeSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "episodes"}
}
func (h *homeSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "resolve"}
}
func (h *homeSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func discoverSource() *homeSource {
	movie := source.Card{Ref: source.Ref{Source: "fake", ID: "dune"}, Title: "Dune", Kind: source.Movie,
		Year: 2021, Tags: []string{"Sci-Fi", "Adventure"},
		Overview:  "A duke's son leads desert warriors against a galactic empire.",
		PosterURL: "https://cdn.example/dune.jpg", BackdropURL: "https://cdn.example/dune-wide.jpg"}
	series := source.Card{Ref: source.Ref{Source: "fake", ID: "severance"}, Title: "Severance",
		Kind: source.Series, Year: 2022, PosterURL: "https://cdn.example/severance.jpg",
		BackdropURL: "https://cdn.example/severance-wide.jpg"}
	return &homeSource{home: source.Home{Sections: []source.Section{
		{ID: "featured", Title: "Featured", Cards: []source.Card{movie, series}},
		{ID: "latest", Title: "Latest", Cards: []source.Card{series}},
	}}, done: make(chan struct{})}
}

func newDiscoverPage(t *testing.T) (*home.Page, *library.Store, *homeSource, <-chan struct{}) {
	t.Helper()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	src := discoverSource()
	finished := make(chan struct{}, 8)
	page := home.New(home.Deps{
		Source:  src,
		Library: store,
		Run: func(fn func()) {
			fn()
			finished <- struct{}{}
		},
		Now: func() time.Time { return time.Date(2026, 11, 15, 20, 0, 0, 0, time.Local) },
	})
	return page, store, src, finished
}

func waitFor(t *testing.T, finished <-chan struct{}) {
	t.Helper()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("home request never finished")
	}
}

func TestDiscoverRendersHeroAndLibraryState(t *testing.T) {
	page, store, _, finished := newDiscoverPage(t)
	page.Start()
	waitFor(t, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	for _, text := range []string{"Dune", "Continue Watching", "This Week", "Nothing watched yet"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	if err := tt.Click("Follow"); err != nil {
		t.Fatalf("follow button: %v", err)
	}
	if !store.IsFollowing(source.Ref{Source: "fake", ID: "dune"}) {
		t.Fatal("follow did not reach the library")
	}
	if !tt.HasText("Following") {
		t.Fatal("button did not switch to the followed state")
	}
}

func TestDiscoverShowsDetailsHint(t *testing.T) {
	page, _, _, finished := newDiscoverPage(t)
	page.Start()
	waitFor(t, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Details"); err != nil {
		t.Fatalf("details button: %v", err)
	}
	if !tt.HasText("The detail page is not connected yet.") {
		t.Fatalf("missing detail hint in %v", tt.Texts())
	}
}

func TestShellShowsDiscoverAndPlaceholder(t *testing.T) {
	shell := appshell.New()
	page, _, _, finished := newDiscoverPage(t)
	shell.SetContent(func(c *ui.Context) ui.Element {
		if shell.Selected() != appshell.Discover {
			return placeholderLike(c, shell.Selected())
		}
		return page.View(c)
	})
	page.Start()
	waitFor(t, finished)
	tt := ui.NewTester(shell.View, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Dune") {
		t.Fatalf("discover content missing in %v", tt.Texts())
	}
	if err := tt.Click(appshell.Following.Label(english)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText(english.PageComing()) {
		t.Fatalf("placeholder missing in %v", tt.Texts())
	}
	if err := tt.Click(appshell.Discover.Label(english)); err != nil {
		t.Fatal(err)
	}
	tt.Frame()
	if !tt.HasText("Dune") {
		t.Fatalf("discover content did not come back in %v", tt.Texts())
	}
}

// placeholderLike mirrors the window's placeholder page without pulling the
// main package into a test binary.
func placeholderLike(c *ui.Context, page appshell.Page) ui.Element {
	t := c.Theme()
	m := locales.In(c.Locale())
	column := ui.Column(c).Fill().Background(t.Background)
	column.Children(func() {
		ui.Text(c, page.Label(m)).FontSize(20)
		ui.Text(c, m.PageComing()).FontSize(13)
	})
	return column
}

func TestHeroActionsShareOneHeight(t *testing.T) {
	page, _, _, finished := newDiscoverPage(t)
	page.Start()
	waitFor(t, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")

	// Find returns the labeled button itself, which is the box the design is
	// about: both actions sit on one line and take one height.
	play, ok := tt.Find("Continue Playing")
	if !ok {
		t.Fatalf("no play action in %v", tt.Texts())
	}
	details, ok := tt.Find("Details")
	if !ok {
		t.Fatalf("no details action in %v", tt.Texts())
	}
	if play.H != 46 || details.H != 46 {
		t.Fatalf("actions are %v and %v tall, want 46", play.H, details.H)
	}
	if d := play.Y - details.Y; d > 0.5 || d < -0.5 {
		t.Fatalf("actions start at different heights: %v vs %v", play.Y, details.Y)
	}
	if play.W < 150 || details.W < 110 {
		t.Fatalf("actions are too tight: play %v wide, details %v wide", play.W, details.W)
	}
}

func TestDiscoverFitsANarrowWindow(t *testing.T) {
	page, _, _, finished := newDiscoverPage(t)
	page.Start()
	waitFor(t, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 460, 760)
	tt.SetLocale("en")

	inset := float32(design.NarrowGutter)
	play, ok := tt.Find("Continue Playing")
	if !ok {
		t.Fatalf("no play action in %v", tt.Texts())
	}
	if play.X < inset-1 || play.X+play.W > float32(460)-inset+1 {
		t.Fatalf("play action leaves the narrow page: %+v", play)
	}
	title, ok := tt.Find("Dune")
	if !ok {
		t.Fatalf("no hero title in %v", tt.Texts())
	}
	if title.X < inset-1 || title.X+title.W > float32(460)-inset+1 {
		t.Fatalf("hero title overflows the narrow page: %+v", title)
	}
}

// TestErrorPageClearsTheNavigation keeps the failure text readable on a wide
// window: the floating navigation sits in the first 56 DIPs, so the page
// starts at the page inset rather than the narrow gutter it used to.
func TestErrorPageClearsTheNavigation(t *testing.T) {
	page := home.New(home.Deps{
		Source: failingSource{},
		Run:    func(fn func()) { fn() },
		Now:    func() time.Time { return time.Date(2026, 11, 15, 20, 0, 0, 0, time.Local) },
	})
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1400, 900)
	tt.Frame()
	rect, ok := tt.Find("This source could not load")
	if !ok {
		t.Fatalf("error title not drawn: %v", tt.Texts())
	}
	if rect.X < design.ContentInset {
		t.Fatalf("title starts at %v, inside the navigation at %v", rect.X, design.ContentInset)
	}
	if w, ok := tt.Find("Retry"); ok && w.W > 200 {
		t.Fatalf("retry button is %v wide, want its label", w.W)
	}
}
