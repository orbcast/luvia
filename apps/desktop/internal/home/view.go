package home

import (
	"fmt"
	"strings"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	// minCardWidth keeps a continue-watching card legible when the window
	// narrows; the column count follows from it.
	minCardWidth  = 150
	heroMinHeight = 380
	heroMaxHeight = 620
	captionWidth  = 760
	actionHeight  = 46
	sectionGap    = 44
	cardGap       = 12
	slideWidth    = 66
	slideHeight   = 38
	posterGap     = 14
)

// View builds the Discover page inside the AppShell's content area.
func (p *Page) View(c *ui.Context) ui.Element {
	p.Start()
	p.redraw = c.Invalidate
	t := c.Theme()
	m := locales.In(c.Locale())
	width, height := c.Size()
	snap := p.snapshot()

	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			switch {
			case len(p.featured) > 0:
				p.heroView(c, t, m, width, height)
				p.sectionView(c, t, m, snap, width)
			case p.err != nil:
				p.errorView(c, t, m)
			default:
				ui.Column(c).FillWidth().Padding(60, design.Inset(width)).Gap(8).Children(func() {
					ui.Text(c, m.HomeLoading()).FontSize(14).TextColor(t.TextMuted)
				})
			}
		})
	})
	return scroll
}

func (p *Page) heroView(c *ui.Context, t *ui.Theme, m locales.Messages, width, height float32) {
	art := design.OnArtwork(t)
	card := p.featured[p.hero]
	heroHeight := clamp(height*0.62, heroMinHeight, heroMaxHeight)
	if design.Docked(width) {
		heroHeight = clamp(height*0.58, 300, heroMaxHeight)
	}
	hero := ui.Box(c).FillWidth().Height(heroHeight).Clip()
	hero.Children(func() {
		p.artwork(c, t, card.BackdropURL, card.PosterURL)

		// The scrims layer absolutely so they cover the artwork instead of
		// stacking beside it, and the last stop paints the page background
		// itself, leaving no line where the banner ends.
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Background(art.Dim)
		// Each scrim is a stop list: Start/End on a box gradient do not line
		// up with the box the way they read, stops do.
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).LinearGradient(ui.LinearGradient{
			Angle: 90,
			Stops: []ui.GradientStop{
				{Color: art.ScrimSides, At: 0},
				{Color: art.Deep, At: art.SideMid},
				{Color: ui.Transparent, At: art.SideEnd},
			},
		})
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).LinearGradient(ui.LinearGradient{
			Angle: 180,
			Stops: []ui.GradientStop{
				{Color: ui.Transparent, At: 0},
				{Color: ui.Transparent, At: art.FadeStart},
				{Color: t.Background, At: art.FadePage},
				{Color: t.Background, At: 1},
			},
		})
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).LinearGradient(ui.LinearGradient{
			Angle: 180,
			Stops: []ui.GradientStop{
				{Color: art.ScrimTop, At: 0},
				{Color: ui.Transparent, At: 0.34},
			},
		})

		p.heroCaption(c, t, m, card, heroHeight, width)
		if !design.Docked(width) {
			p.heroSlides(c, t, width, false)
		}
	})
	p.hovering = hero.Hovered()
}

// artwork paints the slide image, or a quiet gradient while it downloads.
func (p *Page) artwork(c *ui.Context, t *ui.Theme, backdrop, poster string) {
	url := backdrop
	if url == "" {
		url = poster
	}
	if p.deps.Images != nil {
		if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
			// Masking the artwork keeps the banner immersive: it melts into
			// the page instead of ending on a hard edge, whatever it shows.
			ui.Image(c, bitmap).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.Cover).
				MaskLinearGradient(ui.LinearGradient{
					Angle: 180,
					Stops: []ui.GradientStop{
						{Color: design.MaskOpaque(), At: 0},
						{Color: design.MaskOpaque(), At: 0.58},
						{Color: ui.Transparent, At: 1},
					},
				})
			return
		}
	}
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).Gradient(t.Surface, t.Background, 160)
}

