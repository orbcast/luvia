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

	// The panel keeps the episode list alone: the comments are read where they
	// fly, so nothing lists them a second time beside the picture. The pill's
	// own switch keeps its name, which is a control's label, not a list's.
	page.RevealPanel()
	tt.Frame()
	// "Danmaku" is still the name of the pill's switch, so the copy the panel
	// must not carry is the tab's own pair: two pills side by side, one of them
	// listing what the layer already shows. The comments themselves still fly,
	// which is where the viewer reads them.
	for _, unwanted := range []string{"Danmaku Episodes", "Episodes Danmaku", "Comments",
		"ann " + "Second review"} {
		if tt.HasText(unwanted) {
			t.Fatalf("the panel still offers %q: %v", unwanted, tt.Texts())
		}
	}
	if !tt.HasText(english.EpisodesTitle()) {
		t.Fatalf("the panel has no name for the episode list it keeps: %v", tt.Texts())
	}
}

// TestPlayerEpisodeTitleKeepsItsWords covers how an episode is labelled in the
// panel's grid: a source titles its episodes with the date and what happened in
// them, so the chip lets the title run onto a second line instead of cutting it
// down to the date alone.
func TestPlayerEpisodeTitleKeepsItsWords(t *testing.T) {
	base, _ := sampleTitle()
	titled := make([]source.Episode, len(base.episodes))
	copy(titled, base.episodes)
	// The titles the app was complained about carry far more than one chip line
	// of a three-across grid held.
	titled[0].Title = "2026-09-05 回顾 第三期 上"
	titled[1].Title = "2026-09-06 回顾 第三期 下"
	src := &commentingSource{detail: base.detail, episodes: titled}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.detail, titled, 0)
	apply(t, run)
	apply(t, run)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	page.RevealPanel()
	tt.Frame()

	// The whole title is in the chip's text: a chip that had to cut it says so
	// by ending in an ellipsis, which the label never carries.
	for _, want := range []string{titled[0].Title, titled[1].Title} {
		if !pumpUntil(t, run, tt, want) {
			t.Fatalf("the episode chip does not carry %q: %v", want, tt.Texts())
		}
	}
}

// TestPlayerPanelCloseHidesThePanel covers the cross in the panel's header: it
// puts the panel away and leaves the player running, rather than leaving the
// player, which is what the stage's own controls are for.
func TestPlayerPanelCloseHidesThePanel(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{detail: base.detail, episodes: base.episodes}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	left := 0
	page := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnBack:    func() { left++ },
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

	// A player opens with the panel away, so the viewer asks for it first.
	page.RevealPanel()
	tt.Frame()
	// The cross carries the panel's own name, not the player's.
	m := english
	if err := tt.Click(m.CollapsePanel()); err != nil {
		t.Fatalf("the panel's close button is unreachable: %v", err)
	}
	tt.Frame()
	if !page.PanelHidden() {
		t.Fatal("the panel's close button left the panel open")
	}
	if left != 0 {
		t.Fatalf("the panel's close button left the player: %d", left)
	}
	// The picture keeps the control that brings it back.
	if err := tt.Click(m.ExpandPanel()); err != nil {
		t.Fatalf("the reveal control is unreachable: %v", err)
	}
	tt.Frame()
	if page.PanelHidden() {
		t.Fatal("the reveal control did not bring the panel back")
	}
}

// TestPlayerPictureBoxKeepsThePictureWhole covers the box a picture fills on a
// stage whose shape it does not share: the picture keeps its own ratio, centred,
// so the bands the stage leaves on two sides are the viewer's letterbox rather
// than a crop.
func TestPlayerPictureBoxKeepsThePictureWhole(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{detail: base.detail, episodes: base.episodes}
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	// The player is built before the page opens the stream: its first frame is
	// what gives the picture its ratio, and a player built on demand reports
	// 0x0 until that frame has come.
	clip := newGIFPlayer(t)
	playing := player.New(player.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return clip, nil },
	})
	playing.Open(src.detail, src.episodes, 0)
	apply(t, run)
	// The stream opens off the main thread, so the player lands on a callback of
	// its own: one that has not been applied yet leaves the stage without a
	// picture to measure, and the box it would report is empty.
	for i := 0; i < 2 && playing.PictureBox(1600, 900) == (ui.Rect{}); i++ {
		apply(t, run)
	}

	playing.RevealPanel()
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		playing.View(c)
	}, 1600, 900)
	tt.SetLocale("en")
	tt.Frame()

	// The test picture is 2x2 pixels: in a 1600x900 stage it fills 900x900,
	// centred, so 350 stands on either side for the viewer to see.
	box := playing.PictureBox(1600, 900)
	if box.W != 900 || box.H != 900 || box.Y != 0 || box.X != 350 {
		t.Fatalf("a square picture in a 1600x900 stage: %v", box)
	}
	// A stage narrower than the picture leaves the bands above and below.
	if box = playing.PictureBox(400, 900); box.W != 400 || box.H != 400 || box.X != 0 || box.Y != 250 {
		t.Fatalf("a square picture in a 400x900 stage: %v", box)
	}
	// A stage whose shape matches the picture leaves no band at all.
	if box = playing.PictureBox(800, 800); box.W != 800 || box.H != 800 || box.X != 0 || box.Y != 0 {
		t.Fatalf("a square picture in an 800x800 stage: %v", box)
	}
	// A stage with no room, as one built before its first frame, has no box.
	if box = playing.PictureBox(0, 0); box != (ui.Rect{}) {
		t.Fatalf("a stage with no room reported a box: %v", box)
	}
}
