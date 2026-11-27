package search

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	posterRatio = 1.5 // height over width for a 2:3 poster
	// posterWidth and posterGap fix the wide layout's grid: the columns it
	// holds follow from the width, so a poster keeps its size on any screen.
	posterWidth = 168
	posterGap   = 20
	// narrowPoster and narrowGap do the same for a window with the sidebar
	// docked, smaller and closer, so more than one column still fits.
	narrowPoster = 128
	narrowGap    = 14
	// maxColumns caps a shelf in a very wide window; two rows of posters are
	// a search result, not a wall of them.
	maxColumns = 8
	// keywordSize is the keyword's type size; the narrow one keeps a long
	// keyword inside a docked window.
	keywordSize       = 46
	narrowKeywordSize = 34
	// accentRun is the length of the accent segment that ends the rule under
	// the keyword.
	accentRun = 28
	// scrimHeight is how far the poster's foot darkens under the area and
	// year.
	scrimHeight = 80
)

// Overlay colors sit on artwork, so they keep their own values in both themes:
// a dark veil over a bright poster, a light ring around it.
var (
	posterScrim   = ui.RGBA(0, 0, 0, 0.75)   // under the area and year
	posterRing    = ui.RGBA(1, 1, 1, 0.06)   // hairline around the poster
	posterRingLit = ui.RGBA(0, 0, 0, 0.06)   // the same on a light page
	followVeil    = ui.RGBA(0, 0, 0, 0.45)   // an unfollowed bookmark's disc
	followVeilOn  = ui.RGBA(0, 0, 0, 0.65)   // and while hovered
	followInk     = ui.RGBA(1, 1, 1, 0.85)   // its icon
	playDisc      = ui.RGBA(1, 1, 1, 0.92)   // the play disc of a hovered poster
	playInk       = ui.RGBA(0, 0, 0, 0.72)   // its icon
	posterCapInk  = ui.RGBA(1, 1, 1, 0.65)   // the area and year on the scrim
	badgeInk      = ui.RGBA(1, 1, 1, 0.96)   // the quality badge's text
	badgePlate    = ui.RGBA(10, 8, 14, 0.62) // and its plate
)

var (
	// imageIn fades a poster in as its cached bitmap arrives, so a placeholder
	// shows at once and the artwork eases in rather than popping.
	imageIn = ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{}}
	// wordMotion carries a remembered keyword between its resting and hovered
	// color.
	wordMotion = ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true}
	// bodyIn fades the body into its place when a search replaces the idle
	// state or another keyword's titles.
	bodyIn = ui.ElementTransition{
		Duration: 200 * time.Millisecond,
		Enter:    &ui.Motion{Opacity: 0, Y: 8},
		Exit:     &ui.Motion{Opacity: 0, Y: -8},
	}
)

// hovered records that some poster of this frame has the pointer, so a hover
// that moves between posters rebuilds the page in one frame rather than
// lighting up a second one.
var hovered bool

// margins are the page's left and right gutters. The left one clears the
// floating AppShell; the right one is a normal page gutter, so a wide window
// does not leave a dead strip of empty space along its edge.
func margins(width float32) (left, right float32) {
	if design.Docked(width) {
		return design.NarrowGutter, design.NarrowGutter
	}
	return design.Inset(width), design.PageGutter
}

// View builds the search page inside the AppShell's content area.
func (p *Page) View(c *ui.Context) ui.Element {
	p.redraw = c.Invalidate
	t := c.Theme()
	m := locales.In(c.Locale())
	width, _ := c.Size()
	left, right := margins(width)
	hovered = false

	// The page's top margin sits above the scrolling column, so a scrolled
	// result slides under the keyword instead of through it.
	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(0, right, 64+design.BottomClearance(width), left).
			Gap(24).Children(func() {
			p.queryLine(c, t, m, width)
			p.body(c, t, m, width)
		})
	})
	return scroll
}

// queryLine is the keyword itself, the page's one loud element: a magnifier
// and the typed words over a hairline that ends in an accent run, with the
// keywords searched this session at its right. Enter submits, and typing
// settles into a search of its own after the debounce, so the page never
// needs a separate action.
func (p *Page) queryLine(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	docked := design.Docked(width)
	size := float32(keywordSize)
	if docked {
		size = narrowKeywordSize
	}
	ui.Column(c).FillWidth().Padding(28, 0, 0, 0).Children(func() {
		ui.Column(c).FillWidth().Gap(14).Children(func() {
			ui.Row(c).FillWidth().Wrap().AlignItems(ui.End).Gap(16).Children(func() {
				p.field(c, t, m, size)
				p.recent(c, t, m)
			})
			p.rule(c, t)
		})
	})
}