func (p *Page) heroCaption(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card, heroHeight, width float32) {
	art := design.OnArtwork(t)
	inset := design.Inset(width)
	titleSize := clamp(heroHeight*0.135, 32, 54)
	column := ui.Column(c).Absolute().Left(inset).Bottom(36).Gap(14)
	if design.Docked(width) {
		// Narrow: copy owns the full width and keeps clear of the dock.
		titleSize = clamp(heroHeight*0.12, 24, 34)
		column.Right(inset).Bottom(20).Gap(10)
	} else {
		column.Width(clamp(width*0.52, 420, captionWidth))
	}
	column.Children(func() {
		ui.Text(c, card.Title).FontSize(titleSize).Bold().
			TextColor(art.Ink).MaxLines(1)

		p.heroMeta(c, t, m, card)

		if card.Overview != "" {
			ui.Text(c, card.Overview).FontSize(13).LineHeight(1.6).
				TextColor(art.Muted).MaxLines(2)
		}

		p.heroActions(c, t, m, card)

		if message, ok := p.noticeText(m); ok {
			ui.Text(c, message).FontSize(11).TextColor(art.Muted)
		}

		if design.Docked(width) {
			p.heroSlides(c, t, width, true)
		}
	})
}

func (p *Page) heroMeta(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	art := design.OnArtwork(t)
	var parts []string
	if card.Year > 0 {
		parts = append(parts, fmt.Sprint(card.Year))
	}
	for _, tag := range card.Tags {
		if len(parts) >= 3 {
			break
		}
		parts = append(parts, tag)
	}
	ui.Row(c).AlignItems(ui.Center).Gap(10).Children(func() {
		if len(parts) > 0 {
			ui.Text(c, strings.Join(parts, " · ")).FontSize(12).TextColor(art.Muted)
		}
		p.qualityBadge(c, t, m, card)
		p.brandBadge(c, m)
	})
}

// qualityBadge states the hero print's resolution. It borrows the theme's
// inverse pair so it reads light on the dark hero and dark on the washed
// light one, standing clear of the muted metadata around it.
func (p *Page) qualityBadge(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	label := m.QualityHd()
	if hasQuality(card, "4K") {
		label = m.Quality4k()
	}
	bg, fg := design.QualityBadge(t)
	ui.Row(c).AlignItems(ui.Center).Gap(5).Radius(6).Padding(4, 9).Background(bg).Children(func() {
		head, tail := splitBadge(label)
		ui.Text(c, head).FontSize(12).Bold().TextColor(fg)
		if tail != "" {
			ui.Text(c, tail).FontSize(11).TextColor(fg)
		}
	})
}

// brandBadge tags a Dolby Vision title with its brand pill: a small purple
// mark beside the pale label over a near-black chip, legible on both heroes.
func (p *Page) brandBadge(c *ui.Context, m locales.Messages) {
	bg, mark, fg := design.BrandBadge()
	ui.Row(c).AlignItems(ui.Center).Gap(6).Radius(6).Padding(4, 9).Background(bg).Children(func() {
		ui.Box(c).Size(11, 11).Radius(3).Background(mark)
		ui.Text(c, m.DolbyVision()).FontSize(11).TextColor(fg)
	})
}

// hasQuality reports whether a card advertises want in its remarks or tags,
// which is how a source flags a print's resolution.
func hasQuality(card source.Card, want string) bool {
	if strings.Contains(strings.ToUpper(card.Remarks), want) {
		return true
	}
	for _, tag := range card.Tags {
		if strings.Contains(strings.ToUpper(tag), want) {
			return true
		}
	}
	return false
}

