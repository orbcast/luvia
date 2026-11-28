package search

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// fakeSource answers List from a per-keyword table and remembers every query,
// so a test can prove which keyword was asked and how often.
type fakeSource struct {
	mu      sync.Mutex
	answers map[string][]source.Card
	// errs makes a keyword fail; the rest answer normally.
	errs  map[string]error
	last  []source.ListQuery
	calls int
}

func (f *fakeSource) ID() string   { return "fake" }
func (f *fakeSource) Name() string { return "Fake" }

func (f *fakeSource) Home(context.Context) (source.Home, error) {
	return source.Home{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "home"}
}

func (f *fakeSource) List(_ context.Context, query source.ListQuery) (source.Page, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.calls++
	f.last = append(f.last, query)
	if err, ok := f.errs[query.Keyword]; ok {
		return source.Page{}, err
	}
	return source.Page{Cards: f.answers[query.Keyword], Page: query.Page}, nil
}

func (f *fakeSource) Detail(context.Context, source.Ref) (source.Detail, error) {
	return source.Detail{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "detail"}
}

func (f *fakeSource) Episodes(context.Context, source.Ref) ([]source.Episode, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "episodes"}
}

func (f *fakeSource) Resolve(context.Context, source.Episode, string) (source.Playback, error) {
	return source.Playback{}, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "resolve"}
}

func (f *fakeSource) Updates(context.Context, source.Day) ([]source.Update, error) {
	return nil, &source.Error{Kind: source.ErrUnsupported, Source: "fake", Op: "updates"}
}

// queries returns the keywords asked, in order.
func (f *fakeSource) queries() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	out := make([]string, 0, len(f.last))
	for _, query := range f.last {
		out = append(out, query.Keyword)
	}
	return out
}

// lastQuery returns the most recent query.
func (f *fakeSource) lastQuery() source.ListQuery {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.last) == 0 {
		return source.ListQuery{}
	}
	return f.last[len(f.last)-1]
}

// fail makes keyword fail with err.
func (f *fakeSource) fail(keyword string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.errs == nil {
		f.errs = map[string]error{}
	}
	f.errs[keyword] = err
}

// heal lets keyword answer again.
func (f *fakeSource) heal(keyword string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	delete(f.errs, keyword)
}

// gate holds a request until the test opens it, so a second search can start
// while the first is still in flight.
type gate struct {
	open     chan struct{}
	released bool
}

func newGate() *gate { return &gate{open: make(chan struct{})} }

func (g *gate) release() {
	if !g.released {
		g.released = true
		close(g.open)
	}
}

// gatedSource answers the keywords given to gate once the gate opens, and
// everything else at once.
type gatedSource struct {
	*fakeSource
	gates map[string]*gate
}

func (g *gatedSource) List(ctx context.Context, query source.ListQuery) (source.Page, error) {
	if held, ok := g.gates[query.Keyword]; ok {
		<-held.open
	}
	return g.fakeSource.List(ctx, query)
}

func movie(id, title string) source.Card {
	return source.Card{
		Ref: source.Ref{Source: "fake", ID: id}, Title: title, Kind: source.Movie,
		Year: 2021, Area: "US", Remarks: "4K",
	}
}

func series(id, title string) source.Card {
	return source.Card{
		Ref: source.Ref{Source: "fake", ID: id}, Title: title, Kind: source.Series,
		Year: 2022, Tags: []string{"updating"},
	}
}

// newPage builds a page whose main-thread callbacks are queued for the test to
// run on its own goroutine, mirroring the app's hand-off to the main thread so
// state is never written while a frame is building.
func newPage(t *testing.T, deps Deps) (*Page, chan func()) {
	t.Helper()
	queued := make(chan func(), 32)
	deps.Run = func(fn func()) { queued <- fn }
	if deps.Library == nil {
		store, err := library.Open("")
		if err != nil {
			t.Fatal(err)
		}
		deps.Library = store
	}
	page := New(deps)
	page.redraw = func() {}
	return page, queued
}

// wait runs the next queued main-thread callback, applying its state change.
func wait(t *testing.T, queued chan func()) {
	t.Helper()
	select {
	case fn := <-queued:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("the search answer never arrived")
	}
}

