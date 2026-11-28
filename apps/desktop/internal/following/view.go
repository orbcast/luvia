// Package following is the Following destination: every show the viewer
// follows, newest update first, over the continued titles they were watching.
// The page keeps no state of its own: the local library holds the follows and
// the progress, and the page reads one snapshot per frame, so unwatching a
// title somewhere else shows up here on the next draw.
package following

import (
	"fmt"
	"hash/fnv"
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/poster"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	// posterGap separates the cards, minPoster is the narrowest a card may
	// get, and maxColumns caps a wide window's row so a poster keeps its
	// weight instead of shrinking to a stamp.
	posterGap    = 20
	minPoster    = 120
	maxColumns   = 7
	continueWide = 3
	// continueRatio is a continue card's frame: an episode still, not a poster.
	continueRatio = 9.0 / 16.0
)

// Deps is what the page reads and calls. Every title on it comes from the local
// library, so the page needs no source of its own.
type Deps struct {
	Library  *library.Store
	Images   *media.Loader
	OnPlay   func(ref source.Ref)
	OnDetail func(ref source.Ref)
	// Run carries a background task to the UI thread, and Invalidate asks for
	// the next frame: the page reads the library directly, so these only serve
	// the artwork that arrives after the first draw.
	Run        func(func())
	Invalidate func()
}

// Page draws the Following destination.
type Page struct {
	deps Deps
	// poster draws the follow wall: one card shared with Discover, the library
	// and the search results.
	poster poster.Deps
	// redraw asks the window for a frame, drawn is the fingerprint of the
	// snapshot the previous frame showed, and known says whether there was one:
	// a library that changed elsewhere, on another page or from the source's
	// answer, shows up here without a second visit.
	redraw func()
	drawn  uint64
	known  bool
}

// New prepares the page.
func New(deps Deps) *Page {
	p := &Page{deps: deps, poster: poster.Deps{Images: deps.Images}}
	// The artwork and the library both arrive later and ask for the frame that
	// shows them, as on every other wall of cards.
	p.poster.Invalidate = deps.Invalidate
	p.redraw = deps.Invalidate
	if deps.Images != nil {
		deps.Images.SetChanged(deps.Invalidate)
	}
	return p
}

// View builds the page inside the AppShell's content area.
func (p *Page) View(c *ui.Context) ui.Element {
	t := c.Theme()
	m := locales.In(c.Locale())
	width, _ := c.Size()
	left, right := margins(width)
	snap := p.snapshot()

	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().
			Padding(design.PageTopInset(c.TitleBar().Bottom), right, 64+design.BottomClearance(width), left).
			Gap(32).Children(func() {
			p.changed(snap)
			p.header(c, t, m, snap, width)
			if len(snap.Follows) == 0 && len(snap.Continues) == 0 {
				p.empty(c, t, m)
				return
			}
			p.continueSection(c, t, m, snap, width)
			p.followSection(c, t, m, snap, width)
		})
	})
	return scroll
}

// header is the page's one loud line: its name, and how much it holds, so a
// viewer knows at a glance whether the page is theirs.
func (p *Page) header(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	size := float32(26)
	if design.Docked(width) {
		size = 22
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, m.Following()).FontSize(size).Bold().TextColor(t.Text)
		if count := len(snap.Follows); count > 0 {
			ui.Text(c, m.LibraryCount(count)).FontSize(13).TextColor(t.TextMuted)
		}
	})
}

// continueSection is where the viewer left off: the titles the library holds
// progress for, with their episode and how much is left, so the page answers
// "what was I watching" before "what do I follow".
func (p *Page) continueSection(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	if len(snap.Continues) == 0 {
		return
	}
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, m.ContinueWatching()).FontSize(19).Bold().TextColor(t.Text)
		})
		available := width - design.Inset(width) - design.PageGutter
		if design.Docked(width) {
			available = width - 2*design.NarrowGutter
		}
		columns := float32(continueWide)
		gap := float32(posterGap)
		switch {
		case available < 2*minPoster+gap:
			columns = 1
		case available < 3*minPoster+2*gap:
			columns = 2
		}
		cardW := clamp((available-(columns-1)*gap)/columns, minPoster, 320)
		ui.Row(c).FillWidth().Wrap().Gap(gap).AlignItems(ui.Start).Children(func() {
			for i, progress := range snap.Continues {
				if i >= int(columns)*2 {
					break
				}
				p.continueCard(c, t, m, progress, cardW, i)
			}
		})
	})
}

