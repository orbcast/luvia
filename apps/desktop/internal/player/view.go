package player

import (
	"fmt"
	"strconv"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/locales"
)

const panelWidth float32 = 360

var (
	mediaInk   = ui.RGBA(255, 255, 255, 0.94)
	mediaMuted = ui.RGBA(255, 255, 255, 0.62)
)

// View builds the playback screen: the video stage with its overlay controls and
// the now-playing side panel.
func (p *Page) View(c *ui.Context) ui.Element {
	p.redraw = c.Invalidate
	t := c.Theme()
	m := locales.In(c.Locale())
	width, height := c.Size()

	root := ui.Box(c).Fill().Background(design.PlayerStage(t)).Clip()
	root.Children(func() {
		switch {
		case design.Docked(width):
			ui.Column(c).Fill().Children(func() {
				ui.Box(c).FillWidth().Height(height * 0.52).Children(func() { p.stage(c, t, m, width) })
				scroll := ui.Scroll(c).Grow(1)
				scroll.Children(func() { p.panelBody(c, t, m, width) })
			})
		case p.cinema:
			p.stage(c, t, m, width)
		case p.panelHidden:
			p.stage(c, t, m, width)
			p.panelReveal(c, t, m)
		default:
			ui.Row(c).Fill().Children(func() {
				ui.Box(c).Grow(1).FillHeight().Children(func() { p.stage(c, t, m, width-panelWidth) })
				p.panel(c, t, m, height)
			})
		}
	})
	return root
}

// stage is the picture plus the overlay chrome laid over it.
func (p *Page) stage(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	if p.player != nil && p.player.Ready() {
		ui.Video(c, p.player).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.Cover)
	} else {
		p.stageFallback(c, t, m)
	}
	p.stageTop(c, t, m)
	p.stageControls(c, t, m)
}

func (p *Page) stageFallback(c *ui.Context, t *ui.Theme, m locales.Messages) {
	url := p.detail.BackdropURL
	if url == "" {
		url = p.detail.PosterURL
	}
	painted := false
	if p.deps.Images != nil && url != "" {
		if bitmap := p.deps.Images.Bitmap(url); bitmap != nil {
			ui.Image(c, bitmap).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.Cover).Opacity(0.55)
			painted = true
		}
	}
	if !painted {
		ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			Gradient(design.PlayerStage(t), t.Surface, 150)
	}
	if message := p.noticeText(m); message != "" {
		ui.Column(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
			Justify(ui.Center).AlignItems(ui.Center).Gap(8).Children(func() {
			ui.Icon(c, icons.Parse("full-screen-linear")).Size(34, 34).TextColor(mediaMuted)
			ui.Text(c, message).FontSize(13).TextColor(mediaMuted)
		})
	}
}

func (p *Page) stageTop(c *ui.Context, t *ui.Theme, m locales.Messages) {
	back := ui.ButtonBase(c).Key("player-back").Absolute().Left(24).Top(44).Size(40, 40).Radius(20).
		Material(glass.Glass{}).Label(m.Back()).Tooltip(m.Back()).ZIndex(8)
	if back.Hovered() {
		back.Material(glass.Glass{Interactive: true})
	}
	back.Children(func() {
		ui.Icon(c, icons.Parse("alt-arrow-left-linear")).Size(20, 20).TextColor(mediaInk)
	})
	if back.Clicked() {
		p.Back()
	}
	if episode, ok := p.current(); ok {
		ui.Text(c, p.episodeLabel(m, episode)).Absolute().Left(78).Top(56).
			FontSize(13).Bold().TextColor(mediaInk)
	}
}

