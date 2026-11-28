package home

import (
	"context"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

// fakeSource answers Home with a fixed document, answers Updates from its
// handler and fails the rest.
type fakeSource struct {
	home  source.Home
	err   error
	calls int
	done  chan struct{}

	// updates answers one weekday; a nil handler reports ErrUnsupported, as a
	// source without an updates block does. Requests records every weekday
	// asked for, in order, and mu guards both across the request goroutines.
	mu       sync.Mutex
	handler  func(day source.Day) ([]source.Update, error)
	requests []int
}

func (f *fakeSource) ID() string   { return "fake" }
func (f *fakeSource) Name() string { return "Fake" }
func (f *fakeSource) Home(context.Context) (source.Home, error) {
	f.calls++
	if f.done != nil {
		close(f.done)
	}
	return f.home, f.err
}
func (f *fakeSource) List(context.Context, source.ListQuery) (source.Page, error) {
	return source.Page{}, unsupported("list")
}
func (f *fakeSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, unsupported("detail")
}
func (f *fakeSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, unsupported("episodes")
}
func (f *fakeSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, unsupported("resolve")
}
func (f *fakeSource) Updates(_ context.Context, day source.Day) ([]source.Update, error) {
	f.mu.Lock()
	f.requests = append(f.requests, day.Weekday)
	handler := f.handler
	f.mu.Unlock()
	if handler == nil {
		return nil, unsupported("updates")
	}
	return handler(day)
}

// setHandler replaces the Updates answer, so a test can fail one day after the
// page started.
func (f *fakeSource) setHandler(handler func(day source.Day) ([]source.Update, error)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handler = handler
}

// asked returns the weekdays the page requested, in the order it asked.
func (f *fakeSource) asked() []int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]int(nil), f.requests...)
}

func unsupported(op string) error {
	return &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: op}
}

// newPage builds a page whose main-thread callbacks run inline. The returned
// channel holds one token per callback the page applied, so a test can tell
// when an answer has landed.
func newPage(t *testing.T, deps Deps) (*Page, chan struct{}) {
	t.Helper()
	finished := make(chan struct{}, 256)
	if deps.Run == nil {
		deps.Run = func(fn func()) { fn() }
	}
	run := deps.Run
	deps.Run = func(fn func()) {
		run(fn)
		finished <- struct{}{}
	}
	if deps.Now == nil {
		deps.Now = func() time.Time { return time.Date(2026, 11, 15, 20, 0, 0, 0, time.Local) }
	}
	if deps.Library == nil {
		store, err := library.Open("")
		if err != nil {
			t.Fatal(err)
		}
		deps.Library = store
	}
	page := New(deps)
	page.setRedraw(func() {})
	return page, finished
}

// load starts the page and waits for the source answer to be applied.
func load(t *testing.T, page *Page, finished chan struct{}) {
	t.Helper()
	page.Start()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("home request never finished")
	}
}

func wait(t *testing.T, finished chan struct{}) {
	t.Helper()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("page callback never finished")
	}
}

func sampleHome() source.Home {
	movie := source.Card{Ref: source.Ref{Source: "fake", ID: "dune"}, Title: "Dune",
		Kind: source.Movie, BackdropURL: "https://cdn.example/dune.jpg", PosterURL: "https://cdn.example/dune-p.jpg",
		Tags: []string{"Sci-Fi", "Adventure", "Drama"}, Year: 2021}
	series := source.Card{Ref: source.Ref{Source: "fake", ID: "severance"}, Title: "Severance",
		Kind: source.Series, PosterURL: "https://cdn.example/sev.jpg"}
	return source.Home{Sections: []source.Section{
		{ID: "latest", Title: "Latest", Cards: []source.Card{series}},
		{ID: "featured", Title: "Featured", Cards: []source.Card{movie, series}},
	}}
}

func TestWeekStartIsLocalMonday(t *testing.T) {
	// A Sunday evening belongs to the week that started the Monday before.
	sunday := time.Date(2026, 11, 15, 20, 0, 0, 0, time.Local)
	start := weekStart(sunday)
	if start.Weekday() != time.Monday {
		t.Fatalf("week started on %v", start.Weekday())
	}
	if start.Hour() != 0 || start.Minute() != 0 || start.Second() != 0 {
		t.Fatalf("week started at %v", start)
	}
	if days := int(sunday.Sub(start).Hours() / 24); days != 6 {
		t.Fatalf("sunday is %d days into the week", days)
	}
}

