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
	// The stage's top line: the way out and the marker at the left, the pill of
	// controls at the right. All three sit on one baseline — stageTopBox is the
	// height of the capsule either end draws and stageTopHeight is its bottom
	// edge, so the corner and the pill are laid at the same top and come out the
	// same height. The band's blur and the picture's bands are measured from
	// these, so they follow a change here.
	stageTopBox    float32 = stageButtonSize + 2*pillPadding
	stageTopHeight float32 = stageLineTop + stageTopBox
	stageEdge      float32 = 20
	// The gap between the corner's capsule and the marker beside it.
	stageMarkerGap  float32 = 14
	stageMarkerLeft float32 = stageEdge + stageTopBox + stageMarkerGap
	// The marker's label sits on the capsule's centre line rather than its top:
	// a 13px label at the capsule's top edge reads as hanging off the row.
	stageMarkerTop float32 = stageLineTop + (stageTopBox-stageMarkerSize)/2
)

var (
	mediaInk   = ui.RGBA(255, 255, 255, 0.94)
	mediaMuted = ui.RGBA(255, 255, 255, 0.62)
)

// View builds the playback screen: the video stage with its overlay controls and
// the now-playing side panel.
func (p *Page) View(c *ui.Context) ui.Element {
	p.setRedraw(c.Invalidate)
	t := c.Theme()
	m := locales.In(c.Locale())
	width, height := c.Size()

	root := ui.Box(c).Fill().Background(design.PlayerStage(t)).Clip()
	root.Children(func() {
		switch {
		case design.Docked(width):
			ui.Column(c).Fill().Children(func() {
				ui.Box(c).FillWidth().Height(height * 0.52).Children(func() { p.stage(c, t, m, width, height*0.52) })
				scroll := ui.Scroll(c).Grow(1)
				scroll.Children(func() { p.panelBody(c, t, m, width) })
			})
		case p.cinema:
			p.stage(c, t, m, width, height)
		case p.panelHidden:
			p.stage(c, t, m, width, height)
		default:
			ui.Row(c).Fill().Children(func() {
				ui.Box(c).Grow(1).FillHeight().Children(func() { p.stage(c, t, m, width-panelWidth, height) })
				p.panel(c, t, m, height)
			})
		}
	})
	// The settings the frame's controls just changed are written once the
	// frame is built, so a drag reaches the file once it settles.
	p.flushDanmakuSettings()
	return root
}

