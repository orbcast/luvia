package player

import (
	"fmt"
	"math"
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

// The stage's top line: the episode marker at the top-left and the floating
// control cluster at the top-right.
const (
	stageLineTop    float32 = 24
	stageMarkerSize float32 = 13
	stageButtonSize float32 = 34
	pillPadding     float32 = 4
)

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
		default:
			ui.Row(c).Fill().Children(func() {
				ui.Box(c).Grow(1).FillHeight().Children(func() { p.stage(c, t, m, width-panelWidth) })
				p.panel(c, t, m, height)
			})
		}
	})
	return root
}

// stage is the picture plus the overlay chrome laid over it. The video is
// always mounted, whether its first frame has come or not: an engine delivers
// that frame on its own clock, and taking the element away until Ready would
// hide the only frame the player ever shows. Everything that covers the stage
// goes under the video, or nothing of it would show.
func (p *Page) stage(c *ui.Context, t *ui.Theme, m locales.Messages, width float32) {
	if p.loading && p.player == nil && p.notice == noticeNone {
		p.stageOpening(c, m)
	} else if p.player != nil {
		p.stageFallback(c, t, m)
		// The stage keeps the whole frame. Cover scales a picture by the larger
		// of the two ratios, which for a portrait series (short dramas ship
		// 360x640) crops it to a centre band of a wide stage: the picture reads
		// as zoomed in and cut off. Contain scales by the smaller ratio, so the
		// frame fits whole and the backdrop behind it fills the sides; a frame
		// that already matches the stage's ratio fills it either way.
		ui.Video(c, p.player).Absolute().Left(0).Right(0).Top(0).Bottom(0).Fit(ui.Contain)
		// An engine that has not delivered its first frame is still opening the
		// stream, which on a slow line is the longest wait of all; one that has
		// its picture and waits for more data is buffering. Both say so, with the
		// time they have taken, so neither looks like a stage that stopped. A
		// notice of its own covers the stage instead. The stage is also the only
		// thing watching the engine: one that draws nothing never wakes the
		// window again, so a wait that has run too long is seen here, and the
		// stage says the stream gave no picture.
		switch {
		case p.notice != noticeNone:
		case p.stalled():
			// The picture came and has stood still since, which the engine does
			// not say: it is waited on, and past this long it is the same stall
			// as a stream that never drew one.
			p.stageBuffering(c, m)
			p.watchEngine()
		case !p.player.Ready():
			p.stagePreparing(c, m)
			p.watchEngine()
		default:
			// The picture is here and moving. It may still stop moving without
			// the engine saying so, which only a stage that looks again can
			// tell: one that plays keeps asking for frames, and standing is
			// what says when the playhead has stopped for too long. A stream
			// that played out, or that a viewer paused, stands still by itself.
			p.buffered = time.Time{}
			p.watchEngine()
			if p.player != nil && p.player.Playing() {
				c.After(waitTick)
			}
		}
	} else {
		// No stream yet: either the line is still being resolved, which says so
		// above, or its stream is opening. Opening is the engine's own work and
		// may take long, or never return, so it is waited on here, off the
		// stream's own clock, rather than on the page's thread. An open that
		// holds the stage longer than an opening may take is reported, so a
		// stream that never comes back is not a stage that waits for good.
		p.stageFallback(c, t, m)
		if p.notice == noticeNone {
			if p.loading {
				p.stageOpening(c, m)
			} else if !p.watchOpening() {
				p.stagePreparing(c, m)
			}
		}
	}
	p.stageDanmaku(c, width)
	p.stageTop(c, t, m)
	p.stageControls(c, t, m)
}

// stageDanmaku lays the comments over the picture: each one crosses a lane from
// the right edge to the left, at the pace of the playhead.
func (p *Page) stageDanmaku(c *ui.Context, width float32) {
	flying := p.flyingComments(p.position())
	if len(flying) == 0 {
		return
	}
	// The playhead moves on its own, so the layer keeps asking for frames
	// while comments are in flight.
	c.After(50 * time.Millisecond)
	const laneHeight float32 = 30
	top := float32(96)
	for _, item := range flying {
		left := width - (width+240)*item.slide
		ui.Row(c).Absolute().Left(left).Top(top + float32(item.lane)*laneHeight).
			Height(laneHeight).Children(func() {
			ui.Text(c, item.text).FontSize(16).Bold().MaxLines(1).
				TextColor(ui.RGBA(255, 255, 255, 0.95)).TextShadow(0, 1, 3, ui.RGBA(0, 0, 0, 0.75))
		})
	}
}