// render builds a tester over the page, applying the design theme the AppShell
// would, so the page draws with its real palette in tests too.
func render(page *Page, width, height int, dark bool) *ui.Tester {
	tt := ui.NewTesterWith(func(c *ui.Context) {
		theme := design.Theme(c.Theme())
		c.SetTheme(&theme)
		page.View(c)
	}, width, height, ui.TesterOptions{Dark: dark})
	tt.SetLocale("en")
	return tt
}

func TestSubmittingKeywordSearches(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"dune": {movie("dune", "Dune"), series("severance", "Severance")},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()
	if !tt.HasText("Search the catalog") {
		t.Fatalf("idle state missing in %v", tt.Texts())
	}

	page.Search("dune")
	tt.Frame()
	if !tt.HasText("Loading the library") {
		t.Fatalf("loading state missing in %v", tt.Texts())
	}
	wait(t, finished)
	tt.Frame()

	if query := src.lastQuery(); query.Keyword != "dune" || query.Page != 1 {
		t.Fatalf("query was %+v, want keyword dune on page 1", query)
	}
	// Each kind gets its own shelf, named and counted, and a show's second
	// line says where it stands.
	for _, want := range []string{"Dune", "Severance", "4K", "1080P", "Series", "Movies", "Updating"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
}

func TestTypingWithEnterRequestsTheKeyword(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{"dune": {movie("dune", "Dune")}}}
	page, finished := newPage(t, Deps{Source: src})
	// The page owns the debounce itself, and a test does not run a clock.
	page.redraw = func() {}
	tt := render(page, 1100, 720, false)
	tt.Frame()

	if err := tt.Click("Search"); err != nil {
		t.Fatalf("focus the box: %v", err)
	}
	tt.Type("dune")
	tt.Frame()
	tt.Key(0, ui.KeyEnter)
	tt.Frame()

	if got := src.queries(); len(got) == 0 || got[0] != "dune" {
		t.Fatalf("queries are %v, want dune from Enter", got)
	}
	// Enter submits at once; a pending debounce must not ask twice.
	page.stopDebounce()
	wait(t, finished)
	tt.Frame()
	if !tt.HasText("Dune") {
		t.Fatalf("result missing in %v", tt.Texts())
	}
}

func TestEmptySubmitAsksNothing(t *testing.T) {
	src := &fakeSource{}
	page, queued := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()

	page.Search("   ")
	page.Search("")
	select {
	case fn := <-queued:
		fn()
		t.Fatalf("an empty keyword asked the source: %v", src.queries())
	case <-time.After(100 * time.Millisecond):
	}
	tt.Frame()
	if got := src.queries(); len(got) != 0 {
		t.Fatalf("queries are %v, want none", got)
	}
	if page.submitted != "" || page.loading {
		t.Fatalf("empty submit left search %q loading %v", page.submitted, page.loading)
	}
	if !tt.HasText("Search the catalog") {
		t.Fatalf("idle state missing in %v", tt.Texts())
	}
}

func TestLateAnswerIsDropped(t *testing.T) {
	base := &fakeSource{answers: map[string][]source.Card{
		"dune":      {movie("dune", "Dune")},
		"severance": {series("severance", "Severance")},
	}}
	src := &gatedSource{fakeSource: base, gates: map[string]*gate{"dune": newGate()}}
	page, queued := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()

	page.Search("dune")
	page.Search("severance")
	// The second answer lands first, then the stale one.
	wait(t, queued)
	tt.Frame()
	src.gates["dune"].release()
	wait(t, queued)
	tt.Frame()

	if page.loading {
		t.Fatal("page is still loading")
	}
	if page.err != nil {
		t.Fatalf("page reports %v", page.err)
	}
	if !tt.HasText("Severance") {
		t.Fatalf("the newer answer is missing in %v", tt.Texts())
	}
	if tt.HasText("Dune") {
		t.Fatalf("the stale answer landed anyway: %v", tt.Texts())
	}
	if len(page.cards) != 1 || page.cards[0].ID != "severance" {
		t.Fatalf("cards are %+v, want only the newer answer", page.cards)
	}
}

