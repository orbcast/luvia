package tests

import (
	"context"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// commentingSource answers its episode with a media address and a fixed set of
// comments pinned to moments of it.
type commentingSource struct {
	detail   source.Detail
	episodes []source.Episode
	comments []source.Danmaku
}

func (s *commentingSource) ID() string   { return "fake" }
func (s *commentingSource) Name() string { return "Fake Source" }
func (s *commentingSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, nil
}
func (s *commentingSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, nil
}
func (s *commentingSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return s.detail, nil
}
func (s *commentingSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return s.episodes, nil
}
func (s *commentingSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func (s *commentingSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{
		URL:   "https://cdn.example/film.mp4",
		Line:  source.Line{ID: "l1", Label: "Line 1"},
		Lines: []source.Line{{ID: "l1", Label: "Line 1"}},
	}, nil
}

func (s *commentingSource) Comments(context.Context, source.Episode, int) ([]source.Danmaku, error) {
	return s.comments, nil
}

// TestPlayerShowsCommentsAsDanmaku covers the comment layer: the source's
// comments load for the episode, and the ones whose moment has come cross the
// picture, until the switch turns the layer off.
func TestPlayerShowsCommentsAsDanmaku(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{
		detail:   base.detail,
		episodes: base.episodes,
		// The second comment is pinned to the top of the band, so a source's
		// plain comments and its pinned ones are both exercised: the first
		// scrolls, the second sits centered in its lane.
		comments: []source.Danmaku{
			{At: 0, Text: "Opening line"},
			{At: 0, Text: "Pinned line"},
			{At: time.Hour, Text: "Far away line"},
		},
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	apply(t, run)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	// The layer opens off, so the viewer asks for it first; a source's own
	// comments crossing the picture is what the switch is for.
	if page.DanmakuOn() {
		t.Fatal("the comment layer must open off")
	}
	showDanmaku(t, page, tt)
	if !pumpUntil(t, run, tt, "Opening line") {
		t.Fatalf("the comment at the start of the episode is not on the picture: %v", tt.Texts())
	}
	if tt.HasText("Far away line") {
		t.Fatal("a comment an hour in must not show at the start")
	}

	page.ToggleDanmaku()
	tt.Frame()
	if tt.HasText("Opening line") {
		t.Fatal("turning danmaku off left a comment on the picture")
	}
}

// TestPlayerSpreadsUntimedComments covers a source that ships reviews instead of
// timed comments: every entry carries no moment, so they drift across the
// episode one after another rather than all landing at the start.
func TestPlayerSpreadsUntimedComments(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{
		detail:   base.detail,
		episodes: base.episodes,
		comments: []source.Danmaku{
			{Text: "First review", Author: "ann"},
			{Text: "Second review", Author: "bo"},
			{Text: "Third review", Author: "cy"},
		},
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)
	apply(t, run)
	apply(t, run)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	showDanmaku(t, page, tt)
	if !pumpUntil(t, run, tt, "First review") {
		t.Fatalf("the first untimed review is not on the picture: %v", tt.Texts())
	}
	if tt.HasText("Third review") {
		t.Fatal("a review far into the episode must not show at the start")
	}

	page.SelectTab(1)
	page.RevealPanel()
	tt.Frame()
	for _, want := range []string{"First review", "Second review", "Third review", "ann", "cy"} {
		if !pumpUntil(t, run, tt, want) {
			t.Fatalf("the comments tab is missing %q: %v", want, tt.Texts())
		}
	}
}