// stageBuffering shows a quiet marker while the picture that came has stood
// still for longer than a wait may take, timed from the frame the playhead
// stopped on. The engine's own word for it is no use: it goes on calling a
// moving stream buffering long after the picture came back.
func (p *Page) stageBuffering(c *ui.Context, m locales.Messages) {
	if p.buffered.IsZero() {
		p.buffered = time.Now()
	}
	p.stageWaiting(c, m, p.stallAt, m.Buffering())
}

// stagePreparing shows the same marker while the engine opens a stream it has not
// drawn yet, timed from the open: on a slow line that is the longest wait of all.
func (p *Page) stagePreparing(c *ui.Context, m locales.Messages) {
	if p.buffered.IsZero() {
		p.buffered = time.Now()
	}
	p.stageWaiting(c, m, p.buffered, m.Preparing())
}

// stageWaiting draws the marker of a wait that began at since: what is waited on,
// and for how long. A wait that lasts gets the same line the resolution wait has:
// how long, and how fast. Waiting is a state the stage only learns about while it
// draws, so the marker keeps asking for frames until the wait ends.
func (p *Page) stageWaiting(c *ui.Context, m locales.Messages, since time.Time, label string) {
	now := time.Now()
	waited := now.Sub(since)
	c.After(waitTick)
	ui.Row(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Row(c).Radius(999).Padding(8, 16).Background(ui.RGBA(0, 0, 0, 0.5)).
			AlignItems(ui.Center).Gap(10).Children(func() {
			// A ring that fades in and out: the wait is the engine's, so the
			// only thing moving is the marker saying so.
			spin := float32(0.5 + 0.5*math.Sin(float64(now.UnixMilli())/260))
			ui.Box(c).Width(12).Height(12).Radius(6).Border(1.5, ui.RGBA(255, 255, 255, 0.35+0.5*spin))
			ui.Text(c, label).FontSize(13).TextColor(mediaInk)
			ui.Text(c, p.waitText(m, waited)).FontSize(12).TextColor(mediaMuted)
		})
	})
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
			// A stage the stream never filled is a dead end without a way out
			// that does not depend on the panel being open.
			leave := ui.ButtonBase(c).Key("s-leave").Width(112).Height(30).Radius(15).
				Label(m.ClosePlayer()).Tooltip(m.ClosePlayer()).Background(ui.Transparent)
			leave.Children(func() {
				ui.Text(c, m.ClosePlayer()).FontSize(12).TextColor(mediaInk)
			})
			if leave.Clicked() {
				p.Back()
			}
		})
	}
}

// stageTop draws the episode marker at the top-left and the floating control
// cluster (speaker / danmaku / panel buttons) at the top-right. The two sit on
// one line: the cluster's buttons hang at the middle of the marker, so they
// read level with the label of the episode they belong to.
func (p *Page) stageTop(c *ui.Context, t *ui.Theme, m locales.Messages) {
	top := stageLineTop
	if episode, ok := p.current(); ok {
		ui.Text(c, p.episodeLabel(m, episode)).Absolute().Left(84).Top(top).
			FontSize(13).Bold().TextColor(mediaInk)
	}
	cluster := top + stageMarkerSize/2 - (stageButtonSize+2*pillPadding)/2
	p.playerPill(c, m, cluster)
}

// playerPill is the level / speaker / danmaku / panel-toggle cluster at the top-right.
// It is one capsule: the panel toggle turns into the reveal control when the
// panel goes away, so the cluster never breaks into two floating pieces, and
// the level is the thin white track beside the speaker that the system's own
// player shows, never a slider that folds out and back.
func (p *Page) playerPill(c *ui.Context, m locales.Messages, top float32) {
	ui.Row(c).Absolute().Right(24).Top(top).AlignItems(ui.Center).Gap(2).Radius(999).
		Padding(pillPadding).Material(glass.Glass{}).Children(func() {
		p.volumeTrack(c, m)
		p.speaker(c, m)
		p.pillButton(c, "danmaku", "chat-round-linear", m.DanmakuLabel(), p.danmaku, p.ToggleDanmaku)
		if p.panelHidden {
			p.panelReveal(c, m)
			return
		}
		p.pillButton(c, "panel", "widget-2-linear", m.CollapsePanel(), false, p.CollapsePanel)
	})
}

