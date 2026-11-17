// Package detail is the title page: a poster and record for one media, its
// synopsis, cast and crew, and the episode grid that starts playback. It reads
// the record from a source rule and the viewer's follow state from the local
// library, and owns no network or storage of its own.
package detail

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
)

// requestTimeout keeps a stalled upstream from pinning a title page forever.
const requestTimeout = 30 * time.Second

// errNoSource is reported when no rule directory produced a usable source.
var errNoSource = errors.New("no source rule is configured")

// Tabs on the title page, in the order the pill rail draws them.
const (
	tabOverview = iota
	tabCast
	tabComments
	tabCrew
	tabCount
)

// Deps are the collaborators the page reads and calls.
type Deps struct {
	Source  source.Source
	Library *library.Store
	Images  *media.Loader
	Run     func(func())
	// OnPlay opens the player at the chosen episode index.
	OnPlay func(source.Detail, []source.Episode, int)
	// OnBack leaves the title page for whatever opened it.
	OnBack func()
}

// Page is the title page state. Everything here is touched on the main thread.
type Page struct {
	deps Deps

	ref      source.Ref
	detail   source.Detail
	episodes []source.Episode

	loading  bool
	err      error
	started  bool
	gen      uint64 // bumped per request so a late answer is dropped
	tab      int
	selected int // chosen episode index, -1 when none
	expanded bool
	redraw   func()
}

// New prepares the page. Call Open with a reference to load a title.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	p := &Page{deps: deps, selected: -1}
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	return p
}

// Open resets the page to ref and starts loading it.
func (p *Page) Open(ref source.Ref) {
	p.ref = ref
	p.detail = source.Detail{}
	p.episodes = nil
	p.loading, p.err, p.started = false, nil, false
	p.tab, p.expanded, p.selected = tabOverview, false, -1
	p.Start()
}

// Start loads the detail and its episodes once.
func (p *Page) Start() {
	if p.started || p.ref.IsZero() {
		return
	}
	p.started = true
	p.reload()
}

// Retry forgets the failed answer and asks again.
func (p *Page) Retry() {
	p.started = false
	p.Start()
}

func (p *Page) reload() {
	if p.deps.Source == nil {
		p.err, p.loading = errNoSource, false
		p.invalidate()
		return
	}
	p.gen++
	gen := p.gen
	ref := p.ref
	src := p.deps.Source
	p.loading, p.err = true, nil
	p.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		found, err := src.Detail(ctx, ref)
		var episodes []source.Episode
		if err == nil {
			episodes, err = src.Episodes(ctx, ref)
		}
		p.deps.Run(func() {
			if gen != p.gen {
				return // a newer Open took over
			}
			p.loading = false
			if err != nil {
				p.err = err
				p.detail, p.episodes = source.Detail{}, nil
			} else {
				p.detail, p.episodes = found, episodes
				p.selected = p.resumeIndex()
			}
			p.invalidate()
		})
	}()
}

// resumeIndex finds the episode the viewer was last on, else the first.
func (p *Page) resumeIndex() int {
	if len(p.episodes) == 0 {
		return -1
	}
	if p.deps.Library != nil {
		if progress, ok := p.deps.Library.ProgressOf(p.ref); ok {
			for i, episode := range p.episodes {
				if episode.ID == progress.Episode.ID || (episode.Number != 0 && episode.Number == progress.Episode.Number) {
					return i
				}
			}
		}
	}
	return 0
}

// Play asks the player to open episode i.
func (p *Page) Play(i int) {
	if i < 0 || i >= len(p.episodes) {
		return
	}
	p.selected = i
	if p.deps.OnPlay != nil {
		p.deps.OnPlay(p.detail, p.episodes, i)
	}
}

// PlaySelected opens the chosen episode, defaulting to the first.
func (p *Page) PlaySelected() {
	if p.selected < 0 && len(p.episodes) > 0 {
		p.selected = 0
	}
	p.Play(p.selected)
}

// Back leaves the title page.
func (p *Page) Back() {
	if p.deps.OnBack != nil {
		p.deps.OnBack()
	}
}

// SelectTab switches the pill rail.
func (p *Page) SelectTab(i int) {
	if i < 0 || i >= tabCount || i == p.tab {
		return
	}
	p.tab = i
	p.invalidate()
}

// ToggleExpanded shows the full synopsis or cuts it back to a few lines.
func (p *Page) ToggleExpanded() {
	p.expanded = !p.expanded
	p.invalidate()
}

// IsFollowing reports whether the viewer follows this title.
func (p *Page) IsFollowing() bool {
	return p.deps.Library != nil && p.deps.Library.IsFollowing(p.ref)
}

// ToggleFollow adds or drops the title from the followed set.
func (p *Page) ToggleFollow() {
	if p.deps.Library == nil || p.ref.IsZero() {
		return
	}
	if p.deps.Library.IsFollowing(p.ref) {
		if err := p.deps.Library.Unfollow(p.ref); err == nil {
			p.invalidate()
		}
		return
	}
	if err := p.deps.Library.Follow(library.Follow{
		Media: p.ref, Title: p.detail.Title, PosterURL: p.detail.PosterURL,
	}); err == nil {
		p.invalidate()
	}
}

func (p *Page) invalidate() {
	if p.redraw != nil {
		p.redraw()
	}
}
