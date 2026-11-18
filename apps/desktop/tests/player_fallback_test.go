package tests

import (
	"context"
	"sync/atomic"
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// posterSource answers the default line with a poster image, as some lines do
// while claiming mp4, and the other lines with media.
type posterSource struct {
	detail   source.Detail
	episodes []source.Episode
	lines    []source.Line
	urls     map[string]string
	resolves atomic.Int32
}

func (s *posterSource) ID() string   { return "fake" }
func (s *posterSource) Name() string { return "Fake Source" }
func (s *posterSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, nil
}
func (s *posterSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, nil
}
func (s *posterSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return s.detail, nil
}
func (s *posterSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return s.episodes, nil
}
func (s *posterSource) Resolve(_ context.Context, ep source.Episode, line string) (source.Playback, error) {
	s.resolves.Add(1)
	if line == "" {
		line = s.lines[0].ID
	}
	chosen := s.lines[0]
	for _, l := range s.lines {
		if l.ID == line {
			chosen = l
		}
	}
	return source.Playback{URL: s.urls[line], Line: chosen, Lines: s.lines}, nil
}
func (s *posterSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func TestPlayerFallsBackFromPosterLine(t *testing.T) {
	src, _ := sampleTitle()
	poster := &posterSource{
		detail:   src.detail,
		episodes: src.episodes,
		lines: []source.Line{
			{ID: "poster", Label: "Poster"},
			{ID: "broken", Label: "Broken"},
			{ID: "film", Label: "Film"},
		},
		urls: map[string]string{
			"poster": "https://p9-heycan-sign.byteimg.com/tos-cn-i-3jr8j4ixpe/cover~tplv-image.png",
			"broken": "not a url",
			"film":   "https://lf3-csp-sign.bytetos.com/tos-cn-v-x/clip.mp4",
		},
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	var opened []string
	page := player.New(player.Deps{
		Source: poster, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(url string) (*ui.VideoPlayer, error) {
			opened = append(opened, url)
			return newGIFPlayer(t), nil
		},
	})
	page.Open(poster.detail, poster.episodes, 0)
	apply(t, run)

	if len(opened) != 1 {
		t.Fatalf("player opened %d videos", len(opened))
	}
	if want := poster.urls["film"]; opened[0] != want {
		t.Fatalf("the player opened %q, want the first playable line %q", opened[0], want)
	}
}