// stage is the picture plus the overlay chrome laid over it. The video is
// always mounted, whether its first frame has come or not: an engine delivers
// that frame on its own clock, and taking the element away until Ready would
// hide the only frame the player ever shows. Everything that covers the stage
// goes under the video, or nothing of it would show.
func (p *Page) stage(c *ui.Context, t *ui.Theme, m locales.Messages, width, height float32) {
	if p.loading && p.player == nil && p.notice == noticeNone {
		p.stageOpening(c, m)
	} else if p.player != nil {
		p.stageBackdrop(c, t)
		p.stageFallback(c, m)
		// The stage keeps the whole frame. Cover scales a picture by the larger of
		// the two ratios, which for a portrait series (short dramas ship 360x640)
		// crops it to a centre band of a wide stage: the picture reads as zoomed
		// in and cut off. Contain scales by the smaller ratio, so the frame fits
		// whole with a band on two sides; the stage's black and the backdrop's
		// cover fill those bands. A frame that already matches the stage's ratio
		// fills it either way.
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
				// A moving picture is asked for again on the screen's clock: the
				// comments are laid out from the playhead every frame, so the
				// interval is what a viewer sees as their pace.
				c.After(playTick)
			}
		}
	} else {
		// No stream yet: either the line is still being resolved, which says so
		// above, or its stream is opening. Opening is the engine's own work and
		// may take long, or never return, so it is waited on here, off the
		// stream's own clock, rather than on the page's thread. An open that
		// holds the stage longer than an opening may take is reported, so a
		// stream that never comes back is not a stage that waits for good.
		p.stageBackdrop(c, t)
		p.stageFallback(c, m)
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

// stageDanmaku lays the comments over the picture. A scrolled one crosses a lane
// from the right edge to the left at the pace of the playhead; a pinned one sits
// in the middle of its lane for a few seconds. The band, the text, the speed and
// which kinds fly are the viewer's settings, and the controls for them sit just
// under the pill at the top-right while they are unfolded.
func (p *Page) stageDanmaku(c *ui.Context, width float32) {
	_, height := c.Size()
	// The layer walks the playhead itself between the samples the player gives,
	// so a line crosses in even steps instead of the several per second the
	// player reports.
	at := p.layerPosition()
	flying := p.flyingComments(at, width, height)
	set := p.danmakuSettings
	top, bottom := design.DanmakuBand(set.Area)
	// The band is a share of the picture, with the stage's own top line and
	// control bar kept clear of it at either end.
	from := height*top + height*design.DanmakuBandTop*(1-top)
	to := height*bottom - height*design.DanmakuBandBottom*(1-bottom)
	if to < from+design.DanmakuLaneHeight {
		from, to = height*design.DanmakuBandTop, height-design.DanmakuBandBottom*height
	}
	space := rowsIn(from, to)
	// The settings box floats over the picture, so a line passing behind it is
	// faded down: the box reads as a panel over the picture instead of a window
	// the comments show through. Nothing is faded while the box is folded.
	boxEdge := float32(0)
	boxLeft := float32(0)
	if p.danmakuOpen {
		boxLeft = width - danmakuBoxMargin - danmakuBoxWidth(width)
		boxEdge = boxLeft - danmakuBoxGap
	}
	if len(flying) > 0 {
		// The playhead moves on its own, so the layer keeps asking for frames
		// while comments are in flight, at the pace the picture itself is drawn.
		c.After(playTick)
	}
	for _, item := range flying {
		item := item
		row := 0
		// Where the line is right now: the seat it was given when it entered, and
		// how far across it has come since.
		slide := item.slide
		if item.entry > 0 {
			slide = float32(at-item.entry) / float32(p.danmakuWindow())
		}
		switch item.kind {
		case kindScroll:
			// A scrolled line keeps its own row from the top of the band down. A
			// seat past the rows that fit has nowhere to go: squeezing it into
			// the last row would put two lines on one another.
			if item.lane >= space {
				continue
			}
			row = item.lane
			left := width - (width+240)*slide
			fade := clearOfBox(left, lanesWidth(item.text, set.Size), boxLeft, boxEdge, danmakuBoxFade)
			ui.Row(c).Absolute().Left(left).Top(from + float32(row)*design.DanmakuLaneHeight).
				Height(design.DanmakuLaneHeight).AlignItems(ui.Center).Children(func() {
				p.danmakuText(c, item.text, fade)
			})
			continue
		case kindTop:
			// Pinned-above lines hold the middle row of the band, and a band too
			// short to centre one in leaves them out rather than dropping them on
			// another line.
			row = (space - 1) / 2
		default:
			// Pinned-below lines count up from the bottom of the band.
			row = space - 1 - item.lane
		}
		if row < 0 || row >= space {
			continue
		}
		fade := float32(1)
		// A pinned comment holds the middle of its lane and stays for a fixed
		// time, so its row is one line wide and centred rather than measured.
		ui.Row(c).Absolute().Left(0).Right(0).Top(from + float32(row)*design.DanmakuLaneHeight).
			Height(design.DanmakuLaneHeight).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Row(c).Radius(999).Padding(3, 10).Background(design.DanmakuPinnedVeil()).Children(func() {
				p.danmakuText(c, item.text, fade)
			})
		})
	}
	// The settings stay open while the layer is off: they are the viewer's box,
	// and turning danmaku off to see the picture is not a reason to take it away.
	if p.danmakuOpen {
		p.danmakuControls(c, width)
	}
}

// danmakuText is one line of the layer: the viewer's size, weight and opacity,
// over a soft shadow so a faint line still reads on a bright frame.
func (p *Page) danmakuText(c *ui.Context, text string, fade float32) {
	set := p.danmakuSettings
	line := ui.Text(c, text).FontSize(set.Size).MaxLines(1).
		TextColor(design.DanmakuInk(set.Opacity*fade)).
		TextShadow(0, 1, 3, design.DanmakuShadow())
	if set.Bold {
		line.Bold()
	}
}

