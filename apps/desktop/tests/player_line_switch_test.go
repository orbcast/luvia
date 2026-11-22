package tests

import (
	"context"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// newLineTester builds a player over the two-line source, with the side panel
// out so the line dropdown is on screen, and settles it.
func newLineTester(t *testing.T, src source.Source) (*player.Page, chan func(), *ui.Tester) {
	t.Helper()
	withCatalogue := src.(interface {
		Detail(context.Context, source.Ref) (source.Detail, error)
		Episodes(context.Context, source.Ref) ([]source.Episode, error)
	})
	detail, err := withCatalogue.Detail(context.Background(), source.Ref{Source: "fake", ID: "d1"})
	if err != nil {
		t.Fatal(err)
	}
	episodes, err := withCatalogue.Episodes(context.Background(), detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 16)
	var tt *ui.Tester
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	tt = ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	page.Open(detail, episodes, 0)
	drain(t, run)
	page.RevealPanel()
	tt.Frame()
	drain(t, run)
	tt.Frame()
	return page, run, tt
}

// rebindingSource answers every resolve on the episode's first line whatever
// was asked, as a source whose line ids have moved on at the other end does.
type rebindingSource struct{ *lineSource }

func (r *rebindingSource) Resolve(ctx context.Context, ep source.Episode, line string) (source.Playback, error) {
	return r.lineSource.Resolve(ctx, ep, "review")
}

// TestLineSwitchShowsTheLineTheSourceAnswered covers a source that answers on
// another line than the one asked for: the selector says what plays, so the
// viewer is not told they are on a line the picture did not come from.
func TestLineSwitchShowsTheLineTheSourceAnswered(t *testing.T) {
	src := &rebindingSource{lineSource: newLineSource(t)}
	_, run, tt := newLineTester(t, src)

	if err := tt.Click("review"); err != nil {
		t.Fatalf("open the line dropdown: %v", err)
	}
	tt.Frame()
	if err := tt.Click("next"); err != nil {
		t.Fatalf("pick the second line: %v", err)
	}
	drain(t, run)
	tt.Frame()
	drain(t, run)
	tt.Frame()

	if tt.HasText("next") {
		t.Fatalf("the selector names a line the picture did not come from: %v", tt.Texts())
	}
	if !tt.HasText("review") {
		t.Fatalf("the selector does not name the line that played: %v", tt.Texts())
	}
}

// TestPickingAnotherLineResolvesTheEpisodeOnIt covers the playback-line
// dropdown: picking another line resolves the episode again on that line, and
// the selector then names the line the source answers on, which is the one the
// stream was opened from.
func TestPickingAnotherLineResolvesTheEpisodeOnIt(t *testing.T) {
	src := newLineSource(t)
	_, run, tt := newLineTester(t, src)
	// The first resolve came from opening the page; the pick comes after.
	before := len(src.opened())

	if err := tt.Click("review"); err != nil {
		t.Fatalf("open the line dropdown: %v", err)
	}
	tt.Frame()
	if err := tt.Click("next"); err != nil {
		t.Fatalf("pick the second line: %v", err)
	}
	drain(t, run)
	tt.Frame()
	drain(t, run)
	tt.Frame()

	calls := src.opened()
	if len(calls) <= before {
		t.Fatalf("picking another line did not resolve again: %v", calls)
	}
	if last := calls[len(calls)-1]; last != "next" {
		t.Fatalf("the last resolve asked for %q, wanted the picked line", last)
	}
	if !tt.HasText("next") {
		t.Fatalf("the selector does not name the line now playing: %v", tt.Texts())
	}
}