// field is the keyword itself: the stock search field at the keyword's size,
// in the page's inks and with no plate or border of its own. Its magnifier and
// its placeholder are the widget's own drawing, on a Solar icon, so the page
// draws neither; only the words typed are the field's.
//
// The field keeps its internal paddings (the icon gap and the text inset), so
// the words start where the widget places them rather than at a guessed
// offset; the height leaves the keyword a line of room at the size it is set.
func (p *Page) field(c *ui.Context, t *ui.Theme, m locales.Messages, size float32) {
	// The field's own state (focus, caret, text being edited) is identified by
	// the key given before construction, as Key on the widget itself panics.
	c.Key("search-input")
	field := ui.SearchField(c, &p.keyword).Grow(1).Height(size*1.3).
		Label(m.Search()).Placeholder(m.SearchPlaceholder()).FontSize(size).
		TextColor(t.Text).Fill().Background(t.Background).Border(0, t.Background).
		Radius(0)
	if field.Changed() {
		p.typed(p.keyword)
	}
	if field.Submitted() {
		p.Search(p.keyword)
	}
}

// recent lists the keywords searched this session, as quiet words that run
// their search again, on the keyword's own row.
func (p *Page) recent(c *ui.Context, t *ui.Theme, m locales.Messages) {
	if len(p.history) == 0 {
		return
	}
	ui.Row(c).Shrink(0).Padding(0, 0, 12, 0).AlignItems(ui.Center).Gap(14).
		Children(func() {
			ui.Text(c, m.SearchRecent()).FontSize(11).LetterSpacing(0.06).
				TextColor(t.TextMuted)
			for i, keyword := range p.history {
				p.recentWord(c, t, keyword, 13, i == 0)
			}
		})
}

// recentWord is one remembered keyword: plain text until the pointer finds
// it, then the primary ink. The newest is the primary ink already.
func (p *Page) recentWord(c *ui.Context, t *ui.Theme, keyword string, size float32, latest bool) {
	ink := t.TextMuted
	button := ui.ButtonBase(c).Key("history:" + keyword).Height(size * 1.6).
		Radius(4).PaddingX(2).Label(keyword).Tooltip(keyword).
		Background(ui.Transparent).Transition(wordMotion)
	if latest || button.Hovered() {
		ink = t.Text
	}
	button.Children(func() {
		ui.Text(c, keyword).FontSize(size).TextColor(ink)
	})
	if button.Clicked() {
		p.Search(keyword)
	}
}

// rule is the page's horizontal axis: a hairline under the keyword that ends
// in an accent run, and the same line tops every shelf, so the shelves line
// up as one column of text.
func (p *Page) rule(c *ui.Context, t *ui.Theme) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
		ui.Box(c).Grow(1).Height(1).Background(t.Border)
		ui.Box(c).Width(accentRun).Height(2).Radius(1).Background(t.Accent)
	})
}

// body draws the idle, loading, error or result state under the keyword. A
// search in flight keeps the titles already answered on screen, with the
// spinner above them, so the page never blanks out between keywords.
func (p *Page) body(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	key := "search-body:" + strconv.FormatUint(p.gen, 10)
	if p.loading {
		// The body keeps its identity while the spinner joins it.
		key = "search-body:" + p.submitted
	}
	ui.Box(c).Key(key).FillWidth().Padding(26, 0, 0, 0).Transition(bodyIn).Children(func() {
		ui.Column(c).FillWidth().Gap(18).Children(func() {
			if p.loading {
				p.loadingRow(c, t, m)
			}
			switch {
			case p.err != nil:
				// A failure keeps the titles already answered under the message,
				// so a retry does not start from an empty page.
				p.errorView(c, t, m)
				if len(p.cards) > 0 {
					p.shelves(c, t, m, width, p.cards)
				}
			case p.loading && !p.searched:
				// The first keyword is on its way; the spinner above says so.
				return
			case p.lastSearch():
				// The box was cleared: what the last keyword answered stays,
				// under a head that says which keyword it was and how to run
				// it again. This ranks above the idle state, so the page says
				// one thing, not two contradicting ones.
				p.lastShelf(c, t, m, width)
			case len(p.cards) > 0:
				// Answered titles stay while another keyword is in flight,
				// instead of blanking out.
				p.shelves(c, t, m, width, p.cards)
			case p.submitted == "":
				p.idle(c, t, m, width)
			case p.searched:
				p.empty(c, t, m, width)
			}
		})
	})
}

