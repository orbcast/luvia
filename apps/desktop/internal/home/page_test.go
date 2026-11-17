package home

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// fakeSource answers Home with a fixed document and fails the rest.
type fakeSource struct {
	home  source.Home
	err   error
	calls int
	done  chan struct{}
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

func unsupported(op string) error {
	return &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: op}
}

// newPage builds a page whose main-thread callbacks run inline. The returned
// channel reports that a callback finished, so a test reads state only after
// the page applied it.
func newPage(t *testing.T, deps Deps) (*Page, chan struct{}) {
	t.Helper()
	finished := make(chan struct{}, 8)
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
	page.redraw = func() {}
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
	if page.notice != noticeFollowed {
		t.Fatalf("notice %v", page.notice)
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
	if page.notice != noticePlayer {
		t.Fatalf("missing player must be explained, notice %v", page.notice)
	}
	page.detail(card)
	if page.notice != noticeDetail {
		t.Fatalf("missing detail page must be explained, notice %v", page.notice)
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
	page, finished := newPage(t, Deps{Source: &fakeSource{home: sampleHome()}})
	load(t, page, finished)
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	// Dune is not flagged 4K, so the hero wears the HD quality chip beside the
	// Dolby Vision brand chip.
	for _, want := range []string{"1080P", "Dolby Vision"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
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
