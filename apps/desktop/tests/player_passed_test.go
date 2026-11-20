package tests

import (
	"context"
	"errors"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

// lineSource offers one episode on two lines, both of them real media, so the
// head check passes for either and the line the page resolved is the one that
// opens. It records the lines the page asked for, in order.
type lineSource struct {
	detail  source.Detail
	episode source.Episode
	lines   []source.Line
	served  *httptest.Server

	mu    sync.Mutex
	asked []string
}

func newLineSource(t *testing.T) *lineSource {
	t.Helper()
	sample, _ := sampleTitle()
	return &lineSource{
		detail:  sample.detail,
		episode: source.Episode{Media: sample.detail.Ref, ID: "one", Number: 1, Title: "Episode 1"},
		lines:   []source.Line{{ID: "review", Label: "review"}, {ID: "next", Label: "next"}},
		served:  served(t),
	}
}

func (s *lineSource) ID() string   { return "fake" }
func (s *lineSource) Name() string { return "Fake Source" }

func (s *lineSource) Home(context.Context) (source.Home, error) { return source.Home{}, nil }

func (s *lineSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, nil
}

func (s *lineSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, nil
}

func (s *lineSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return s.detail, nil
}

func (s *lineSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return []source.Episode{s.episode}, nil
}

// Resolve answers on the line asked for, or on the episode's first line when no
// line is named, which is how a source picks one for itself.
func (s *lineSource) Resolve(_ context.Context, _ source.Episode, line string) (source.Playback, error) {
	chosen := s.lines[0]
	if line != "" {
		for _, offered := range s.lines {
			if offered.ID == line {
				chosen = offered
			}
		}
	}
	s.mu.Lock()
	s.asked = append(s.asked, chosen.ID)
	s.mu.Unlock()
	// The lines are copied out: the page writes the playback it resolved, and
	// that must not reach into what this source set up.
	return source.Playback{
		URL:   s.served.URL + "/" + chosen.ID + ".mp4",
		Line:  chosen,
		Lines: append([]source.Line(nil), s.lines...),
	}, nil
}

// opened reports the lines resolved so far, in order.
func (s *lineSource) opened() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.asked...)
}

// The line a stream that gave no picture came from is passed over, and the
// episode opens on the next line that answers with media, so a viewer whose
// stream drew nothing gets a picture from another route instead of the same
// failure again.
func TestPlayerPassesOverALineThatGaveNoPicture(t *testing.T) {
	src := newLineSource(t)
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	opened := make(chan string, 4)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(url string) (*ui.VideoPlayer, error) {
			opened <- url
			return newGIFPlayer(t), nil
		},
	})
	page.Open(src.detail, episodes, 0)
	apply(t, run)
	if !page.RetryOtherLine() {
		t.Fatal("a line that gave no picture must give way to the next one")
	}
	apply(t, run)
	if got := src.opened(); len(got) != 2 || got[0] != "review" || got[1] != "next" {
		t.Fatalf("the player resolved %v, want the failing line passed over", got)
	}
	if got := <-opened; got == "" {
		t.Fatal("the next line opened nothing")
	}
}

// A viewer who asks for another line when the episode offers only one is told
// the stream gave no picture, rather than left waiting on the same line again.
func TestPlayerReportsTheOnlyLineThatGaveNoPicture(t *testing.T) {
	src := newLineSource(t)
	src.lines = src.lines[:1]
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
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
	page.Open(src.detail, episodes, 0)
	apply(t, run)
	page.RetryOtherLine()
	apply(t, run)
	if got := src.opened(); len(got) != 1 {
		t.Fatalf("the player resolved %v, want the line it already had", got)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	m := locales.In("en")
	if !tt.HasText(m.NoPicture()) {
		t.Fatalf("a stream with no other line is not reported: %v", tt.Texts())
	}
}

// An engine that refuses the stream of the line it opened gives way to another
// line, the same as a stream that drew no picture: the stage reports a failure
// only once the episode has no line left.
func TestPlayerPassesOverALineTheEngineRefused(t *testing.T) {
	src := newLineSource(t)
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(url string) (*ui.VideoPlayer, error) {
			if strings.Contains(url, "/review.mp4") {
				return nil, errors.New("test: the engine refused the stream")
			}
			return newGIFPlayer(t), nil
		},
	})
	page.Open(src.detail, episodes, 0)
	drain(t, run)
	if got := src.opened(); len(got) != 2 || got[0] != "review" || got[1] != "next" {
		t.Fatalf("the player resolved %v, want the refused line passed over", got)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if m := locales.In("en"); tt.HasText(m.PlaybackFailed()) || tt.HasText(m.NoPicture()) {
		t.Fatalf("a line that answered with media still reads as failed: %v", tt.Texts())
	}
}

// drain runs the work the page queued until it has none left, so a resolve that
// hands back an open, and an open that hands back another resolve, settle.
func drain(t *testing.T, run chan func()) {
	t.Helper()
	for empty := false; !empty; {
		select {
		case fn := <-run:
			fn()
		case <-time.After(200 * time.Millisecond):
			empty = true
		}
	}
}

// Every line of an episode that the engine refuses is passed over once, and the
// stage then reports the failure: a line is never opened again, so a viewer is
// not left watching the same wait restart.
func TestPlayerSettlesWhenEveryLineIsRefused(t *testing.T) {
	src := newLineSource(t)
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 64)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) {
			return nil, errors.New("test: the engine refused the stream")
		},
	})
	page.Open(src.detail, episodes, 0)
	drain(t, run)
	if got := src.opened(); len(got) != 2 {
		t.Fatalf("the player resolved %v, want each line tried once", got)
	}
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if m := locales.In("en"); !tt.HasText(m.PlaybackFailed()) {
		t.Fatalf("a stage with no line left does not report the failure: %v", tt.Texts())
	}
}

// A line that failed stays failed: the frames the stage draws after the failure
// do not resolve it again, so a viewer sees the failure once instead of a wait
// that restarts every frame.
func TestPlayerDoesNotRetryALineItAlreadyFailed(t *testing.T) {
	src := newLineSource(t)
	src.lines = src.lines[:1]
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 64)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) {
			return nil, errors.New("test: the engine refused the stream")
		},
	})
	page.Open(src.detail, episodes, 0)
	drain(t, run)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	for i := 0; i < 5; i++ {
		tt.Frame()
		drain(t, run)
	}
	if got := src.opened(); len(got) != 1 {
		t.Fatalf("the player resolved %v, want the failed line left alone", got)
	}
	if !tt.HasText(locales.In("en").PlaybackFailed()) {
		t.Fatalf("the failure is not on the stage: %v", tt.Texts())
	}
}
