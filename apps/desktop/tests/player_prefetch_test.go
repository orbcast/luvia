package tests

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/videocache"
)

// playlistSource answers every episode with an HLS playlist on the test server,
// so the player can be watched for the warm-up it starts on the next one.
type playlistSource struct {
	detail   source.Detail
	episodes []source.Episode
	url      string
	mu       sync.Mutex
	resolved []string
}

func (s *playlistSource) ID() string   { return "fake" }
func (s *playlistSource) Name() string { return "Fake Source" }
func (s *playlistSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, nil
}
func (s *playlistSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, nil
}
func (s *playlistSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return s.detail, nil
}
func (s *playlistSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return s.episodes, nil
}
func (s *playlistSource) Resolve(_ context.Context, episode source.Episode, _ string) (source.Playback, error) {
	s.mu.Lock()
	s.resolved = append(s.resolved, episode.ID)
	s.mu.Unlock()
	return source.Playback{
		URL:  s.url + "/" + episode.ID + "/index.m3u8",
		Line: source.Line{ID: "l1", Label: "Line 1"},
	}, nil
}
func (s *playlistSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

func (s *playlistSource) asked() []string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return append([]string(nil), s.resolved...)
}

// Opening an episode warms the one that follows it, so choosing that episode
// next starts on bytes already on disk instead of a cold fetch.
func TestPlayerWarmsTheNextEpisode(t *testing.T) {
	var mu sync.Mutex
	hits := map[string]int{}
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		if !strings.HasSuffix(r.URL.Path, ".ts") || r.Header.Get("Range") != "bytes=0-0" {
			hits[r.URL.Path]++
		}
		mu.Unlock()
		body := "#EXTM3U\n#EXTINF:4,\nseg0.ts\n#EXT-X-ENDLIST\n"
		if strings.HasSuffix(r.URL.Path, ".ts") {
			body = "SEGMENT"
		}
		w.Header().Set("Content-Length", itoa(len(body)))
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(upstream.Close)

	src, ref := sampleTitle()
	episodes := src.episodes
	playlist := &playlistSource{detail: src.detail, episodes: episodes, url: upstream.URL}
	cache, err := videocache.New(t.TempDir(), videocache.Options{Client: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: playlist, Library: store, Cache: cache,
		Run:       func(fn func()) { run <- fn },
		OnBack:    func() {},
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, episodes, 0)
	apply(t, run)

	// The second episode is resolved and warmed; the first is only played. The
	// warm-up resolves on a goroutine and hands its fetch back to the main
	// thread, so the queued callbacks are drained until the fetch shows up.
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		mu.Lock()
		warmed := hits["/"+episodes[1].ID+"/index.m3u8"] > 0
		mu.Unlock()
		if warmed {
			break
		}
		select {
		case fn := <-run:
			fn()
		case <-time.After(5 * time.Millisecond):
		}
	}
	mu.Lock()
	playlistHits, segmentHits := hits["/"+episodes[1].ID+"/index.m3u8"], hits["/"+episodes[1].ID+"/seg0.ts"]
	mu.Unlock()
	if playlistHits == 0 {
		t.Fatalf("the next episode's playlist was never warmed: %v", hits)
	}
	if segmentHits == 0 {
		t.Fatal("the next episode's first segment was never warmed")
	}

	// Warming follows the episode that follows, not one further out.
	mu.Lock()
	far := hits["/"+episodes[2].ID+"/index.m3u8"]
	mu.Unlock()
	if far != 0 {
		t.Fatalf("the episode after next was warmed %d times", far)
	}
	_ = ref
}