func (p *Page) stageControls(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Padding(0, 22, 16, 22).Gap(10).Children(func() {
		if episode, ok := p.current(); ok {
			ui.Text(c, p.detail.Title+"  "+p.episodeLabel(m, episode)).FontSize(13).Bold().TextColor(mediaInk)
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(12).Children(func() {
			ui.Text(c, clock(p.position())).FontSize(11).TextColor(mediaInk)
			p.scrubber(c)
			ui.Text(c, clock(p.duration())).FontSize(11).TextColor(mediaInk)
		})

		if p.volumeOpen {
			p.volumePanel(c, m)
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(8).Children(func() {
				p.playButton(c, m)
				p.iconButton(c, "next", "skip-next-linear", m.NextEpisode(), p.Next)
			})
			ui.Spacer(c)
			ui.Row(c).AlignItems(ui.Center).Gap(2).Radius(999).Padding(4).Material(glass.Glass{}).Children(func() {
				p.iconButton(c, "prev", "skip-previous-linear", m.PreviousEpisode(), p.Previous)
				p.rateButton(c, m)
				p.volumeButton(c, m)
				p.subtitleButton(c, m)
				p.fullscreenButton(c, m)
			})
		})
	})
}

// scrubber is the thin progress track; pressing it seeks.
func (p *Page) scrubber(c *ui.Context) {
	if b := p.scrubHandle.Bounds(c); b.W > 0 {
		p.scrubW = b.W
	}
	frac := float32(p.fraction())
	track := ui.ButtonBase(c).Key("scrub").Grow(1).Height(20).Label("seek").Bind(&p.scrubHandle)
	track.Children(func() {
		ui.Box(c).Absolute().Left(0).Right(0).Top(8).Height(4).Radius(2).Background(design.PlayerTrack())
		ui.Box(c).Absolute().Left(0).Top(8).Height(4).Radius(2).Background(mediaInk).WidthPercent(frac * 100)
		if p.scrubW > 0 {
			ui.Box(c).Absolute().Top(5).Left(frac*p.scrubW-4).Size(9, 9).Radius(5).Background(mediaInk)
		}
	})
	if x, _, over := track.PointerPosition(); over && track.Pressed() && p.scrubW > 0 {
		p.SeekFraction(float64(x / p.scrubW))
		p.invalidate()
	}
}

func (p *Page) playButton(c *ui.Context, m locales.Messages) {
	icon, label := "play-bold", m.PlayNow()
	if p.playing {
		icon, label = "pause-bold", m.PauseLabel()
	}
	button := ui.ButtonBase(c).Key("play").Size(44, 44).Radius(22).Label(label).Tooltip(label).
		Material(glass.Glass{})
	if button.Hovered() {
		button.Material(glass.Glass{Interactive: true})
	}
	button.Children(func() { ui.Icon(c, icons.Parse(icon)).Size(20, 20).TextColor(mediaInk) })
	if button.Clicked() {
		p.TogglePlay()
	}
}

func (p *Page) rateButton(c *ui.Context, m locales.Messages) {
	label := rateLabel(p.rate)
	button := ui.ButtonBase(c).Key("rate").Height(34).Radius(9).PaddingX(10).Label(m.PlaybackSpeed()).
		Tooltip(m.PlaybackSpeed()).Background(ui.Transparent)
	button.Children(func() { ui.Text(c, label).FontSize(12).Bold().TextColor(mediaInk) })
	if button.Clicked() {
		p.CycleRate()
	}
}

// volumeButton opens the volume slider; a small chevron shows whether it is
// expanded, as a disclosure control.
func (p *Page) volumeButton(c *ui.Context, m locales.Messages) {
	button := ui.ButtonBase(c).Key("volume").Height(34).Radius(9).PaddingX(8).
		Label(m.VolumeLabel()).Tooltip(m.VolumeLabel()).Background(ui.Transparent)
	if p.volumeOpen || button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() {
		ui.Row(c).AlignItems(ui.Center).Gap(4).Children(func() {
			ui.Icon(c, icons.Parse("volume-loud-linear")).Size(18, 18).TextColor(mediaInk)
			chevron := ui.Icon(c, icons.Parse("alt-arrow-down-linear")).Size(10, 10).TextColor(mediaMuted)
			if p.volumeOpen {
				chevron.Rotate(180)
			}
		})
	})
	if button.Clicked() {
		p.ToggleVolumePanel()
	}
}