// danmakuControls is the settings box under the pill: the screen area as four
// presets, then opacity, size, speed and how many rows, then the kinds of
// comment that fly. It floats over the picture at the top-right, level with the
// pill's own right edge, so opening it never moves the button that opened it.
func (p *Page) danmakuControls(c *ui.Context, width float32) {
	m := locales.In(c.Locale())
	ui.Column(c).Absolute().Right(danmakuBoxMargin).Top(stageTopHeight+design.DanmakuLaneHeight+6).
		Width(danmakuBoxWidth(width)).Radius(18).Padding(16).Gap(14).Background(design.PlayerPanelGlassFill()).
		Border(1, design.PlayerPanelGlassBorder()).Children(func() {
		p.danmakuPresets(c, m)
		p.danmakuSlider(c, "opacity", m.DanmakuOpacity(),
			float64(p.danmakuSettings.Opacity)*100, 10, 100,
			func(v float64) { p.editDanmakuSettings(func(s *Settings) { s.Opacity = float32(v / 100) }) })
		p.danmakuSlider(c, "size", m.DanmakuSize(),
			float64(p.danmakuSettings.Size), 12, 30,
			func(v float64) { p.editDanmakuSettings(func(s *Settings) { s.Size = float32(v) }) })
		p.danmakuSlider(c, "speed", m.DanmakuSpeed(),
			speedAt(p.danmakuSettings.Speed), 0, 100,
			func(v float64) { p.editDanmakuSettings(func(s *Settings) { s.Speed = speedFor(v) }) })
		p.danmakuSlider(c, "lanes", m.DanmakuLanes(),
			float64(p.danmakuSettings.Lanes), 1, 14,
			func(v float64) { p.editDanmakuSettings(func(s *Settings) { s.Lanes = int(v + 0.5) }) })
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			p.danmakuToggle(c, "scroll", m.DanmakuBlockScroll(), p.danmakuSettings.Scroll,
				func(on bool) { p.editDanmakuSettings(func(s *Settings) { s.Scroll = on }) })
			p.danmakuToggle(c, "top", m.DanmakuBlockTop(), p.danmakuSettings.Top,
				func(on bool) { p.editDanmakuSettings(func(s *Settings) { s.Top = on }) })
		})
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			p.danmakuToggle(c, "bottom", m.DanmakuBlockBottom(), p.danmakuSettings.Bottom,
				func(on bool) { p.editDanmakuSettings(func(s *Settings) { s.Bottom = on }) })
			p.danmakuToggle(c, "bold", m.DanmakuBold(), p.danmakuSettings.Bold,
				func(on bool) { p.editDanmakuSettings(func(s *Settings) { s.Bold = on }) })
		})
	})
}

// danmakuPresets is the screen area as four presets in a row, each a miniature
// of the picture with the band the comments may use filled in, so the area is
// seen rather than read. The name sits under the miniature, on one line: four
// of them share the box's width, and a wrapped label would make one preset
// taller than its neighbours and push the whole box out of shape.
func (p *Page) danmakuPresets(c *ui.Context, m locales.Messages) {
	labels := []string{m.DanmakuAreaFull(), m.DanmakuAreaTop(), m.DanmakuAreaMiddle(), m.DanmakuAreaBottom()}
	ui.Column(c).FillWidth().Gap(8).Children(func() {
		ui.Text(c, m.DanmakuArea()).FontSize(11.5).TextColor(mediaMuted)
		ui.Row(c).FillWidth().Gap(6).Children(func() {
			for i, label := range labels {
				index := i
				on := index == p.danmakuSettings.Area
				button := ui.ButtonBase(c).Key("dm-area:" + strconv.Itoa(i)).Grow(1).Height(50).Radius(9).
					Label(label).Tooltip(label).Background(ui.RGBA(255, 255, 255, 0.06))
				if on {
					button.Background(ui.RGBA(255, 255, 255, 0.18))
				} else if button.Hovered() {
					button.Background(ui.RGBA(255, 255, 255, 0.12))
				}
				button.Children(func() {
					// The miniature is the picture's own shape, 30 x 17, with
					// the band filled in and a hairline at either end of it: the
					// preset reads at a glance, and the label under it stays one
					// line wide.
					const frameW, frameH float32 = 30, 17
					top, bottom := design.DanmakuBand(index)
					ink := ui.RGBA(255, 255, 255, 0.70)
					if on {
						ink = ui.RGBA(255, 255, 255, 1)
					}
					ui.Column(c).Fill().AlignItems(ui.Center).Justify(ui.Center).Gap(5).Children(func() {
						// The miniature is the frame's own shape with the band laid
						// in it: the padding above and below stands for the empty
						// picture, and the band fills what is left between them.
						// Padding, not absolute placement, so the four presets are
						// plainly four different spans of the same frame.
						ui.Box(c).Width(frameW).Height(frameH).Radius(3).
							Padding(frameH*top, 0, frameH*(1-bottom), 0).
							Background(ui.RGBA(0, 0, 0, 0.45)).Children(func() {
							ui.Box(c).Fill().Background(ink)
						})
						ui.Text(c, label).FontSize(10).TextColor(mediaInk).NoWrap()
					})
				})
				if button.Clicked() {
					p.editDanmakuSettings(func(s *Settings) { s.Area = index })
				}
			}
		})
	})
}