// splitBadge separates a quality label into its headline figure and the words
// after it, so "4K Ultra HD" reads as a bold "4K" beside a quieter "Ultra HD".
func splitBadge(label string) (head, tail string) {
	label = strings.TrimSpace(label)
	if i := strings.IndexAny(label, " \u3000"); i >= 0 {
		return label[:i], strings.TrimSpace(label[i:])
	}
	return label, ""
}

func (p *Page) heroActions(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	art := design.OnArtwork(t)
	ui.Row(c).FillWidth().Gap(12).AlignItems(ui.Center).Wrap().Children(func() {
		// A button already lays its children out in a row, so the icon and
		// label are direct children and every action shares one height.
		play := ui.ButtonBase(c).Key("hero-play").Height(actionHeight).Radius(12).PaddingX(26).Gap(10).
			Background(t.Inverse).TextColor(t.InverseText).Label(m.PlayAction())
		if play.Hovered() {
			play.Background(t.Inverse.Mix(t.InverseText, 0.08))
		}
		play.Children(func() {
			ui.Icon(c, icons.Parse("play-bold")).Size(14, 14)
			ui.Text(c, m.PlayAction()).FontSize(13).Bold()
		})
		if play.Clicked() {
			p.play(card)
		}

		details := ui.ButtonBase(c).Key("hero-details").Height(actionHeight).Radius(12).PaddingX(24).Gap(10).
			Material(glass.Glass{}).TextColor(art.Ink).Label(m.Details())
		if details.Hovered() {
			details.Material(glass.Glass{Interactive: true})
		}
		details.Children(func() {
			ui.Icon(c, icons.Parse("info-circle-linear")).Size(14, 14)
			ui.Text(c, m.Details()).FontSize(13)
		})
		if details.Clicked() {
			p.detail(card)
		}

		following := p.deps.Library != nil && p.deps.Library.IsFollowing(card.Ref)
		followLabel := m.Follow()
		if following {
			followLabel = m.FollowingShow()
		}
		follow := ui.ButtonBase(c).Key("hero-follow").Size(actionHeight, actionHeight).Radius(12).
			Label(followLabel).TextColor(art.Ink).Tooltip(followLabel)
		if follow.Hovered() {
			follow.Background(art.Veil).Material(glass.Glass{})
		}
		if following {
			follow.TextColor(t.Accent)
		}
		follow.Children(func() {
			ui.Icon(c, icons.Parse("bookmark-linear")).Size(17, 17)
		})
		if follow.Clicked() {
			p.toggleFollow(card)
		}
	})
}

func (p *Page) heroSlides(c *ui.Context, t *ui.Theme, width float32, inline bool) {
	art := design.OnArtwork(t)
	if len(p.featured) < 2 {
		return
	}
	w, h := float32(slideWidth), float32(slideHeight)
	row := ui.Row(c).AlignItems(ui.End).Gap(8)
	if inline {
		w, h = 46, 28
		row.FillWidth().Gap(6)
	} else {
		row.Absolute().Right(design.ContentMargin).Bottom(30)
	}
	row.Children(func() {
		ui.Text(c, fmt.Sprintf("%02d / %02d", p.hero+1, len(p.featured))).FontSize(11).
			TextColor(art.Muted).MarginEnd(4)
		for i, slide := range p.featured {
			index := i
			ui.Column(c).Width(w).Gap(5).Children(func() {
				thumb := ui.ButtonBase(c).Size(w, h).Radius(5).Clip().
					Background(t.Surface).Label(slide.Title).Tooltip(slide.Title)
				if index == p.hero {
					thumb.Border(1.5, art.Ink)
				}
				thumb.Children(func() {
					url := slide.BackdropURL
					if url == "" {
						url = slide.PosterURL
					}
					if p.deps.Images != nil {
						if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
							ui.Image(c, bitmap).Fill().Fit(ui.Cover)
						}
					}
				})
				if thumb.Clicked() {
					p.Show(index)
				}
				bar := ui.Box(c).FillWidth().Height(2).Radius(1).Background(art.Track)
				if index == p.hero {
					bar.Background(t.Accent)
				}
			})
		}
	})
}

