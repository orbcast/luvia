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
	"luvia/apps/desktop/internal/poster"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const (
	// minCardWidth keeps a continue-watching card legible when the window
	// narrows; the column count follows from it.
	minCardWidth  = 150
	heroMinHeight = 320
	heroMaxHeight = 430
	// heroCoverShare is how much of the window's height the banner's cover
	// takes in the wide layout, and heroBreath the room the banner leaves below
	// the cover and its copy. A docked page sizes its banner by heroDockedShare
	// instead, because its copy sits beside a smaller cover.
	heroCoverShare  = 0.36
	heroBreath      = 40
	heroDockedShare = 0.48
	// heroShortHeight is the window height under which the banner stops
	// following the window and moves to its floor, because a banner that
	// tracks a 400-pixel window would push its own copy off the bottom.
	heroShortHeight = 560
	captionWidth    = 760
	actionHeight    = 46
	sectionGap      = 40
	cardGap         = 12
	slideWidth      = 66
	slideHeight     = 38
	posterGap       = 14

	// dayPosterRows caps what one day shows: two rows of posters, which is six
	// cards in the wide layout's three columns.
	dayPosterRows = 2
	// dayPosterMinWidth is the narrowest artwork a title stays recognizable
	// at, and what decides how many columns a day's row takes.
	dayPosterMinWidth = 110

	// coverGap separates the banner's cover from its copy, coverRadius rounds
	// that cover, and coverRatio is a poster's height over its width, so the
	// hero cover keeps the artwork's own shape instead of a hero-sized crop.
	coverGap    = 28
	coverRadius = 16
	coverRatio  = poster.Ratio

	// continueScrim is how tall the shading under a continue-watching frame's
	// track and play mark is, in share of the frame's height and in pixels.
	continueScrimShare = 0.55
	continueScrimMin   = 56

	// dayDot is the mark a day with updates wears in a window too narrow for
	// its count: the count is dropped, the news is not.
	dayDot = 5
	// dayCountWidth is the width under which the day drops its count.
	dayCountWidth = 76
)

// View builds the Discover page inside the AppShell's content area.
func (p *Page) View(c *ui.Context) ui.Element {
	p.Start()
	p.setRedraw(c.Invalidate)
	t := c.Theme()
	m := locales.In(c.Locale())
	width, height := c.Size()
	snap := p.snapshot()
	// One frame, one banner: the sections and the slide the picture shows are
	// read together, so a source answer that lands mid-frame cannot leave the
	// frame a slide index its cards no longer have.
	featured, _ := p.banner()
	_, err := p.state()

	scroll := ui.Scroll(c).Fill().Background(t.Background).ScrollbarAxes(false, true)
	scroll.Children(func() {
		ui.Column(c).FillWidth().Children(func() {
			switch {
			case len(featured) > 0:
				p.heroView(c, t, m, width, height)
				p.sectionView(c, t, m, snap, width)
			case err != nil:
				p.errorView(c, t, m, err)
			default:
				ui.Column(c).FillWidth().
					Padding(design.PageTopInset(c.TitleBar().Bottom), design.Inset(width)).Gap(8).
					Children(func() {
						ui.Text(c, m.HomeLoading()).FontSize(14).TextColor(t.TextMuted)
					})
			}
		})
	})
	return scroll
}

func (p *Page) heroView(c *ui.Context, t *ui.Theme, m locales.Messages, width, height float32) {
	art := design.OnArtwork(t)
	featured, slide := p.banner()
	if slide >= len(featured) {
		return
	}
	card := featured[slide]
	heroHeight, coverHeight := heroBand(c.TitleBar().Bottom, width, height)
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
				{Color: art.ScrimSides.Mix(art.Deep, art.SideMid), At: art.SideMid},
				{Color: art.Deep.Mix(ui.Transparent, art.SideEnd), At: art.SideEnd},
				{Color: ui.Transparent, At: 1},
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

		bandBottom := p.heroCaption(c, t, m, card, heroHeight, coverHeight, width)
		if !design.Docked(width) {
			p.heroSlides(c, t, width, false, bandBottom)
		}
	})
	p.hovering = hero.Hovered()
}