// continueCard is one continued title: its still, the shading that keeps the
// progress and the play mark readable over a bright frame, and the episode
// under it.
func (p *Page) continueCard(c *ui.Context, t *ui.Theme, m locales.Messages, progress library.Progress, w float32, index int) {
	art := design.OnArtwork(t)
	card := source.Card{Ref: progress.Media, Title: progress.Title, PosterURL: progress.PosterURL}
	button := ui.ButtonBase(c).Width(w).Radius(12).Label(progress.Title)
	button.Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			frame := ui.Box(c).FillWidth().Height(w * continueRatio).Radius(12).Clip().
				Background(t.Surface)
			frame.Children(func() {
				if p.deps.Images != nil {
					if bitmap := p.deps.Images.Bitmap(progress.PosterURL); bitmap != nil {
						ui.Image(c, bitmap).Fill().Fit(ui.Cover)
					}
				}
				ui.Box(c).FillWidth().Height(w * continueRatio * 0.55).Absolute().
					Left(0).Right(0).Bottom(0).LinearGradient(ui.LinearGradient{
					Angle: 180,
					Stops: []ui.GradientStop{
						{Color: ui.Transparent, At: 0},
						{Color: art.Foot.Mix(ui.Transparent, 0.35), At: 0.45},
						{Color: art.Foot, At: 1},
					},
				})
				if progress.Duration > 0 {
					watched := float32(progress.Position.Seconds() / progress.Duration.Seconds() * 100)
					// The watched part goes first and the track covers the rest of the
					// line: a full-width track laid first would be the row's whole width,
					// and the accent would be pushed out of it edge to edge.
					ui.Row(c).Absolute().Left(10).Right(10).Bottom(10).Height(3).Radius(2).Children(func() {
						ui.Box(c).Height(3).Radius(2).Background(t.Accent).WidthPercent(clamp(watched, 0, 100))
						ui.Box(c).FillWidth().Height(3).Radius(2).Background(art.Track)
					})
				}
				ui.Icon(c, icons.Parse("play-bold")).Absolute().Right(10).Bottom(14).Size(12, 12).
					TextColor(art.Ink).Shadow(0, 1, 2, 0, design.PlayMarkShadow())
			})
			ui.Column(c).FillWidth().Gap(2).Children(func() {
				ui.Text(c, progress.Title).FontSize(13).Bold().TextColor(t.Text).MaxLines(1)
				parts := []string{}
				if episode := episodeLabel(m, progress.Episode); episode != "" {
					parts = append(parts, episode)
				}
				if left := progress.Remaining(); left > 0 {
					parts = append(parts, m.TimeLeft(clock(m, left)))
				}
				line := strings.Join(parts, m.ListSeparator())
				if line != "" {
					ui.Text(c, line).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
				}
			})
		})
	})
	if button.Clicked() && p.deps.OnPlay != nil {
		p.deps.OnPlay(card.Ref)
	}
}

// followSection is the wall itself: every followed title, the most recently
// updated first, each opening its detail page.
func (p *Page) followSection(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	if len(snap.Follows) == 0 {
		return
	}
	available := width - design.Inset(width) - design.PageGutter
	if design.Docked(width) {
		available = width - 2*design.NarrowGutter
	}
	gap := float32(posterGap)
	columns := int((available + gap) / (minPoster + gap))
	columns = clampInt(columns, 1, maxColumns)
	cardW := (available-gap*float32(columns-1))/float32(columns) - 0.5

	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, m.FollowingShows()).FontSize(19).Bold().TextColor(t.Text)
			ui.Text(c, m.LibraryCount(len(snap.Follows))).FontSize(12).TextColor(t.TextMuted)
		})
		ui.Row(c).FillWidth().Wrap().Gap(gap).AlignItems(ui.Start).Children(func() {
			for i, follow := range snap.Follows {
				p.followCard(c, t, m, follow, cardW, i)
			}
		})
	})
}

// followCard is one followed show on the shared poster card, its update date
// under the title and its follow switch on the artwork, so the page can let a
// show go without leaving it.
func (p *Page) followCard(c *ui.Context, t *ui.Theme, m locales.Messages, follow library.Follow, w float32, index int) {
	card := source.Card{Ref: follow.Media, Title: follow.Title, PosterURL: follow.PosterURL}
	p.poster.View(c, t, m, poster.Card{
		Ref:       follow.Media,
		Title:     follow.Title,
		PosterURL: follow.PosterURL,
		// The one fact the library keeps about a follow besides its artwork:
		// the day it last updated, marked like any other update line.
		Subtitle:       p.updatedOn(c, follow),
		SubtitleAccent: !follow.UpdatedAt.IsZero(),
	}, w, index, poster.Actions{
		Open: func() {
			if p.deps.OnDetail != nil {
				p.deps.OnDetail(card.Ref)
			}
		},
		Follow: &poster.Follow{
			Ref: card.Ref,
			// The switch's accessible name says what pressing it does, not
			// what the card is: a viewer who lands on it hears "unfollow".
			Label: m.Unfollow(),
			On:    true,
			Toggle: func() {
				if p.deps.Library != nil {
					_ = p.deps.Library.Unfollow(card.Ref)
				}
			},
		},
	})
}