// loadingRow is the spinner and its message, drawn under the keyword while a
// search is on its way.
func (p *Page) loadingRow(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Spinner(c).Size(16, 16).TextColor(t.TextMuted)
		ui.Text(c, m.LibraryLoading()).FontSize(13).TextColor(t.TextMuted)
	})
}

// lastSearch reports whether the page is showing what the last keyword
// answered while the box holds nothing: the state that offers the way back
// instead of a flat sentence.
func (p *Page) lastSearch() bool {
	return p.submitted != "" && strings.TrimSpace(p.keyword) == "" && len(p.cards) > 0
}

// lastShelf is the last keyword's titles under one head: the page's rule, the
// keyword, and the way to search it again. The kinds are not split here,
// because the head already names one search, and its own shelves would read
// as several.
func (p *Page) lastShelf(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	ui.Column(c).FillWidth().Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		ui.Row(c).FillWidth().Padding(10, 0, 12, 0).AlignItems(ui.Center).Gap(10).
			Children(func() {
				ui.Text(c, m.SearchLastQuery()).FontSize(15).FontWeight(600).
					TextColor(t.Text)
				ui.Text(c, p.submitted).FontSize(12).TextColor(t.TextMuted).
					MaxLines(1)
				ui.Box(c).Grow(1)
				p.searchAgain(c, t, m)
			})
		p.grid(c, t, m, width, p.cards)
	})
}

// searchAgain runs the last keyword once more, from the head of its shelf.
func (p *Page) searchAgain(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ink := t.TextMuted
	keyword := p.submitted
	button := ui.ButtonBase(c).Key("search-again").Shrink(0).Height(26).Radius(4).
		PaddingX(0).Gap(6).Label(m.SearchSearchAgain()).
		Background(ui.Transparent).Transition(wordMotion)
	if button.Hovered() {
		ink = t.Text
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("refresh-linear")).Size(13, 13).TextColor(ink)
		ui.Text(c, m.SearchSearchAgain()).FontSize(12).TextColor(ink)
	})
	if button.Clicked() {
		p.Search(keyword)
	}
}

// shelves groups the answered titles by kind, each shelf a rule with its name
// and count above its posters. A kind that answered nothing gets no shelf, so
// a search for films shows one, not four empty ones.
func (p *Page) shelves(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, cards []source.Card) {
	ui.Column(c).FillWidth().Gap(28).Children(func() {
		for _, group := range groupByKind(cards) {
			p.shelf(c, t, m, width, group.kind, group.cards)
		}
	})
}

// shelf is one kind's posters under the page's axis.
func (p *Page) shelf(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, kind source.Kind, cards []source.Card) {
	ui.Column(c).FillWidth().Children(func() {
		p.shelfHead(c, t, m, kind, len(cards))
		p.grid(c, t, m, width, cards)
	})
}

// shelfHead is the rule, the kind's name and how many it holds. It is not a
// card or a frame: the rule is the page's own, and the posters hang under it.
func (p *Page) shelfHead(c *ui.Context, t *ui.Theme, m locales.Messages, kind source.Kind, count int) {
	ui.Column(c).FillWidth().Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		ui.Row(c).FillWidth().Padding(10, 0, 12, 0).AlignItems(ui.Center).Gap(10).
			Children(func() {
				ui.Text(c, kindLabel(m, kind)).FontSize(15).FontWeight(600).
					TextColor(t.Text)
				ui.Text(c, m.LibraryCount(count)).FontSize(12).TextColor(t.TextMuted)
			})
	})
}

// grid lays a shelf's posters out in the columns the page holds, two rows at
// most: what the source answered first is what the user is looking for.
func (p *Page) grid(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, cards []source.Card) {
	left, right := margins(width)
	w, gap, columns := gridFor(width-left-right, design.Docked(width))
	shown := cards
	if len(shown) > columns*2 {
		shown = shown[:columns*2]
	}
	ui.Row(c).FillWidth().Wrap().Gap(gap).AlignItems(ui.Start).Children(func() {
		for i, card := range shown {
			p.posterCard(c, t, m, card, w, i)
		}
	})
}