// heroBand returns the banner's height and its cover's height. The cover follows
// the window's height, and the banner hugs it: the cover hangs from the chrome
// line, the copy sits on its bottom edge, and heroBreath is the room the banner
// leaves below that band before the page's sections begin. A short window stops
// the banner from growing, so the copy keeps its lines instead of being clipped;
// a docked page stacks the copy beside the cover and keeps to a share of the
// window, with the cover a thumbnail.
func heroBand(barBottom, width, height float32) (heroHeight, coverHeight float32) {
	if design.Docked(width) {
		heroHeight = clamp(height*heroDockedShare, 280, heroMaxHeight)
		coverHeight = clamp(heroHeight*0.5, 132, 190)
		return heroHeight, coverHeight
	}
	window := height
	if window < heroShortHeight {
		window = heroShortHeight
	}
	coverHeight = clamp(window*heroCoverShare, 192, 330)
	heroHeight = design.ChromeTop(barBottom) + coverHeight + heroBreath
	if heroHeight < heroMinHeight {
		heroHeight = heroMinHeight
	}
	return heroHeight, coverHeight
}

// artwork paints the slide's cover across the banner, blurred: the still is
// the hero's ground, a wash of its own colors, while the cover card and the
// copy carry the details. It stays opaque long enough to meet the page, so the
// banner still ends without a line.
func (p *Page) artwork(c *ui.Context, t *ui.Theme, backdrop, posterURL string) {
	url := backdrop
	if url == "" {
		url = posterURL
	}
	if p.deps.Images != nil {
		if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
			ui.Image(c, bitmap).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.Cover).
				Blur(design.ArtworkBlur()).
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

// heroCaption lays the banner's cover and copy on one band: the cover hangs
// from the chrome line, and the copy sits on the cover's own bottom edge, so the
// two read as one block rather than the copy sinking to the banner's floor. It
// returns how far that band ends from the banner's bottom, which is where the
// slides' thumbnails sit. A docked page has no chrome line to hang from, so the
// cover and copy share the floor instead.
func (p *Page) heroCaption(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card, heroHeight, coverHeight, width float32) float32 {
	art := design.OnArtwork(t)
	inset := design.Inset(width)
	titleSize := clamp(heroHeight*0.13, 30, 50)
	available := width - 2*inset
	copyWidth := clamp(width*0.52, 420, captionWidth)
	bottom, gap := float32(32), float32(coverGap)
	chromeTop := design.ChromeTop(c.TitleBar().Bottom)
	coverWidth := coverHeight / coverRatio
	docked := design.Docked(width)
	if docked {
		// Narrow: the copy owns what the cover leaves and keeps clear of the
		// dock.
		titleSize = clamp(heroHeight*0.11, 22, 32)
		copyWidth = available
		bottom, gap = 18, 14
	}
	// A poster never takes more than its share of the line, and the copy keeps
	// its own room beside it.
	coverWidth = clamp(coverWidth, 112, available*0.38)
	if room := available - coverWidth - gap; copyWidth > room {
		copyWidth = room
	}

	row := ui.Row(c).Absolute().Left(inset).Gap(gap).AlignItems(ui.End).
		Width(coverWidth + gap + copyWidth)
	if docked {
		row.Bottom(bottom)
	} else {
		row.Top(chromeTop).Height(coverWidth * coverRatio)
	}
	row.Children(func() {
		cover := p.heroCover(c, t, card, coverWidth)
		if !docked {
			cover.AlignSelf(ui.Start)
		}

		column := ui.Column(c).Width(copyWidth).Gap(12)
		if docked {
			column.Gap(9)
		}
		column.Children(func() {
			title := ui.Text(c, card.Title).FontSize(titleSize).Bold().TextColor(art.Ink)
			if docked {
				title.MaxLines(2)
			} else {
				title.MaxLines(1)
			}

			p.heroMeta(c, t, m, card)

			if card.Overview != "" {
				// Two lines of synopsis: enough to place the title, short
				// enough that a 400-pixel window still shows the actions.
				ui.Text(c, card.Overview).FontSize(13).LineHeight(1.55).
					TextColor(art.Muted).MaxLines(2)
			}

			p.heroActions(c, t, m, card)

			if message, ok := p.noticeText(m); ok {
				ui.Text(c, message).FontSize(11).TextColor(art.Muted)
			}

			if docked {
				p.heroSlides(c, t, width, true, 0)
			}
		})
	})

	if docked {
		return bottom
	}
	return heroHeight - chromeTop - coverWidth*coverRatio
}

// heroCover is the banner's own cover: the poster card floating beside the
// copy, so a slide reads as a title with its artwork rather than as a
// photograph with text laid over it.
func (p *Page) heroCover(c *ui.Context, t *ui.Theme, card source.Card, w float32) ui.Element {
	shadow, border := design.HeroCover(t)
	frame := ui.Box(c).Width(w).Height(w*coverRatio).Radius(coverRadius).Clip().
		Background(t.Surface).Border(1, border).Shadow(0, 18, 40, 0, shadow)
	frame.Children(func() {
		if p.deps.Images == nil {
			return
		}
		url := card.PosterURL
		if url == "" {
			url = card.BackdropURL
		}
		if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
			ui.Image(c, bitmap).Fill().Fit(ui.Cover)
		}
	})
	return frame
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
			ui.Text(c, strings.Join(parts, m.ListSeparator())).FontSize(12).TextColor(art.Muted)
		}
		p.qualityBadge(c, t, m, card)
		p.brandBadge(c, t, m, card)
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

