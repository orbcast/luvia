package tests

import (
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
)

// chromeTick is the pace a playing picture asks for frames at, walked by the
// tests below: it is the interval the bars' idle window could be restarted on.
const chromeTick = 16 * time.Millisecond

// chromePage is a player over a clock the test walks itself, so the idle window
// the bars keep to can be read frame by frame instead of watched in real time.
// The playhead is the test's too: a GIF holds its own at zero, and the bars are
// timed by whether the picture under them moves.
func chromePage(t *testing.T, playhead func() time.Duration) (*player.Page, *fakeClock, *ui.Tester) {
	t.Helper()
	src, _ := sampleTitle()
	src.playback.URL = "mem://clip"
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 16)
	clock := &fakeClock{at: time.Unix(0, 0)}
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.SetClock(clock.now)
	page.SetPlayhead(playhead)
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
	return page, clock, tt
}

// tickFrame advances the page's clock and draws one frame of it, as the
// stage's own play tick does while a picture plays.
func tickFrame(clock *fakeClock, tt *ui.Tester, step time.Duration) {
	clock.at = clock.at.Add(step)
	tt.Frame()
}

// parkedPointer reports whether the pointer is out of the way of the stage: at
// the test's own origin, which is the stage's top-left corner, the pointer rests
// on the chrome, and a resting pointer is what keeps a bar up. Every case here
// is about an idle viewer, so the pointer is moved off the bars first.
func parkedPointer(tt *ui.Tester) {
	moveTo(nil, tt, 200, 300)
}

// moveTo walks the pointer to (x, y) in two steps and draws the frame in
// between: a move that lands on the coordinate it already had is no move at all.
func moveTo(t *testing.T, tt *ui.Tester, x, y float32) {
	if t != nil {
		t.Helper()
	}
	tt.Move(x-1, y-1)
	tt.Frame()
	tt.Move(x, y)
	tt.Frame()
}

// TestChromeHidesWhilePlaying covers the idle window a playing picture keeps:
// once the playhead moves, two and a half idle seconds are enough to take the
// bars away, though the playhead moves on every frame under them. A window that
// restarted on each of those frames would never run out, and the bars would
// stay over the film for as long as it played.
func TestChromeHidesWhilePlaying(t *testing.T) {
	var played time.Duration
	page, clock, tt := chromePage(t, func() time.Duration { return played })
	parkedPointer(tt)

	const tick = chromeTick
	// The opening wait: the stream has drawn, but its playhead has not moved.
	for i := 0; i < 40; i++ {
		tickFrame(clock, tt, tick)
	}
	if !tt.HasText("Danmaku") {
		t.Fatalf("the bars left a stage still opening: %v", tt.Texts())
	}
	// The picture starts: this frame is the one that arms the window.
	played = 100 * time.Millisecond
	tickFrame(clock, tt, tick)
	if !tt.HasText("Danmaku") {
		t.Fatalf("the bars left the frame the picture started: %v", tt.Texts())
	}
	// Just under the window, with the playhead moving on every frame.
	for elapsed := time.Duration(0); elapsed < design.PlayerAutoHide-tick; elapsed += tick {
		played += tick
		tickFrame(clock, tt, tick)
	}
	if !tt.HasText("Danmaku") {
		t.Fatalf("the bars left before the idle window ran out: %v", tt.Texts())
	}
	// Past it, and they go, though the playhead is still moving.
	played += 2 * tick
	tickFrame(clock, tt, 2*tick)
	if tt.HasText("Danmaku") {
		t.Fatalf("a playing picture kept its bars past %v: %v", design.PlayerAutoHide, tt.Texts())
	}
	page.Back()
}