func (p *Page) sectionView(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	inset := design.Inset(width)
	ui.Column(c).FillWidth().Padding(24, inset, 64+design.BottomClearance(width), inset).
		Gap(sectionGap).Children(func() {
		p.continueSection(c, t, m, snap, width)
		p.weekSection(c, t, m, snap, width)
	})
}

func (p *Page) sectionHeader(c *ui.Context, t *ui.Theme, title string, trailing func()) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Text(c, title).FontSize(19).Bold().TextColor(t.Text)
		ui.Spacer(c)
		if trailing != nil {
			trailing()
		}
	})
}

func (p *Page) continueSection(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	ui.Column(c).FillWidth().Gap(16).Children(func() {
		p.sectionHeader(c, t, m.ContinueWatching(), func() {
			all := ui.ButtonBase(c).Height(28).Padding(8, 0).Radius(8).Label(m.ViewAll())
			ink := t.TextMuted
			if all.Hovered() {
				all.Background(t.SurfaceHover)
				ink = t.Text
			}
			all.Children(func() {
				ui.Row(c).Gap(4).AlignItems(ui.Center).Children(func() {
					ui.Text(c, m.ViewAll()).FontSize(12).TextColor(ink)
					ui.Icon(c, icons.Parse("alt-arrow-right-linear")).Size(14, 14).TextColor(ink)
				})
			})
			if all.Clicked() && p.deps.OnAll != nil {
				p.deps.OnAll()
			}
		})

		if len(snap.Continues) == 0 {
			p.emptyPanel(c, t, m.EmptyContinue(), m.EmptyContinueHint())
			return
		}
		available := width - 2*design.Inset(width)
		columns := 4
		switch {
		case available < 2*minCardWidth+cardGap:
			columns = 1
		case available < 3*minCardWidth+2*cardGap:
			columns = 2
		case available < 4*minCardWidth+3*cardGap:
			columns = 3
		}
		cardW := clamp((available-float32(columns-1)*cardGap)/float32(columns), minCardWidth, 260)
		ui.Row(c).FillWidth().Gap(cardGap).AlignItems(ui.Start).Children(func() {
			for i, progress := range snap.Continues {
				if i >= columns {
					break
				}
				p.continueCard(c, t, m, progress, cardW)
			}
		})
	})
}

func (p *Page) continueCard(c *ui.Context, t *ui.Theme, m locales.Messages, progress library.Progress, w float32) {
	art := design.OnArtwork(t)
	card := source.Card{Ref: progress.Media, Title: progress.Title, PosterURL: progress.PosterURL}
	button := ui.ButtonBase(c).Width(w).Radius(12).Label(progress.Title).Background(ui.Transparent)
	button.Children(func() {
		ui.Column(c).FillWidth().Gap(8).Children(func() {
			frame := ui.Box(c).FillWidth().Height(w * 9 / 16).Radius(12).Clip().Background(t.Surface)
			frame.Children(func() {
				if p.deps.Images != nil {
					if bitmap := p.deps.Images.Bitmap(progress.PosterURL); bitmap != nil {
						ui.Image(c, bitmap).Fill().Fit(ui.Cover)
					}
				}
				if progress.Duration > 0 {
					p.track(c, t, progress, w)
				}
				ui.Icon(c, icons.Parse("play-bold")).Absolute().Right(10).Bottom(14).Size(12, 12).
					TextColor(art.Ink)
			})
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
				ui.Text(c, progress.Title).FontSize(13).Bold().TextColor(t.Text).MaxLines(1)
				ui.Spacer(c)
				if left := progress.Remaining(); left > 0 {
					ui.Text(c, p.timeLeft(m, left)).FontSize(11).
						TextColor(t.TextMuted).MaxLines(1)
				}
			})
		})
	})
	if button.Clicked() {
		p.play(card)
	}
}