// empty is the page with nothing in it: what a follow is, and where to make
// one, so the page never reads as broken.
func (p *Page) empty(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Radius(16).Background(t.Surface).Border(1, t.Border).
		Padding(28, 26).Gap(6).Children(func() {
		ui.Text(c, m.FollowingEmpty()).FontSize(15).Bold().TextColor(t.Text)
		ui.Text(c, m.FollowingEmptyHint()).FontSize(13).TextColor(t.TextMuted)
	})
}

// updatedOn writes the day a follow last updated, the one fact the library
// keeps about it beyond its title and artwork. The date is written for the
// viewer's locale rather than in the catalog's own month names: "Jan 2" on a
// Chinese page is a date in another language than the one around it, and the
// discovery calendar and the update cards already read theirs the same way.
func (p *Page) updatedOn(c *ui.Context, follow library.Follow) string {
	if follow.UpdatedAt.IsZero() {
		return ""
	}
	return i18n.FormatDate(c.Locale(), follow.UpdatedAt, i18n.DateOptions{Skeleton: "MMMd"})
}

// changed asks for another frame when the library is not what this page last
// drew. The page reads the store every frame, but a store that changed on
// another page would otherwise wait for the next unrelated redraw to be seen
// here. Asking only on a change keeps the chain short: the redraw shows the new
// snapshot, records it, and the frame after it sees nothing new.
func (p *Page) changed(snap library.Snapshot) {
	stamp := fingerprint(snap)
	switch {
	case !p.known:
		// The first frame cannot compare anything; it only records.
		p.known, p.drawn = true, stamp
	case stamp != p.drawn:
		p.drawn = stamp
		if p.redraw != nil {
			p.redraw()
		}
	}
}

// fingerprint is a cheap, stable digest of a snapshot: the page only needs to
// know whether the library moved, not where.
func fingerprint(snap library.Snapshot) uint64 {
	hash := fnv.New64a()
	for _, follow := range snap.Follows {
		fmt.Fprintf(hash, "%s|%s|%s|%d;", follow.Media.Source, follow.Media.ID, follow.Title, follow.UpdatedAt.Unix())
	}
	for _, progress := range snap.Continues {
		fmt.Fprintf(hash, "%s|%s|%s|%d|%d|%d|%d;", progress.Media.Source, progress.Media.ID,
			progress.Title, progress.Episode.Number, progress.Position, progress.Duration,
			progress.UpdatedAt.Unix())
	}
	return hash.Sum64()
}

// snapshot reads the library once per frame, so both sections show one state.
func (p *Page) snapshot() library.Snapshot {
	if p.deps.Library == nil {
		return library.Snapshot{}
	}
	return p.deps.Library.Snapshot()
}

// episodeLabel names the episode the viewer stopped in: its number, the one
// part every source gives an episode, with the title after it when the source
// named one.
func episodeLabel(m locales.Messages, episode source.Episode) string {
	var parts []string
	if episode.Number > 0 {
		parts = append(parts, m.EpisodeNumber(strconv.Itoa(episode.Number)))
	}
	if episode.Title != "" {
		parts = append(parts, episode.Title)
	}
	return strings.Join(parts, m.ListSeparator())
}

// clock writes a duration as the page's own units, without a leading zero
// hour: "42 minutes", "1 hour 5 minutes".
func clock(m locales.Messages, d time.Duration) string {
	hours := int(d / time.Hour)
	minutes := int(d/time.Minute) % 60
	var parts []string
	if hours > 0 {
		parts = append(parts, strconv.Itoa(hours)+" "+m.HoursShort())
	}
	if minutes > 0 || hours == 0 {
		parts = append(parts, strconv.Itoa(minutes)+" "+m.MinutesShort())
	}
	return strings.Join(parts, m.ListSeparator())
}

// margins are the page's left and right gutters: the left one clears the
// floating AppShell, the right one is a normal page gutter.
func margins(width float32) (left, right float32) {
	if design.Docked(width) {
		return design.NarrowGutter, design.NarrowGutter
	}
	return design.Inset(width), design.PageGutter
}

func clamp(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}

func clampInt(value, low, high int) int {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