// brandBadge tags a title with its brand pill, and only when the source itself
// states the brand: a print's remarks or tags carry the mark, so a title that
// says nothing gets no chip. An unconditional badge would put a brand on every
// slide, which is a claim about the print that no source made.
func (p *Page) brandBadge(c *ui.Context, t *ui.Theme, m locales.Messages, card source.Card) {
	if !hasBrand(card) {
		return
	}
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

// hasBrand reports whether the source states the title's print is Dolby Vision.
// The mark is the brand's, so it is only shown on the source's own word: in the
// remarks, or among one of the tags.
func hasBrand(card source.Card) bool {
	for _, candidate := range append([]string{card.Remarks}, card.Tags...) {
		lower := strings.ToLower(candidate)
		if strings.Contains(lower, "dolby vision") || strings.Contains(lower, "dolby-vision") {
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
			name := "bookmark-linear"
			if following {
				name = "bookmark-bold"
			}
			ui.Icon(c, icons.Parse(name)).Size(17, 17)
		})
		if follow.Clicked() {
			p.toggleFollow(card)
		}
	})
}

// heroSlides is the slide picker: inline under the copy on a docked page, and
// in the banner's bottom-right corner otherwise, where it sits on the same line
// as the copy's own bottom so the slide's two clusters read together.
func (p *Page) heroSlides(c *ui.Context, t *ui.Theme, width float32, inline bool, bottom float32) {
	art := design.OnArtwork(t)
	featured, hero := p.banner()
	if len(featured) < 2 {
		return
	}
	w, h := float32(slideWidth), float32(slideHeight)
	row := ui.Row(c).AlignItems(ui.End).Gap(8)
	if inline {
		w, h = 46, 28
		row.FillWidth().Gap(6)
	} else {
		row.Absolute().Right(design.ContentMargin).Bottom(bottom)
	}
	row.Children(func() {
		ui.Text(c, fmt.Sprintf("%02d / %02d", hero+1, len(featured))).FontSize(11).
			TextColor(art.Muted).MarginEnd(4)
		for i, slide := range featured {
			index := i
			ui.Column(c).Width(w).Gap(5).Children(func() {
				thumb := ui.ButtonBase(c).Size(w, h).Radius(5).Clip().
					Background(t.Surface).Label(slide.Title).Tooltip(slide.Title)
				if index == hero {
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
				if index == hero {
					bar.Background(t.Accent)
				}
			})
		}
	})
}

func (p *Page) sectionView(c *ui.Context, t *ui.Theme, m locales.Messages, snap library.Snapshot, width float32) {
	inset := design.Inset(width)
	ui.Column(c).FillWidth().Padding(design.PageTop, inset, 64+design.BottomClearance(width), inset).
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
				// A still can be almost white, so the track and the play mark
				// get a foot of their own: the shading is the frame's height,
				// so it holds on a wide card and a narrow one alike.
				height := float32(continueScrimMin)
				if share := w * 9 / 16 * continueScrimShare; share > height {
					height = share
				}
				ui.Box(c).Absolute().Left(0).Right(0).Bottom(0).Height(height).LinearGradient(ui.LinearGradient{
					Angle: 180,
					Stops: []ui.GradientStop{
						{Color: ui.Transparent, At: 0},
						{Color: art.Foot.Mix(ui.Transparent, 0.35), At: 0.45},
						{Color: art.Foot, At: 1},
					},
				})
				if progress.Duration > 0 {
					p.track(c, t, progress)
				}
				ui.Icon(c, icons.Parse("play-bold")).Absolute().Right(10).Bottom(14).Size(12, 12).
					TextColor(art.Ink).Shadow(0, 1, 2, 0, design.PlayMarkShadow())
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

// track draws the watched part of an episode along the bottom of its frame. The
// filled part goes first and the unfilled one covers the rest of the line, so
// the accent is what a viewer sees as progress.
func (p *Page) track(c *ui.Context, t *ui.Theme, progress library.Progress) {
	art := design.OnArtwork(t)
	watched := float32(progress.Position.Seconds() / progress.Duration.Seconds() * 100)
	ui.Row(c).Absolute().Left(10).Right(10).Bottom(10).Height(3).Radius(2).Children(func() {
		ui.Box(c).Height(3).Radius(2).Background(art.Track).FillWidth()
		ui.Box(c).Height(3).Radius(2).Background(t.Accent).WidthPercent(clamp(watched, 0, 100))
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
	scheduled, asked := p.dayUpdates(p.selected)
	loading := p.dayLoading(p.selected)

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
			if loading {
				ui.Text(c, m.LibraryLoading()).FontSize(12).TextColor(t.TextMuted)
			} else if count := len(scheduled); asked && count > 0 {
				ui.Text(c, m.Updates(count)).FontSize(12).TextColor(t.TextMuted)
			} else if len(updates) > 0 {
				ui.Text(c, m.Updates(len(updates))).FontSize(12).TextColor(t.TextMuted)
			}
		})

		switch {
		case len(snap.Follows) == 0 && len(scheduled) == 0 && !loading:
			p.emptyPanel(c, t, m.EmptyWeek(), m.EmptyContinueHint())
		case len(scheduled) > 0:
			columns := float32(3)
			if design.Docked(width) {
				columns = 2
			}
			p.dayPosters(c, t, m, scheduled, available, columns)
		case loading:
			p.emptyPanel(c, t, m.LibraryLoading(), m.EmptyContinueHint())
		case len(updates) > 0:
			columns := float32(3)
			if design.Docked(width) {
				columns = 2
			}
			posterW := clamp((available-(columns-1)*posterGap)/columns, 120, 220)
			ui.Row(c).FillWidth().Gap(posterGap).AlignItems(ui.Start).Wrap().Children(func() {
				for i, follow := range updates {
					p.posterCard(c, t, m, follow, posterW, i)
				}
			})
		}
	})
}