func TestErrorShowsAndRetryRepeats(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{"dune": {movie("dune", "Dune")}}}
	src.fail("dune", errors.New("boom"))
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()

	page.Search("dune")
	wait(t, finished)
	tt.Frame()
	if !tt.HasText("This library could not load") || !tt.HasText("boom") || !tt.HasText("Retry") {
		t.Fatalf("error state missing in %v", tt.Texts())
	}

	before := len(src.queries())
	src.heal("dune")
	if err := tt.Click("Retry"); err != nil {
		t.Fatalf("click Retry: %v", err)
	}
	tt.Frame()
	wait(t, finished)
	tt.Frame()

	if got := src.queries(); len(got) != before+1 || got[len(got)-1] != "dune" {
		t.Fatalf("queries are %v, want dune asked again", got)
	}
	if page.err != nil {
		t.Fatalf("retry kept the error: %v", page.err)
	}
	if !tt.HasText("Dune") {
		t.Fatalf("retry did not load results in %v", tt.Texts())
	}
}

func TestHistoryChipRerunsKeyword(t *testing.T) {
	cards := map[string][]source.Card{
		"dune":      {movie("dune", "Dune")},
		"severance": {series("severance", "Severance")},
	}
	src := &fakeSource{answers: cards}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()

	page.Search("dune")
	wait(t, finished)
	tt.Frame()
	page.Search("severance")
	wait(t, finished)
	tt.Frame()

	if len(page.history) != 2 || page.history[0] != "severance" || page.history[1] != "dune" {
		t.Fatalf("history is %v, want the newest keyword first", page.history)
	}
	// Both chips show while the newest keyword's titles are on screen.
	for _, chip := range []string{"dune", "severance"} {
		if _, ok := tt.Find(chip); !ok {
			t.Fatalf("history chip %q missing in %v", chip, tt.Texts())
		}
	}

	before := len(src.queries())
	if err := tt.Click("dune"); err != nil {
		t.Fatalf("click history chip: %v", err)
	}
	tt.Frame()
	wait(t, finished)
	tt.Frame()

	if got := src.queries(); len(got) != before+1 || got[len(got)-1] != "dune" {
		t.Fatalf("queries are %v, want dune asked again", got)
	}
	if !tt.HasText("Dune") {
		t.Fatalf("chip did not run its search in %v", tt.Texts())
	}
	if page.keyword != "dune" || page.submitted != "dune" {
		t.Fatalf("box holds %q and answers %q, want dune", page.keyword, page.submitted)
	}
}

func TestHistoryKeepsEightKeywords(t *testing.T) {
	src := &fakeSource{}
	page, queued := newPage(t, Deps{Source: src})
	for i := 0; i < 12; i++ {
		page.Search(fmt.Sprintf("word %d", i))
		wait(t, queued)
	}
	if len(page.history) != historyLimit {
		t.Fatalf("history holds %d keywords, want %d", len(page.history), historyLimit)
	}
	if page.history[0] != "word 11" {
		t.Fatalf("history head is %q, want the newest keyword", page.history[0])
	}
	// A repeated keyword moves to the head instead of appearing twice.
	page.Search("word 11")
	wait(t, queued)
	seen := map[string]int{}
	for _, keyword := range page.history {
		seen[keyword]++
	}
	if seen["word 11"] != 1 || len(page.history) != historyLimit {
		t.Fatalf("history is %v, want one head entry of the repeated keyword", page.history)
	}
}

func TestBookmarkFollowsResult(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{"dune": {movie("dune", "Dune")}}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	ref := source.Ref{Source: "fake", ID: "dune"}
	if page.isFollowing(ref) {
		t.Fatal("result starts followed")
	}
	if err := tt.Click("Follow"); err != nil {
		t.Fatalf("click bookmark: %v", err)
	}
	tt.Frame()
	if !page.isFollowing(ref) {
		t.Fatal("bookmark did not follow the result")
	}
	if err := tt.Click("Following"); err != nil {
		t.Fatalf("click bookmark again: %v", err)
	}
	tt.Frame()
	if page.isFollowing(ref) {
		t.Fatal("bookmark did not unfollow the result")
	}
}

func TestOpeningResultCallsDetail(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{"dune": {movie("dune", "Dune")}}}
	var opened []source.Ref
	page, finished := newPage(t, Deps{
		Source: src, OnDetail: func(ref source.Ref) { opened = append(opened, ref) },
	})
	tt := render(page, 1100, 720, false)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	if err := tt.Click("Dune"); err != nil {
		t.Fatalf("click result: %v", err)
	}
	if len(opened) != 1 || opened[0].ID != "dune" {
		t.Fatalf("detail opened for %v, want the result", opened)
	}
}

