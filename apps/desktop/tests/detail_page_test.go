package tests

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/detail"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// titleSource answers Detail, Episodes and Resolve from canned data so the
// title and player pages can be driven without a network.
type titleSource struct {
	detail   source.Detail
	episodes []source.Episode
	playback source.Playback
	resolves atomic.Int32
}

func (s *titleSource) ID() string   { return "fake" }
func (s *titleSource) Name() string { return "Fake Source" }
func (s *titleSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, nil
}
func (s *titleSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, nil
}
func (s *titleSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return s.detail, nil
}
func (s *titleSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return s.episodes, nil
}
func (s *titleSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	s.resolves.Add(1)
	return s.playback, nil
}
func (s *titleSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func sampleTitle() (*titleSource, source.Ref) {
	ref := source.Ref{Source: "fake", ID: "d1"}
	episodes := make([]source.Episode, 10)
	for i := range episodes {
		episodes[i] = source.Episode{
			Media: ref, ID: "ep-" + itoa(i+1), Number: i + 1, Title: "Episode " + itoa(i+1),
		}
	}
	detail := source.Detail{
		Ref: ref, Title: "Cyberpunk Edgerunners 2", Kind: source.Anime, Year: 2026,
		Area: "Japan", Tags: []string{"Sci-Fi", "Action"}, Overview: "A return to Night City.",
		Remarks: "10 episodes", Actors: []string{"Actor A", "Actor B"}, Directors: []string{"Director X"},
		Cast: []source.CastMember{
			{Name: "Actor A", Role: "饰 甲"},
			{Name: "Actor B", Role: "饰 乙"},
		},
		Crew: []source.CastMember{
			{Name: "Director X", Role: "导演"},
		},
		EpisodeCount: 10,
	}
	return &titleSource{
		detail:   detail,
		episodes: episodes,
		playback: source.Playback{URL: "", Line: source.Line{ID: "l1", Label: "Line 1"},
			Lines: []source.Line{{ID: "l1", Label: "Line 1"}, {ID: "l2", Label: "Line 2"}}},
	}, ref
}

func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [4]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + n%10)
		n /= 10
	}
	return string(buf[i:])
}

func newDetailTester(t *testing.T, page *detail.Page) *ui.Tester {
	t.Helper()
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	return tt
}

// TestDetailBackButtonKeepsTheChromeLine covers where the way out sits: on the
// window's one chrome line, at the page's own gutter, so it reads as part of
// the copy it returns to instead of floating on a number of its own.
func TestDetailBackButtonKeepsTheChromeLine(t *testing.T) {
	src, ref := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := detail.New(detail.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnPlay: func(source.Detail, []source.Episode, int) {},
		OnBack: func() {},
	})
	page.Open(ref)
	apply(t, run)

	for _, bar := range []ui.TitleBar{{}, {Height: 28, Bottom: 28, Left: 78}} {
		tt := ui.NewTester(func(c *ui.Context) {
			theme := design.Theme(ui.DarkTheme())
			c.SetTheme(&theme)
			page.View(c)
		}, 1180, 716)
		tt.SetTitleBar(bar)
		tt.SetLocale("en")
		tt.Frame()
		box, ok := tt.Find("Back")
		if !ok {
			t.Fatalf("the back button is missing in %v", tt.Texts())
		}
		if d := box.Y - design.ChromeTop(bar.Bottom); d < -1 || d > 1 {
			t.Fatalf("the back button starts at %v, not on the chrome line %v", box.Y, design.ChromeTop(bar.Bottom))
		}
		// The page's copy starts on the same gutter, so the button reads as
		// part of the column it returns to.
		copyBox, ok := tt.Find("Synopsis")
		if !ok {
			t.Fatalf("the page's copy is missing in %v", tt.Texts())
		}
		if box.X != copyBox.X {
			t.Fatalf("the back button starts at %v, not at the copy's gutter %v", box.X, copyBox.X)
		}
	}
}

func TestDetailRendersPlaysAndSwitchesTabs(t *testing.T) {
	src, ref := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	played, backed := -2, false
	page := detail.New(detail.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnPlay: func(_ source.Detail, _ []source.Episode, i int) { played = i },
		OnBack: func() { backed = true },
	})
	page.Open(ref)
	apply(t, run)

	tt := newDetailTester(t, page)
	for _, text := range []string{"Cyberpunk Edgerunners 2", "Sci-Fi", "2026", "Play", "Episodes", "Overview", "Cast", "Crew"} {
		if !tt.HasText(text) {
			t.Fatalf("missing %q in %v", text, tt.Texts())
		}
	}

	if err := tt.Click("Play"); err != nil {
		t.Fatalf("play: %v", err)
	}
	if played != 0 {
		t.Fatalf("play opened episode %d, want 0", played)
	}

	// An episode chip plays that episode.
	if err := tt.Click("Episode 2"); err != nil {
		t.Fatalf("episode: %v", err)
	}
	if played != 1 {
		t.Fatalf("episode click played %d, want 1", played)
	}

	// The cast tab swaps the body to the actor list.
	if err := tt.Click("Cast"); err != nil {
		t.Fatalf("cast tab: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Actor A") {
		t.Fatalf("cast tab did not list actors: %v", tt.Texts())
	}
	// The role each actor plays sits beside the name, so the tab says who
	// plays whom instead of listing bare names.
	if !tt.HasText("饰 甲") || !tt.HasText("饰 乙") {
		t.Fatalf("cast tab did not show the roles: %v", tt.Texts())
	}
	// The crew tab carries the job each member did.
	if err := tt.Click("Crew"); err != nil {
		t.Fatalf("crew tab: %v", err)
	}
	tt.Frame()
	if !tt.HasText("Director X") || !tt.HasText("导演") {
		t.Fatalf("crew tab did not show the jobs: %v", tt.Texts())
	}

	if err := tt.Click("Back"); err != nil {
		t.Fatalf("back: %v", err)
	}
	if !backed {
		t.Fatal("back did not call OnBack")
	}
}