// gridFor splits the available width into fixed poster columns: as many as
// fit, at most maxColumns, and one even in a window too narrow for two. A
// docked window takes the smaller poster, so it still holds two columns.
func gridFor(available float32, docked bool) (w, gap float32, columns int) {
	w, gap = posterWidth, posterGap
	if docked {
		w, gap = narrowPoster, narrowGap
	}
	columns = int((available + gap) / (w + gap))
	columns = max(columns, 1)
	columns = min(columns, maxColumns)
	return w, gap, columns
}

// posterCard is one 2:3 poster with its badges, title, bookmark and metadata.
// The poster is the card: no plate, no frame, only the artwork and the lines
// under it.
func (p *Page) posterCard(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card, w float32, index int) {
	ref := card.Ref.String()
	ui.Column(c).Key("card:" + ref).Width(w).Gap(9).Transition(ui.ElementTransition{
		// A poster slides up into its place once, as its row appears.
		Duration: 260 * time.Millisecond,
		Enter:    &ui.Motion{Y: 10},
	}).Children(func() {
		// A small per-index offset lifts the posters in one after another, so
		// a new answer flows onto the page instead of all popping at once.
		motion := ui.Snappy
		motion.Delay = time.Duration(index%12) * 22 * time.Millisecond
		lifted := false
		frame := ui.ButtonBase(c).Key("poster:" + ref).Width(w).
			Height(w * posterRatio).Radius(12).Clip().Background(t.Surface).
			Label(card.Title)
		if frame.Hovered() || frame.FocusWithin() {
			lifted = true
		}
		frame.Children(func() {
			p.artwork(c, t, card)
			p.scrim(c, t)
			p.quality(c, t, m, card)
			p.follow(c, t, m, card)
			p.caption(c, t, m, card)
			if lifted {
				p.playDisc(c)
			}
		})
		frame.DrawOver(func(painter *ui.Painter, r ui.Rect) {
			ring := posterRing
			if !t.Dark {
				ring = posterRingLit
			}
			// On the poster's own edge: the clip keeps the outer half, the
			// inner half is the hairline the reference draws.
			painter.Stroke(r, ring, 12, 1)
		})
		if lifted {
			// A poster lifts under the pointer, and keeps the lifted place of
			// pointer moves on, so a hover that passes lifts one poster and
			// settles it, rather than moving the page under the cursor.
			frame.AnimatePose(ui.Pose{Y: -3, Scale: 1.012}, motion)
		}
		if frame.Clicked() {
			p.detail(card)
		}
		p.title(c, t, m, card)
		p.meta(c, t, m, card)
	})
}

// artwork is the poster image, or the placeholder while it loads or when the
// source gave none.
func (p *Page) artwork(c *ui.Context, t *ui.Theme, card source.Card) {
	if card.PosterURL != "" {
		if bitmap := p.deps.Images.Bitmap(card.PosterURL); bitmap != nil {
			ui.Image(c, bitmap).Fill().Fit(ui.Cover).Transition(imageIn)
			return
		}
	}
	ui.Box(c).Fill().Background(t.Surface).Center().Children(func() {
		ui.Icon(c, icons.Parse("clapperboard-linear")).Size(30, 30).
			TextColor(t.TextMuted).Opacity(0.45)
	})
}

// scrim darkens the poster's foot so the area and year stay readable over a
// bright frame.
func (p *Page) scrim(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(scrimHeight).
		LinearGradient(ui.LinearGradient{
			// 180, not 90: MyGo's angle is CSS's, 0 pointing up, so 180 runs
			// from the box's top, the scrim's dark end, to its bottom.
			From: posterScrim, To: ui.Transparent, Angle: 180,
		})
}

// quality is the source's resolution, top left on the poster.
func (p *Page) quality(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	label := m.QualityHd()
	if has4K(card) {
		label = m.Quality4k()
	}
	ui.Box(c).Absolute().Left(10).Top(10).Radius(5).Padding(2, 6).
		Background(badgePlate).Center().Children(func() {
		ui.Text(c, label).FontSize(10).FontWeight(600).TextColor(badgeInk)
	})
}