// volumeTrack is the level itself, a track with its filled part and knob, as
// the system's player lays it beside the speaker. Pressing it sets the level
// where the pointer lands and dragging along it keeps setting it.
func (p *Page) volumeTrack(c *ui.Context, m locales.Messages) {
	frac := float32(clampF(p.volume, 0, 1))
	track := ui.ButtonBase(c).Key("p-volume").Width(volumeTrackWidth).Height(stageButtonSize).
		Label(m.VolumeLabel()).Tooltip(m.VolumeLabel()).Background(ui.Transparent).Cursor(ui.CursorResizeEW)
	track.Children(func() {
		const barHeight, knob float32 = 4, 12
		middle := (stageButtonSize - barHeight) / 2
		ui.Box(c).Absolute().Left(0).Right(0).Top(middle).Height(barHeight).Radius(barHeight / 2).
			Background(ui.RGBA(255, 255, 255, 0.28))
		ui.Box(c).Absolute().Left(0).Top(middle).Height(barHeight).Radius(barHeight / 2).
			Background(mediaInk).Width((volumeTrackWidth-knob)*frac + knob/2)
		ui.Box(c).Absolute().Top((stageButtonSize-knob)/2).Left((volumeTrackWidth-knob)*frac).
			Size(knob, knob).Radius(knob / 2).Background(mediaInk)
	})
	// Where the pointer is along the track is the level: a press sets it at
	// once and every move of the drag follows it, so the level never lags
	// behind the hand. A muted video comes back as soon as the level is raised.
	if x, _, over := track.PointerPosition(); over && track.Pressed() {
		p.SetVolume(clampF(float64(x)/float64(volumeTrackWidth-volumeKnob), 0, 1))
	}
}

// speaker is the sound's switch, as on the system's player: a press silences
// the video, or brings it back at the level the track holds.
func (p *Page) speaker(c *ui.Context, m locales.Messages) {
	tip := m.MuteLabel()
	if p.Audible() {
		tip = m.UnmuteLabel()
	}
	button := ui.ButtonBase(c).Key("p-mute").Size(stageButtonSize, stageButtonSize).Radius(999).
		Label(m.MuteLabel()).Tooltip(tip).Background(ui.Transparent)
	ink := mediaMuted
	if p.Audible() {
		ink = mediaInk
	}
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	icon := "volume-loud-linear"
	if !p.Audible() {
		icon = "volume-cross-linear"
	}
	button.Children(func() { ui.Icon(c, icons.Parse(icon)).Size(18, 18).TextColor(ink) })
	if button.Clicked() {
		p.ToggleMute()
	}
}

// volumeTrackWidth is the length of the level track beside the speaker, and
// volumeKnob the knob's diameter, which the travel stops short of so the knob
// stays inside the track at either end.
const (
	volumeTrackWidth float32 = 110
	volumeKnob       float32 = 12
)

// panelReveal is the single control the picture keeps while the side panel is
// away: it brings the episode list and lines back when the viewer wants them.
// It lives inside the pill, so the cluster stays one capsule whether the panel
// is out or away.
func (p *Page) panelReveal(c *ui.Context, m locales.Messages) {
	label := m.ExpandPanel()
	button := ui.ButtonBase(c).Key("p-panel-reveal").Size(stageButtonSize, stageButtonSize).Radius(999).Label(label).
		Tooltip(label).Background(ui.Transparent)
	ink := mediaMuted
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
		ink = mediaInk
	}
	button.Children(func() { ui.Icon(c, icons.Parse("widget-2-linear")).Size(18, 18).TextColor(ink) })
	if button.Clicked() {
		p.RevealPanel()
	}
}

func (p *Page) pillButton(c *ui.Context, key, icon, tip string, active bool, fn func()) {
	button := ui.ButtonBase(c).Key("p-"+key).Size(stageButtonSize, stageButtonSize).Radius(999).Label(tip).Tooltip(tip).
		Background(ui.Transparent)
	ink := mediaMuted
	switch {
	case active:
		button.Background(ui.RGBA(255, 255, 255, 0.16))
		ink = mediaInk
	case button.Hovered():
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Icon(c, icons.Parse(icon)).Size(18, 18).TextColor(ink) })
	if button.Clicked() {
		fn()
	}
}

