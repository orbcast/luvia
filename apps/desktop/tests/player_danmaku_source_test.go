package tests

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/danmaku"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
)

// lookupSource answers a title with whatever reason the test asked for.
type lookupSource struct {
	items  []source.Danmaku
	reason danmaku.Reason
	err    error
	asked  []string
}

func (l *lookupSource) Lookup(_ context.Context, title string, episode int) (danmaku.Result, error) {
	l.asked = append(l.asked, title)
	if l.err != nil {
		return danmaku.Result{Reason: danmaku.ReasonFailed}, l.err
	}
	return danmaku.Result{Items: l.items, Reason: l.reason}, nil
}

// plainSource is a source with no comments of its own, which is what a rule
// without a comments block looks like.
type plainSource struct{ *commentingSource }

func (s plainSource) Comments(context.Context, source.Episode, int) ([]source.Danmaku, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "comments"}
}

// newDanmakuPage builds a player over src with the lookup attached.
func newDanmakuPage(t *testing.T, src source.Source, lookup danmaku.Source) (*player.Page, chan func(), *ui.Tester) {
	t.Helper()
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	run := make(chan func(), 8)
	page := player.New(player.Deps{
		Source: src, Danmaku: lookup, Library: store, Run: func(fn func()) { run <- fn },
		OpenVideo: func(string) (*ui.VideoPlayer, error) { return newGIFPlayer(t), nil },
	})
	page.Open(src.(*commentingSource).detail, src.(*commentingSource).episodes, 0)
	apply(t, run)
	apply(t, run)
	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	return page, run, tt
}

// pumpUntil drains the page's queued callbacks and redraws until the frame
// carries want, or the deadline passes. The lookup answers on its own
// goroutine, so how many callbacks it takes to land is not fixed: draining a
// set number of them is a race against the scheduler, not a check.
func pumpUntil(t *testing.T, run chan func(), tt *ui.Tester, want string) bool {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for {
		if tt.HasText(want) {
			return true
		}
		if time.Now().After(deadline) {
			return false
		}
		select {
		case fn := <-run:
			fn()
		case <-time.After(2 * time.Millisecond):
		}
		tt.Frame()
	}
}

// TestPlayerDrawsTheLookupsComments covers the layer being fed from outside the
// rule: the source itself carries nothing, and the lookup's comments are what
// crosses the picture.
func TestPlayerDrawsTheLookupsComments(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{detail: base.detail, episodes: base.episodes}
	lookup := &lookupSource{
		reason: danmaku.ReasonReady,
		items: []source.Danmaku{
			{At: 0, Text: "Found elsewhere"},
			{At: time.Hour, Text: "Much later"},
		},
	}
	_, run, tt := newDanmakuPage(t, src, lookup)

	if !pumpUntil(t, run, tt, "Found elsewhere") {
		t.Fatalf("the lookup's comment is not on the picture: %v", tt.Texts())
	}
	if len(lookup.asked) != 1 {
		t.Fatalf("looked up %d times, want once", len(lookup.asked))
	}
	if lookup.asked[0] != base.detail.Title {
		t.Fatalf("looked up %q, want the title", lookup.asked[0])
	}
	if tt.HasText("Much later") {
		t.Fatal("a comment an hour in must not show at the start")
	}
}

// TestPlayerPrefersTheLookupOverTheRulesReviews covers the order the two
// answers are weighed in: a comment with a moment can cross the picture, a
// review cannot, so the lookup wins when it has anything.
func TestPlayerPrefersTheLookupOverTheRulesReviews(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{
		detail: base.detail, episodes: base.episodes,
		comments: []source.Danmaku{{Text: "A review with no moment"}},
	}
	lookup := &lookupSource{
		reason: danmaku.ReasonReady,
		items:  []source.Danmaku{{At: 0, Text: "A comment with a moment"}},
	}
	page, run, tt := newDanmakuPage(t, src, lookup)

	if !pumpUntil(t, run, tt, "A comment with a moment") {
		t.Fatalf("the lookup's comment is not on the picture: %v", tt.Texts())
	}
	if tt.HasText("A review with no moment") {
		t.Fatal("the rule's review showed instead of the lookup's comment")
	}
	page.SelectTab(1)
	page.RevealPanel()
	tt.Frame()
	if tt.HasText("A review with no moment") {
		t.Fatal("the rule's review is in the panel after the lookup answered")
	}
}

