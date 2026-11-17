package tests

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/catalog"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// catalogSource answers List with a small, category-filtered catalog and
// everything else as unsupported, so the resource library can be driven in a
// test without a network.
type catalogSource struct{ cards []source.Card }

func (s *catalogSource) ID() string   { return "fake" }
func (s *catalogSource) Name() string { return "Fake" }
func (s *catalogSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "home"}
}
func (s *catalogSource) List(_ context.Context, q source.ListQuery) (source.Page, error) {
	found := make([]source.Card, 0, len(s.cards))
	for _, card := range s.cards {
		if q.Kind == "" || card.Kind == q.Kind {
			found = append(found, card)
		}
	}
	return source.Page{Cards: found, Page: 1}, nil
}
func (s *catalogSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "detail"}
}
func (s *catalogSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "episodes"}
}
func (s *catalogSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "resolve"}
}

func catalogCards() []source.Card {
	return []source.Card{
		{Ref: source.Ref{Source: "fake", ID: "interstellar"}, Title: "Interstellar", Kind: source.Movie,
			Year: 2014, Area: "US", Tags: []string{"4K", "Sci-Fi"}, Remarks: "4K"},
		{Ref: source.Ref{Source: "fake", ID: "severance"}, Title: "Severance", Kind: source.Series,
			Year: 2022, Area: "US", Tags: []string{"Sci-Fi"}},
		{Ref: source.Ref{Source: "fake", ID: "spirited"}, Title: "Spirited Away", Kind: source.Anime,
			Year: 2001, Area: "JP"},
		{Ref: source.Ref{Source: "fake", ID: "minidrama"}, Title: "Mini Drama", Kind: source.Series,
			Year: 2025, Tags: []string{"short"}},
	}
}

// newCatalogPage wires a page whose main-thread callbacks are queued rather
// than run inline, so a test applies them itself on its own goroutine and the
// load never races a frame.
func newCatalogPage(t *testing.T) (*catalog.Page, *library.Store, chan func()) {
	t.Helper()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := catalog.New(catalog.Deps{
		Source:  &catalogSource{cards: catalogCards()},
		Library: store,
		Run:     func(fn func()) { run <- fn },
	})
	return page, store, run
}

// apply runs the next queued main-thread callback, waiting for it if needed.
func apply(t *testing.T, run chan func()) {
	t.Helper()
	select {
	case fn := <-run:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("catalog callback never ran")
	}
}

func TestCatalogRendersAndFiltersCategories(t *testing.T) {
	page, _, run := newCatalogPage(t)
	page.Start()
	apply(t, run)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(c.Theme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	for _, text := range []string{"Library", "All", "Movies", "Series", "Anime", "Variety", "Shorts", "Interstellar", "Severance"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	// The movie tab reloads and narrows the grid to films only.
	if err := tt.Click("Movies"); err != nil {
		t.Fatalf("movie tab: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Interstellar") {
		t.Fatalf("movie tab lost its film in %v", tt.Texts())
	}
	if tt.HasText("Severance") {
		t.Fatalf("movie tab still lists a series: %v", tt.Texts())
	}

	// The short-drama tab narrows the unfiltered listing by metadata.
	if err := tt.Click("Shorts"); err != nil {
		t.Fatalf("short tab: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Mini Drama") {
		t.Fatalf("short tab lost its title in %v", tt.Texts())
	}
	if tt.HasText("Interstellar") {
		t.Fatalf("short tab still lists a movie: %v", tt.Texts())
	}
}

// TestShellNavigatesToCatalog mirrors the window wiring: the resource library
// destination shows the catalog page while other destinations stay placeholders.
func TestShellNavigatesToCatalog(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := catalog.New(catalog.Deps{
		Source:  &catalogSource{cards: catalogCards()},
		Library: store,
		Run:     func(fn func()) { run <- fn },
	})
	shell := appshell.New()
	shell.SetContent(func(c *ui.Context) ui.Element {
		if shell.Selected() == appshell.Library {
			return page.View(c)
		}
		return ui.Column(c).Fill().Background(c.Theme().Background)
	})
	tt := ui.NewTester(shell.View, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click(appshell.Library.Label(english)); err != nil {
		t.Fatalf("library nav: %v", err)
	}
	apply(t, run)
	tt.Frame()
	if !tt.HasText("Interstellar") {
		t.Fatalf("library destination did not show the catalog in %v", tt.Texts())
	}
}