// dayPosters lays out one day's scheduled titles, as narrow as the column count
// allows: a busy day then shows six posters in the room three wide ones take,
// and a quiet one stays legible. The column count is the widest the width fits,
// capped so one day never floods the page.
func (p *Page) dayPosters(c *ui.Context, t *ui.Theme, m locales.Messages, scheduled []source.Update, available, maxColumns float32) {
	columns := float32(1)
	for candidate := maxColumns; candidate > 1; candidate-- {
		if fit := (available - (candidate-1)*posterGap) / candidate; fit >= dayPosterMinWidth {
			columns = candidate
			break
		}
	}
	limit := int(columns) * dayPosterRows
	if limit > len(scheduled) {
		limit = len(scheduled)
	}
	posterW := clamp((available-(columns-1)*posterGap)/columns, 96, 220)
	ui.Row(c).FillWidth().Gap(posterGap).AlignItems(ui.Start).Wrap().Children(func() {
		for i := 0; i < limit; i++ {
			p.updateCard(c, t, m, scheduled[i], posterW, i)
		}
	})
}

// dayCell is one day of the week strip. The day the viewer picked is the cell's
// focus: it takes the accent ring and an accent wash. Today is only a quiet
// marker on top of that, a small accent dot beside its weekday, so the two
// states never fight for the same cell and picking another day stays visible.
// A narrow cell drops its count for a dot, so a day with news still says so.
func (p *Page) dayCell(c *ui.Context, t *ui.Theme, m locales.Messages, day time.Time, index, today int, w float32, snap library.Snapshot) {
	count := len(updatesOn(snap.Follows, day))
	if scheduled, ok := p.dayUpdates(index); ok && len(scheduled) > 0 {
		count = len(scheduled)
	}
	selected := index == p.selected
	cell := ui.ButtonBase(c).Width(w).Radius(14).Padding(10, 12).Label(m.Updates(count))
	ink := t.TextMuted
	switch {
	case selected:
		cell.Background(design.CategoryWash(t)).Border(1.5, t.Accent)
		ink = t.Text
	default:
		cell.Background(t.Surface)
		if cell.Hovered() {
			cell.Background(t.SurfaceHover)
			ink = t.Text
		}
	}
	cell.Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Column(c).Gap(2).Children(func() {
				ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
					weekday := strings.ToUpper(i18n.FormatDate(c.Locale(), day, i18n.DateOptions{Skeleton: "EEE"}))
					if index == today {
						weekday = m.Today()
					}
					ui.Text(c, weekday).FontSize(10).TextColor(ink)
					if index == today {
						ui.Box(c).Size(4, 4).Radius(2).Background(t.Accent)
					}
				})
				ui.Text(c, fmt.Sprintf("%02d", day.Day())).FontSize(19).Bold().TextColor(ink)
			})
			ui.Spacer(c)
			switch {
			case count == 0:
			case w >= dayCountWidth:
				ui.Text(c, fmt.Sprint(count)).FontSize(12).TextColor(ink)
			default:
				ui.Box(c).Size(dayDot, dayDot).Radius(dayDot / 2).Background(t.Accent)
			}
		})
	})
	if cell.Clicked() {
		p.selectDay(index)
	}
}

