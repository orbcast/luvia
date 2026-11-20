package tests

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/videocache"
	"luvia/apps/desktop/locales"
)

// A wait with nothing moving on it leaves a slow line looking broken, so both
// waits say how long they have run.
func TestWaitLabelsCountTheWait(t *testing.T) {
	for _, tc := range []struct {
		name  string
		wait  time.Duration
		label string
	}{
		{"opening", 0, "0:00"},
		{"seconds", 7*time.Second + 400*time.Millisecond, "0:07"},
		{"minutes", 2*time.Minute + 9*time.Second, "2:09"},
		{"negative", -time.Second, "0:00"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := player.WaitLabel(tc.wait); got != tc.label {
				t.Fatalf("wait label of %v = %q, want %q", tc.wait, got, tc.label)
			}
		})
	}
}

// The speed is written as a viewer reads it: a fast line in MB/s, a slow one in
// KB/s, because "0.03 MB/s" tells a viewer nothing.
func TestSpeedLabelsReadTheLine(t *testing.T) {
	for _, tc := range []struct {
		name  string
		rate  float64
		value string
		unit  string
	}{
		{"kilobytes", 320 * 1000, "320", "KB"},
		{"trickle", 800, "0.8", "KB"},
		{"rounding", 1250 * 1000, "1.25", "MB"},
		{"megabytes", 12.5 * 1000 * 1000, "12.5", "MB"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			value, unit := player.SpeedLabel(tc.rate)
			if value != tc.value || unit != tc.unit {
				t.Fatalf("speed label of %v = %q %q, want %q %q", tc.rate, value, unit, tc.value, tc.unit)
			}
		})
	}
}

// The resolution wait covers the stage from the moment the episode is opened
// until its line answers, which on a slow line is the whole wait a viewer sees.
func TestPlayerShowsTheOpeningWait(t *testing.T) {
	src, _ := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	// The resolution is left to finish in the background: the callbacks it
	// queues are never drained, so the stage stays on the wait it opened with.
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnBack:    func() {},
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	m := locales.In("en")
	if !tt.HasText(m.Preparing()) {
		t.Fatalf("the opening wait is missing from %v", tt.Texts())
	}
	if !tt.HasText(m.Waited(player.WaitLabel(0))) {
		t.Fatalf("the wait says nothing of its length in %v", tt.Texts())
	}
	// Nothing has been fetched yet, so there is no speed to show: a number here
	// would be the previous episode's.
	for _, text := range tt.Texts() {
		if text == m.LoadSpeed("1.00") || text == m.LoadSpeedKB("320") {
			t.Fatalf("the opening wait showed a speed of nothing: %v", tt.Texts())
		}
	}
}

// A wait on a line that is moving says how fast, so a slow transfer reads as
// slow rather than as a stage that stopped.
func TestPlayerShowsTheTransferSpeed(t *testing.T) {
	// The body holds after its first chunk, so the transfer is still moving when
	// the stage is drawn, and ends as soon as the test lets it.
	release := make(chan struct{})
	var once sync.Once
	unblock := func() { once.Do(func() { close(release) }) }
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", strconv.Itoa(4*1000*1000))
		_, _ = w.Write([]byte(strings.Repeat("x", 500*1000)))
		w.(http.Flusher).Flush()
		<-release
	}))
	t.Cleanup(upstream.Close)
	t.Cleanup(unblock)
	cache, err := videocache.New(t.TempDir(), videocache.Options{Client: upstream.Client()})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	read := make(chan struct{})
	go func() {
		defer close(read)
		resp, err := http.Get(cache.ProxyURL(upstream.URL + "/clip.mp4"))
		if err != nil {
			return
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		resp.Body.Close()
	}()
	deadline := time.Now().Add(10 * time.Second)
	for cache.Transfer().Rate <= 0 {
		if time.Now().After(deadline) {
			t.Fatal("the cache never reported a transfer")
		}
		time.Sleep(10 * time.Millisecond)
	}

	src, _ := sampleTitle()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Cache: cache,
		Run:       func(fn func()) { run <- fn },
		OnBack:    func() {},
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, src.episodes, 0)

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()

	shown := false
	for _, text := range tt.Texts() {
		if strings.HasSuffix(text, "/s") && strings.Contains(text, "Waiting") {
			shown = true
		}
	}
	if !shown {
		t.Fatalf("the wait shows no speed of its transfer: %v", tt.Texts())
	}

	// The transfer is let go before the cache closes, or the close would wait on
	// a read the test never finished.
	unblock()
	select {
	case <-read:
	case <-time.After(10 * time.Second):
		t.Fatal("the read never finished")
	}
}