// volumePanel is the slider the volume button discloses, right-aligned over the
// control bar.
func (p *Page) volumePanel(c *ui.Context, m locales.Messages) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
		ui.Spacer(c)
		ui.Row(c).AlignItems(ui.Center).Gap(10).Radius(12).Padding(8, 12).Material(glass.Glass{}).Children(func() {
			ui.Icon(c, icons.Parse("volume-loud-linear")).Size(16, 16).TextColor(mediaInk)
			slider := ui.Slider(c, &p.volume, 0, 1).Width(150).Height(22)
			slider.OnChange(func() { p.SetVolume(p.volume) })
			ui.Text(c, fmt.Sprintf("%d%%", int(p.volume*100+0.5))).FontSize(11).TextColor(mediaInk)
		})
	})
}

// subtitleButton toggles the subtitle layer; its active state is a lit pill.
func (p *Page) subtitleButton(c *ui.Context, m locales.Messages) {
	ink := mediaMuted
	button := ui.ButtonBase(c).Key("subtitle").Size(34, 34).Radius(9).Label(m.SubtitlesLabel()).
		Tooltip(m.SubtitlesLabel()).Background(ui.Transparent)
	if p.showSubtitle {
		button.Background(ui.RGBA(255, 255, 255, 0.16))
		ink = mediaInk
	} else if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Icon(c, icons.Parse("subtitles-linear")).Size(18, 18).TextColor(ink) })
	if button.Clicked() {
		p.ToggleSubtitle()
	}
}

func (p *Page) fullscreenButton(c *ui.Context, m locales.Messages) {
	label := m.FullscreenLabel()
	if p.cinema {
		label = m.ExitFullscreenLabel()
	}
	button := ui.ButtonBase(c).Key("fullscreen").Size(34, 34).Radius(9).Label(label).
		Tooltip(label).Background(ui.Transparent)
	button.Children(func() { ui.Icon(c, icons.Parse("full-screen-linear")).Size(18, 18).TextColor(mediaInk) })
	if button.Clicked() {
		p.ToggleCinema()
	}
}

func (p *Page) iconButton(c *ui.Context, key, icon, tip string, fn func()) {
	button := ui.ButtonBase(c).Key("p-"+key).Size(34, 34).Radius(9).Label(tip).Tooltip(tip).
		Background(ui.Transparent)
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Icon(c, icons.Parse(icon)).Size(18, 18).TextColor(mediaInk) })
	if button.Clicked() && fn != nil {
		fn()
	}
}

func (p *Page) panel(c *ui.Context, t *ui.Theme, m locales.Messages, height float32) {
	bg, border := design.PlayerPanel(t)
	box := ui.Box(c).Width(panelWidth).Height(height).Background(bg).Border(1, border)
	box.Children(func() {
		scroll := ui.Scroll(c).Fill()
		scroll.Children(func() { p.panelBody(c, t, m, panelWidth) })
		// A slim handle on the panel's left seam, vertically centered, folds the
		// panel away; the same grip returns on the right seam to reopen it.
		handle := ui.ButtonBase(c).Key("p-collapse").Absolute().Left(-10).Top(height/2-28).
			Size(20, 56).Radius(10).Material(glass.Glass{Interactive: false}).
			Label(m.CollapsePanel()).Tooltip(m.CollapsePanel()).ZIndex(8)
		if handle.Hovered() {
			handle.Material(glass.Glass{Interactive: true})
		}
		handle.Children(func() {
			ui.Icon(c, icons.Parse("alt-arrow-right-linear")).Size(16, 16).TextColor(t.TextMuted)
		})
		if handle.Clicked() {
			p.TogglePanel()
		}
	})
}

