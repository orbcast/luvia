package catalog

import (
	"strconv"
	"strings"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	tabHeight       = 52
	tabHeightDocked = 44
	tabGap          = 12
	posterRatio     = 1.5 // height over width for a 2:3 poster
	minPosterWidth  = 120
	maxColumns      = 6
	posterGap       = 20
	// tabColsDocked lays the six category pills three to a row on a narrow
	// window, so they wrap into two even rows instead of running off the edge.
	tabColsDocked = 3
)

var (
	// layoutIn fades a fresh grid or list into its place when the layout or the
	// category changes, so the body swaps like a page instead of a slide show.
	layoutIn = ui.ElementTransition{
		Duration: 200 * time.Millisecond,
		Enter:    &ui.Motion{Opacity: 0, Y: 8},
		Exit:     &ui.Motion{Opacity: 0, Y: -8},
	}
	// tabMotion carries the selected category's ring and fill between tabs.
	tabMotion = ui.ElementTransition{Duration: 140 * time.Millisecond, Colors: true}
	// imageIn fades a poster in as its cached bitmap arrives, so a placeholder
	// shows at once and the artwork eases in rather than popping.
	imageIn = ui.ElementTransition{Duration: 220 * time.Millisecond, Enter: &ui.Motion{}}
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

// View builds the resource library inside the AppShell's content area.
func (p *Page) View(c *ui.Context) ui.Element {
	p.Start()
	p.redraw = c.Invalidate
	t := c.Theme()
	m := locales.In(c.Locale())
	width, _ := c.Size()
	left, right := margins(width)

	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(28, right, 64+design.BottomClearance(width), left).
			Gap(20).Children(func() {
			p.header(c, t, m, width)
			p.tabs(c, t, m, width)
			p.filters(c, t, m, width)
			p.results(c, t, m, width)
		})
	})
	return scroll
}

// header draws the title and its count. Search lives in the global dock, so the
// page does not repeat it here.
func (p *Page) header(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	titleSize := float32(26)
	if design.Docked(width) {
		titleSize = 22
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Text(c, m.Library()).FontSize(titleSize).Bold().TextColor(t.Text)
		ui.Text(c, m.LibraryCount(len(p.cards))).FontSize(13).TextColor(t.TextMuted)
	})
}

// tabs draws the six category pills. A wide window fits them on one row; a
// docked one lays them three to a row so they wrap neatly instead of clipping.
func (p *Page) tabs(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	left, right := margins(width)
	available := width - left - right
	gap := float32(tabGap)
	height := float32(tabHeight)
	fontSize := float32(15)
	columns := catCount
	if design.Docked(width) {
		height, fontSize, columns = tabHeightDocked, 13, tabColsDocked
	}
	cardW := (available - gap*float32(columns-1)) / float32(columns)
	if cardW < 56 {
		cardW = 56
	}
	ui.Row(c).FillWidth().Wrap().Gap(gap).Children(func() {
		for i := 0; i < catCount; i++ {
			p.tab(c, t, m, i, cardW, height, fontSize)
		}
	})
}

func (p *Page) tab(c *ui.Context, t *ui.Theme, m locales.Messages, index int, w, h, fontSize float32) {
	tint := design.CategoryTints()[index]
	label := categoryLabel(m, index)
	button := ui.ButtonBase(c).Key("cat:" + strconv.Itoa(index)).Width(w).Height(h).
		Radius(12).Label(label).Background(tint.Background).Transition(tabMotion)
	if index == p.category {
		button.Border(2, design.CategoryRing())
	}
	button.Children(func() {
		ui.Text(c, label).FontSize(fontSize).Bold().TextColor(tint.Foreground)
	})
	if button.Clicked() {
		p.selectCategory(index)
	}
}

// filters draws the title/favorite toggles, the three drop-downs and the
// grid/list switch on one line.
func (p *Page) filters(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	if design.Docked(width) {
		// Narrow: the toggles keep their own row and the controls wrap below,
		// so nothing runs off the edge of a docked page.
		ui.Column(c).FillWidth().Gap(10).Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(16).Children(func() {
				p.titleFilter(c, t, m)
				p.favoriteFilter(c, t, m)
			})
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Wrap().Children(func() {
				p.selectField(c, t, &p.typeMode, typeLabels(m), width)
				p.selectField(c, t, &p.status, statusLabels(m), width)
				p.selectField(c, t, &p.sortMode, sortLabels(m), width)
				p.viewToggle(c, t, m)
			})
		})
		return
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(14).Children(func() {
		p.titleFilter(c, t, m)
		p.favoriteFilter(c, t, m)
		ui.Spacer(c)
		p.selectField(c, t, &p.typeMode, typeLabels(m), width)
		p.selectField(c, t, &p.status, statusLabels(m), width)
		p.selectField(c, t, &p.sortMode, sortLabels(m), width)
		p.viewToggle(c, t, m)
	})
}

