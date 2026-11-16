// Package home is the Discover page: a rotating hero, the viewer's unfinished
// playback and this week's followed updates, all fed by one source rule and
// the viewer's local library.
package home

import (
	"context"
	"errors"
	"time"

	"github.com/egoist/mygo"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
)

const heroLimit = 4

// errNoSource is reported when no rule directory produced a usable source.
var errNoSource = errors.New("no source rule is configured")

// Deps are the collaborators the page reads and calls. The page owns no
// network or storage of its own.
type Deps struct {
	Source   source.Source
	Library  *library.Store
	Images   *media.Loader
	Interval time.Duration
	Run      func(func())
	Now      func() time.Time
	OnPlay   func(source.Ref)
	OnDetail func(source.Ref)
	OnAll    func()
}

type notice int

const (
	noticeNone notice = iota
	noticePlayer
	noticeDetail
	noticeFollowed
	noticeUnfollowed
)

// Page is the Discover page state. Everything here is touched on the main
// thread: the view, the carousel ticker and every callback run through Deps.Run.
type Page struct {
	deps      Deps
	home      source.Home
	featured  []source.Card
	hero      int
	hovering  bool
	selected  int
	requested bool
	loading   bool
	err       error
	notice    notice
	redraw    func()
}

// New prepares the page. Interval is how long a hero slide stays before the
// next one, and zero turns rotation off.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	if deps.Now == nil {
		deps.Now = time.Now
	}
	p := &Page{deps: deps, selected: weekdayIndex(deps.Now())}
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	if deps.Interval > 0 {
		go p.rotate()
	}
	return p
}

// Start asks the source for its home sections once. Retry starts again.
func (p *Page) Start() {
	if p.deps.Source == nil {
		p.err = errNoSource
		return
	}
	if p.requested {
		return
	}
	p.requested, p.loading, p.err = true, true, nil
	src := p.deps.Source
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		found, err := src.Home(ctx)
		p.deps.Run(func() {
			p.loading = false
			if err != nil {
				p.err = err
			} else {
				p.accept(found)
			}
			p.invalidate()
		})
	}()
}

// Retry forgets the failed answer and asks the source again.
func (p *Page) Retry() {
	p.requested, p.err = false, nil
	p.Start()
}

func (p *Page) accept(found source.Home) {
	p.home = found
	p.featured = featured(found)
	p.hero = 0
}

// featured picks the section whose cards carry the widest artwork, which is
// what an immersive banner needs, and falls back to the first section.
func featured(found source.Home) []source.Card {
	best, bestScore := -1, 0
	for i, section := range found.Sections {
		score := 0
		for _, card := range section.Cards {
			if card.BackdropURL != "" {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = i, score
		}
	}
	if best < 0 {
		for i, section := range found.Sections {
			if len(section.Cards) > 0 {
				best = i
				break
			}
		}
	}
	if best < 0 {
		return nil
	}
	cards := found.Sections[best].Cards
	if len(cards) > heroLimit {
		cards = cards[:heroLimit]
	}
	return cards
}

func (p *Page) rotate() {
	ticker := time.NewTicker(p.deps.Interval)
	defer ticker.Stop()
	for range ticker.C {
		p.deps.Run(p.advance)
	}
}

// advance moves to the next slide unless the viewer is looking at the hero.
func (p *Page) advance() {
	if p.hovering || len(p.featured) < 2 {
		return
	}
	p.hero = (p.hero + 1) % len(p.featured)
	p.invalidate()
}

// Show makes slide i the hero.
func (p *Page) Show(i int) {
	if i < 0 || i >= len(p.featured) || i == p.hero {
		return
	}
	p.hero = i
	p.invalidate()
}

func (p *Page) invalidate() {
	if p.redraw != nil {
		p.redraw()
	}
}

func (p *Page) flash(n notice) {
	p.notice = n
	p.invalidate()
	time.AfterFunc(4*time.Second, func() {
		p.deps.Run(func() {
			if p.notice == n {
				p.notice = noticeNone
				p.invalidate()
			}
		})
	})
}

func (p *Page) toggleFollow(card source.Card) {
	if p.deps.Library == nil {
		return
	}
	if p.deps.Library.IsFollowing(card.Ref) {
		if err := p.deps.Library.Unfollow(card.Ref); err != nil {
			return
		}
		p.flash(noticeUnfollowed)
		return
	}
	err := p.deps.Library.Follow(library.Follow{
		Media: card.Ref, Title: card.Title, PosterURL: card.PosterURL,
	})
	if err == nil {
		p.flash(noticeFollowed)
	}
}

func (p *Page) play(card source.Card) {
	if p.deps.OnPlay != nil {
		p.deps.OnPlay(card.Ref)
		return
	}
	p.flash(noticePlayer)
}

func (p *Page) detail(card source.Card) {
	if p.deps.OnDetail != nil {
		p.deps.OnDetail(card.Ref)
		return
	}
	p.flash(noticeDetail)
}

// weekStart is Monday at midnight, local time.
func weekStart(now time.Time) time.Time {
	day := now.AddDate(0, 0, -weekdayIndex(now))
	return time.Date(day.Year(), day.Month(), day.Day(), 0, 0, 0, 0, day.Location())
}

// weekdayIndex numbers weekdays from Monday, the order the week strip shows.
func weekdayIndex(t time.Time) int {
	return (int(t.Weekday()) + 6) % 7
}

// updatesOn lists the followed media updated on day, newest first as the
// snapshot already orders them.
func updatesOn(follows []library.Follow, day time.Time) []library.Follow {
	var found []library.Follow
	for _, follow := range follows {
		if follow.UpdatedAt.IsZero() {
			continue
		}
		at := follow.UpdatedAt.In(day.Location())
		if at.Year() == day.Year() && at.YearDay() == day.YearDay() {
			found = append(found, follow)
		}
	}
	return found
}

// snapshot reads the library once per frame so every section sees one state.
func (p *Page) snapshot() library.Snapshot {
	if p.deps.Library == nil {
		return library.Snapshot{}
	}
	return p.deps.Library.Snapshot()
}

// progressOf learns whether the viewer stopped in the middle of a media.
func (p *Page) progressOf(ref source.Ref, snap library.Snapshot) (library.Progress, bool) {
	for _, progress := range snap.Continues {
		if progress.Media == ref {
			return progress, true
		}
	}
	return library.Progress{}, false
}