// track draws the watched part of an episode along the bottom of its frame.
func (p *Page) track(c *ui.Context, t *ui.Theme, progress library.Progress, w float32) {
	art := design.OnArtwork(t)
	watched := float32(progress.Position.Seconds() / progress.Duration.Seconds() * 100)
	ui.Row(c).Absolute().Left(10).Right(10).Bottom(10).Height(3).Radius(2).Children(func() {
		ui.Box(c).Height(3).Radius(2).Background(t.Accent).WidthPercent(clamp(watched, 0, 100))
		ui.Box(c).Height(3).Radius(2).Background(art.Track).FillWidth()
	})
}

func (p *Page) weekSection(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	now := p.deps.Now()
	start := weekStart(now)
	days := [7]time.Time{}
	for i := range days {
		days[i] = start.AddDate(0, 0, i)
	}
	today := weekdayIndex(now)
	if p.selected < 0 || p.selected > 6 {
		p.selected = today
	}
	day := days[p.selected]
	updates := updatesOn(snap.Follows, day)

	ui.Column(c).FillWidth().Gap(16).Children(func() {
		p.sectionHeader(c, t, m.ThisWeek(), func() {
			opts := i18n.DateOptions{Skeleton: "MMMd"}
			ui.Text(c, m.WeekRange(
				i18n.FormatDate(c.Locale(), start, opts),
				i18n.FormatDate(c.Locale(), days[6], i18n.DateOptions{Skeleton: "d"}),
			)).FontSize(12).TextColor(t.TextMuted)
		})

		available := width - 2*design.Inset(width)
		gap, low := float32(8), float32(84)
		if design.Docked(width) {
			gap, low = 6, 44
		}
		cellW := clamp((available-6*gap)/7, low, 160)
		ui.Row(c).FillWidth().Gap(gap).Children(func() {
			for i, item := range days {
				p.dayCell(c, t, m, item, i, today, cellW, snap)
			}
		})

		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, i18n.FormatDate(c.Locale(), day, i18n.DateOptions{Skeleton: "MMMMd"})).
				FontSize(12).TextColor(t.TextMuted)
			ui.Spacer(c)
			if len(updates) > 0 {
				ui.Text(c, m.Updates(len(updates))).FontSize(12).TextColor(t.TextMuted)
			}
		})

		switch {
		case len(snap.Follows) == 0:
			p.emptyPanel(c, t, m.EmptyWeek(), m.EmptyContinueHint())
		case len(updates) > 0:
			columns := float32(3)
			if design.Docked(width) {
				columns = 2
			}
			posterW := clamp((available-(columns-1)*posterGap)/columns, 120, 220)
			ui.Row(c).FillWidth().Gap(posterGap).AlignItems(ui.Start).Wrap().Children(func() {
				for _, follow := range updates {
					p.posterCard(c, t, m, follow, posterW)
				}
			})
		}
	})
}

func (p *Page) dayCell(c *ui.Context, t *ui.Theme, m locales.Messages, day time.Time, index, today int, w float32, snap library.Snapshot) {
	count := len(updatesOn(snap.Follows, day))
	cell := ui.ButtonBase(c).Width(w).Radius(14).Padding(10, 12).Label(m.Updates(count))
	ink := t.TextMuted
	switch {
	case index == today:
		cell.Background(t.Inverse)
		ink = t.InverseText
	case index == p.selected:
		cell.Background(t.Surface)
		ink = t.Text
	default:
		if cell.Hovered() {
			cell.Background(t.SurfaceHover)
			ink = t.Text
		}
	}
	cell.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				weekday := strings.ToUpper(i18n.FormatDate(c.Locale(), day, i18n.DateOptions{Skeleton: "EEE"}))
				if index == today {
					weekday = m.Today()
				}
				ui.Text(c, weekday).FontSize(10).TextColor(ink)
				ui.Text(c, fmt.Sprintf("%02d", day.Day())).FontSize(19).Bold().TextColor(ink)
			})
			ui.Spacer(c)
			if count > 0 && w >= 76 {
				ui.Text(c, fmt.Sprint(count)).FontSize(12).TextColor(ink)
			}
		})
	})
	if cell.Clicked() {
		p.selected = index
		p.invalidate()
	}
}

