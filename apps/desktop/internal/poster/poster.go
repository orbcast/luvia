// Package poster draws the one 2:3 card every wall of titles shares: the
// artwork, its badges, the follow switch, the play disc a hover brings, and the
// two lines under it. The Discover, Library, Following and Search pages all lay
// their titles out with it, so a title looks and behaves the same wherever a
// viewer meets it.
//
// The package knows what a card looks like, never where a click goes: the page
// hands in the cards, the bitmaps, and the callbacks, and keeps its own business
// state. It depends on the design tokens, the local icons, the source's card
// type, the images and the copy, and on nothing else.
package poster

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

// Ratio is the card's height over its width: a 2:3 poster.
const Ratio = 1.5

const (
	// gap is the space between the poster and the lines under it.
	gap = 9
	// scrimHeight is how far up the poster's foot shading reaches.
	scrimHeight = 96
	// scrimRatio caps the shading at a share of a short poster, so the foot
	// of a small card stays a foot rather than half the artwork.
	scrimRatio = 0.42
	// badgeInset is the corner the quality badge and the follow switch keep,
	// and badgeStep how far a second badge sits under the first.
	badgeInset = 10
	badgeStep  = 20
	// titleHeight and metaHeight reserve one line each, so cards in a row
	// line their second lines up whether or not a title has one.
	titleHeight = 21
	metaHeight  = 18
	// stagger is the delay between neighbours arriving, so a new wall of
	// titles flows in row by row instead of popping all at once.
	stagger = 22 * time.Millisecond
	// staggerSteps keeps a long shelf from waiting on a long delay.
	staggerSteps = 12
)

// Run is one stretch of a title: the words the viewer typed, and the rest.
type Run struct {
	Text  string
	Match bool
}

// Card is what one poster draws.
type Card struct {
	// Ref identifies the title; it keys the card and the follow switch, so a
	// redraw finds the same card in the same place.
	Ref source.Ref
	// Title is the name, and what a screen reader hears for the poster.
	Title string
	// Year and Area make the footnote along the artwork's foot.
	Year int
	Area string
	// Badges are the small chips at the top left, in the order drawn.
	Badges []Badge
	// Note is the quiet line over the footnote's right end, such as when a
	// card last changed.
	Note string
	// PosterURL is the artwork; an empty one leaves the placeholder mark.
	PosterURL string
	// Keywords are the words of Title to draw in the accent; nil for none.
	Keywords []Run
	// Subtitle is an optional second line under the title, drawn muted: the
	// facts of a card, the state of a show, or the viewer's own progress.
	Subtitle string
	// SubtitleAccent colours the subtitle in the accent, for a line that
	// speaks of the viewer's own state rather than the title's.
	SubtitleAccent bool
}

// Badge is a chip on the artwork: a label, and the colours it wears.
type Badge struct {
	Label string
	// Background and Ink are the chip's own colours, from the design tokens.
	Background, Ink ui.Color
}

// Follow is the follow switch on the poster's top right.
type Follow struct {
	// Ref is the title the switch follows; it keys the switch, and is what the
	// callback below closes over.
	Ref source.Ref
	// Label names the switch for readers and for its tooltip.
	Label string
	// On draws the accent disc with a solid bookmark: the title is followed.
	On bool
	// Toggle runs when the switch is pressed.
	Toggle func()
}

// Actions are the callbacks a card offers.
type Actions struct {
	// Open runs on a click anywhere on the poster or its title.
	Open func()
	// Follow, when non-nil, draws the switch on the poster.
	Follow *Follow
}

// Deps are the collaborators a wall of cards draws with.
type Deps struct {
	// Images supplies the artwork; without it every card shows the
	// placeholder mark.
	Images *media.Loader
	// Invalidate asks the window for the next frame when a bitmap arrives, so
	// a poster fades in the moment it loads instead of on the next unrelated
	// redraw.
	Invalidate func()
}

// Key is the frame's card key, and its poster's: a page that needs to find the
// same card across frames keeps it stable.
func Key(ref source.Ref) string { return "card:" + ref.String() }

// imageIn is how long a poster takes to appear once its bitmap arrives.
var imageIn = ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{Opacity: 0}}

// FrameKey is the key of the poster frame inside a card.
func FrameKey(ref source.Ref) string { return "poster:" + ref.String() }

// FollowKey is the key of the follow switch inside a card.
func FollowKey(ref source.Ref) string { return "mark:" + ref.String() }

