package detail

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	posterRatio    = 1.5
	railWidth      = 340
	episodeRatio   = 1.55
	minEpisodeWide = 96
)

// View builds the title page on top of a blurred copy of its own artwork.
func (p *Page) View(c *ui.Context) ui.Element {
	p.redraw = c.Invalidate
	t := c.Theme()
	m := locales.In(c.Locale())
	width, _ := c.Size()

	root := ui.Box(c).Fill().Background(t.Background).Clip()
	root.Children(func() {
		p.backdrop(c, t)
		switch {
		case p.loading:
			p.stateView(c, t, m, m.DetailLoading())
		case p.err != nil:
			p.errorView(c, t, m)
		default:
			p.scroll(c, t, m, width)
		}
		p.backButton(c, t, m, width, c.TitleBar().Bottom)
	})
	return root
}

// backdrop blurs the title's own artwork and washes it towards the page so the
// glass chips and copy read on any frame.
func (p *Page) backdrop(c *ui.Context, t *ui.Theme) {
	url := p.detail.BackdropURL
	if url == "" {
		url = p.detail.PosterURL
	}
	if p.deps.Images != nil && url != "" {
		if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
			ui.Image(c, bitmap).Absolute().Left(0).Right(0).Top(0).Bottom(0).
				Fit(ui.Cover).Blur(design.ArtworkBlur()).Opacity(0.95).ZIndex(-2)
		}
	}
	scrim := design.DetailBackdrop(t)
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).ZIndex(-1).LinearGradient(ui.LinearGradient{
		Angle: 180,
		Stops: []ui.GradientStop{
			{Color: scrim, At: 0},
			{Color: scrim, At: 0.35},
			{Color: t.Background, At: 0.92},
			{Color: t.Background, At: 1},
		},
	})
}

// backButton is the page's way out, kept in the window's top-left corner
// outside the scrolling copy. Its top shares the line of the window controls
// the AppShell floats in the other corner, and its left is the page's own
// gutter, so it lines up with the copy it returns to rather than floating on
// a number of its own.
func (p *Page) backButton(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, barBottom float32) {
	back := ui.ButtonBase(c).Key("detail-back").Absolute().Left(pageInset(width)).Top(design.ChromeTop(barBottom)).
		Size(40, 40).Radius(20).Material(glass.Glass{}).Label(m.Back()).Tooltip(m.Back()).ZIndex(30)
	if back.Hovered() {
		back.Material(glass.Glass{Interactive: true})
	}
	back.Children(func() {
		ui.Icon(c, icons.Parse("alt-arrow-left-linear")).Size(20, 20).TextColor(t.Text)
	})
	if back.Clicked() {
		p.Back()
	}
}

func (p *Page) scroll(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	inset := pageInset(width)
	scroll := ui.Scroll(c).Fill().ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Padding(88, inset, 48+design.BottomClearance(width), inset).
			Gap(26).Children(func() {
			p.hero(c, t, m, width)
			p.tabRail(c, t, m)
			p.mainAndRail(c, t, m, width)
		})
	})
}

// hero is the poster beside the title block.
func (p *Page) hero(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	posterW := clamp(width*0.26, 200, 320)
	if design.Docked(width) {
		posterW = clamp(width*0.55, 160, 240)
		ui.Column(c).FillWidth().Gap(22).Children(func() {
			p.poster(c, t, posterW)
			p.titleBlock(c, t, m, width, false)
		})
		return
	}
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(34).Children(func() {
		p.poster(c, t, posterW)
		p.titleBlock(c, t, m, width, true)
	})
}

func (p *Page) poster(c *ui.Context, t *ui.Theme, w float32) {
	frame := ui.Box(c).Width(w).Height(w*posterRatio).Radius(16).Clip().
		Background(t.Surface).Shadow(0, 20, 44, 0, ui.RGBA(0, 0, 0, 0.48))
	frame.Children(func() {
		if p.deps.Images != nil {
			if bitmap := p.deps.Images.Bitmap(p.detail.PosterURL); bitmap != nil {
				ui.Image(c, bitmap).Fill().Fit(ui.Cover)
			}
		}
	})
}