// danmakuSlider is one setting from lo to hi with its current value beside its
// name. The value the slider reports goes back into the layer at once, so a
// drag is seen on the picture as it happens, and the settings are written once
// the frame is built rather than at every step.
func (p *Page) danmakuSlider(c *ui.Context, key, label string, value, lo, hi float64, apply func(float64)) {
	shown := strconv.Itoa(int(value))
	if key == "opacity" {
		shown = strconv.Itoa(int(value)) + "%"
	}
	ui.Column(c).FillWidth().Gap(6).Children(func() {
		ui.Row(c).FillWidth().AlignItems(ui.Center).Children(func() {
			ui.Text(c, label).FontSize(11.5).TextColor(mediaMuted).Grow(1)
			ui.Text(c, shown).FontSize(11).Bold().TextColor(mediaInk)
		})
		// A plain slider, not a stepped one: the sites people know by heart
		// sweep opacity and size smoothly rather than snapping to marks. Its
		// state is its own, so the key goes on the context before it is built
		// rather than on the element afterwards.
		slider := ui.Slider(c.Key("dm-"+key), &value, lo, hi).Height(18)
		slider.Label(label)
		slider.Draw(func(painter *ui.Painter, r ui.Rect) {
			const knob float32 = 12
			frac := float32(0)
			if hi > lo {
				frac = float32((value - lo) / (hi - lo))
			}
			x := r.X + knob/2 + (r.W-knob)*frac
			middle := r.Y + r.H/2
			painter.Fill(ui.Rect{X: r.X + knob/2, Y: middle - 2, W: r.W - knob, H: 4}, design.PlayerTrack(), 2)
			painter.Fill(ui.Rect{X: r.X + knob/2, Y: middle - 2, W: (r.W - knob) * frac, H: 4}, mediaInk, 2)
			painter.Fill(ui.Rect{X: x - knob/2, Y: middle - knob/2, W: knob, H: knob}, mediaInk, knob/2)
		})
		if slider.Changed() {
			apply(value)
		}
	})
}

// danmakuToggle is one kind of comment's switch, as a chip that stays lit while
// that kind flies.
func (p *Page) danmakuToggle(c *ui.Context, key, label string, on bool, set func(bool)) {
	chip := ui.ButtonBase(c).Key("dm-kind:" + key).Grow(1).Height(28).Radius(8).Label(label).Tooltip(label).
		Background(ui.RGBA(255, 255, 255, 0.06))
	ink := mediaMuted
	if on {
		chip.Background(ui.RGBA(255, 255, 255, 0.20))
		ink = mediaInk
	} else if chip.Hovered() {
		chip.Background(ui.RGBA(255, 255, 255, 0.12))
		ink = mediaInk
	}
	chip.Children(func() { ui.Text(c, label).FontSize(11).TextColor(ink).MaxLines(1) })
	if chip.Clicked() {
		set(!on)
	}
}

// speedAt and speedFor are the two ends of the speed slider: the slider runs
// from slow to fast, while a comment's crossing time runs from long to short.
func speedAt(seconds float64) float64 {
	return (danmakuSlowest - seconds) / (danmakuSlowest - danmakuFastest) * 100
}

