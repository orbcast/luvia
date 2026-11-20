package tests

import (
	"bytes"
	"context"
	"image"
	"image/color"
	"image/gif"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/locales"
)

// stalledStage is a player page over a picture that never moves, which is a
// stream that stopped giving frames: the engine goes on saying it plays, so only
// the stage can tell, and only by looking again.
type stalledStage struct {
	page *player.Page
	run  chan func()
	tt   *ui.Tester
}

// newStalledStage opens the episode of src on a decoder's video, which plays on
// the stage's own clock, so a test that leaves the clock alone gets a picture
// that stands still.
func newStalledStage(t *testing.T, src *lineSource, open func(t *testing.T) *ui.VideoPlayer) *stalledStage {
	t.Helper()
	episodes, err := src.Episodes(context.Background(), src.detail.Ref)
	if err != nil {
		t.Fatal(err)
	}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	s := &stalledStage{run: make(chan func(), 64)}
	s.page = player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { s.run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return open(t), nil },
	})
	s.page.Open(src.detail, episodes, 0)
	drain(t, s.run)
	s.tt = ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		s.page.View(c)
	}, 1280, 900)
	s.tt.SetLocale("en")
	return s
}

// frame draws the stage and settles what the draw queued, so a stream the stage
// gave up on is replaced by the next one before the next draw.
func (s *stalledStage) frame(t *testing.T) {
	t.Helper()
	s.tt.Frame()
	drain(t, s.run)
}

// A stream that drew and then stopped moving gives way to the next line of the
// episode: a stage that only waited on the engine would hold the same picture
// for good, since an engine stuck on a dead stream says nothing.
func TestPlayerPassesOverALineThatStoppedMoving(t *testing.T) {
	src := newLineSource(t)
	st := newStalledStage(t, src, newGIFPlayer)
	st.frame(t)
	if got := src.opened(); len(got) != 1 || got[0] != "review" {
		t.Fatalf("the player resolved %v, want the first line", got)
	}
	st.page.BackdateStall(time.Minute)
	st.frame(t)
	if got := src.opened(); len(got) != 2 || got[1] != "next" {
		t.Fatalf("the player resolved %v, want the stalled line passed over", got)
	}
}

// The same stream, on an episode whose only line it is: the stage holds its
// picture while a wait may last, and reports a stream that gave no picture once
// the playhead has stood still for longer.
func TestPlayerReportsAStreamThatStoppedMoving(t *testing.T) {
	src := newLineSource(t)
	src.lines = src.lines[:1]
	st := newStalledStage(t, src, newGIFPlayer)
	m := locales.In("en")
	st.frame(t)
	if st.tt.HasText(m.NoPicture()) || st.tt.HasText(m.PlaybackFailed()) {
		t.Fatalf("a picture that just stopped is reported at once: %v", st.tt.Texts())
	}
	st.page.BackdateStall(time.Minute)
	st.frame(t)
	if !st.tt.HasText(m.NoPicture()) {
		t.Fatalf("a stream that stopped is not reported: %v", st.tt.Texts())
	}
}

// A picture that moves on is not a stall, however long the stage has watched it,
// and the wait of a later one is timed from its own start.
func TestPlayerLeavesAMovingPictureAlone(t *testing.T) {
	src := newLineSource(t)
	st := newStalledStage(t, src, newSlidingGIFPlayer)
	m := locales.In("en")
	st.frame(t)
	st.page.BackdateStall(time.Minute)
	st.tt.Advance(time.Second)
	st.frame(t)
	if st.tt.HasText(m.Buffering()) || st.tt.HasText(m.NoPicture()) {
		t.Fatalf("a picture that moves is waited on: %v", st.tt.Texts())
	}
	if got := src.opened(); len(got) != 1 {
		t.Fatalf("the player resolved %v, want the line it had", got)
	}
}

// A picture a viewer paused stands still by choice, so it is never a stall: the
// stage leaves it alone however long it stays.
func TestPlayerLeavesAPausedPictureAlone(t *testing.T) {
	src := newLineSource(t)
	st := newStalledStage(t, src, newGIFPlayer)
	m := locales.In("en")
	st.frame(t)
	st.page.TogglePlay()
	st.page.BackdateStall(time.Minute)
	st.frame(t)
	if st.tt.HasText(m.Buffering()) || st.tt.HasText(m.NoPicture()) {
		t.Fatalf("a paused picture is waited on: %v", st.tt.Texts())
	}
	if got := src.opened(); len(got) != 1 {
		t.Fatalf("the player resolved %v, want the line it had", got)
	}
}

// newSlidingGIFPlayer is a two-frame video whose frames last a second each, so a
// stage whose clock moves sees its playhead move.
func newSlidingGIFPlayer(t *testing.T) *ui.VideoPlayer {
	t.Helper()
	decoder, err := ui.NewGIFDecoder(slidingGIF())
	if err != nil {
		t.Fatal(err)
	}
	pl, err := ui.NewVideoPlayer(decoder)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pl.Close)
	return pl
}

// slidingGIF is a two-frame 2×2 GIF, a second a frame.
func slidingGIF() []byte {
	g := &gif.GIF{Config: image.Config{Width: 2, Height: 2}}
	for _, ink := range []color.RGBA{{255, 0, 0, 255}, {0, 0, 255, 255}} {
		frame := image.NewPaletted(image.Rect(0, 0, 2, 2), color.Palette{ink})
		g.Image = append(g.Image, frame)
		g.Delay = append(g.Delay, 100)
		g.Disposal = append(g.Disposal, gif.DisposalNone)
	}
	var b bytes.Buffer
	_ = gif.EncodeAll(&b, g)
	return b.Bytes()
}