func TestWeekdayIndexUsesMondayFirst(t *testing.T) {
	monday := time.Date(2026, 11, 9, 0, 0, 0, 0, time.Local)
	if got := weekdayIndex(monday); got != 0 {
		t.Fatalf("monday index %d", got)
	}
	if got := weekdayIndex(monday.AddDate(0, 0, 6)); got != 6 {
		t.Fatalf("sunday index %d", got)
	}
}

func TestUpdatesOnMatchesLocalDay(t *testing.T) {
	day := time.Date(2026, 11, 12, 0, 0, 0, 0, time.Local)
	follows := []library.Follow{
		{Media: source.Ref{Source: "fake", ID: "a"}, Title: "A", UpdatedAt: day.Add(9 * time.Hour)},
		{Media: source.Ref{Source: "fake", ID: "b"}, Title: "B", UpdatedAt: day.AddDate(0, 0, -1)},
	}
	updates := updatesOn(follows, day)
	if len(updates) != 1 || updates[0].Title != "A" {
		t.Fatalf("updates %+v", updates)
	}
}

func TestFeaturedPrefersBackdropsAndCaps(t *testing.T) {
	var cards []source.Card
	for i := 0; i < heroLimit+2; i++ {
		ref := source.Ref{Source: "fake", ID: string(rune('a' + i))}
		card := source.Card{Ref: ref, Title: "Card", BackdropURL: "https://cdn.example/" + ref.ID + ".jpg"}
		cards = append(cards, card)
	}
	found := source.Home{Sections: []source.Section{
		{ID: "posters", Cards: []source.Card{{Ref: source.Ref{Source: "fake", ID: "x"}, Title: "X"}}},
		{ID: "wide", Cards: cards},
	}}
	got := featured(found)
	if len(got) != heroLimit {
		t.Fatalf("featured %d cards", len(got))
	}
	if got[0].Ref.ID != "a" {
		t.Fatalf("wrong section picked: %+v", got)
	}
	if featured(source.Home{}) != nil {
		t.Fatal("empty document returned featured cards")
	}
}

func TestStartRequestsOnceAndAcceptsHome(t *testing.T) {
	src := &fakeSource{home: sampleHome()}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	page.Start()
	if src.calls != 1 {
		t.Fatalf("home requested %d times", src.calls)
	}
	if len(page.featured) != 2 || page.featured[0].Title != "Dune" {
		t.Fatalf("featured %+v", page.featured)
	}
	if page.loading || page.err != nil {
		t.Fatalf("state loading=%v err=%v", page.loading, page.err)
	}
}

func TestStartReportsFailureAndRetryRecovers(t *testing.T) {
	src := &fakeSource{err: errors.New("boom")}
	page, finished := newPage(t, Deps{Source: src})
	load(t, page, finished)
	if page.err == nil {
		t.Fatal("failure not recorded")
	}
	src.err = nil
	src.home = sampleHome()
	page.Retry()
	wait(t, finished)
	if page.err != nil {
		t.Fatalf("retry kept the error: %v", page.err)
	}
	if len(page.featured) == 0 {
		t.Fatal("retry did not load the home document")
	}
}

func TestStartWithoutSourceExplains(t *testing.T) {
	page, _ := newPage(t, Deps{})
	page.Start()
	if !errors.Is(page.err, errNoSource) {
		t.Fatalf("err %v", page.err)
	}
}

func TestAdvanceWrapsAndPausesWhileHovering(t *testing.T) {
	page, finished := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	load(t, page, finished)
	page.advance()
	if page.hero != 1 {
		t.Fatalf("hero %d", page.hero)
	}
	page.advance()
	if page.hero != 0 {
		t.Fatalf("hero did not wrap: %d", page.hero)
	}
	page.hovering = true
	page.advance()
	if page.hero != 0 {
		t.Fatal("hovering must pause rotation")
	}
}