func TestClearingBoxKeepsAnsweredTitles(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{"dune": {movie("dune", "Dune")}}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, false)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	page.Search("")
	tt.Frame()
	if !tt.HasText("Dune") {
		t.Fatalf("clearing the box blanked the results: %v", tt.Texts())
	}
	// Retry with nothing submitted asks the box again, not the old keyword.
	before := len(src.queries())
	page.Retry()
	select {
	case fn := <-finished:
		fn()
		t.Fatalf("retry with an empty box asked the source: %v", src.queries())
	case <-time.After(100 * time.Millisecond):
	}
	if got := src.queries(); len(got) != before {
		t.Fatalf("queries are %v, want no new one", got)
	}
}

func TestMissingSourceReportsAnError(t *testing.T) {
	page, queued := newPage(t, Deps{})
	tt := render(page, 1100, 720, false)
	tt.Frame()

	page.Search("dune")
	tt.Frame()
	if page.err == nil {
		t.Fatal("search without a source reported no error")
	}
	if !tt.HasText("Retry") {
		t.Fatalf("error state missing in %v", tt.Texts())
	}
	select {
	case fn := <-queued:
		fn()
		t.Fatalf("a missing source started a request")
	default:
	}
}

func TestNarrowLayoutDrawsResults(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"dune": {movie("dune", "Dune"), series("severance", "Severance")},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 430, 820, true)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	if !tt.HasText("Dune") || !tt.HasText("Severance") {
		t.Fatalf("narrow layout lost results in %v", tt.Texts())
	}
	if _, ok := tt.Find("Search"); !ok {
		t.Fatalf("search box missing in %v", tt.Texts())
	}
}

func TestIdleKeepsRememberedKeywords(t *testing.T) {
	page, finished := newPage(t, Deps{Source: &fakeSource{}})
	tt := render(page, 1100, 720, true)
	tt.Frame()

	page.Search("dune")
	wait(t, finished)
	page.Search("")
	tt.Frame()

	// The box is empty again, so the page offers what it searched instead of
	// claiming the source had no match.
	if !tt.HasText("Search the catalog") || !tt.HasText("dune") {
		t.Fatalf("idle state lost its remembered keyword in %v", tt.Texts())
	}
	if tt.HasText("has no match") {
		t.Fatalf("an empty box reported no results: %v", tt.Texts())
	}
}

func TestAnsweredTitleHighlightsTheKeyword(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"Dune": {movie("part-two", "Dune: Part Two")},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, true)
	tt.Frame()
	page.Search("Dune")
	wait(t, finished)
	tt.Frame()

	// The match is drawn as its own run of text, so the accent can color it.
	var runs []string
	for _, text := range tt.Texts() {
		if text == "Dune" || strings.HasPrefix(text, ": Part Two") {
			runs = append(runs, text)
		}
	}
	if !strings.Contains(strings.Join(runs, "|"), "Dune|: Part Two") {
		t.Fatalf("the keyword is not its own run in %v", tt.Texts())
	}
}

func TestEmptyStateRetryAsksAgain(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, true)
	tt.Frame()
	page.Search("nothing")
	wait(t, finished)
	tt.Frame()

	if !tt.HasText("has no match") || !tt.HasText("nothing") {
		t.Fatalf("empty state missing in %v", tt.Texts())
	}
	before := len(src.queries())
	if err := tt.Click("Retry"); err != nil {
		t.Fatalf("clicking retry: %v", err)
	}
	wait(t, finished)
	if got := src.queries(); len(got) != before+1 || got[len(got)-1] != "nothing" {
		t.Fatalf("queries are %v, want nothing again", got)
	}
}

func TestShelvesFollowTheKindOrder(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"dune": {
			{Ref: source.Ref{ID: "anime"}, Title: "Anime Hit", Kind: source.Anime},
			movie("movie", "Movie Hit"),
			{Ref: source.Ref{ID: "show"}, Title: "Show Hit", Kind: source.Series},
		},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, true)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	var order []string
	for _, text := range tt.Texts() {
		switch text {
		case "Series", "Movies", "Anime":
			order = append(order, text)
		}
	}
	if strings.Join(order, ">") != "Series>Movies>Anime" {
		t.Fatalf("shelf order is %v, want Series, Movies, Anime", order)
	}
}

