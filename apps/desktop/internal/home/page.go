// Package home is the Discover page: a rotating hero, the viewer's unfinished
// playback and this week's followed updates, all fed by one source rule and
// the viewer's local library.
package home

import (
	"context"
	"errors"
	"sync"
	"time"

	"github.com/egoist/mygo"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
)

const (
	heroLimit = 4
	// weekdayCount is Monday through Sunday, the days the week strip lists.
	weekdayCount = 7
	// dayLimit keeps the day requests under what upstreams tolerate, as the
	// week strip asks for every day it shows.
	dayLimit = 30 * time.Second
)

// errNoSource is reported when no rule directory produced a usable source.
var errNoSource = errors.New("no source rule is configured")

// Deps are the collaborators the page reads and calls. The page owns no
// network or storage of its own.
type Deps struct {
	Source   source.Source
	Library  *library.Store
	Images   *media.Loader
	Interval time.Duration
	// Updates reports whether the source has an updates block. When it is
	// false the week strip only counts the viewer's own follows and the page
	// never asks the source for a day.
	Updates  bool
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
	// redraw is what a change asks the window to repaint, and mu guards it with
	// the rest of the state below: a day answer asks for that frame.
	redraw func()

	// mu guards what the week strip shares between the frames that read it and
	// the answers that land while one builds. days caches what the source
	// reported for one weekday, so clicking a day twice shows the second answer
	// without asking again. asked records the days a request left for, false
	// until the source answers, so a day in flight is not asked twice.
	mu        sync.Mutex
	days      map[int][]source.Update
	dayErrors map[int]error
	asked     map[int]bool
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
	p := &Page{
		deps: deps, selected: weekdayIndex(deps.Now()),
		days: map[int][]source.Update{}, dayErrors: map[int]error{}, asked: map[int]bool{},
	}
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
	if p.deps.Updates {
		p.loadWeek()
	}
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

// loadWeek asks the source for every day of the week in progress, one request
// a day. It runs once per page: a day that failed keeps its failure, and the
// viewer sees the follows for that day instead, as clicking the day again only
// reads the cache by design.
func (p *Page) loadWeek() {
	for index := 0; index < weekdayCount; index++ {
		p.loadDay(index)
	}
}

// loadDay asks the source for one weekday. A day already in flight or answered
// is left alone, so the caller can hook this to every selection. The
// bookkeeping, like every other part of the page state, stays on the main
// thread: the answer only carries its weekday back.
func (p *Page) loadDay(index int) {
	if !p.deps.Updates || p.deps.Source == nil {
		return
	}
	if index < 0 || index >= weekdayCount || p.dayRequested(index) {
		return
	}
	p.markAsked(index)
	src := p.deps.Source
	ask := source.Day{Weekday: index + 1}
	run := p.deps.Run
	// The closure carries its own copy of the weekday: loadDay runs in a loop
	// that reuses the parameter, so the goroutine must not read it later.
	go func(weekday int) {
		ctx, cancel := context.WithTimeout(context.Background(), dayLimit)
		defer cancel()
		found, err := src.Updates(ctx, ask)
		run(func() {
			p.acceptDay(weekday, found, err)
		})
	}(index)
}

// markAsked records that a request for the weekday is in flight.
func (p *Page) markAsked(index int) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.asked[index] = false
}

// dayRequested reports whether a request for the weekday left, whether or not
// it came back.
func (p *Page) dayRequested(index int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, sent := p.asked[index]
	return sent
}

// dayAnswered reports whether the source answered the weekday, with titles or
// with an error.
func (p *Page) dayAnswered(index int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	return p.asked[index]
}

// weekPending reports whether some day of the week is still waiting for its
// answer. A page without an updates block has nothing pending.
func (p *Page) weekPending() bool {
	if !p.deps.Updates || p.deps.Source == nil || !p.requested {
		return false
	}
	p.mu.Lock()
	defer p.mu.Unlock()
	for index := 0; index < weekdayCount; index++ {
		if !p.asked[index] {
			return true
		}
	}
	return false
}

// acceptDay stores one day's answer. The page never blanks the strip for a day
// the source could not answer: the follows listed for that day still show.
func (p *Page) acceptDay(index int, found []source.Update, err error) {
	p.mu.Lock()
	if err != nil {
		p.dayErrors[index] = err
	} else {
		p.days[index] = found
	}
	p.asked[index] = true
	p.mu.Unlock()
	p.invalidate()
}

// selectDay shows the updates of one weekday, asking for them when the page has
// no answer yet.
func (p *Page) selectDay(index int) {
	if index < 0 || index >= weekdayCount {
		return
	}
	p.selected = index
	p.loadDay(index)
	p.invalidate()
}

// dayUpdates returns the cached answer for a weekday and whether the source
// answered it at all.
func (p *Page) dayUpdates(index int) ([]source.Update, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()
	found, ok := p.days[index]
	return found, ok
}

// dayLoading reports whether the source has been asked for a weekday and has
// not reported back, which is what the day panel shows while it waits. A day
// without a source or without an updates block is never pending.
func (p *Page) dayLoading(index int) bool {
	if !p.deps.Updates || p.deps.Source == nil || !p.requested {
		return false
	}
	return !p.dayAnswered(index)
}

// dayFailed reports whether the source could not answer a weekday, which is the
// case the panel covers by showing the follows of that day.
func (p *Page) dayFailed(index int) bool {
	p.mu.Lock()
	defer p.mu.Unlock()
	_, failed := p.dayErrors[index]
	return failed
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

// setRedraw remembers how to ask the window for a frame.
func (p *Page) setRedraw(fn func()) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.redraw = fn
}

func (p *Page) invalidate() {
	p.mu.Lock()
	redraw := p.redraw
	p.mu.Unlock()
	if redraw != nil {
		redraw()
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
