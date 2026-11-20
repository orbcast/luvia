package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
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
	// The poster is the original line and says media; the CDN answers it with
	// a still, so the next line that answers with media takes over.
	addresses := map[string]string{
		"poster": "https://p9-heycan-sign.byteimg.com/tos-cn-i-3jr8j4ixpe/cover~tplv-image.png",
		"film":   served(t).URL + "/clip.mp4",
	}
	got := openWithSource(t, addresses, []string{"poster", "film"})
	if got != addresses["film"] {
		t.Fatalf("the player opened %q, want the next line %q", got, addresses["film"])
	}
}

// served answers its paths with fixed heads, as the CDNs that mislabel their
// video do: /signed is an ISO base media file under a poster extension and an
// image Content-Type, and /clip is plainly mp4.
func served(t *testing.T) *httptest.Server {
	t.Helper()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/signed") {
			w.Header().Set("Content-Type", "image/jpeg")
			_, _ = w.Write(isoMedia)
			return
		}
		w.Header().Set("Content-Type", "video/mp4")
		_, _ = w.Write(isoMedia)
	}))
	t.Cleanup(server.Close)
	return server
}

// isoMedia is the opening of an ISO base media file: a size, then ftyp.
var isoMedia = append([]byte{0x00, 0x00, 0x00, 0x20}, []byte("ftypisom\x00\x00\x02\x00isomiso2avc1mp41")...)

// jpegHead is the opening of a JPEG.
var jpegHead = []byte{0xFF, 0xD8, 0xFF, 0xE0, 0x00, 0x10, 'J', 'F', 'I', 'F', 0x00}

// openWithLines plays one episode of a fake source whose lines answer with the
// given addresses, and returns the address the player opened.
func openWithLines(t *testing.T, addresses map[string]string) string {
	t.Helper()
	return openWithSource(t, addresses, nil)
}

// openWithSource is openWithLines with the original line named separately, as
// a test describing a poster line that claims to be media says it.
func openWithSource(t *testing.T, addresses map[string]string, order []string) string {
	t.Helper()
	src, _ := sampleTitle()
	if order == nil {
		order = []string{"signed", "film"}
	}
	lines := make([]source.Line, 0, len(addresses))
	for _, id := range order {
		if _, ok := addresses[id]; ok {
			lines = append(lines, source.Line{ID: id, Label: id})
		}
	}
	box := &posterSource{detail: src.detail, episodes: src.episodes, lines: lines, urls: addresses}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	var opened []string
	page := player.New(player.Deps{
		Source: box, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(url string) (*ui.VideoPlayer, error) {
			opened = append(opened, url)
			return newGIFPlayer(t), nil
		},
	})
	page.Open(box.detail, box.episodes, 0)
	apply(t, run)
	// The stream opens off the page's thread, so the work it hands back is a
	// callback of its own: wait for it before reading what was opened.
	apply(t, run)
	if len(opened) != 1 {
		t.Fatalf("player opened %d videos", len(opened))
	}
	return opened[0]
}

// TestPlayerTakesVideoBehindPosterExtension covers the line that answers with
// a .jpg address the CDN serves as an ISO base media file, Content-Type and
// all: the bytes say video, so the player opens that very line.
func TestPlayerTakesVideoBehindPosterExtension(t *testing.T) {
	server := served(t)
	addresses := map[string]string{
		"signed": server.URL + "/signed/poster.jpg~tplv-image.image?sig=1",
		"film":   server.URL + "/clip.mp4",
	}
	if got, want := openWithLines(t, addresses), addresses["signed"]; got != want {
		t.Fatalf("the player opened %q, want the resolved line %q", got, want)
	}
}

// TestPlayerSkipsTheAddressTheBytesDisown covers the other half: a JPEG that
// arrives where the line promised mp4 sends the viewer to the next line.
func TestPlayerSkipsTheAddressTheBytesDisown(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasPrefix(r.URL.Path, "/signed") {
			_, _ = w.Write(jpegHead)
			return
		}
		_, _ = w.Write(isoMedia)
	}))
	t.Cleanup(server.Close)
	addresses := map[string]string{
		"signed": server.URL + "/signed/poster?sig=1",
		"film":   server.URL + "/clip.mp4",
	}
	if got, want := openWithLines(t, addresses), addresses["film"]; got != want {
		t.Fatalf("the player opened %q, want the next line %q", got, want)
	}
}

// staleLineSource hands out a fresh set of line ids per episode, as the source
// that binds a line id to one episode's listing does.
type staleLineSource struct {
	*posterSource
	episodeLines map[string][]source.Line
}

func (s *staleLineSource) Resolve(_ context.Context, ep source.Episode, line string) (source.Playback, error) {
	lines, ok := s.episodeLines[ep.ID]
	if !ok {
		lines = s.lines
	}
	offered := false
	for _, entry := range lines {
		if entry.ID == line {
			offered = true
		}
	}
	if line != "" && !offered {
		return source.Playback{}, &source.Error{
			Kind: source.ErrNotFound, Source: "fake", Op: "resolve",
		}
	}
	chosen := lines[0]
	if offered {
		chosen = source.Line{ID: line, Label: line}
	}
	return source.Playback{
		URL: s.urls[chosen.ID], Line: chosen, Lines: lines,
	}, nil
}

// TestPlayerSwitchingEpisodeOnAStaleLineId covers the line the viewer is
// watching disappearing from the next episode's listing: the switch must land
// on that episode's own default line instead of failing with not_found.
func TestPlayerSwitchingEpisodeOnAStaleLineId(t *testing.T) {
	server := served(t)
	src, _ := sampleTitle()
	addresses := map[string]string{
		"ep1line": server.URL + "/clip.mp4",
		"ep2line": server.URL + "/clip.mp4",
	}
	box := &staleLineSource{
		posterSource: &posterSource{
			detail: src.detail, episodes: src.episodes,
			lines: []source.Line{{ID: "ep1line", Label: "ep1line"}},
			urls:  addresses,
		},
		episodeLines: map[string][]source.Line{
			src.episodes[1].ID: {{ID: "ep2line", Label: "ep2line", Default: true}},
		},
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	var opened []string
	page := player.New(player.Deps{
		Source: box, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(url string) (*ui.VideoPlayer, error) {
			opened = append(opened, url)
			return newGIFPlayer(t), nil
		},
	})
	page.Open(box.detail, box.episodes, 0)
	apply(t, run)
	apply(t, run)
	page.Next()
	apply(t, run)
	apply(t, run)
	if len(opened) != 2 {
		t.Fatalf("the player opened %d videos, want the second episode too", len(opened))
	}
	if got := opened[1]; got != addresses["ep2line"] {
		t.Fatalf("the second episode opened %q, want its own line", got)
	}
}