// View lays one card at width w: the poster above, the title and the subtitle
// under it.
func (d Deps) View(c *ui.Context, t *ui.Theme, m locales.Messages, card Card, w float32, index int, actions Actions) {
	motion := ui.Snappy
	motion.Delay = time.Duration(index%staggerSteps) * stagger
	column := ui.Column(c).Key(Key(card.Ref)).Width(w).Gap(gap).Transition(ui.ElementTransition{
		Duration: 260 * time.Millisecond,
		Enter:    &ui.Motion{Y: 10},
	})
	column.Children(func() {
		d.frame(c, t, m, card, w, motion, actions)
		d.title(c, t, card, actions)
		d.subtitle(c, t, card)
	})
}

// frame is the artwork itself: a clipped 2:3 poster that answers a click, lifts
// under the pointer and shows the play disc when it does.
func (d Deps) frame(c *ui.Context, t *ui.Theme, m locales.Messages, card Card, w float32,
	motion ui.Animation, actions Actions) {
	art := design.OnArtwork(t)
	lifted := false
	frame := ui.ButtonBase(c).Key(FrameKey(card.Ref)).Width(w).Height(w * Ratio).
		Radius(12).Clip().Background(t.Surface).Label(card.Title)
	if frame.Hovered() || frame.FocusWithin() {
		lifted = true
	}
	frame.Children(func() {
		d.artwork(c, t, card, w)
		d.shade(c, art, w)
		for i, badge := range card.Badges {
			d.badge(c, badge, i)
		}
		if actions.Follow != nil {
			d.follow(c, t, *actions.Follow)
		}
		d.footnote(c, m, art, card)
		if lifted {
			d.playDisc(c)
		}
	})
	frame.DrawOver(func(painter *ui.Painter, r ui.Rect) {
		// The hairline sits on the poster's own edge: the clip keeps the
		// outer half, the inner half is the line a viewer sees.
		painter.Stroke(r, art.Border, 12, 1)
	})
	if lifted {
		// The poster rests where the pointer found it and settles back on its
		// own, so passing over a wall does not slide the page.
		frame.AnimatePose(ui.Pose{Y: -3, Scale: 1.012}, motion)
	}
	if frame.Clicked() && actions.Open != nil {
		actions.Open()
	}
}

// artwork is the poster image, or the placeholder while it loads or when the
// source gave none.
func (d Deps) artwork(c *ui.Context, t *ui.Theme, card Card, w float32) {
	if card.PosterURL != "" && d.Images != nil {
		if bitmap := d.Images.Bitmap(card.PosterURL); bitmap != nil {
			ui.Image(c, bitmap).Fill().Fit(ui.Cover).Transition(imageIn)
			return
		}
		if d.Invalidate != nil {
			// Asking for the next frame is what keeps a poster from waiting
			// on some other event to appear; it is a request, not a timer.
			d.Invalidate()
		}
	}
	ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Parse("clapperboard-linear")).Size(30, 30).
			TextColor(t.TextMuted).Opacity(0.45)
	})
}

// shade is the foot shading that keeps the footnote and any line laid over the
// artwork's bottom readable, over a bright poster as over a dark one.
func (d Deps) shade(c *ui.Context, art design.Artwork, w float32) {
	height := float32(scrimHeight)
	if cap := w * Ratio * scrimRatio; height > cap {
		height = cap
	}
	ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(height).
		LinearGradient(ui.LinearGradient{
			// 180 is CSS's angle, 0 pointing up, so it runs from the box's
			// top, the shading's dark end, down to its foot.
			From: art.Foot, To: ui.Transparent, Angle: 180,
		})
}

// badge draws one chip at the top left, stepped down under the ones before it
// so two badges stack instead of covering each other.
func (d Deps) badge(c *ui.Context, badge Badge, index int) {
	ui.Box(c).Absolute().Left(badgeInset).Top(badgeInset+float32(index)*badgeStep).
		Radius(5).Padding(2, 6).Background(badge.Background).Center().Children(func() {
		ui.Text(c, badge.Label).FontSize(10).FontWeight(600).TextColor(badge.Ink)
	})
}