func (p *Page) stageControls(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Column(c).Absolute().Left(0).Right(0).Bottom(0).Padding(0, 24, 16, 24).Gap(8).Children(func() {
		if episode, ok := p.current(); ok {
			ui.Text(c, p.detail.Title+"  "+p.episodeLabel(m, episode)).FontSize(14).Bold().TextColor(mediaInk)
		}

		ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(10).Children(func() {
			ui.Text(c, clock(p.position())).FontSize(11).TextColor(mediaInk)
			p.scrubber(c)
			// Until the engine learns the stream's length both ends read 00:00,
			// which looks like a film that cannot start; the mark says the
			// length is unknown rather than pretending it is zero.
			ui.Text(c, lengthLabel(p.duration())).FontSize(11).TextColor(mediaInk)
		})

		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Row(c).AlignItems(ui.Center).Gap(6).Children(func() {
				p.iconButton(c, "rewind", "rewind-10-seconds-back-linear", m.Rewind10(), func() { p.SeekBy(-10 * time.Second) })
				p.playButton(c, m)
				p.iconButton(c, "forward", "rewind-10-seconds-forward-linear", m.Forward10(), func() { p.SeekBy(10 * time.Second) })
			})
			ui.Spacer(c)
			ui.Row(c).AlignItems(ui.Center).Gap(2).Radius(999).Padding(4).Material(glass.Glass{}).Children(func() {
				p.iconButton(c, "prev", "skip-previous-linear", m.PreviousEpisode(), p.Previous)
				p.iconButton(c, "next", "skip-next-linear", m.NextEpisode(), p.Next)
				p.rateButton(c, m)
				p.fullscreenButton(c, m)
			})
		})
	})
}

// lengthLabel is the total time, or a dash while the engine has not learned it.
func lengthLabel(total time.Duration) string {
	if total <= 0 {
		return "--:--"
	}
	return clock(total)
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
		Background(ui.Transparent)
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Icon(c, icons.Parse(icon)).Size(24, 24).TextColor(mediaInk) })
	if button.Clicked() {
		p.TogglePlay()
	}
}

func (p *Page) rateButton(c *ui.Context, m locales.Messages) {
	label := rateLabel(p.rate)
	button := ui.ButtonBase(c).Key("rate").Size(34, 34).Radius(9).Label(m.PlaybackSpeed()).
		Tooltip(m.PlaybackSpeed()).Background(ui.Transparent)
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Text(c, label).FontSize(12).Bold().TextColor(mediaInk) })
	if button.Clicked() {
		p.CycleRate()
	}
}

func (p *Page) fullscreenButton(c *ui.Context, m locales.Messages) {
	label := m.FullscreenLabel()
	if p.cinema {
		label = m.ExitFullscreenLabel()
	}
	button := ui.ButtonBase(c).Key("fullscreen").Size(34, 34).Radius(9).Label(label).
		Tooltip(label).Background(ui.Transparent)
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() { ui.Icon(c, icons.Parse("full-screen-linear")).Size(18, 18).TextColor(mediaInk) })
	if button.Clicked() {
		p.ToggleFullScreen()
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

func (p *Page) panelHeader(c *ui.Context, t *ui.Theme, m locales.Messages) {
	ui.Row(c).FillWidth().AlignItems(ui.Start).Children(func() {
		ui.Column(c).Grow(1).Gap(5).Children(func() {
			ui.Text(c, m.NowPlaying()).FontSize(10).LetterSpacing(1.6).TextColor(t.TextMuted)
			ui.Text(c, p.detail.Title).FontSize(16).Bold().TextColor(t.Text).MaxLines(2)
			if episode, ok := p.current(); ok {
				ui.Text(c, fmt.Sprintf("ep.%d  %s", episode.Number, episode.Title)).FontSize(12).TextColor(t.TextMuted)
			}
			ui.Text(c, p.metaLine(m)).FontSize(11).TextColor(t.TextMuted)
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

// metaLine is the duration and year shown under the episode title.
func (p *Page) metaLine(m locales.Messages) string {
	parts := []string{fmt.Sprintf("%s %s", m.DurationLabel(), clock(p.duration()))}
	if p.detail.Year > 0 {
		parts = append(parts, strconv.Itoa(p.detail.Year))
	}
	return joinMeta(parts)
}

func joinMeta(parts []string) string {
	out := ""
	for i, part := range parts {
		if i > 0 {
			out += " · "
		}
		out += part
	}
	return out
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
	case noticeNoFrame:
		return m.NoPicture()
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