// titleFilter shows every title; picking it clears the favorite filter.
func (p *Page) titleFilter(c *ui.Context, t *ui.Theme, m locales.Messages) {
	active := !p.favorites
	label := m.FilterAllTitles()
	ink := t.TextMuted
	if active {
		ink = t.Text
	}
	button := ui.ButtonBase(c).Key("filter:all").Height(30).Radius(8).Padding(8, 4).
		Label(label).Background(ui.Transparent).TextColor(ink)
	button.Children(func() {
		text := ui.Text(c, label).FontSize(13).TextColor(ink)
		if active {
			text.Bold()
		}
	})
	if button.Clicked() && p.favorites {
		p.favorites = false
		p.invalidate()
	}
}

// favoriteFilter narrows to the followed titles; picking it turns it on.
func (p *Page) favoriteFilter(c *ui.Context, t *ui.Theme, m locales.Messages) {
	active := p.favorites
	label := m.FilterFavorites()
	ink := t.TextMuted
	if active {
		ink = t.Accent
	}
	button := ui.ButtonBase(c).Key("filter:fav").Height(30).Radius(8).Padding(8, 4).
		Label(label).Background(ui.Transparent).TextColor(ink)
	button.Children(func() {
		ui.Row(c).Gap(5).AlignItems(ui.Center).Children(func() {
			ui.Icon(c, icons.Parse("bookmark-linear")).Size(13, 13).TextColor(ink)
			text := ui.Text(c, label).FontSize(13).TextColor(ink)
			if active {
				text.Bold()
			}
		})
	})
	if button.Clicked() && !p.favorites {
		p.favorites = true
		p.invalidate()
	}
}

// selectField is one surface drop-down over a list of labels.
func (p *Page) selectField(c *ui.Context, t *ui.Theme, value *int, labels []string, width float32) {
	w := float32(132)
	if design.Docked(width) {
		w = 104
	}
	sel := ui.SelectBase(c, value)
	sel.Trigger.Width(w).Height(36).Radius(10).PaddingX(12).
		AlignItems(ui.Center).Gap(8).Background(t.Surface).Label(labels[*value])
	sel.Trigger.Children(func() {
		ui.Text(c, labels[*value]).FontSize(13).TextColor(t.Text).Grow(1)
		ui.Icon(c, icons.Parse("alt-arrow-down-linear")).Size(13, 13).TextColor(t.TextMuted)
	})
	sel.Popup(func(panel ui.Element) {
		panel.Padding(6).Radius(12).Material(glass.Glass{})
		for i, option := range labels {
			item := sel.Item(i).Padding(7, 12).Radius(8).Label(option)
			if item.Highlighted() {
				item.Background(design.NavigationTint())
			}
			item.Children(func() {
				ui.Text(c, option).FontSize(13).TextColor(t.Text)
			})
		}
	})
}

// viewToggle shows the grid/list pair, the active layout highlighted.
func (p *Page) viewToggle(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
		p.viewButton(c, t, true, "widget-2-linear", m.GridView())
		p.viewButton(c, t, false, "list-linear", m.ListView())
	})
}

func (p *Page) viewButton(c *ui.Context, t *ui.Theme, grid bool, name, label string) {
	on := p.grid == grid
	ink := t.TextMuted
	button := ui.ButtonBase(c).Key("view:"+name).Size(34, 34).Radius(9).Label(label).Tooltip(label).
		Transition(tabMotion)
	if on {
		button.Background(t.Surface).Border(1, t.Border)
		ink = t.Text
	} else {
		button.Background(ui.Transparent)
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse(name)).Size(18, 18).TextColor(ink)
	})
	if button.Clicked() && !on {
		p.grid = grid
		p.invalidate()
	}
}

// results shows the loading, error, empty or filled body under the controls.
func (p *Page) results(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	switch {
	case p.loading:
		p.loadingView(c, t, m)
		return
	case p.err != nil:
		p.errorView(c, t, m)
		return
	}
	cards := p.visible()
	p.sectionRow(c, t, m, len(cards))
	if len(cards) == 0 {
		p.stateText(c, t, m.LibraryEmpty(), 14)
		return
	}
	mode := "list"
	if p.grid {
		mode = "grid"
	}
	// Keying by layout lets a grid/list swap cross-fade as one body instead of
	// snapping, so the page keeps a sense of place.
	ui.Box(c).Key("layout:" + mode).FillWidth().Transition(layoutIn).Children(func() {
		if p.grid {
			p.gridView(c, t, m, width, cards)
			return
		}
		p.listView(c, t, m, cards)
	})
}

