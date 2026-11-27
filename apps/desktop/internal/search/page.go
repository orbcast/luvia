// Package search is the search destination: one keyword line, a short history
// of what was searched this session, and the titles the source answered,
// grouped into one shelf per kind. The page keeps view state only; every title
// it shows comes from the source, and the viewer's followed set from the local
// library. The layout and its reasoning are in docs/search-page.md.
package search

import (
	"context"
	"errors"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
)

const (
	// requestTimeout keeps a stalled upstream from pinning a search forever.
	requestTimeout = 30 * time.Second
	// debounce is how long typing may settle before the box searches by
	// itself, so a keyword is asked once instead of once per keystroke.
	debounce = 300 * time.Millisecond
	// historyLimit is how many submitted keywords the page remembers.
	historyLimit = 8
)

// errNoSource is reported when no rule directory produced a usable source.
var errNoSource = errors.New("no source rule is configured")

// Deps are the collaborators the page reads and calls. The page owns no
// network or storage of its own.
type Deps struct {
	Source   source.Source
	Library  *library.Store
	Images   *media.Loader
	Run      func(func())
	Now      func() time.Time
	OnPlay   func(source.Ref)
	OnDetail func(source.Ref)
}

// Page is the search state. Everything here is touched on the main thread:
// the view, the callbacks the controls run, and the goroutine answers handed
// back through Deps.Run.
type Page struct {
	deps Deps

	// keyword is what the box holds, the field the input binds; submitted is
	// the keyword the results on screen answer, which a Retry repeats.
	keyword   string
	submitted string
	history   []string

	cards []source.Card
	// searched reports whether a keyword was answered at least once, so the
	// idle state is told apart from an empty answer.
	searched bool
	loading  bool
	err      error
	// gen is bumped per request so a late answer can be dropped.
	gen uint64
	// timer is the typing debounce in flight, cancelled by a newer keystroke
	// or a submit.
	timer *time.Timer
	// redraw asks for another frame; it is set by the view.
	redraw func()
	// mu guards the fields an answer writes: the request goroutine reads the
	// generation and the view may build a frame while it lands.
	mu sync.Mutex
}

// New prepares the page with an empty box and no history.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	p := &Page{deps: deps}
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	return p
}

// Search runs keyword at once, cancelling any typing debounce first. An empty
// keyword asks nothing: the page returns to its idle state and keeps the
// titles already answered for as long as the box is cleared.
func (p *Page) Search(keyword string) {
	keyword = strings.TrimSpace(keyword)
	p.stopDebounce()
	if keyword == "" {
		// Emptying the box drops the request in flight with its answer.
		p.mu.Lock()
		p.keyword = ""
		p.gen++
		p.loading, p.err = false, nil
		p.submitted = ""
		p.mu.Unlock()
		p.invalidate()
		return
	}
	p.mu.Lock()
	p.keyword = keyword
	p.mu.Unlock()
	p.remember(keyword)
	p.request(keyword)
}

// typed schedules the search the box runs by itself once typing settles. It
// reports whether it took the change at all.
func (p *Page) typed(keyword string) {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" {
		p.stopDebounce()
		return
	}
	p.stopDebounce()
	p.timer = time.AfterFunc(debounce, func() { p.deps.Run(func() { p.Search(keyword) }) })
}

// stopDebounce cancels a typing debounce that has not fired yet.
func (p *Page) stopDebounce() {
	if p.timer != nil {
		p.timer.Stop()
		p.timer = nil
	}
}

// Retry asks the source again for the keyword on screen.
func (p *Page) Retry() {
	if p.submitted == "" {
		p.Search(p.keyword)
		return
	}
	p.request(p.submitted)
}

// request asks for the first page of keyword, keeping what is on screen until
// the answer arrives so the page never blanks out between searches.
func (p *Page) request(keyword string) {
	src := p.deps.Source
	p.mu.Lock()
	p.submitted = keyword
	p.gen++
	gen := p.gen
	if src == nil {
		p.err = errNoSource
		p.loading = false
		p.mu.Unlock()
		p.invalidate()
		return
	}
	p.loading, p.err = true, nil
	p.mu.Unlock()
	p.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		found, err := src.List(ctx, source.ListQuery{Keyword: keyword, Page: 1})
		p.deps.Run(func() {
			p.mu.Lock()
			if gen != p.gen {
				p.mu.Unlock()
				return // a newer search has taken over
			}
			p.loading = false
			p.err = err
			if err == nil {
				p.cards = found.Cards
				p.searched = true
			}
			p.mu.Unlock()
			p.invalidate()
		})
	}()
}

// remember puts keyword at the head of the session history, once, keeping the
// list to its limit.
func (p *Page) remember(keyword string) {
	next := make([]string, 0, len(p.history)+1)
	next = append(next, keyword)
	for _, seen := range p.history {
		if seen != keyword && len(next) < historyLimit {
			next = append(next, seen)
		}
	}
	p.history = next
}

// detail hands a result to the detail page when one is wired.
func (p *Page) detail(card source.Card) {
	if p.deps.OnDetail != nil {
		p.deps.OnDetail(card.Ref)
	}
}

// play hands a result to the player when one is wired.
func (p *Page) play(card source.Card) {
	if p.deps.OnPlay != nil {
		p.deps.OnPlay(card.Ref)
	}
}

// toggleFollow adds or drops a result from the followed set and redraws so the
// bookmark turns with it.
func (p *Page) toggleFollow(card source.Card) {
	if p.deps.Library == nil {
		return
	}
	if p.deps.Library.IsFollowing(card.Ref) {
		if err := p.deps.Library.Unfollow(card.Ref); err == nil {
			p.invalidate()
		}
		return
	}
	if err := p.deps.Library.Follow(library.Follow{
		Media: card.Ref, Title: card.Title, PosterURL: card.PosterURL,
	}); err == nil {
		p.invalidate()
	}
}

// isFollowing reports whether the viewer follows ref, safe without a library.
func (p *Page) isFollowing(ref source.Ref) bool {
	if p.deps.Library == nil {
		return false
	}
	return p.deps.Library.IsFollowing(ref)
}

// invalidate asks for another frame if a view has drawn one yet.
func (p *Page) invalidate() {
	if p.redraw != nil {
		p.redraw()
	}
}