func TestShowKeepsIndexInRange(t *testing.T) {
	page, finished := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	load(t, page, finished)
	page.Show(1)
	if page.hero != 1 {
		t.Fatalf("hero %d", page.hero)
	}
	page.Show(9)
	page.Show(-1)
	if page.hero != 1 {
		t.Fatal("out of range index changed the hero")
	}
}

func TestToggleFollowWritesThroughLibrary(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	page, _ := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}, Library: store})
	card := sampleHome().Sections[1].Cards[0]
	page.toggleFollow(card)
	if !store.IsFollowing(card.Ref) {
		t.Fatal("follow not stored")
	}
	if page.currentNotice() != noticeFollowed {
		t.Fatalf("notice %v", page.currentNotice())
	}
	page.toggleFollow(card)
	if store.IsFollowing(card.Ref) {
		t.Fatal("unfollow not stored")
	}
}

func TestPlayAndDetailUseCallbacks(t *testing.T) {
	var played, detailed source.Ref
	page, _ := newPage(t, Deps{
		Source:   &fakeSource{home: sampleHome()},
		OnPlay:   func(ref source.Ref) { played = ref },
		OnDetail: func(ref source.Ref) { detailed = ref },
	})
	card := sampleHome().Sections[1].Cards[0]
	page.play(card)
	page.detail(card)
	if played != card.Ref || detailed != card.Ref {
		t.Fatalf("callbacks played=%v detailed=%v", played, detailed)
	}
	page.deps.OnPlay, page.deps.OnDetail = nil, nil
	page.play(card)
	if page.currentNotice() != noticePlayer {
		t.Fatalf("missing player must be explained, notice %v", page.currentNotice())
	}
	page.detail(card)
	if page.currentNotice() != noticeDetail {
		t.Fatalf("missing detail page must be explained, notice %v", page.currentNotice())
	}
}

func TestHasQualityReadsRemarksAndTags(t *testing.T) {
	if !hasQuality(source.Card{Remarks: "4K HDR"}, "4K") {
		t.Fatal("remarks did not flag 4K")
	}
	tagged := source.Card{Tags: []string{"Dolby", "1080P"}}
	if hasQuality(tagged, "4K") {
		t.Fatal("1080P tag read as 4K")
	}
	if !hasQuality(tagged, "1080P") {
		t.Fatal("tag did not flag 1080P")
	}
	if hasQuality(source.Card{}, "4K") {
		t.Fatal("empty card flagged 4K")
	}
}

func TestSplitBadgeSeparatesHeadline(t *testing.T) {
	if head, tail := splitBadge("4K Ultra HD"); head != "4K" || tail != "Ultra HD" {
		t.Fatalf("split %q %q", head, tail)
	}
	if head, tail := splitBadge("1080P"); head != "1080P" || tail != "" {
		t.Fatalf("single token split %q %q", head, tail)
	}
}