func (p *Page) titleBlock(c *ui.Context, t *ui.Theme, m locales.Messages, width float32, grow bool) {
	s := design.DetailSurfaces(t)
	column := ui.Column(c).Gap(14)
	if grow {
		column.Grow(1)
	} else {
		column.FillWidth()
	}
	column.Children(func() {
		p.tagRow(c, t, s, p.detail.Tags, 6)

		titleSize := clamp(width*0.032, 30, 48)
		ui.Text(c, p.detail.Title).FontSize(titleSize).Bold().TextColor(t.Text).MaxLines(2)

		p.metaChips(c, t, m, s)

		if p.detail.Overview != "" {
			overview := ui.Text(c, p.detail.Overview).FontSize(13.5).LineHeight(1.7).TextColor(t.TextMuted)
			if !p.expanded {
				overview.MaxLines(3)
			}
		}

		p.actions(c, t, m)

		p.sourceLine(c, t, m)
	})
}

// tagRow lays the leading tags out as outlined pills.
func (p *Page) tagRow(c *ui.Context, t *ui.Theme, s design.Surfaces, tags []string, limit int) {
	if len(tags) == 0 {
		return
	}
	if limit > 0 && len(tags) > limit {
		tags = tags[:limit]
	}
	ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
		for _, tag := range tags {
			ui.Row(c).Radius(7).Padding(4, 11).Background(s.TagBg).Border(1, s.TagBorder).Children(func() {
				ui.Text(c, tag).FontSize(12).TextColor(s.TagInk)
			})
		}
	})
}

// metaChips are the quiet metric pills under the title.
func (p *Page) metaChips(c *ui.Context, t *ui.Theme, m locales.Messages, s design.Surfaces) {
	type metric struct{ icon, text string }
	var items []metric
	if p.detail.Year > 0 {
		items = append(items, metric{"calendar-linear", strconv.Itoa(p.detail.Year)})
	}
	if label := kindLabel(m, p.detail.Kind); label != "" {
		items = append(items, metric{"clapperboard-linear", label})
	}
	if p.detail.Area != "" {
		items = append(items, metric{"map-point-linear", p.detail.Area})
	}
	if p.detail.Remarks != "" {
		items = append(items, metric{"fire-linear", p.detail.Remarks})
	}
	if p.detail.EpisodeCount > 0 && p.detail.Remarks == "" {
		items = append(items, metric{"playlist-linear", m.EpisodeCount(p.detail.EpisodeCount)})
	}
	if len(items) == 0 {
		return
	}
	ui.Row(c).FillWidth().Wrap().Gap(10).Children(func() {
		for _, item := range items {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Radius(8).Padding(5, 11).
				Background(s.Chip).Border(1, s.ChipBorder).Children(func() {
				ui.Icon(c, icons.Parse(item.icon)).Size(14, 14).TextColor(s.ChipInk)
				ui.Text(c, item.text).FontSize(12).TextColor(s.ChipInk)
			})
		}
	})
}

func (p *Page) actions(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		play := ui.ButtonBase(c).Key("detail-play").Height(48).Radius(12).PaddingX(30).Gap(9).
			Background(t.Inverse).TextColor(t.InverseText).Label(m.PlayNow())
		if play.Hovered() {
			play.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		play.Children(func() {
			ui.Icon(c, icons.Parse("play-bold")).Size(16, 16).TextColor(t.InverseText)
			ui.Text(c, m.PlayNow()).FontSize(14).Bold().TextColor(t.InverseText)
		})
		if play.Clicked() {
			p.PlaySelected()
		}

		following := p.IsFollowing()
		label := m.Follow()
		if following {
			label = m.FollowingShow()
		}
		follow := ui.ButtonBase(c).Key("detail-follow").Height(48).Radius(12).PaddingX(24).Gap(9).
			Material(glass.Glass{}).TextColor(t.Text).Label(label)
		if follow.Hovered() {
			follow.Material(glass.Glass{Interactive: true})
		}
		follow.Children(func() {
			icon := "bookmark-linear"
			ink := t.Text
			if following {
				icon, ink = "bookmark-bold", t.Accent
			}
			ui.Icon(c, icons.Parse(icon)).Size(16, 16).TextColor(ink)
			ui.Text(c, label).FontSize(14).TextColor(t.Text)
		})
		if follow.Clicked() {
			p.ToggleFollow()
		}
	})
}

