package tests

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/search"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

// searchSource answers List with one title per keyword and nothing else, so a
// test drives the search destination the way the window wires it.
type searchSource struct{}

func (searchSource) ID() string   { return "fake" }
func (searchSource) Name() string { return "Fake" }
func (searchSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "home"}
}
func (searchSource) List(_ context.Context, query source.ListQuery) (source.Page, error) {
	title := "Nothing"
	if query.Keyword != "" {
		title = "Hit for " + query.Keyword
	}
	return source.Page{Cards: []source.Card{{
		Ref: source.Ref{Source: "fake", ID: "found"}, Title: title, Kind: source.Series, Year: 2026,
	}}}, nil
}
func (searchSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "detail"}
}
func (searchSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "episodes"}
}
func (searchSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "resolve"}
}
func (searchSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

// TestSearchDestinationReplacesThePlaceholder drives the same switch the window
// builds: the Search destination draws the search page, and a submitted
// keyword puts a source answer on it.
func TestSearchDestinationReplacesThePlaceholder(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := search.New(search.Deps{
		Source: searchSource{}, Library: store, Run: func(fn func()) { run <- fn },
	})
	shell := appshell.New()
	shell.SetContent(func(c *ui.Context) ui.Element {
		if shell.Selected() == appshell.Search {
			return page.View(c)
		}
		return ui.Text(c, "OTHER")
	})

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(c.Theme())
		c.SetTheme(&theme)
		shell.View(c)
	}, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Search"); err != nil {
		t.Fatalf("navigate to search: %v", err)
	}
	tt.Frame()
	m := locales.In("en")
	if !tt.HasText(m.SearchIdle()) {
		t.Fatalf("search destination did not draw its page: %v", tt.Texts())
	}
	if tt.HasText(m.PageComing()) {
		t.Fatalf("search still shows the placeholder: %v", tt.Texts())
	}

	page.Search("dune")
	select {
	case fn := <-run:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("search answer never arrived")
	}
	tt.Frame()
	if !tt.HasText("Hit for dune") {
		t.Fatalf("search result missing in %v", tt.Texts())
	}
}
