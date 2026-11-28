// Package catalog is the resource library: one browsable grid of every title
// a source offers, narrowed by category tabs, a search box and a few filters.
// The page keeps only view state; the catalog itself always comes from the
// source, and the viewer's own followed set from the local library.
package catalog

import (
	"context"
	"errors"
	"sort"
	"time"

	"github.com/egoist/mygo"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/poster"
	"luvia/apps/desktop/internal/source"
)

// requestTimeout keeps a stalled upstream from pinning a load forever.
const requestTimeout = 30 * time.Second

// errNoSource is reported when no rule directory produced a usable source.
var errNoSource = errors.New("no source rule is configured")

// Deps are the collaborators the page reads and calls. The page owns no
// network or storage of its own.
type Deps struct {
	Source   source.Source
	Library  *library.Store
	Images   *media.Loader
	Run      func(func())
	OnPlay   func(source.Ref)
	OnDetail func(source.Ref)
}

// Category tabs, in the order the design's tints and the labels are drawn.
const (
	catAll = iota
	catMovie
	catSeries
	catAnime
	catVariety
	catShort
	catCount
)

// Page is the resource library state. Everything here is touched on the main
// thread: the view, the callbacks the controls run, and the goroutine answers
// handed back through Deps.Run.
type Page struct {
	deps Deps

	query     string
	category  int
	favorites bool
	typeMode  int
	status    int
	sortMode  int
	grid      bool

	// cards is the raw answer for the current category and keyword; the
	// filters below narrow it at draw time so they cost no extra request.
	cards []source.Card

	loading bool
	err     error
	started bool
	gen     uint64 // bumped per request so a late answer can be dropped
	redraw  func()
	// poster draws every wall of titles on the page: one card shared with
	// Discover, Following and the search results.
	poster poster.Deps
}

// New prepares the page with the defaults the reference lays out: the "all"
// category in a poster grid.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	p := &Page{deps: deps, category: catAll, grid: true, poster: poster.Deps{Images: deps.Images}}
	p.poster.Invalidate = p.invalidate
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	return p
}

// Start asks the source for the first page once. Retry and every category or
// search change load again.
func (p *Page) Start() {
	if p.started {
		return
	}
	p.started = true
	p.reload()
}

// reload asks for the current category and keyword, discarding any answer that
// belongs to an earlier request.
func (p *Page) reload() {
	if p.deps.Source == nil {
		p.err = errNoSource
		p.loading = false
		p.invalidate()
		return
	}
	p.gen++
	gen := p.gen
	keyword := p.query
	kind := categoryKind(p.category)
	src := p.deps.Source
	p.loading, p.err = true, nil
	p.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		found, err := src.List(ctx, source.ListQuery{Keyword: keyword, Kind: kind, Page: 0})
		p.deps.Run(func() {
			if gen != p.gen {
				return // a newer request has taken over
			}
			p.loading = false
			if err != nil {
				p.err = err
				p.cards = nil
			} else {
				p.cards = found.Cards
			}
			p.invalidate()
		})
	}()
}

// selectCategory switches tabs and loads that category's first page.
func (p *Page) selectCategory(index int) {
	if index < 0 || index >= catCount || index == p.category {
		return
	}
	p.category = index
	p.reload()
}

// Retry forgets the failed answer and asks the source again.
func (p *Page) Retry() {
	p.reload()
}

// visible narrows the loaded cards by the client-side filters and orders them,
// so a change of filter redraws without a round trip.
func (p *Page) visible() []source.Card {
	out := make([]source.Card, 0, len(p.cards))
	for _, card := range p.cards {
		if p.category == catShort && !isShort(card) {
			continue
		}
		if p.favorites && !p.isFollowing(card.Ref) {
			continue
		}
		if !p.typeMatches(card) {
			continue
		}
		if !p.statusMatches(card) {
			continue
		}
		out = append(out, card)
	}
	switch p.sortMode {
	case 1: // latest first; the source's own order breaks ties
		sort.SliceStable(out, func(i, j int) bool { return out[i].Year > out[j].Year })
	case 2:
		// No rating travels in a Card, so the choice stays visible and the
		// order is left as the source returned it.
	}
	return out
}

// typeMatches applies the type drop-down on top of the category tab.
func (p *Page) typeMatches(card source.Card) bool {
	switch p.typeMode {
	case 1:
		return card.Kind == source.Movie
	case 2:
		return card.Kind == source.Series
	case 3:
		return card.Kind == source.Anime
	case 4:
		return card.Kind == source.Variety
	case 5:
		return isShort(card)
	default:
		return true
	}
}

// statusMatches applies the status drop-down.
func (p *Page) statusMatches(card source.Card) bool {
	switch p.status {
	case 1:
		return isUpdating(card)
	case 2:
		return !isUpdating(card)
	default:
		return true
	}
}

// categoryKind maps a tab onto the source query. The "all" and "short" tabs
// both list without a kind; "short" narrows by metadata at draw time because
// the upstream folds short dramas into series.
func categoryKind(index int) source.Kind {
	switch index {
	case catMovie:
		return source.Movie
	case catSeries:
		return source.Series
	case catAnime:
		return source.Anime
	case catVariety:
		return source.Variety
	default:
		return ""
	}
}

// detail hands the card to the detail page when one is wired.
func (p *Page) detail(card source.Card) {
	if p.deps.OnDetail != nil {
		p.deps.OnDetail(card.Ref)
	}
}

// play hands the card to the player when one is wired.
func (p *Page) play(card source.Card) {
	if p.deps.OnPlay != nil {
		p.deps.OnPlay(card.Ref)
	}
}

// toggleFollow adds or drops the card from the followed set and redraws so the
// bookmark turns with it, as the switch on the poster runs.
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