// sourceLine is the small "selected source · episodes" caption under the actions.
func (p *Page) sourceLine(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(18).Wrap().Children(func() {
		p.captionPair(c, t, m.SelectedSource(), sourceName(p.deps.Source))
		if p.detail.EpisodeCount > 0 {
			p.captionPair(c, t, m.SeriesEpisodes(), strconv.Itoa(p.detail.EpisodeCount))
		}
	})
}

func (p *Page) captionPair(c *ui.Context, t *ui.Theme, label, value string) {
	ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted)
		ui.Text(c, value).FontSize(12).Bold().TextColor(t.Text)
	})
}

func (p *Page) tabRail(c *ui.Context, t *ui.Theme, m locales.Messages) {
	s := design.DetailSurfaces(t)
	labels := []string{m.TabOverview(), m.TabCast(), m.TabComments(), m.TabCrew()}
	ui.Row(c).FillWidth().Children(func() {
		rail := ui.Row(c).Radius(999).Padding(4).Gap(2).Background(s.TabBg)
		rail.Children(func() {
			for i, label := range labels {
				index := i
				button := ui.ButtonBase(c).Key("detail-tab:" + strconv.Itoa(i)).Height(34).Radius(999).
					PaddingX(16).Label(label).Background(ui.Transparent)
				ink := s.TabInk
				if index == p.tab {
					button.Background(s.TabSel)
					ink = s.TabSelInk
				}
				button.Children(func() {
					text := ui.Text(c, label).FontSize(13).TextColor(ink)
					if index == p.tab {
						text.Bold()
					}
				})
				if button.Clicked() {
					p.SelectTab(index)
				}
			}
		})
	})
}

func (p *Page) mainAndRail(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	content := width - 2*pageInset(width)
	if design.Docked(width) {
		ui.Column(c).FillWidth().Gap(28).Children(func() {
			p.leftColumn(c, t, m, content)
			p.rail(c, t, m, 0)
		})
		return
	}
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(32).Children(func() {
		left := ui.Column(c).Grow(1).Gap(28)
		left.Children(func() { p.leftColumn(c, t, m, content-railWidth-32) })
		p.rail(c, t, m, railWidth)
	})
}

func (p *Page) leftColumn(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	p.watchSection(c, t, m, width)
	p.tabContent(c, t, m)
}

// watchSection is the source pills, the now-playing caption and the episode grid.
func (p *Page) watchSection(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	s := design.DetailSurfaces(t)
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.End).Children(func() {
			ui.Column(c).Gap(3).Children(func() {
				ui.Text(c, m.WatchEyebrow()).FontSize(10).LetterSpacing(2).TextColor(t.TextMuted)
				ui.Text(c, m.EpisodesTitle()).FontSize(20).Bold().TextColor(t.Text)
			})
			ui.Spacer(c)
			if n := len(p.episodes); n > 0 {
				ui.Text(c, m.EpisodeCount(n)).FontSize(12).TextColor(t.TextMuted)
			}
		})

		ui.Column(c).FillWidth().Gap(8).Children(func() {
			ui.Text(c, m.PlaybackSource()).FontSize(12).TextColor(t.TextMuted)
			ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
				ui.Row(c).Radius(8).Padding(5, 12).Background(s.Chip).Border(1, s.ChipBorder).Children(func() {
					ui.Text(c, sourceName(p.deps.Source)).FontSize(12).Bold().TextColor(t.Text)
				})
			})
			ui.Text(c, m.CurrentPlaying(p.detail.Title)).FontSize(12).TextColor(t.TextMuted)
		})

		if len(p.episodes) == 0 {
			ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
			return
		}
		p.episodeGrid(c, t, m, width)
	})
}

func (p *Page) episodeGrid(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	gap := float32(10)
	available := width
	columns := int((available + gap) / (minEpisodeWide + gap))
	if columns < 2 {
		columns = 2
	}
	if columns > 8 {
		columns = 8
	}
	chipW := (available - gap*float32(columns-1)) / float32(columns)
	if chipW < 68 {
		chipW = 68
	}
	ui.Row(c).FillWidth().Wrap().Gap(gap).Children(func() {
		for i, episode := range p.episodes {
			p.episodeChip(c, t, m, episode, i, chipW)
		}
	})
}