// follow is the bookmark on the poster's top right: a solid accent disc when
// the title is followed, a dark veil when it is not.
func (p *Page) follow(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	following := p.isFollowing(card.Ref)
	button := ui.ButtonBase(c).Key("mark:"+card.Ref.String()).Size(28, 28).
		Radius(14).Label(m.FilterFavorites()).Tooltip(m.FilterFavorites()).
		Absolute().Right(10).Top(10)
	switch {
	case following:
		button.Background(t.Accent)
	case button.Hovered():
		button.Background(followVeilOn)
	default:
		button.Background(followVeil)
	}
	ink := followInk
	if following {
		ink = t.AccentText
	}
	button.TextColor(ink)
	button.Children(func() {
		name := "bookmark-linear"
		if following {
			name = "bookmark-bold"
		}
		ui.Icon(c, icons.Parse(name)).Size(14, 14).TextColor(ink)
	})
	if button.Clicked() {
		p.toggleFollow(card)
	}
}

// caption is the area and year along the poster's foot.
func (p *Page) caption(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	facts := captionParts(card)
	if len(facts) == 0 {
		return
	}
	ui.Row(c).Absolute().Left(10).Right(10).Bottom(9).AlignItems(ui.Center).
		Children(func() {
			ui.Text(c, strings.Join(facts, m.ListSeparator())).FontSize(10).
				LetterSpacing(0.06).TextColor(posterCapInk).MaxLines(1)
		})
}

// playDisc is the disc that appears on a hovered poster, saying the whole
// poster opens the title.
func (p *Page) playDisc(c *ui.Context) {
	ui.Box(c).Fill().Center().Children(func() {
		ui.Box(c).Size(40, 40).Radius(20).Background(playDisc).Center().
			Children(func() {
				ui.Icon(c, icons.Parse("play-bold")).Size(16, 16).TextColor(playInk)
			})
	})
}

// title is the card's first line: the name, with the typed words in the
// accent, one line long.
func (p *Page) title(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	ui.Column(c).FillWidth().Height(21).Children(func() {
		button := ui.ButtonBase(c).FillWidth().Height(21).Radius(4).
			Label(card.Title).Background(ui.Transparent).Shrink(0)
		if button.Hovered() {
			button.Background(t.SurfaceHover)
		}
		if button.Clicked() {
			p.detail(card)
		}
		button.Children(func() {
			ui.Row(c).FillWidth().Children(func() {
				for _, run := range highlight(card.Title, p.submitted) {
					ink := t.Text
					if run.match {
						ink = t.Accent
					}
					ui.Text(c, run.text).FontSize(15).FontWeight(600).
						TextColor(ink).MaxLines(1).Shrink(0)
				}
			})
		})
	})
}

// meta is the card's second line: for a show, the episodes it has reached or
// holds, in the accent; otherwise the year and area.
func (p *Page) meta(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	if show := isShow(card); show {
		// A show says where it stands, even when the source's remarks name no
		// episode count, so the line under a season never goes missing.
		status := m.StatusCompleted()
		if isUpdating(card) {
			status = m.StatusUpdating()
		}
		line := status
		if episodes, ok := seasonLine(m, card); ok {
			line = m.SearchStatus(status, episodes)
		}
		ui.Row(c).FillWidth().Height(18).AlignItems(ui.Center).Gap(5).Children(func() {
			ui.Icon(c, icons.Parse("fire-linear")).Size(12, 12).TextColor(t.Accent)
			ui.Text(c, line).FontSize(12).TextColor(t.Accent).MaxLines(1)
		})
		return
	}
	facts := captionParts(card)
	if len(facts) == 0 {
		return
	}
	ui.Text(c, strings.Join(facts, m.ListSeparator())).FontSize(12).
		TextColor(t.TextMuted).MaxLines(1)
}

// idle is what the page says before a search, or once the box is cleared: what
// the box searches, and the keywords it can offer from this session.
func (p *Page) idle(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	ui.Column(c).FillWidth().Children(func() {
		ui.Text(c, m.SearchIdle()).FontSize(15).FontWeight(600).TextColor(t.Text)
		ui.Text(c, m.SearchIdleHint()).FontSize(13).TextColor(t.TextMuted).
			Padding(4, 0, 0, 0)
		if len(p.history) > 0 {
			p.recentSection(c, t, m)
		} else {
			ui.Text(c, m.SearchIdleExample()).FontSize(13).
				TextColor(t.TextMuted).Padding(6, 0, 0, 0)
		}
	})
}