// updateCard is one title the source schedules for a day: its poster, its title
// and the status line the source states, e.g. how far the episodes have come.
func (p *Page) updateCard(c *ui.Context, t *ui.Theme, m locales.Messages, update source.Update, w float32, index int) {
	bg, fg := design.PosterStatus(true)
	p.poster.View(c, t, m, poster.Card{
		Ref:       update.Card.Ref,
		Title:     update.Title,
		PosterURL: update.Card.PosterURL,
		// These cards are the week's fresh updates, so they wear the updating
		// status pill. The note is the source's own status line, data rather
		// than interface copy, so it shows as reported.
		Badges:   []poster.Badge{{Label: m.StatusUpdating(), Background: bg, Ink: fg}},
		Subtitle: update.Note,
	}, w, index, poster.Actions{Open: func() { p.detail(update.Card) }})
}

func (p *Page) posterCard(c *ui.Context, t *ui.Theme, m locales.Messages, follow library.Follow, w float32, index int) {
	card := source.Card{Ref: follow.Media, Title: follow.Title, PosterURL: follow.PosterURL}
	p.poster.View(c, t, m, poster.Card{
		Ref:       follow.Media,
		Title:     follow.Title,
		PosterURL: follow.PosterURL,
		// The update date under the title, the one fact the library keeps
		// about a follow besides its artwork.
		Subtitle: p.followMeta(c, follow),
	}, w, index, poster.Actions{Open: func() { p.detail(card) }})
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
	ui.Column(c).FillWidth().Radius(14).Background(t.Surface).Border(1, t.Border).
		Padding(18, 20).Gap(4).Children(func() {
		ui.Text(c, title).FontSize(13).Bold().TextColor(t.Text)
		ui.Text(c, hint).FontSize(12).TextColor(t.TextMuted)
	})
}

// errorView is the page's failure: the text stays clear of the floating
// navigation, and the failure itself is passed in rather than read from the
// page, so one frame draws one answer.
func (p *Page) errorView(c *ui.Context, t *ui.Theme, m locales.Messages, err error) {
	// The inset keeps the text clear of the floating navigation, and the
	// retry button is sized to its label rather than the page: a failure to
	// load is a short message, and a full-width button reads as the point of
	// the page.
	width, _ := c.Size()
	ui.Column(c).FillWidth().Padding(design.PageTopInset(c.TitleBar().Bottom), design.Inset(width)).
		Gap(12).Children(func() {
		ui.Text(c, m.HomeError()).FontSize(18).Bold().TextColor(t.Text)
		if err != nil {
			ui.Text(c, err.Error()).FontSize(12).TextColor(t.TextMuted).MaxLines(3)
		}
		// A signed source fails this way for one fixable reason: the machine
		// has no value for the variable the rule names. Saying so here saves
		// a reader from reading a signature error that looks like a bug.
		if kind, ok := source.KindOf(err); ok && kind == source.ErrUnauthorized {
			ui.Text(c, m.AccessKeyNeeded()).FontSize(12).TextColor(t.TextMuted)
		}
		ui.Row(c).Children(func() {
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
	})
}

func (p *Page) noticeText(m locales.Messages) (string, bool) {
	switch p.currentNotice() {
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

func clamp(value, low, high float32) float32 {
	if value < low {
		return low
	}
	if value > high {
		return high
	}
	return value
}