func (p *Page) episodeChip(c *ui.Context, t *ui.Theme, m locales.Messages, episode source.Episode, index int, w float32) {
	bg, border, ink := design.EpisodeChip(t, index == p.selected)
	label := episode.Title
	if label == "" {
		label = m.EpisodeNumber(strconv.Itoa(episode.Number))
	}
	button := ui.ButtonBase(c).Key("episode:"+episode.ID).Width(w).Height(58).Radius(10).
		Background(bg).Border(1, border).Label(label)
	button.Children(func() {
		ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(3).Children(func() {
			num := "EP " + fmt.Sprintf("%02d", episode.Number)
			ui.Text(c, num).FontSize(13).Bold().TextColor(ink)
			ui.Text(c, label).FontSize(11).TextColor(ink).MaxLines(1)
		})
	})
	if button.Clicked() {
		p.Play(index)
	}
}

// tabContent is the body under the episode grid; the overview tab shows the
// full synopsis, the others list cast, comments or crew.
func (p *Page) tabContent(c *ui.Context, t *ui.Theme, m locales.Messages) {
	switch p.tab {
	case tabOverview:
		p.synopsis(c, t, m)
	case tabCast:
		p.people(c, t, m, m.TabCast(), p.detail.Actors)
	case tabCrew:
		p.people(c, t, m, m.TabCrew(), p.detail.Directors)
	default:
		ui.Column(c).FillWidth().Gap(6).Children(func() {
			ui.Text(c, m.TabComments()).FontSize(14).Bold().TextColor(t.Text)
			ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
		})
	}
}

func (p *Page) synopsis(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, m.AboutEyebrow()).FontSize(10).LetterSpacing(2).TextColor(t.TextMuted)
		ui.Text(c, m.AboutTitle()).FontSize(16).Bold().TextColor(t.Text)
		if p.detail.Overview == "" {
			ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
			return
		}
		ui.Text(c, p.detail.Overview).FontSize(13.5).LineHeight(1.75).TextColor(t.TextMuted)
		toggle := ui.ButtonBase(c).Key("detail-expand").Height(26).Radius(8).PaddingX(2).
			Label(m.ExpandAll()).Background(ui.Transparent)
		toggle.Children(func() {
			label := m.ExpandAll()
			if p.expanded {
				label = m.CollapseAll()
			}
			ui.Text(c, label).FontSize(12).Bold().TextColor(t.Accent)
		})
		if toggle.Clicked() {
			p.ToggleExpanded()
		}
	})
}

func (p *Page) people(c *ui.Context, t *ui.Theme, m locales.Messages, title string, people []string) {
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		ui.Text(c, title).FontSize(14).Bold().TextColor(t.Text)
		if len(people) == 0 {
			ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
			return
		}
		s := design.DetailSurfaces(t)
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, person := range people {
				ui.Row(c).Radius(999).Padding(6, 14).Background(s.TagBg).Border(1, s.TagBorder).Children(func() {
					ui.Text(c, person).FontSize(12.5).TextColor(t.Text)
				})
			}
		})
	})
}

// rail is the right-hand collection, profile and tag cards.
func (p *Page) rail(c *ui.Context, t *ui.Theme, m locales.Messages, w float32) {
	column := ui.Column(c).Gap(18)
	if w > 0 {
		column.Width(w)
	} else {
		column.FillWidth()
	}
	column.Children(func() {
		p.collectionCard(c, t, m)
		p.profileCard(c, t, m)
		p.tagCard(c, t, m)
	})
}

func (p *Page) card(c *ui.Context, t *ui.Theme) ui.Element {
	s := design.DetailSurfaces(t)
	box := ui.Column(c).FillWidth().Gap(12).Radius(16).Padding(18).
		Background(s.Card).Border(1, s.CardBorder)
	return box
}

