package tests

import (
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/locales"
)

// The stage of one episode of a fake source, opened on the player the test hands
// it: the cases here are about what the stage shows while a stream that opened
// has nothing to draw on it.
type stage struct {
	page *player.Page
	run  chan func()
	tt   *ui.Tester
	// drawInOpen is set by a test whose open never returns: the stage is drawn
	// from inside that open, once the work of opening is queued, because the
	// page is never drawn after it again.
	drawInOpen bool
}

// openStage opens one episode on the player open hands out, and draws the page
// once. The page opens the stream off its own thread: a stream that never
// returns from its open leaves this one free, as it does in the app.
func openStage(t *testing.T, open func() (*ui.VideoPlayer, error)) *stage {
	t.Helper()
	src, _ := sampleTitle()
	s := newStage(t, src, open)
	s.open(t, src)
	return s
}

// newStage is the page of one episode of src on the player open hands out, with
// its line not resolved yet.
func newStage(t *testing.T, src *titleSource, open func() (*ui.VideoPlayer, error)) *stage {
	t.Helper()
	src.playback.URL = "https://example.invalid/clip.mp4"
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	s := &stage{run: run}
	s.page = player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnBack: func() {},
		OpenVideo: func(string) (*ui.VideoPlayer, error) {
			if s.drawInOpen {
				s.draw(t, s.queue(t))
			}
			return open()
		},
	})
	s.tt = ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		s.page.View(c)
	}, 1280, 900)
	s.tt.SetLocale("en")
	return s
}

// open resolves the line of the episode and draws what the page queued for it.
func (s *stage) open(t *testing.T, src *titleSource) {
	t.Helper()
	s.page.Open(src.detail, src.episodes, 0)
	// The line resolves, and the stream opens, on the page's own threads: only
	// the callbacks they queue reach this one.
	apply(t, s.run)
	s.tt.Frame()
}

// openStalledStage opens an episode whose stream opened and never gave a
// picture, with its open still out. The stage is drawn from inside the open,
// because an open that never returns is the last the page is looked at while it
// holds the stream.
func openStalledStage(t *testing.T) *stage {
	t.Helper()
	held := heldStream(t)
	blocked := make(chan struct{})
	t.Cleanup(func() { close(blocked) })
	src, _ := sampleTitle()
	s := newStage(t, src, func() (*ui.VideoPlayer, error) {
		<-blocked
		return <-held, nil
	})
	s.tt.Frame()
	// The first draw passes no judgement on the stall: the wait has only just
	// started.
	s.drawInOpen = true
	s.open(t, src)
	return s
}

// queue takes the work the page has queued, and hands back what is left of it.
func (s *stage) queue(t *testing.T) []func() {
	t.Helper()
	var left []func()
	for {
		select {
		case fn := <-s.run:
			left = append(left, fn)
		default:
			return left
		}
	}
}

// draw runs the work the page queued, then draws the page.
func (s *stage) draw(t *testing.T, work []func()) {
	t.Helper()
	for _, fn := range work {
		fn()
	}
	s.tt.Frame()
}

// frame draws the page again, as a viewer sees it, once the work the page has
// queued has run.
func (s *stage) frame(t *testing.T) {
	t.Helper()
	work := s.queue(t)
	if len(work) == 0 {
		time.Sleep(50 * time.Millisecond)
		work = s.queue(t)
	}
	s.draw(t, work)
}

// TestPlayerShowsAPictureWhileItsEngineDraws covers the ordinary path: a player
// with its picture is left to the picture, with no wait over it.
func TestPlayerShowsAPictureWhileItsEngineDraws(t *testing.T) {
	st := openStage(t, func() (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil })
	st.frame(t)
	m := locales.In("en")

	for _, text := range []string{m.Preparing(), m.Buffering()} {
		if st.tt.HasText(text) {
			t.Fatalf("a drawn picture still shows %q: %v", text, st.tt.Texts())
		}
	}
	// The controls of the picture are there, so the stage is the player's.
	if !st.tt.HasText(m.PauseLabel()) || !st.tt.HasText(m.PlaybackSpeed()) {
		t.Fatalf("the stage lost its controls: %v", st.tt.Texts())
	}
}

// TestPlayerWaitsOnAStreamThatNeverDraws covers the player of a stream that
// opened and drew nothing: the stage says so while it waits, with the time it
// has taken, rather than showing the picture's chrome over an empty frame.
func TestPlayerWaitsOnAStreamThatNeverDraws(t *testing.T) {
	st := openStalledStage(t)
	m := locales.In("en")

	if !st.tt.HasText(m.Preparing()) {
		t.Fatalf("a stream with no picture shows no wait: %v", st.tt.Texts())
	}
	if !st.tt.HasText(m.Waited(player.WaitLabel(0))) {
		t.Fatalf("the wait says nothing of its length: %v", st.tt.Texts())
	}

	// A wait within the timeout keeps counting, and is not yet a failure.
	st.page.BackdateWait(3 * time.Second)
	st.frame(t)
	if !st.tt.HasText(m.Waited(player.WaitLabel(3 * time.Second))) {
		t.Fatalf("the wait does not count while the stream stalls: %v", st.tt.Texts())
	}
	if st.tt.HasText(m.NoPicture()) {
		t.Fatalf("a stall within the opening timeout already reads as a failure: %v", st.tt.Texts())
	}
}

// TestPlayerReportsAStreamThatGaveNoPicture covers the stall that never ends: a
// stream that has held the stage without a picture for longer than an opening
// may take is reported as one that gave none, instead of waiting for good.
func TestPlayerReportsAStreamThatGaveNoPicture(t *testing.T) {
	st := openStalledStage(t)
	m := locales.In("en")

	st.page.BackdateWait(time.Minute)
	st.frame(t)

	if !st.tt.HasText(m.NoPicture()) {
		t.Fatalf("a stream that never draws is never reported: %v", st.tt.Texts())
	}
	if st.tt.HasText(m.Preparing()) {
		t.Fatalf("the wait stayed over the failure: %v", st.tt.Texts())
	}
	// The way out of the stage stays: the viewer closes the player from here.
	if !st.tt.HasText(m.ClosePlayer()) {
		t.Fatalf("the failed stage keeps no way out: %v", st.tt.Texts())
	}
}

// TestPlayerReportsAnEngineError covers a player that failed to open: the stage
// reports the failure, rather than waiting out the timeout.
func TestPlayerReportsAnEngineError(t *testing.T) {
	st := openStage(t, func() (*ui.VideoPlayer, error) {
		return nil, errors.New("test: the engine failed")
	})
	m := locales.In("en")
	st.frame(t)

	if !st.tt.HasText(m.PlaybackFailed()) {
		t.Fatalf("an engine error shows nothing of the failure: %v", st.tt.Texts())
	}
	if st.tt.HasText(m.Preparing()) {
		t.Fatalf("the wait stayed over a failed engine: %v", st.tt.Texts())
	}
}