// follow is the bookmark on the poster's top right: a solid accent disc when
// the title is followed, a dark veil when it is not, so it reads over any
// artwork.
func (d Deps) follow(c *ui.Context, t *ui.Theme, follow Follow) {
	button := ui.ButtonBase(c).Key(FollowKey(follow.Ref)).Size(28, 28).
		Radius(14).Label(follow.Label).Tooltip(follow.Label).
		Absolute().Right(badgeInset).Top(badgeInset)
	switch {
	case follow.On:
		button.Background(t.Accent)
	case button.Hovered():
		button.Background(design.PosterVeilOn())
	default:
		button.Background(design.PosterVeil())
	}
	ink := design.PosterVeilInk()
	if follow.On {
		ink = t.AccentText
	}
	button.TextColor(ink)
	button.Children(func() {
		name := "bookmark-linear"
		if follow.On {
			name = "bookmark-bold"
		}
		ui.Icon(c, icons.Parse(name)).Size(14, 14).TextColor(ink)
	})
	if button.Clicked() {
		follow.Toggle()
	}
}

// footnote is the area and year along the poster's foot, with the optional note
// in front of them on the right.
func (d Deps) footnote(c *ui.Context, m locales.Messages, art design.Artwork, card Card) {
	facts := footnoteParts(m, card)
	if len(facts) == 0 && card.Note == "" {
		return
	}
	ui.Row(c).Absolute().Left(badgeInset).Right(badgeInset).Bottom(9).
		AlignItems(ui.Center).Gap(6).Children(func() {
		if len(facts) > 0 {
			ui.Text(c, strings.Join(facts, m.ListSeparator())).FontSize(10).
				LetterSpacing(0.06).TextColor(art.InkDim).MaxLines(1)
		}
		if card.Note == "" {
			return
		}
		ui.Spacer(c)
		ui.Text(c, card.Note).FontSize(10).TextColor(art.InkDim).MaxLines(1)
	})
}

// playDisc is the disc a hovered poster shows, saying the whole poster opens
// the title.
func (d Deps) playDisc(c *ui.Context) {
	ui.Box(c).Fill().Center().Children(func() {
		ui.Box(c).Size(40, 40).Radius(20).Background(design.PosterPlayDisc()).Center().
			Children(func() {
				ui.Icon(c, icons.Parse("play-bold")).Size(16, 16).TextColor(design.PosterPlayInk())
			})
	})
}

// title is the card's first line: the name, with the typed words in the
// accent, one line long and clickable as the poster is.
func (d Deps) title(c *ui.Context, t *ui.Theme, card Card, actions Actions) {
	ui.Column(c).FillWidth().Height(titleHeight).Children(func() {
		button := ui.ButtonBase(c).FillWidth().Height(titleHeight).Radius(4).
			Label(card.Title).Background(ui.Transparent).Shrink(0)
		if button.Hovered() {
			button.Background(t.SurfaceHover)
		}
		if button.Clicked() && actions.Open != nil {
			actions.Open()
		}
		button.Children(func() {
			ui.Row(c).FillWidth().Children(func() {
				for _, run := range titleRuns(card) {
					ink := t.Text
					if run.Match {
						ink = t.Accent
					}
					ui.Text(c, run.Text).FontSize(15).FontWeight(600).
						TextColor(ink).MaxLines(1).Shrink(0)
				}
			})
		})
	})
}

// subtitle is the card's second line, or the empty space that keeps the next
// row aligned with a card that has one.
func (d Deps) subtitle(c *ui.Context, t *ui.Theme, card Card) {
	if card.Subtitle == "" {
		return
	}
	ink := t.TextMuted
	if card.SubtitleAccent {
		ink = t.Accent
	}
	ui.Row(c).FillWidth().Height(metaHeight).AlignItems(ui.Center).Gap(5).Children(func() {
		if card.SubtitleAccent {
			ui.Icon(c, icons.Parse("fire-linear")).Size(12, 12).TextColor(ink)
		}
		ui.Text(c, card.Subtitle).FontSize(12).TextColor(ink).MaxLines(1)
	})
}

// titleRuns is the title split at the words to highlight; without a highlight,
// the whole title is one run.
func titleRuns(card Card) []Run {
	if len(card.Keywords) == 0 {
		return []Run{{Text: card.Title}}
	}
	return card.Keywords
}

// footnoteParts are the poster foot's facts: the area and the year, so a
// yearless title shows only its area.
func footnoteParts(m locales.Messages, card Card) []string {
	var parts []string
	if card.Area != "" {
		parts = append(parts, card.Area)
	}
	if card.Year > 0 {
		parts = append(parts, strconv.Itoa(card.Year))
	}
	return parts
}