func speedFor(value float64) float64 {
	return danmakuSlowest - clampF(value, 0, 100)/100*(danmakuSlowest-danmakuFastest)
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

// stageBackdrop lays the stage the picture sits on: the near-black letterbox,
// in the stage's own colour, and nothing over it.
//
// The room round a picture is black because the picture is the only thing on the
// stage that carries information, as every desktop player leaves it. A cover
// blurred into the bands was tried twice and read wrong both times: at 45% it
// was a second, competing picture and a third of the stage wide on each side of
// a portrait drama, and at 10% it was a flat grey wash that made a pale frame
// look like a badly lit one. The bands are the stage's own colour now, so a
// picture that does not match the window's shape reads as a picture on black.
//
// One colour, not a gradient: the gradient this drew ran from the stage to the
// theme's surface, which is #FFFBFE in the light theme, so the letterbox climbed
// from near-black on one side to almost white on the other — measured in the
// running app at luma 52 at the left edge and 189 at the right beside a portrait
// picture. A band is the stage's colour or it is not a band.
func (p *Page) stageBackdrop(c *ui.Context, t *ui.Theme) {
	ui.Box(c).Absolute().Left(0).Right(0).Top(0).Bottom(0).
		Background(design.PlayerStage(t))
}

// pictureBox is the box a picture of the given ratio fills inside a stage of
// the given size once scaled to fit whole, centred, as Contain scales it. A
// stage with no picture yet, or with one whose ratio is not known, returns an
// empty box.
func (p *Page) pictureBox(width, height float32) ui.Rect {
	if p.player == nil || width <= 0 || height <= 0 {
		return ui.Rect{}
	}
	w, h := pictureRatio(p.player)
	if w <= 0 || h <= 0 {
		return ui.Rect{}
	}
	picture := w / h
	fitW, fitH := width, height
	if width/height > picture {
		fitW = height * picture
	} else {
		fitH = width / picture
	}
	return ui.Rect{X: (width - fitW) / 2, Y: (height - fitH) / 2, W: fitW, H: fitH}
}

// pictureRatio is the size a video's frames have, or the size the stream says
// they will have while the first frame is still on its way: an engine reports
// 0x0 until then, and the box the picture fills is what the backdrop needs from
// the very first frame.
func pictureRatio(player *ui.VideoPlayer) (w, h float32) {
	if player == nil {
		return 0, 0
	}
	if w, h := player.Size(); w > 0 && h > 0 {
		return float32(w), float32(h)
	}
	return 0, 0
}

// stageBand is one of the four bands round a picture on the stage, the picture
// itself being the box between them: dark when the picture's ratio and the
// stage's differ, none of them when the two match.
type stageBand struct {
	ui.Rect
}

// stageBands returns the bands round the picture within the stage, in the
// order they are laid: the top and bottom bands span the stage, the left and
// right bands fill the rows between them.
func stageBands(picture, stage ui.Rect) []stageBand {
	if picture.W <= 0 || picture.H <= 0 || stage.W <= 0 || stage.H <= 0 {
		return nil
	}
	var bands []stageBand
	top, bottom := picture.Y, stage.H-(picture.Y+picture.H)
	left, right := picture.X, stage.W-(picture.X+picture.W)
	if top > 0 {
		bands = append(bands, stageBand{ui.Rect{Y: 0, W: stage.W, H: top}})
	}
	if bottom > 0 {
		bands = append(bands, stageBand{ui.Rect{Y: picture.Y + picture.H, W: stage.W, H: bottom}})
	}
	if left > 0 {
		bands = append(bands, stageBand{ui.Rect{Y: picture.Y, W: left, H: picture.H}})
	}
	if right > 0 {
		bands = append(bands, stageBand{ui.Rect{X: picture.X + picture.W, Y: picture.Y, W: right, H: picture.H}})
	}
	return bands
}

// stageFallback covers a stage the stream did not fill: it says why, and it
// offers the way out that does not depend on the panel being open.
func (p *Page) stageFallback(c *ui.Context, m locales.Messages) {
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

// stageTop draws the way out and the episode marker at the top-left, and the
// floating control cluster (speaker / danmaku / panel buttons) at the top-right.
// The marker and the cluster sit on one line: the cluster's buttons hang at the
// middle of the marker, so they read level with the label of the episode they
// belong to. The way out is the first thing on that line, above the marker in
// the top-left corner a window's own close button would occupy: a stage that is
// still opening draws nothing but this, so leaving never depends on the
// controls below having been laid yet.
func (p *Page) stageTop(c *ui.Context, t *ui.Theme, m locales.Messages) {
	p.stageLeave(c, m)
	if episode, ok := p.current(); ok {
		ui.Text(c, p.episodeLabel(m, episode)).Absolute().Left(stageMarkerLeft).Top(stageMarkerTop).
			FontSize(13).Bold().TextColor(mediaInk)
	}
	p.playerPill(c, m, stageLineTop)
}

// stageLeave is the stage's way out, laid at the top-left where a viewer looks
// first for one. It is a capsule of its own rather than a button in the pill at
// the right: the way out never moves when the panel folds or the cluster
// grows, and a player's leave is not one of the picture's controls.
//
// The arrow says what it does — the stage is a page, and leaving it steps back
// to the one under it — rather than borrowing the cross the side panel uses to
// put itself away. Two crosses a screen apart, one closing a panel and one
// closing the player, ask a viewer to read a tooltip to tell them apart.
func (p *Page) stageLeave(c *ui.Context, m locales.Messages) {
	button := ui.ButtonBase(c).Key("leave-corner").Absolute()
	button.Left(stageEdge).Top(stageLineTop).Height(stageTopBox).Width(stageTopBox).
		Radius(stageTopBox / 2).Label(m.ClosePlayer()).Tooltip(m.ClosePlayer()).
		Material(glass.Glass{})
	if button.Hovered() {
		button.Background(ui.RGBA(255, 255, 255, 0.12))
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("alt-arrow-left-linear")).Size(18, 18).TextColor(mediaInk)
	})
	if button.Clicked() {
		p.Back()
	}
}