// loadingView pairs the load message with a live spinner, so a slow source
// reads as working rather than frozen.
func (p *Page) loadingView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().Padding(48, 0).AlignItems(ui.Center).Gap(10).Children(func() {
		ui.Spinner(c).Size(16, 16).TextColor(t.TextMuted)
		ui.Text(c, m.LibraryLoading()).FontSize(14).TextColor(t.TextMuted)
	})
}

// sectionRow names the current category and counts the titles drawn.
func (p *Page) sectionRow(c *ui.Context, t *ui.Theme, m locales.Messages, count int) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, categoryTitle(m, p.category)).FontSize(14).Bold().TextColor(t.Text)
		ui.Text(c, m.LibraryCount(count)).FontSize(12).TextColor(t.TextMuted)
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
	// category flows onto the page instead of all popping at once.
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

// posterPlaceholder fills a poster frame that has no artwork yet (loading or
// missing) with a quiet mark, so an empty card reads as empty rather than
// broken.
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

// listView draws the compact rows the list toggle chooses.
func (p *Page) listView(c *ui.Context, t *ui.Theme, m locales.Messages, cards []source.Card) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		for i, card := range cards {
			p.listRow(c, t, m, card, i)
		}
	})
}

func (p *Page) listRow(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card, index int) {
	motion := ui.Snappy
	motion.Delay = time.Duration(index%12) * 22 * time.Millisecond
	row := ui.Row(c).Key("row-wrap:" + card.Ref.String()).FillWidth().AlignItems(ui.Center).Gap(12).
		Transition(ui.ElementTransition{Animation: &motion, Enter: &ui.Motion{Y: 8}})
	row.Children(func() {
		main := ui.ButtonBase(c).Key("row:"+card.Ref.String()).Grow(1).Radius(10).
			Padding(8, 10).Label(card.Title).Background(t.Surface)
		main.Children(func() {
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(14).Children(func() {
				thumb := ui.Box(c).Width(44).Height(66).Radius(8).Clip().Background(t.Background)
				thumb.Children(func() {
					p.posterPlaceholder(c, t)
					if p.deps.Images != nil {
						if bitmap := p.deps.Images.Bitmap(card.PosterURL); bitmap != nil {
							ui.Image(c, bitmap).Fill().Fit(ui.Cover).Transition(imageIn)
						}
					}
				})
				ui.Column(c).Grow(1).Gap(4).Children(func() {
					ui.Text(c, card.Title).FontSize(14).Bold().TextColor(t.Text).MaxLines(1)
					if meta := p.metaLine(m, card); meta != "" {
						ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
					}
				})
			})
		})
		if main.Clicked() {
			p.detail(card)
		}
		p.bookmark(c, t, m, card)
	})
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

// stateText draws a single quiet line for the loading and empty states.
func (p *Page) stateText(c *ui.Context, t *ui.Theme, text string, size float32) {
	ui.Column(c).FillWidth().Padding(48, 0).Children(func() {
		ui.Text(c, text).FontSize(size).TextColor(t.TextMuted)
	})
}

// errorView explains a failed load and offers the retry button.
func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Padding(32, 0).Gap(12).Children(func() {
		ui.Text(c, m.LibraryError()).FontSize(17).Bold().TextColor(t.Text)
		if p.err != nil {
			ui.Text(c, p.err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
		retry := ui.ButtonBase(c).Height(34).Radius(10).PaddingX(16).Label(m.Retry()).
			Background(t.Inverse).TextColor(t.InverseText)
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

// categoryLabel names one category tab.
func categoryLabel(m locales.Messages, index int) string {
	switch index {
	case catMovie:
		return m.CategoryMovie()
	case catSeries:
		return m.CategorySeries()
	case catAnime:
		return m.CategoryAnime()
	case catVariety:
		return m.CategoryVariety()
	case catShort:
		return m.CategoryShort()
	default:
		return m.CategoryAll()
	}
}

// categoryTitle names the section drawn, using "all titles" for the all tab as
// the reference does.
func categoryTitle(m locales.Messages, index int) string {
	if index == catAll {
		return m.FilterAllTitles()
	}
	return categoryLabel(m, index)
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

func typeLabels(m locales.Messages) []string {
	return []string{m.FilterAllTypes(), m.CategoryMovie(), m.CategorySeries(),
		m.CategoryAnime(), m.CategoryVariety(), m.CategoryShort()}
}

func statusLabels(m locales.Messages) []string {
	return []string{m.FilterAllStatus(), m.StatusUpdating(), m.StatusCompleted()}
}

func sortLabels(m locales.Messages) []string {
	return []string{m.FilterDefaultSort(), m.SortLatest(), m.SortRating()}
}