// TestPlayerFallsBackToTheRulesComments covers a lookup that has nothing: the
// rule's own comments still feed the layer, so a show the site does not carry
// keeps the comments the rule does.
func TestPlayerFallsBackToTheRulesComments(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{
		detail: base.detail, episodes: base.episodes,
		comments: []source.Danmaku{{At: 0, Text: "Only the rule has this"}},
	}
	lookup := &lookupSource{reason: danmaku.ReasonNoMatch}
	_, run, tt := newDanmakuPage(t, src, lookup)

	if !pumpUntil(t, run, tt, "Only the rule has this") {
		t.Fatalf("the rule's comment is not on the picture: %v", tt.Texts())
	}
}

// TestPlayerSaysWhyTheLayerIsEmpty covers the reason the panel reports. An
// empty list with no reason reads like a broken player, which is what a viewer
// cannot tell apart from a show that simply has no comments.
func TestPlayerSaysWhyTheLayerIsEmpty(t *testing.T) {
	cases := []struct {
		name   string
		lookup *lookupSource
		want   string
	}{
		{name: "no match", lookup: &lookupSource{reason: danmaku.ReasonNoMatch}, want: "No danmaku source matched this show"},
		{name: "no episode", lookup: &lookupSource{reason: danmaku.ReasonNoEpisode}, want: "The show was found, but not this episode"},
		{name: "empty", lookup: &lookupSource{reason: danmaku.ReasonEmpty}, want: "This episode has no danmaku yet"},
		{name: "failed", lookup: &lookupSource{reason: danmaku.ReasonFailed, err: errors.New("offline")}, want: "The danmaku source did not answer"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			base, _ := sampleTitle()
			src := &commentingSource{detail: base.detail, episodes: base.episodes}
			page, run, tt := newDanmakuPage(t, src, c.lookup)
			page.SelectTab(1)
			page.RevealPanel()
			tt.Frame()
			if !pumpUntil(t, run, tt, c.want) {
				t.Fatalf("the panel does not say %q: %v", c.want, tt.Texts())
			}
		})
	}
}

// TestPlayerSaysWhenTheSourceHasNoComments covers a rule without a comments
// block and no lookup at all: the panel names the reason instead of leaving an
// empty list.
func TestPlayerSaysWhenTheSourceHasNoComments(t *testing.T) {
	base, _ := sampleTitle()
	src := plainSource{&commentingSource{detail: base.detail, episodes: base.episodes}}
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
	page.SelectTab(1)
	page.RevealPanel()
	tt.Frame()
	if !pumpUntil(t, run, tt, "This source carries no danmaku") {
		t.Fatalf("the panel does not name the missing capability: %v", tt.Texts())
	}
}

// TestPlayerAsksForTheEpisodesNumber covers the episode the lookup is asked
// about: the site splits a show into parts, so the number has to be the
// episode's own, not its position in the list.
func TestPlayerAsksForTheEpisodesNumber(t *testing.T) {
	base, _ := sampleTitle()
	src := &commentingSource{detail: base.detail, episodes: base.episodes}
	lookup := &lookupSource{reason: danmaku.ReasonNoMatch}
	page, run, _ := newDanmakuPage(t, src, lookup)
	page.SelectEpisode(4)
	deadline := time.Now().Add(5 * time.Second)
	for len(lookup.asked) < 2 && time.Now().Before(deadline) {
		select {
		case fn := <-run:
			fn()
		case <-time.After(2 * time.Millisecond):
		}
	}
	if len(lookup.asked) < 2 {
		t.Fatalf("looked up %d times, want the second episode too", len(lookup.asked))
	}
}