// TestChromeHidesUnderAContinuingPicture keeps the picture playing under the
// hidden bars: the frames after they go must not bring them back on their own.
func TestChromeHidesUnderAContinuingPicture(t *testing.T) {
	var played time.Duration
	page, clock, tt := chromePage(t, func() time.Duration { return played })
	parkedPointer(tt)

	played = 250 * time.Millisecond
	tickFrame(clock, tt, chromeTick)
	for elapsed := time.Duration(0); elapsed <= design.PlayerAutoHide; elapsed += chromeTick {
		played += chromeTick
		tickFrame(clock, tt, chromeTick)
	}
	if tt.HasText("Danmaku") {
		t.Fatalf("the bars did not hide under a playing picture: %v", tt.Texts())
	}
	for i := 0; i < 10; i++ {
		played += chromeTick
		tickFrame(clock, tt, chromeTick)
	}
	if tt.HasText("Danmaku") {
		t.Fatalf("the bars came back with no input: %v", tt.Texts())
	}
	page.Back()
}

// TestChromeStaysWhilePaused checks the first state that never hides: a picture
// held still is a viewer deciding what to do next, and seeking is what they do.
func TestChromeStaysWhilePaused(t *testing.T) {
	var played time.Duration
	page, clock, tt := chromePage(t, func() time.Duration { return played })
	parkedPointer(tt)

	// A viewer holds the picture still at once, and the playhead stands, as a
	// paused one does.
	page.TogglePlay()
	tt.Frame()
	played = 250 * time.Millisecond
	tickFrame(clock, tt, chromeTick)
	// Long enough to hide several times over, had the picture played.
	for elapsed := time.Duration(0); elapsed <= 3*design.PlayerAutoHide; elapsed += 100 * time.Millisecond {
		tickFrame(clock, tt, 100*time.Millisecond)
	}
	if !tt.HasText("Danmaku") {
		t.Fatalf("a paused picture hid its bars: %v", tt.Texts())
	}
	page.Back()
}

// TestChromeStaysWhileTheSettingsBoxIsOpen checks the second state: a box that
// vanished mid-drag would be worse than one that stays.
func TestChromeStaysWhileTheSettingsBoxIsOpen(t *testing.T) {
	var played time.Duration
	page, clock, tt := chromePage(t, func() time.Duration { return played })
	parkedPointer(tt)
	page.SetDanmakuOpen(true)
	tt.Frame()

	played = 250 * time.Millisecond
	tickFrame(clock, tt, chromeTick)
	for elapsed := time.Duration(0); elapsed <= 3*design.PlayerAutoHide; elapsed += 100 * time.Millisecond {
		played += 100 * time.Millisecond
		tickFrame(clock, tt, 100*time.Millisecond)
	}
	if !tt.HasText("Danmaku") {
		t.Fatalf("the unfolded settings box hid its bars: %v", tt.Texts())
	}
	page.Back()
}

// TestChromeComesBackOnAPointerMove checks that hidden bars wake on the pointer:
// an idle window takes them away, and one move anywhere in the stage brings them
// back with a whole idle window in front of them.
func TestChromeComesBackOnAPointerMove(t *testing.T) {
	var played time.Duration
	page, clock, tt := chromePage(t, func() time.Duration { return played })
	parkedPointer(tt)

	played = 250 * time.Millisecond
	tickFrame(clock, tt, chromeTick)
	for elapsed := time.Duration(0); elapsed <= design.PlayerAutoHide; elapsed += chromeTick {
		played += chromeTick
		tickFrame(clock, tt, chromeTick)
	}
	if tt.HasText("Danmaku") {
		t.Fatalf("the bars did not hide before the pointer moved: %v", tt.Texts())
	}

	// A move in the middle of the picture: inside the stage, away from both
	// bars, so the bars come back without being held.
	moveTo(t, tt, 230, 320)
	if !tt.HasText("Danmaku") {
		t.Fatalf("a pointer move did not bring the bars back: %v", tt.Texts())
	}

	// The window after a wake is a whole one: the moving playhead alone does
	// not take them again until it has run out.
	for elapsed := time.Duration(0); elapsed < design.PlayerAutoHide-chromeTick; elapsed += chromeTick {
		played += chromeTick
		tickFrame(clock, tt, chromeTick)
	}
	if !tt.HasText("Danmaku") {
		t.Fatalf("the bars left in less than a full idle window after a move: %v", tt.Texts())
	}
	played += 2 * chromeTick
	tickFrame(clock, tt, 2*chromeTick)
	if tt.HasText("Danmaku") {
		t.Fatalf("the idle window after a move never ran out: %v", tt.Texts())
	}
	page.Back()
}