// recentSection lists every keyword this session searched, each a line that
// runs it again, under the page's axis.
func (p *Page) recentSection(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Padding(26, 0, 0, 0).Children(func() {
		ui.Box(c).FillWidth().Height(1).Background(t.Border)
		ui.Row(c).FillWidth().Padding(10, 0, 8, 0).AlignItems(ui.Center).Gap(10).
			Children(func() {
				ui.Text(c, m.SearchRecent()).FontSize(15).FontWeight(600).
					TextColor(t.Text)
				ui.Text(c, m.SearchRecentSession()).FontSize(12).
					TextColor(t.TextMuted)
			})
		ui.Column(c).FillWidth().Children(func() {
			for _, keyword := range p.history {
				p.recentRow(c, t, keyword)
			}
		})
	})
}

// recentRow is one remembered keyword in the idle list: a full-width line
// that reads as a word until the pointer finds it.
func (p *Page) recentRow(c *ui.Context, t *ui.Theme, keyword string) {
	ink := t.TextMuted
	button := ui.ButtonBase(c).Key("history:" + keyword).FillWidth().Height(30).
		Radius(6).Justify(ui.Start).Label(keyword).Tooltip(keyword).
		Background(ui.Transparent).Transition(wordMotion)
	if button.Hovered() {
		ink = t.Text
	}
	button.Children(func() {
		ui.Text(c, keyword).FontSize(14).TextColor(ink).MaxLines(1)
	})
	if button.Clicked() {
		p.Search(keyword)
	}
}

// empty is what a search that answered nothing says: the keyword, a word on
// why it may have answered nothing, a hint on how to search again, the retry,
// and where to go instead.
func (p *Page) empty(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	maxWidth := float32(760)
	if design.Docked(width) {
		maxWidth = width - design.NarrowGutter*2
	}
	ui.Column(c).FillWidth().MaxWidth(maxWidth).Gap(12).Children(func() {
		ui.Text(c, m.SearchNoResults(p.submitted)).FontSize(34).FontWeight(600).
			LetterSpacing(-0.02).TextColor(t.Text)
		ui.Text(c, m.SearchNoResultsHint(p.sourceName(m))).FontSize(14).
			LineHeight(1.6).TextColor(t.TextMuted)
		// Left on their own, as the page's copy is.
		ui.Row(c).FillWidth().Wrap().Gap(20).AlignItems(ui.Center).Children(func() {
			p.searchHint(c, t, m)
			p.retryLink(c, t, m)
		})
		ui.Column(c).FillWidth().Padding(30, 0, 0, 0).Children(func() {
			ui.Box(c).FillWidth().Height(1).Background(t.Border)
			ui.Text(c, m.SearchBrowseLibrary()).FontSize(12).
				TextColor(t.TextMuted).Padding(12, 0, 0, 0)
		})
	})
}

// searchHint says how to search again without naming any keyword: the source
// answered nothing for this one, and no remembered word is promised to answer
// either.
func (p *Page) searchHint(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Text(c, m.SearchNoResultsHintAgain()).FontSize(13).TextColor(t.TextMuted)
}

// retryLink is the quiet way back to the source, beside the hint.
func (p *Page) retryLink(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ink := t.TextMuted
	button := ui.ButtonBase(c).Key("search-retry").Shrink(0).Height(26).Radius(4).
		Padding(10, 0, 0, 0).Gap(6).Label(m.Retry()).Background(ui.Transparent)
	if button.Hovered() {
		ink = t.Text
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("refresh-linear")).Size(13, 13).TextColor(ink)
		ui.Text(c, m.Retry()).FontSize(13).TextColor(ink)
	})
	if button.Clicked() {
		p.Retry()
	}
}

// errorView says what failed and offers the retry, which keeps its weight so
// it reads as the page's one action.
func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, m.LibraryError()).FontSize(17).FontWeight(600).TextColor(t.Text)
		ui.Text(c, p.err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		button := ui.ButtonBase(c).Key("search-retry").Height(34).Radius(10).
			PaddingX(16).Label(m.Retry()).Background(t.Inverse)
		if button.Hovered() {
			button.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		button.Children(func() {
			ui.Text(c, m.Retry()).FontSize(12).FontWeight(600).
				TextColor(t.InverseText)
		})
		if button.Clicked() {
			p.Retry()
		}
	})
}