func (p *Page) posterCard(c *ui.Context, t *ui.Theme, m locales.Messages, follow library.Follow, w float32) {
	card := source.Card{Ref: follow.Media, Title: follow.Title, PosterURL: follow.PosterURL}
	button := ui.ButtonBase(c).Width(w).Radius(12).Label(follow.Title).Background(ui.Transparent)
	button.Children(func() {
		ui.Column(c).FillWidth().Gap(7).Children(func() {
			frame := ui.Box(c).FillWidth().Height(w * 1.5).Radius(12).Clip().Background(t.Surface)
			frame.Children(func() {
				if p.deps.Images != nil {
					if bitmap := p.deps.Images.Bitmap(follow.PosterURL); bitmap != nil {
						ui.Image(c, bitmap).Fill().Fit(ui.Cover)
					}
				}
				// These cards are the week's fresh updates, so they wear the
				// updating status pill; the update date follows under the title.
				bg, fg := design.PosterStatus(true)
				ui.Row(c).Absolute().Left(8).Top(8).Radius(5).Padding(2, 7).Background(bg).Children(func() {
					ui.Text(c, m.StatusUpdating()).FontSize(10).Bold().TextColor(fg)
				})
			})
			ui.Text(c, follow.Title).FontSize(13).Bold().TextColor(t.Text).MaxLines(1)
			if meta := p.followMeta(c, follow); meta != "" {
				ui.Text(c, meta).FontSize(11).TextColor(t.TextMuted).MaxLines(1)
			}
		})
	})
	if button.Clicked() {
		p.detail(card)
	}
}

// followMeta writes the day a followed series last updated, the one fact the
// library keeps about each follow beyond its title and artwork.
func (p *Page) followMeta(c *ui.Context, follow library.Follow) string {
	if follow.UpdatedAt.IsZero() {
		return ""
	}
	return i18n.FormatDate(c.Locale(), follow.UpdatedAt, i18n.DateOptions{Skeleton: "MMMd"})
}

func (p *Page) emptyPanel(c *ui.Context, t *ui.Theme, title, hint string) {
	ui.Column(c).FillWidth().Radius(14).Background(t.Surface).Padding(18, 20).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(t.Text)
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).FillWidth().Padding(80, design.NarrowGutter).Gap(12).Children(func() {
		ui.Text(c, m.HomeError()).FontSize(18).Bold().TextColor(t.Text)
		if p.err != nil {
			ui.Text(c, p.err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
		retry := ui.ButtonBase(c).Height(34).Radius(10).Padding(16, 0).Label(m.Retry()).
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

func (p *Page) noticeText(m locales.Messages) (string, bool) {
	switch p.notice {
	case noticePlayer:
		return m.PlayerSoon(), true
	case noticeDetail:
		return m.DetailSoon(), true
	case noticeFollowed:
		return m.FollowedFlash(), true
	case noticeUnfollowed:
		return m.UnfollowedFlash(), true
	}
	return "", false
}

// timeLeft writes a duration with the units of the current language.
func (p *Page) timeLeft(m locales.Messages, d time.Duration) string {
	hours := int(d / time.Hour)
	minutes := int(d/time.Minute) % 60
	var parts []string
	if hours > 0 {
		parts = append(parts, fmt.Sprintf("%d %s", hours, m.HoursShort()))
	}
	if minutes > 0 || hours == 0 {
		parts = append(parts, fmt.Sprintf("%d %s", minutes, m.MinutesShort()))
	}
	return m.TimeLeft(strings.Join(parts, " "))
}

func cardWidth(heroHeight float32) float32 { return heroHeight * 1.1 }

func clamp(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