// playerPill is the level / speaker / danmaku / panel-toggle cluster at the top-right.
// It is one capsule: the panel toggle turns into the reveal control when the
// panel goes away, so the cluster never breaks into two floating pieces, and
// the level is the thin white track beside the speaker that the system's own
// player shows, never a slider that folds out and back.
func (p *Page) playerPill(c *ui.Context, m locales.Messages, top float32) {
	ui.Row(c).Absolute().Right(stageEdge).Top(top).AlignItems(ui.Center).Gap(2).Radius(999).
		Padding(pillPadding).Material(glass.Glass{}).Children(func() {
		p.volumeTrack(c, m)
		p.speaker(c, m)
		// One press turns the comments on and off, and a second, separate press
		// folds and unfolds the settings: a viewer who wants a clean picture
		// turns the layer off without losing the box they were adjusting, and
		// one who wants it back does not get a box dropped over the picture
		// again.
		p.danmakuButton(c, m)
		p.danmakuSettingsButton(c, m)
		if p.panelHidden {
			p.panelReveal(c, m)
			return
		}
		p.pillButton(c, "panel", "widget-2-linear", m.CollapsePanel(), false, p.CollapsePanel)
	})
}

// danmakuButton is the layer's switch: a press turns the comments on the
// picture on and off, and nothing else. The settings are their own button, so
// turning the comments off keeps the box exactly where the viewer left it.
func (p *Page) danmakuButton(c *ui.Context, m locales.Messages) {
	// The button keeps its own name whatever state it is in: the label a reader
	// hears is the control ("Danmaku"), while the tooltip says what the next
	// press will do.
	button := ui.ButtonBase(c).Key("p-danmaku").Size(stageButtonSize, stageButtonSize).Radius(999).
		Label(m.DanmakuLabel()).Tooltip(m.DanmakuLabel()).Background(ui.Transparent)
	ink := mediaMuted
	switch {
	case p.danmaku:
		// On is a filled pill, the same figure the speaker and the panel
		// toggle take when they are on.
		button.Background(mediaInk)
		ink = ui.RGBA(20, 20, 22, 1)
	case button.Hovered():
		button.Background(ui.RGBA(255, 255, 255, 0.12))
		ink = mediaInk
	}
	button.Children(func() { ui.Icon(c, icons.Parse("chat-round-linear")).Size(18, 18).TextColor(ink) })
	if button.Clicked() {
		p.ToggleDanmaku()
	}
}