func TestGridFillsTheContentWidth(t *testing.T) {
	// A docked window still holds two columns, and neither falls under the
	// narrowest a poster may get.
	w, narrowGap, columns := gridFor(430 - design.NarrowGutter*2)
	if columns < 2 {
		t.Fatalf("narrow grid holds %d columns at %v wide posters", columns, w)
	}
	if w < narrowPoster {
		t.Fatalf("narrow grid's poster is %v wide, under %v", w, narrowPoster)
	}
	if narrowGap != posterGap {
		t.Fatalf("narrow grid's gap is %v, want %v", narrowGap, posterGap)
	}

	// A wide page fills its width: the columns and the gaps add up to the
	// content, so a full row ends where the shelf's head does.
	content := float32(1440 - design.PageInset - design.PageGutter)
	wide, wideGap, wideColumns := gridFor(content)
	if wideGap != posterGap {
		t.Fatalf("wide grid's gap is %v, want %v", wideGap, posterGap)
	}
	span := float32(wideColumns)*wide + float32(wideColumns-1)*wideGap
	// Columns divide the width evenly and each keeps half a unit of the
	// share back, so a full row lands on the page's right edge to within half
	// a unit per column. Anything more is a strip left at the edge.
	slack := float32(wideColumns) * 0.5
	if span > content || content-span > slack {
		t.Fatalf("wide grid spans %v of %v: a strip is left at the right", span, content)
	}
}

func TestClearedBoxLabelsTheLastResults(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"dune": {movie("dune", "Dune")},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, true)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	tt.Frame()

	if tt.HasText("Last search") {
		t.Fatalf("a running keyword's results carry a last-search head: %v", tt.Texts())
	}

	// Clear the box as the field's own button does: the keyword and the
	// submitted search part ways, and the view reads that as the last search.
	page.keyword = ""
	page.invalidate()
	tt.Frame()
	if !page.lastSearch() {
		t.Fatalf("cleared box is not read as the last search (submitted %q, box %q, %d cards)",
			page.submitted, page.keyword, len(page.cards))
	}
	if !tt.HasText("Last search") || !tt.HasText("Search again") {
		t.Fatalf("cleared box did not label the last results: %v", tt.Texts())
	}
	if tt.HasText("Search the catalog") || tt.HasText("browse the library") {
		t.Fatalf("the cleared page says two contradictory things: %v", tt.Texts())
	}

	before := len(src.queries())
	if err := tt.Click("Search again"); err != nil {
		t.Fatalf("clicking search again: %v", err)
	}
	wait(t, finished)
	tt.Frame()
	if got := src.queries(); len(got) != before+1 || got[len(got)-1] != "dune" {
		t.Fatalf("queries are %v, want dune again", got)
	}
	if page.lastSearch() || tt.HasText("Last search") {
		t.Fatalf("the head outlived the search it named: %v", tt.Texts())
	}
}

func TestEmptyStateNamesNoUnverifiedKeyword(t *testing.T) {
	src := &fakeSource{answers: map[string][]source.Card{
		"dune": {movie("dune", "Dune")},
	}}
	page, finished := newPage(t, Deps{Source: src})
	tt := render(page, 1100, 720, true)
	tt.Frame()
	page.Search("dune")
	wait(t, finished)
	page.Search("nothing")
	wait(t, finished)
	tt.Frame()

	if !tt.HasText("has no match") {
		t.Fatalf("empty state missing in %v", tt.Texts())
	}
	// "dune" answered, but the empty state may not offer it as something that
	// will answer: the historical words stay on the keyword line, and the
	// page's own copy only tells the user how to search again.
	if !tt.HasText("Try another spelling") {
		t.Fatalf("empty state has no hint: %v", tt.Texts())
	}
	for _, text := range tt.Texts() {
		if strings.Contains(text, "Try") {
			if strings.Contains(text, "dune") {
				t.Fatalf("hint names a keyword: %q", text)
			}
		}
	}
}