// sourceName is the source's own name, for copy that says who answered.
func (p *Page) sourceName(m locales.Messages) string {
	if p.deps.Source == nil {
		return m.SourcesLabel()
	}
	return p.deps.Source.Name()
}

// group is one kind of title and the cards of that kind.
type group struct {
	kind  source.Kind
	cards []source.Card
}

// kindOrder is the order the shelves appear in, whatever order the source
// answered in.
var kindOrder = []source.Kind{source.Series, source.Movie, source.Anime, source.Variety}

// groupByKind sorts the cards into shelves, one per kind that answered, in
// kindOrder.
func groupByKind(cards []source.Card) []group {
	var groups []group
	for _, kind := range kindOrder {
		var of []source.Card
		for _, card := range cards {
			if card.Kind == kind {
				of = append(of, card)
			}
		}
		if len(of) > 0 {
			groups = append(groups, group{kind: kind, cards: of})
		}
	}
	// A kind outside the order is still titles the source answered.
	var rest []source.Card
	for _, card := range cards {
		known := false
		for _, kind := range kindOrder {
			if card.Kind == kind {
				known = true
				break
			}
		}
		if !known {
			rest = append(rest, card)
		}
	}
	if len(rest) > 0 {
		groups = append(groups, group{cards: rest})
	}
	return groups
}

// run is a run of a title, matched when it is the keyword the user typed.
type run struct {
	text  string
	match bool
}

// highlight splits title at every plain occurrence of keyword, so the words
// the user typed show in the accent. An empty keyword matches nothing.
func highlight(title, keyword string) []run {
	keyword = strings.TrimSpace(keyword)
	if keyword == "" || title == "" {
		return []run{{text: title}}
	}
	var runs []run
	rest := title
	for {
		at := strings.Index(rest, keyword)
		if at < 0 {
			if rest != "" {
				runs = append(runs, run{text: rest})
			}
			return runs
		}
		if at > 0 {
			runs = append(runs, run{text: rest[:at]})
		}
		runs = append(runs, run{text: keyword, match: true})
		rest = rest[at+len(keyword):]
	}
}

// captionParts are the poster foot's facts: the area and the year, so a
// yearless title shows only its area.
func captionParts(card source.Card) []string {
	var parts []string
	if card.Area != "" {
		parts = append(parts, card.Area)
	}
	if card.Year > 0 {
		parts = append(parts, strconv.Itoa(card.Year))
	}
	return parts
}

// isShow reports whether a card is an episodic series or animation, the kinds
// whose second line says where the show stands.
func isShow(card source.Card) bool {
	return card.Kind == source.Series || card.Kind == source.Anime
}

// seasonLine is the episodes a show has reached, when its source's remarks
// name a count: the ones it has aired while it still updates, or how many it
// holds as a whole. A source that names none has no such phrase, and the
// card shows the status alone.
func seasonLine(m locales.Messages, card source.Card) (string, bool) {
	n, ok := episodeCount(card.Remarks)
	if !ok {
		return "", false
	}
	count := strconv.Itoa(n)
	if isUpdating(card) {
		return m.EpisodesUpdating(count), true
	}
	return m.EpisodesComplete(n), true
}

// episodeCount reads the episodes a source wrote in its remarks, as the
// number before epUnit.
func episodeCount(remarks string) (int, bool) {
	runes := []rune(remarks)
	end := -1
	for i, r := range runes {
		if r == epUnit {
			end = i
			break
		}
	}
	if end < 0 {
		return 0, false
	}
	digits := ""
	for i := end - 1; i >= 0 && isDigit(runes[i]); i-- {
		digits = string(runes[i]) + digits
	}
	if digits == "" {
		return 0, false
	}
	n, err := strconv.Atoi(digits)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

// isDigit reports whether r is a digit.
func isDigit(r rune) bool { return r >= '0' && r <= '9' }

// kindLabel names a kind for a shelf head.
func kindLabel(m locales.Messages, kind source.Kind) string {
	switch kind {
	case source.Movie:
		return m.CategoryMovie()
	case source.Series:
		return m.CategorySeries()
	case source.Anime:
		return m.CategoryAnime()
	case source.Variety:
		return m.CategoryVariety()
	}
	return string(kind)
}
