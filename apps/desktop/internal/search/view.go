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
	posterRatio    = 1.5 // height over width for a 2:3 poster
	minPosterWidth = 120
	maxColumns     = 6
	posterGap      = 20
	// chipHeight is one history chip; chips wrap under the box.
	chipHeight = 30
)

var (
	// imageIn fades a poster in as its cached bitmap arrives, so a placeholder
	// shows at once and the artwork eases in rather than popping.
	imageIn = ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{}}
	// chipMotion carries a history chip's surface between its resting and
	// hovered pose.
	chipMotion = ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true}
	// bodyIn fades the result body into its place when a search replaces the
	// idle state or another keyword's titles.
	bodyIn = ui.ElementTransition{
		Duration: 200 * time.Millisecond,
		Enter:    &ui.Motion{Opacity: 0, Y: 8},
		Exit:     &ui.Motion{Opacity: 0, Y: -8},
	}
)

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

	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(28, right, 64+design.BottomClearance(width), left).
			Gap(18).Children(func() {
			p.box(c, t, m, width)
			p.historyRow(c, t, width)
			p.body(c, t, m, width)
		})
	})
	return scroll
}

// box is the keyword field. Enter submits, and typing settles into a search of
// its own after the debounce, so the page never needs a separate action.
func (p *Page) box(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	height := float32(46)
	fontSize := float32(16)
	if design.Docked(width) {
		height, fontSize = 42, 15
	}
	// The field's own state (focus, caret, text being edited) is identified by
	// the key given before construction, as Key on the widget itself panics.
	c.Key("search-input")
	field := ui.SearchField(c, &p.keyword).FillWidth().Height(height).
		Label(m.Search()).Placeholder(m.SearchPlaceholder()).FontSize(fontSize).
		Background(t.Surface).Border(1, t.Border).Radius(12).PaddingX(14)
	if field.Changed() {
		p.typed(p.keyword)
	}
	if field.Submitted() {
		p.Search(p.keyword)
	}
}

// historyRow lists the keywords this session already searched, as chips that
// run their search again.
func (p *Page) historyRow(c *ui.Context, t *ui.Theme, width float32) {
	if len(p.history) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		for _, keyword := range p.history {
			p.chip(c, t, keyword, width)
		}
	})
}

// chip is one remembered keyword: a quiet surface that lifts under the
// pointer and runs the search again when picked.
func (p *Page) chip(c *ui.Context, t *ui.Theme, keyword string, width float32) {
	size := float32(13)
	if design.Docked(width) {
		size = 12
	}
	button := ui.ButtonBase(c).Key("history:"+keyword).Height(chipHeight).Radius(9).
		PaddingX(12).Gap(6).Label(keyword).Tooltip(keyword).
		Background(t.Surface).Border(1, t.Border).Transition(chipMotion)
	if button.Hovered() {
		button.Background(t.SurfaceHover)
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("magnifer-linear")).Size(13, 13).TextColor(t.TextMuted)
		ui.Text(c, keyword).FontSize(size).TextColor(t.Text)
	})
	if button.Clicked() {
		p.Search(keyword)
	}
}

// body draws the idle, loading, error or result state under the chips. A
// search in flight keeps the titles already answered on screen, with the
// spinner above them, so the page never blanks out between keywords.
func (p *Page) body(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	if p.loading {
		p.loadingRow(c, t, m)
	}
	key := "search-body:" + strconv.FormatUint(p.gen, 10)
	if p.loading {
		// The body keeps its identity while the spinner joins it.
		key = "search-body:" + p.submitted
	}
	ui.Box(c).Key(key).FillWidth().Transition(bodyIn).Children(func() {
		switch {
		case p.err != nil:
			// A failure keeps the titles already answered under the message,
			// so a retry does not start from an empty page.
			p.errorView(c, t, m)
			if len(p.cards) > 0 {
				p.gridView(c, t, m, width, p.cards)
			}
		case p.loading && !p.searched:
			// The first keyword is on its way; the spinner above says so.
			return
		case len(p.cards) > 0:
			// Answered titles stay while another keyword is in flight, and
			// while the box is cleared, instead of blanking out.
			p.gridView(c, t, m, width, p.cards)
		case p.submitted == "":
			p.stateText(c, t, m.SearchIdle())
		case p.searched:
			p.stateText(c, t, m.LibraryEmpty())
		}
	})
}

// loadingRow is the spinner and its message, drawn under the chips while a
// keyword is on its way.
func (p *Page) loadingRow(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().Padding(4, 0).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Spinner(c).Size(16, 16).TextColor(t.TextMuted)
		ui.Text(c, m.LibraryLoading()).FontSize(13).TextColor(t.TextMuted)
	})
}

// gridView lays the posters out in as many columns as fit, capped at six.
func (p *Page) gridView(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, cards []source.Card) {
	left, right := margins(width)
	available := width - left - right
	gap := float32(posterGap)
	columns := int((available + gap) / (minPosterWidth + gap))
	if columns < 1 {
		columns = 1
	}
	if columns > maxColumns {
		columns = maxColumns
	}
	// A hair under the exact share keeps floating rounding from pushing the
	// last column of a full row onto the next line.
	cardW := (available-gap*float32(columns-1))/float32(columns) - 0.5
	ui.Row(c).FillWidth().Wrap().Gap(gap).AlignItems(ui.Start).Children(func() {
		for i, card := range cards {
			p.posterCard(c, t, m, card, cardW, i)
		}
	})
}