func (p *Page) panelBody(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	p.applyLine()
	ui.Column(c).FillWidth().Padding(20, 20, 24, 20).Gap(16).Children(func() {
		p.panelHeader(c, t, m)

		if p.detail.Overview != "" {
			overview := ui.Text(c, p.detail.Overview).FontSize(12.5).LineHeight(1.6).TextColor(t.TextMuted)
			if !p.expanded {
				overview.MaxLines(3)
			}
			expand := ui.ButtonBase(c).Key("p-expand").Height(22).PaddingX(0).Label(m.ExpandAll()).
				Background(ui.Transparent)
			expand.Children(func() {
				label := m.ExpandAll()
				if p.expanded {
					label = m.CollapseAll()
				}
				ui.Text(c, label).FontSize(12).Bold().TextColor(t.Accent)
			})
			if expand.Clicked() {
				p.ToggleExpanded()
			}
		}

		p.panelTabs(c, t, m)
		p.sourceSelector(c, t, m, width)
		ui.Text(c, m.CurrentPlaying(p.detail.Title)).FontSize(11.5).TextColor(t.TextMuted)

		switch p.tab {
		case tabComments:
			ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
		default:
			ui.Text(c, m.EpisodesTitle()).FontSize(12).TextColor(t.TextMuted)
			p.panelEpisodes(c, t, m, width)
		}
	})
}

// panelReveal is the arrow left floating on the video once the now-playing
// panel is hidden, so the viewer can bring it back.
func (p *Page) panelReveal(c *ui.Context, t *ui.Theme, m locales.Messages) {
	_, height := c.Size()
	button := ui.ButtonBase(c).Key("p-reveal").Absolute().Right(6).Top(height/2-28).Size(20, 56).Radius(10).
		Material(glass.Glass{}).Label(m.ExpandPanel()).Tooltip(m.ExpandPanel()).ZIndex(8)
	if button.Hovered() {
		button.Material(glass.Glass{Interactive: true})
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("alt-arrow-left-linear")).Size(16, 16).TextColor(mediaMuted)
	})
	if button.Clicked() {
		p.TogglePanel()
	}
}

func (p *Page) panelHeader(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Gap(5).Children(func() {
			ui.Text(c, m.NowPlaying()).FontSize(10).LetterSpacing(1.6).TextColor(t.TextMuted)
			ui.Text(c, p.detail.Title).FontSize(16).Bold().TextColor(t.Text).MaxLines(2)
			if episode, ok := p.current(); ok {
				ui.Text(c, fmt.Sprintf("ep.%d  %s", episode.Number, episode.Title)).FontSize(12).TextColor(t.TextMuted)
			}
			if d := p.duration(); d > 0 {
				ui.Text(c, fmt.Sprintf("%s %s", m.DurationLabel(), clock(d))).FontSize(11).TextColor(t.TextMuted)
			}
		})
		close := ui.ButtonBase(c).Key("p-close").Size(30, 30).Radius(15).Label(m.ClosePlayer()).
			Tooltip(m.ClosePlayer()).Background(ui.Transparent)
		if close.Hovered() {
			close.Background(t.SurfaceHover)
		}
		close.Children(func() { ui.Icon(c, icons.Parse("close-circle-linear")).Size(20, 20).TextColor(t.TextMuted) })
		if close.Clicked() {
			p.Back()
		}
	})
}

func (p *Page) panelTabs(c *ui.Context, t *ui.Theme, m locales.Messages) {
	labels := []string{m.EpisodesTab(), m.TabComments()}
	ui.Row(c).FillWidth().Children(func() {
		ui.Row(c).Radius(999).Padding(3).Gap(2).Background(t.Surface).Children(func() {
			for i, label := range labels {
				index := i
				button := ui.ButtonBase(c).Key("p-tab:" + strconv.Itoa(i)).Height(30).Radius(999).
					PaddingX(14).Label(label).Background(ui.Transparent)
				ink := t.TextMuted
				if index == p.tab {
					button.Background(t.SurfaceHover)
					ink = t.Text
				}
				button.Children(func() { ui.Text(c, label).FontSize(12).TextColor(ink) })
				if button.Clicked() {
					p.SelectTab(index)
				}
			}
		})
	})
}