// danmakuSettingsButton folds and unfolds the layer's settings. It sits beside
// the switch whether the layer is on or off: the box is where the kinds of
// comment are chosen, and turning one back on starts from the box.
func (p *Page) danmakuSettingsButton(c *ui.Context, m locales.Messages) {
	tip := m.DanmakuPanelUnfold()
	if p.danmakuOpen {
		tip = m.DanmakuPanelFold()
	}
	button := ui.ButtonBase(c).Key("p-danmaku-settings").Size(stageButtonSize, stageButtonSize).Radius(999).
		Label(m.DanmakuSettings()).Tooltip(tip).Background(ui.Transparent)
	ink := mediaMuted
	switch {
	case p.danmakuOpen:
		// Unfolded is a filled pill, the way the panel toggle reads open.
		button.Background(mediaInk)
		ink = ui.RGBA(20, 20, 22, 1)
	case button.Hovered():
		button.Background(ui.RGBA(255, 255, 255, 0.12))
		ink = mediaInk
	}
	button.Children(func() {
		ui.Icon(c, icons.Parse("settings-minimalistic-linear")).Size(18, 18).TextColor(ink)
	})
	if button.Clicked() {
		p.SetDanmakuOpen(!p.danmakuOpen)
	}
}

// volumeTrack is the level itself, a track with its filled part and knob, as
// the system's player lays it beside the speaker. Pressing it sets the level
// where the pointer lands and dragging along it keeps setting it.
func (p *Page) volumeTrack(c *ui.Context, m locales.Messages) {
	frac := float32(clampF(p.volume, 0, 1))
	track := ui.ButtonBase(c).Key("p-volume").Width(volumeTrackWidth + 2*volumeInset).Height(stageButtonSize).
		Label(m.VolumeLabel()).Tooltip(m.VolumeLabel()).Background(ui.Transparent).Cursor(ui.CursorResizeEW)
	// The track's own box is inset at both ends inside the button, so the knob
	// at rest and at full stays clear of the capsule's rounded edge.
	length := volumeTrackWidth - volumeKnob
	track.Children(func() {
		const barHeight, knob float32 = 4, 12
		middle := (stageButtonSize - barHeight) / 2
		ui.Box(c).Absolute().Left(volumeInset).Top(middle).Height(barHeight).Radius(barHeight / 2).
			Width(volumeTrackWidth).Background(ui.RGBA(255, 255, 255, 0.28))
		ui.Box(c).Absolute().Left(volumeInset).Top(middle).Height(barHeight).Radius(barHeight / 2).
			Background(mediaInk).Width(length*frac + knob/2)
		ui.Box(c).Absolute().Left(volumeInset+length*frac).Top((stageButtonSize-knob)/2).
			Size(knob, knob).Radius(knob / 2).Background(mediaInk)
	})
	// Where the pointer is along the track is the level: a press sets it at
	// once and every move of the drag follows it, so the level never lags
	// behind the hand. A muted video comes back as soon as the level is raised.
	if x, _, over := track.PointerPosition(); over && track.Pressed() {
		p.SetVolume(clampF(float64(x-volumeInset)/float64(length), 0, 1))
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
// stays inside the track at either end. volumeInset is the room the track keeps
// at either end: the pill's padding alone would leave the knob touching the
// capsule's edge, so the level starts a few units in and ends before the
// speaker's own button begins.
const (
	volumeTrackWidth float32 = 110
	volumeKnob       float32 = 12
	volumeInset      float32 = 8
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
	// An active switch fills its pill: a translucent wash over a bright
	// picture is indistinguishable from a button that does nothing, which is
	// how a viewer reads an enabled danmaku layer as a dead one.
	ink := mediaMuted
	switch {
	case active:
		button.Background(mediaInk)
		ink = ui.RGBA(20, 20, 22, 1)
	case button.Hovered():
		button.Background(ui.RGBA(255, 255, 255, 0.12))
		ink = mediaInk
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
		case tabDanmaku:
			p.panelDanmaku(c, t, m)
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
		// The cross here puts the panel away, not the player: it is the
		// panel's own close button, so a viewer who is done with the episode
		// list keeps watching. The stage's own control in the pill does the
		// same, and the stage's fallback keeps the way out of the player.
		close := ui.ButtonBase(c).Key("p-close").Size(30, 30).Radius(15).Label(m.CollapsePanel()).
			Tooltip(m.CollapsePanel()).Background(ui.Transparent)
		if close.Hovered() {
			close.Background(t.SurfaceHover)
		}
		close.Children(func() { ui.Icon(c, icons.Parse("close-circle-linear")).Size(20, 20).TextColor(t.TextMuted) })
		if close.Clicked() {
			p.CollapsePanel()
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
	labels := []string{m.EpisodesTab(), m.DanmakuTab()}
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

// panelDanmaku lists the danmaku the episode carries. The same entries fly over
// the picture, so this tab is the readable copy of that layer.
func (p *Page) panelDanmaku(c *ui.Context, t *ui.Theme, m locales.Messages) {
	if len(p.comments) == 0 {
		ui.Text(c, p.commentWhyText(m)).FontSize(13).TextColor(t.TextMuted)
		return
	}
	ui.Column(c).FillWidth().Gap(10).Children(func() {
		for _, comment := range p.comments {
			ui.Column(c).FillWidth().Gap(3).Children(func() {
				if comment.Author != "" {
					ui.Text(c, comment.Author).FontSize(11.5).Bold().TextColor(t.TextMuted).MaxLines(1)
				}
				ui.Text(c, comment.Text).FontSize(12.5).LineHeight(1.5).TextColor(t.Text)
			})
		}
	})
}

// commentWhyText says, in the viewer's language, why the layer over the
// picture carries what it carries. An empty list with no reason reads like a
// broken player, which is exactly what a viewer cannot tell apart from a source
// that simply has no comments.
func (p *Page) commentWhyText(m locales.Messages) string {
	switch p.commentWhy {
	case commentNoMatch:
		return m.DanmakuNoMatch()
	case commentNoEpisode:
		return m.DanmakuNoEpisode()
	case commentEmpty:
		return m.DanmakuEmpty()
	case commentFailed:
		return m.DanmakuFailed()
	case commentUnsupported:
		return m.DanmakuUnsupported()
	case commentReady:
		return m.EmptyTab()
	default:
		return m.DanmakuLoading()
	}
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

// Geometry of the settings box, named once so the box itself and the layer that
// fades behind it always agree on where it is.
const (
	danmakuBoxWidthMax float32 = 268
	danmakuBoxMargin   float32 = 24
	danmakuBoxGap      float32 = 12
	// danmakuBoxFade is how much of the crossing a line spends fading down as it
	// nears the box: a short share, so a line is either plainly readable or
	// plainly behind the box, not half of both.
	danmakuBoxFade float32 = 0.12
)

// danmakuBoxWidth is how wide the settings box is at this stage width: the
// picture's own width less a margin either side when the picture is too narrow
// to hang it in.
func danmakuBoxWidth(width float32) float32 {
	if width < danmakuBoxWidthMax+96 {
		return width - 48
	}
	return danmakuBoxWidthMax
}

// clearOfBox is how much of a line is left showing when its span of the picture
// runs into the settings box: 1 while it is clear of the box, 0 once it is
// inside it, and a slide between the two as it goes under. A pinned line that
// stays under the box ends up invisible rather than hidden, which is the same
// thing to the viewer and costs no separate rule.
func clearOfBox(left, textWidth, boxLeft, boxEdge, fade float32) float32 {
	if boxEdge <= 0 {
		return 1
	}
	right := left + textWidth
	switch {
	case right <= boxEdge:
		return 1
	case left >= boxLeft:
		return 0
	}
	// The line is partway in: measure how far its right edge has crossed the gap
	// under the box.
	if fade <= 0 || boxLeft <= boxEdge {
		return 0
	}
	inside := (right - boxEdge) / (boxLeft - boxEdge)
	if inside <= 0 {
		return 1
	}
	if inside >= 1 {
		return 0
	}
	return 1 - inside
}

// lanesWidth is how wide a line of the layer is, measured the way the layer
// measures it when it seats lanes.
func lanesWidth(text string, size float32) float32 {
	return float32(textWidth(text, size))
}

// rowsIn is how many rows of the layer fit between from and to, the top and
// bottom of the band the viewer picked.
func rowsIn(from, to float32) int {
	fits := int((to - from) / design.DanmakuLaneHeight)
	if fits < 1 {
		fits = 1
	}
	return fits
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