// posterCard is one 2:3 poster with its badges, title, bookmark and metadata.
func (p *Page) posterCard(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card, w float32, index int) {
	// A small per-index delay staggers the posters in as they arrive, so a new
	// answer flows onto the page instead of all popping at once.
	motion := ui.Snappy
	motion.Delay = time.Duration(index%12) * 22 * time.Millisecond
	column := ui.Column(c).Key("card:" + card.Ref.String()).Width(w).Gap(8).
		Transition(ui.ElementTransition{Animation: &motion, Enter: &ui.Motion{Y: 10}})
	column.Children(func() {
		frame := ui.ButtonBase(c).Key("poster:" + card.Ref.String()).Width(w).Height(w * posterRatio).
			Radius(12).Clip().Background(t.Surface).Label(card.Title)
		frame.Children(func() {
			p.posterPlaceholder(c, t)
			if p.deps.Images != nil {
				if bitmap := p.deps.Images.Bitmap(card.PosterURL); bitmap != nil {
					ui.Image(c, bitmap).Fill().Fit(ui.Cover).Transition(imageIn)
				}
			}
			p.qualityBadge(c, card)
			p.statusBadge(c, m, card)
		})
		if frame.Clicked() {
			p.detail(card)
		}
		// Hover lifts the poster a touch; this pose only paints, so a wall of
		// posters stays smooth.
		pose := ui.Pose{}
		if frame.Hovered() {
			pose = ui.Pose{Y: -4, Scale: 1.012}
		}
		frame.AnimatePose(pose, ui.Snappy)

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(6).Children(func() {
			title := ui.ButtonBase(c).Grow(1).Height(20).Radius(4).
				Label(card.Title).Background(ui.Transparent)
			title.Children(func() {
				ui.Text(c, card.Title).FontSize(13).Bold().TextColor(t.Text).MaxLines(1)
			})
			if title.Clicked() {
				p.detail(card)
			}
			p.bookmark(c, t, m, card)
		})

		if meta := p.metaLine(m, card); meta != "" {
			ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
		}
	})
}

// posterPlaceholder fills a poster frame that has no artwork yet with a quiet
// mark, so an empty card reads as empty rather than broken.
func (p *Page) posterPlaceholder(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, icons.Parse("clapperboard-linear")).Size(30, 30).TextColor(t.TextMuted).Opacity(0.45)
	})
}

// qualityBadge is the small 4K/1080P chip at a poster's top left.
func (p *Page) qualityBadge(c *ui.Context, card source.Card) {
	label := "1080P"
	if has4K(card) {
		label = "4K"
	}
	bg, fg := design.PosterBadge()
	ui.Box(c).Absolute().Left(8).Top(8).Radius(5).Padding(2, 6).Background(bg).Children(func() {
		ui.Text(c, label).FontSize(9).Bold().TextColor(fg)
	})
}

// statusBadge is the updating/completed chip at a poster's top right.
func (p *Page) statusBadge(c *ui.Context, m locales.Messages, card source.Card) {
	updating := isUpdating(card)
	label := m.StatusCompleted()
	if updating {
		label = m.StatusUpdating()
	}
	bg, fg := design.PosterStatus(updating)
	ui.Box(c).Absolute().Right(8).Top(8).Radius(5).Padding(2, 6).Background(bg).Children(func() {
		ui.Text(c, label).FontSize(9).Bold().TextColor(fg)
	})
}

// bookmark is the per-card follow switch.
func (p *Page) bookmark(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	following := p.isFollowing(card.Ref)
	label := m.FilterFavorites()
	ink := t.TextMuted
	if following {
		ink = t.Accent
	}
	button := ui.ButtonBase(c).Key("mark:"+card.Ref.String()).Size(22, 22).Radius(6).
		Label(label).Tooltip(label).Background(ui.Transparent)
	button.Children(func() {
		ui.Icon(c, icons.Parse("bookmark-linear")).Size(15, 15).TextColor(ink)
	})
	if button.Clicked() {
		p.toggleFollow(card)
	}
}

// metaLine joins the year, kind and a place or first tag under a poster.
func (p *Page) metaLine(m locales.Messages, card source.Card) string {
	var parts []string
	if card.Year > 0 {
		parts = append(parts, strconv.Itoa(card.Year))
	}
	if kind := kindLabel(m, card.Kind); kind != "" {
		parts = append(parts, kind)
	}
	area := card.Area
	if area == "" && len(card.Tags) > 0 {
		area = card.Tags[0]
	}
	if area != "" {
		parts = append(parts, area)
	}
	return strings.Join(parts, " · ")
}

// stateText draws a single quiet line for the idle and empty states.
func (p *Page) stateText(c *ui.Context, t *ui.Theme, text string) {
	ui.Column(c).FillWidth().Padding(48, 0).Children(func() {
		ui.Text(c, text).FontSize(14).TextColor(t.TextMuted)
	})
}

// errorView explains a failed search and offers the retry button.
func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Padding(32, 0).Gap(12).Children(func() {
		ui.Text(c, m.LibraryError()).FontSize(17).Bold().TextColor(t.Text)
		if p.err != nil {
			ui.Text(c, p.err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
		retry := ui.ButtonBase(c).Key("search-retry").Height(34).Radius(10).PaddingX(16).
			Label(m.Retry()).Background(t.Inverse).TextColor(t.InverseText)
		if retry.Hovered() {
			retry.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		retry.Children(func() {
			ui.Text(c, m.Retry()).FontSize(12).Bold().TextColor(t.InverseText)
		})
		if retry.Clicked() {
			p.Retry()
		}
	})
}

// kindLabel names a media kind with the same words as its category tab.
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
	default:
		return ""
	}
}