// sourceSelector is the playback-line dropdown; it resolves another line when
// the viewer picks one.
func (p *Page) sourceSelector(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	lines := p.playback.Lines
	if len(lines) == 0 {
		return
	}
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(8).Children(func() {
		ui.Text(c, m.PlaybackSource()).FontSize(11.5).TextColor(t.TextMuted)
		sel := ui.SelectBase(c, &p.lineID)
		sel.Trigger.Grow(1).Height(32).Radius(9).PaddingX(10).AlignItems(ui.Center).Gap(8).
			Background(t.Surface)
		sel.Trigger.Children(func() {
			ui.Text(c, lineLabel(lines, p.lineID)).FontSize(12.5).TextColor(t.Text).Grow(1).MaxLines(1)
			ui.Icon(c, icons.Parse("alt-arrow-down-linear")).Size(13, 13).TextColor(t.TextMuted)
		})
		sel.Popup(func(panel ui.Element) {
			panel.Padding(6).Radius(12).Material(glass.Glass{})
			for _, line := range lines {
				value := line.ID
				item := sel.Item(value).Padding(7, 10).Radius(8).Label(lineLabelText(line))
				if item.Highlighted() {
					item.Background(design.NavigationTint())
				}
				item.Children(func() { ui.Text(c, lineLabelText(line)).FontSize(12.5).TextColor(t.Text) })
			}
		})
	})
}

func (p *Page) panelEpisodes(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	if len(p.episodes) == 0 {
		ui.Text(c, m.EmptyTab()).FontSize(13).TextColor(t.TextMuted)
		return
	}
	gap := float32(8)
	body := width - 40
	columns := float32(3)
	chipW := (body-gap*(columns-1))/columns - 1.5
	ui.Row(c).FillWidth().Wrap().Gap(gap).Children(func() {
		for i, episode := range p.episodes {
			index := i
			label := episode.Title
			if label == "" {
				label = m.EpisodeNumber(strconv.Itoa(episode.Number))
			}
			selected := index == p.index
			button := ui.ButtonBase(c).Key("panel-ep:" + episode.ID).Width(chipW).Height(44).Radius(9).
				Background(t.Surface).Label(label)
			if selected {
				button.Background(t.SurfaceHover).Border(1, t.Text)
			}
			button.Children(func() {
				ink := t.TextMuted
				if selected {
					ink = t.Text
				}
				ui.Text(c, label).FontSize(12).TextColor(ink).MaxLines(1)
			})
			if button.Clicked() {
				p.SelectEpisode(index)
			}
		}
	})
}

func (p *Page) noticeText(m locales.Messages) string {
	switch p.notice {
	case noticeUnavailable:
		return m.EpisodeUnavailable()
	case noticeUnsupported:
		return m.VideoUnsupported()
	case noticeFailed:
		return m.PlaybackFailed()
	default:
		if p.err != nil {
			return p.err.Error()
		}
		return ""
	}
}

func (p *Page) episodeLabel(m locales.Messages, episode source.Episode) string {
	if episode.Title != "" {
		return episode.Title
	}
	return m.EpisodeNumber(strconv.Itoa(episode.Number))
}

func lineLabel(lines []source.Line, id string) string {
	for _, line := range lines {
		if line.ID == id {
			return lineLabelText(line)
		}
	}
	if len(lines) > 0 {
		return lineLabelText(lines[0])
	}
	return ""
}

func lineLabelText(line source.Line) string {
	if line.Label != "" {
		return line.Label
	}
	return line.ID
}

func rateLabel(rate float64) string {
	if rate == float64(int(rate)) {
		return strconv.Itoa(int(rate)) + "x"
	}
	return strconv.FormatFloat(rate, 'g', -1, 64) + "x"
}

// clock writes a media time as m:ss, or h:mm:ss past an hour.
func clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	total := int(d / time.Second)
	h, m, s := total/3600, (total/60)%60, total%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%02d:%02d", m, s)
}