func TestHeroShowsQualityAndBrandBadges(t *testing.T) {
	home := sampleHome()
	home.Sections[1].Cards[0].Remarks = "4K HDR · Dolby Vision"
	page, finished := newPage(t, Deps{Source: &fakeSource{home: home}})
	load(t, page, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	// The print's own remarks state the brand, so the hero wears both chips:
	// the resolution and the brand the source named.
	for _, want := range []string{"4K", "Dolby Vision"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
}

func TestHeroBrandBadgeNeedsTheSourcesWord(t *testing.T) {
	// The sample's Dune states neither brand nor an unusual resolution, so the
	// hero wears the standard HD chip and no brand pill at all: a brand the
	// source never named is a claim about the print, not a decoration.
	page, finished := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	load(t, page, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	if tt.HasText("Dolby Vision") {
		t.Fatalf("brand pill without the source's word: %v", tt.Texts())
	}
	if !tt.HasText("1080P") {
		t.Fatalf("missing the HD chip in %v", tt.Texts())
	}
	if hasBrand(source.Card{Tags: []string{"Dolby Vision"}}) != true {
		t.Fatal("a tag naming the brand did not flag it")
	}
}

func TestHeroShows4kBadgeWhenFlagged(t *testing.T) {
	home := sampleHome()
	home.Sections[1].Cards[0].Remarks = "4K HDR"
	page, finished := newPage(t, Deps{Source: &fakeSource{home: home}})
	load(t, page, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	if !tt.HasText("4K") || !tt.HasText("Ultra HD") {
		t.Fatalf("4K badge missing in %v", tt.Texts())
	}
	if tt.HasText("1080P") {
		t.Fatalf("HD badge shown for a 4K title: %v", tt.Texts())
	}
}

// weekSource answers every weekday with a fixed number of titles, each one
// named after its weekday and numbered so a test can tell them apart.
func weekSource(counts map[int]int) *fakeSource {
	src := &fakeSource{home: sampleHome()}
	src.handler = func(day source.Day) ([]source.Update, error) {
		var found []source.Update
		for i := 0; i < counts[day.Weekday]; i++ {
			title := fmt.Sprintf("Day %d Show %d", day.Weekday, i+1)
			found = append(found, source.Update{
				Card: source.Card{
					Ref:       source.Ref{Source: "fake", ID: fmt.Sprintf("d%d-%d", day.Weekday, i)},
					Title:     title,
					Kind:      source.Series,
					PosterURL: fmt.Sprintf("https://cdn.example/d%d-%d.jpg", day.Weekday, i),
				},
				Note: fmt.Sprintf("Status %d-%d", day.Weekday, i+1),
			})
		}
		return found, nil
	}
	return src
}

// drain waits for one answer the page applied through Deps.Run.
func drain(t *testing.T, finished chan struct{}) {
	t.Helper()
	select {
	case <-finished:
	case <-time.After(5 * time.Second):
		t.Fatal("page callback never finished")
	}
}

// settleWeek waits until the page asked its source for every day of the week
// and applied every answer. Counting callbacks cannot stand in for this: the
// days answer in any order, and the page records a request before it starts the
// goroutine, so this reads the page state after each callback instead.
func settleWeek(t *testing.T, page *Page, src *fakeSource, finished chan struct{}) {
	t.Helper()
	deadline := time.After(5 * time.Second)
	for {
		if !page.weekPending() && len(src.asked()) == weekdayCount {
			return
		}
		select {
		case <-finished:
		case <-deadline:
			t.Fatal("week requests never finished")
		}
	}
}

// followedOn stores a follow updated on the given date, so the local library
// has something for the strip to count.
func followedOn(t *testing.T, store *library.Store, id, title string, at time.Time) {
	t.Helper()
	err := store.Follow(library.Follow{
		Media: source.Ref{Source: "fake", ID: id}, Title: title, UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
}

func TestUpdatesOffKeepsEarliestWeekForFollows(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	// The page's clock reads Sunday 15 November 2026; two follows land on
	// Thursday the 12th and one on Monday the 9th.
	followedOn(t, store, "a", "Alpha", time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local))
	followedOn(t, store, "b", "Beta", time.Date(2026, 11, 12, 21, 0, 0, 0, time.Local))
	followedOn(t, store, "c", "Gamma", time.Date(2026, 11, 9, 7, 0, 0, 0, time.Local))
	src := weekSource(nil)
	page, finished := newPage(t, Deps{Source: src, Library: store})
	load(t, page, finished)

	if asked := src.asked(); len(asked) != 0 {
		t.Fatalf("Updates=false requested weekdays %v", asked)
	}
	if len(page.days) != 0 || page.dayLoading(page.selected) {
		t.Fatalf("cache %v loading=%v", page.days, page.dayLoading(page.selected))
	}

	// Thursday is selected through the strip and still counts the library.
	page.selectDay(3)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{"Alpha", "Beta", "2 updates"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if tt.HasText("Gamma") {
		t.Fatalf("monday follow leaked into thursday: %v", tt.Texts())
	}
	if asked := src.asked(); len(asked) != 0 {
		t.Fatalf("clicking a day with Updates=false requested %v", asked)
	}
}

func TestUpdatesOnFillsWeekAndListsSelectedDay(t *testing.T) {
	counts := map[int]int{5: 3, 7: 2}
	src := weekSource(counts)
	page, finished := newPage(t, Deps{Source: src, Updates: true})
	load(t, page, finished)
	settleWeek(t, page, src, finished)

	if asked := src.asked(); len(asked) != weekdayCount {
		t.Fatalf("requested weekdays %v", asked)
	}
	for weekday := 1; weekday <= weekdayCount; weekday++ {
		found, ok := page.dayUpdates(weekday - 1)
		if !ok || len(found) != counts[weekday] {
			t.Fatalf("weekday %d cached %d updates (asked %v)", weekday, len(found), ok)
		}
	}

	// Sunday 15 November is the last day of the week and carries two titles.
	page.selectDay(6)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{
		"Day 7 Show 1", "Status 7-1", "Day 7 Show 2", "Status 7-2", "2 updates",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if tt.HasText("Status 5-1") {
		t.Fatalf("another day's titles leaked into the panel: %v", tt.Texts())
	}

	// The strip counts what the source reported, not the empty library.
	page.selectDay(4)
	tt.Frame()
	if !tt.HasText("Day 5 Show 1") || !tt.HasText("3 updates") {
		t.Fatalf("friday panel missing in %v", tt.Texts())
	}
}

func TestSelectingNewDayAsksOnceAndCacheAnswers(t *testing.T) {
	src := weekSource(map[int]int{1: 1, 3: 2})
	page, finished := newPage(t, Deps{Source: src, Updates: true})
	load(t, page, finished)
	settleWeek(t, page, src, finished)
	before := len(src.asked())

	for i := 0; i < 3; i++ {
		page.selectDay(2)
		if page.selected != 2 {
			t.Fatalf("selection %d", page.selected)
		}
	}
	if asked := src.asked(); len(asked) != before {
		t.Fatalf("cached day was requested: %v", asked)
	}

	// A day that failed counts as answered, so it is not asked again either.
	page.acceptDay(4, nil, errors.New("boom"))
	if page.loadDay(4); len(src.asked()) != before {
		t.Fatalf("failed day was requested again: %v", src.asked())
	}
	if !page.dayFailed(4) {
		t.Fatal("day failure not recorded")
	}
}

func TestDayFailureFallsBackToFollowsWithoutTouchingOtherDays(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	followedOn(t, store, "fallback", "Fallback Show", time.Date(2026, 11, 13, 8, 0, 0, 0, time.Local))
	src := weekSource(map[int]int{1: 1, 3: 2, 5: 4})
	src.setHandler(func(day source.Day) ([]source.Update, error) {
		if day.Weekday == 5 {
			return nil, &source.Error{Kind: source.ErrNetwork, Source: "fake", Op: "updates"}
		}
		found, _ := weekSource(map[int]int{1: 1, 3: 2, 5: 4}).handler(day)
		return found, nil
	})
	page, finished := newPage(t, Deps{Source: src, Library: store, Updates: true})
	load(t, page, finished)
	settleWeek(t, page, src, finished)

	// The failing Friday shows the follow recorded that day, and the other
	// days keep their answer.
	page.selectDay(4)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Fallback Show") {
		t.Fatalf("friday fallback missing in %v", tt.Texts())
	}
	page.selectDay(2)
	tt.Frame()
	if !tt.HasText("Day 3 Show 1") {
		t.Fatalf("wednesday lost its answer: %v", tt.Texts())
	}
	if _, ok := page.dayUpdates(0); !ok {
		t.Fatal("monday answer dropped")
	}
}

func TestDayPanelShowsWaitingStateWhileADayIsOutstanding(t *testing.T) {
	// The page cannot finish the week before the test has seen what it shows
	// in the meantime, so the runner holds every weekday answer. The home
	// sections are filled straight from the source and stay out of the
	// runner's way: they are what the page draws the week inside, and a test
	// about the waiting week needs a page it can draw at all.
	held := make(chan func(), 32)
	src := weekSource(map[int]int{7: 1})
	page, _ := newPage(t, Deps{
		Source: src, Updates: true,
		Run: func(fn func()) { held <- fn },
	})
	home, err := src.Home(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	page.requested, page.loading = true, false
	page.accept(home)
	page.selectDay(6)
	page.loadWeek()
	if !page.dayLoading(6) {
		t.Fatal("an unanswered day must report a loading state")
	}
	if !page.weekPending() {
		t.Fatal("the week must still be pending while its answers wait")
	}
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	if tt.HasText("Day 7 Show 1") {
		t.Fatalf("an unanswered day must not list titles: %v", tt.Texts())
	}

	// Applying the answers fills the panel and retires the waiting state.
	for page.weekPending() {
		select {
		case fn := <-held:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("a day answer never reached the page")
		}
	}
	if page.dayLoading(6) || page.weekPending() {
		t.Fatal("answered week still reports waiting")
	}
	tt.Frame()
	for _, want := range []string{"Day 7 Show 1", "Status 7-1"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
}

func TestDayPostersCapAtSixAndKeepTheNarrowShape(t *testing.T) {
	var scheduled []source.Update
	for i := 0; i < 10; i++ {
		scheduled = append(scheduled, source.Update{
			Card: source.Card{
				Ref:   source.Ref{Source: "fake", ID: fmt.Sprintf("cap-%d", i)},
				Title: fmt.Sprintf("Cap Show %d", i+1),
			},
		})
	}
	src := weekSource(nil)
	page, finished := newPage(t, Deps{Source: src, Updates: true})
	load(t, page, finished)
	settleWeek(t, page, src, finished)
	page.acceptDay(3, scheduled, nil)

	for _, tc := range []struct {
		width int
		want  int
	}{{width: 1100, want: 6}, {width: 460, want: 4}} {
		page.selectDay(3)
		tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, tc.width, 720)
		tt.SetLocale("en")
		tt.Frame()
		shown := 0
		for i := 0; i < len(scheduled); i++ {
			if tt.HasText(fmt.Sprintf("Cap Show %d", i+1)) {
				shown++
			}
		}
		if shown != tc.want {
			t.Fatalf("width %d showed %d posters, want %d", tc.width, shown, tc.want)
		}
	}
}

func TestUpdateCardOpensTheDetailPage(t *testing.T) {
	var opened source.Ref
	src := weekSource(nil)
	page, finished := newPage(t, Deps{
		Source:   src,
		Updates:  true,
		OnDetail: func(ref source.Ref) { opened = ref },
	})
	load(t, page, finished)
	settleWeek(t, page, src, finished)
	card := source.Card{Ref: source.Ref{Source: "fake", ID: "clicked"}, Title: "Clicked Show"}
	page.acceptDay(0, []source.Update{{Card: card, Note: "Status"}}, nil)
	page.selectDay(0)

	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 1400)
	tt.SetLocale("en")
	tt.Frame()
	if err := tt.Click("Clicked Show"); err != nil {
		t.Fatalf("click: %v", err)
	}
	if opened != card.Ref {
		t.Fatalf("detail opened for %v", opened)
	}
}

func TestEmptyWeekKeepsTheEmptyPanel(t *testing.T) {
	src := weekSource(nil)
	page, finished := newPage(t, Deps{Source: src, Updates: true})
	load(t, page, finished)
	settleWeek(t, page, src, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{
		"Follow a series to see its weekly updates here.",
		"Your progress waits here once playback starts.",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
}

// The notice is cleared by a timer, so a page that flashes one is read from a
// goroutine the page does not own. Both the write and the comparison have to
// hold the same lock the drawing threads do.
func TestFlashNoticeIsReadableWhileTheTimerRuns(t *testing.T) {
	page, _ := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			page.flash(noticePlayer)
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 200; i++ {
			_ = page.currentNotice()
			_, _ = page.noticeText(locales.In("en"))
		}
	}()
	wg.Wait()
	if !page.clearNotice(page.currentNotice()) {
		t.Fatal("the flashed notice could not be cleared")
	}
	if page.currentNotice() != noticeNone {
		t.Fatalf("notice stayed %v", page.currentNotice())
	}
}

func TestSelectDayIgnoresOutOfRangeIndex(t *testing.T) {
	page, _ := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}, Updates: true})
	page.selectDay(9)
	page.selectDay(-1)
	if page.selected != weekdayIndex(page.deps.Now()) {
		t.Fatalf("selection moved to %d", page.selected)
	}
}

func TestWeekPendingWithoutAnUpdatesBlock(t *testing.T) {
	page, finished := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	load(t, page, finished)
	if page.weekPending() {
		t.Fatal("a page without an updates block has nothing pending")
	}
	if page.dayLoading(0) {
		t.Fatal("a page without an updates block is never waiting")
	}
}