func (p *Page) collectionCard(c *ui.Context, t *ui.Theme, m locales.Messages) {
	p.card(c, t).Children(func() {
		ui.Text(c, m.Collection()).FontSize(14).Bold().TextColor(t.Text)
		following := p.IsFollowing()
		label := m.Follow()
		if following {
			label = m.FollowingShow()
		}
		button := ui.ButtonBase(c).Key("rail-follow").FillWidth().Height(40).Radius(10).
			Background(t.Inverse).TextColor(t.InverseText).Label(label)
		if button.Hovered() {
			button.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		button.Children(func() {
			icon := "bookmark-linear"
			if following {
				icon = "bookmark-bold"
			}
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Icon(c, icons.Parse(icon)).Size(15, 15).TextColor(t.InverseText)
				ui.Text(c, label).FontSize(13).Bold().TextColor(t.InverseText)
			})
		})
		if button.Clicked() {
			p.ToggleFollow()
		}
		if p.detail.EpisodeCount > 0 {
			p.fieldRow(c, t, m.FieldEpisodes(), strconv.Itoa(p.detail.EpisodeCount))
		}
		if p.deps.Library != nil {
			if progress, ok := p.deps.Library.ProgressOf(p.ref); ok && progress.Episode.Number > 0 {
				p.fieldRow(c, t, m.TabOverview(), m.EpisodeNumber(strconv.Itoa(progress.Episode.Number)))
			}
		}
	})
}

func (p *Page) profileCard(c *ui.Context, t *ui.Theme, m locales.Messages) {
	p.card(c, t).Children(func() {
		ui.Text(c, m.ProfileInfo()).FontSize(14).Bold().TextColor(t.Text)
		if label := kindLabel(m, p.detail.Kind); label != "" {
			p.fieldRow(c, t, m.FieldType(), label)
		}
		if p.detail.Area != "" {
			p.fieldRow(c, t, m.FieldArea(), p.detail.Area)
		}
		if p.detail.Year > 0 {
			p.fieldRow(c, t, m.FieldYear(), strconv.Itoa(p.detail.Year))
		}
		if p.detail.Remarks != "" {
			p.fieldRow(c, t, m.FieldStatus(), p.detail.Remarks)
		}
		if len(p.detail.Directors) > 0 {
			p.fieldRow(c, t, m.FieldDirectors(), strings.Join(p.detail.Directors, m.ListSeparator()))
		}
		if len(p.detail.Actors) > 0 {
			p.fieldRow(c, t, m.FieldActors(), strings.Join(p.detail.Actors, m.ListSeparator()))
		}
	})
}

func (p *Page) tagCard(c *ui.Context, t *ui.Theme, m locales.Messages) {
	if len(p.detail.Tags) == 0 {
		return
	}
	s := design.DetailSurfaces(t)
	p.card(c, t).Children(func() {
		ui.Text(c, m.TagsLabel()).FontSize(14).Bold().TextColor(t.Text)
		ui.Row(c).FillWidth().Wrap().Gap(8).Children(func() {
			for _, tag := range p.detail.Tags {
				ui.Row(c).Radius(999).Padding(4, 11).Background(s.TagBg).Border(1, s.TagBorder).Children(func() {
					ui.Text(c, tag).FontSize(11.5).TextColor(s.TagInk)
				})
			}
		})
	})
}

func (p *Page) fieldRow(c *ui.Context, t *ui.Theme, label, value string) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Gap(12).Children(func() {
		ui.Text(c, label).FontSize(12).TextColor(t.TextMuted).Width(64)
		ui.Text(c, value).FontSize(12.5).TextColor(t.Text).Grow(1)
	})
}

func (p *Page) stateView(c *ui.Context, t *ui.Theme, m locales.Messages, text string) {
	ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Text(c, text).FontSize(14).TextColor(t.TextMuted)
	})
}

func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).Fill().Justify(ui.Center).AlignItems(ui.Center).Gap(12).Children(func() {
		ui.Text(c, m.DetailError()).FontSize(18).Bold().TextColor(t.Text)
		if p.err != nil {
			ui.Text(c, p.err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
		retry := ui.ButtonBase(c).Height(34).Radius(10).PaddingX(16).Label(m.Retry()).
			Background(t.Inverse).TextColor(t.InverseText)
		if retry.Hovered() {
			retry.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		retry.Children(func() { ui.Text(c, m.Retry()).FontSize(12).Bold().TextColor(t.InverseText) })
		if retry.Clicked() {
			p.Retry()
		}
	})
}

// kindLabel names a media kind with the words of its category tab.
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

func sourceName(src source.Source) string {
	if src == nil {
		return "—"
	}
	return src.Name()
}

func pageInset(width float32) float32 {
	if design.Docked(width) {
		return design.NarrowGutter
	}
	return 48
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
