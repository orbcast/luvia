// Package player is the playback screen: a video stage resolved from a source
// rule, its overlay controls, and a now-playing side panel listing episodes and
// playback lines. It records progress to the local library and owns no video of
// its own beyond the system player it opens.
package player

import (
	"context"
	"errors"
	"net/url"
	"path"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"

	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/videocache"
)

const requestTimeout = 30 * time.Second

var errNoSource = errors.New("no source rule is configured")

// notice classifies the failure a toolbar or panel reports.
type notice int

const (
	noticeNone notice = iota
	noticeUnavailable
	noticeUnsupported
	noticeFailed
)

// Tab under the now-playing header.
const (
	tabEpisodes = iota
	tabComments
)

// Deps are the collaborators the player reads and calls. OpenVideo is injectable
// so tests can run without a system media framework; production uses video.OpenURL.
type Deps struct {
	Source    source.Source
	Library   *library.Store
	Images    *media.Loader
	Cache     *videocache.Cache
	Run       func(func())
	OnBack    func()
	OpenVideo func(string) (*ui.VideoPlayer, error)
}

// Page is the player state. Everything here is touched on the main thread.
type Page struct {
	deps Deps

	detail      source.Detail
	episodes    []source.Episode
	index       int
	lineID      string
	appliedLine string

	player   *ui.VideoPlayer
	playback source.Playback

	loading bool
	err     error
	notice  notice
	started bool
	gen     uint64

	playing      bool
	muted        bool
	volume       float64
	restore      float64   // level to return to when unmuting
	volumeOpen   bool      // the speaker has its level track out
	volumeUntil  time.Time // when the open track folds back on its own
	rate         float64
	showSubtitle bool
	danmaku      bool
	panelHidden  bool
	cinema       bool
	expanded     bool
	tab          int
	scrub        float64 // 0..1 while the viewer drags the track
	scrubbing    bool
	scrubHandle  ui.Handle
	scrubW       float32
	redraw       func()
}

var rates = []float64{1, 1.25, 1.5, 2, 0.75, 0.5}

// defaultVolume is the level a new player keeps in reserve: playback starts
// muted, and unmuting restores this level.
const defaultVolume = 0.7

// volumeLinger is how long the level track stays out after the last time the
// pointer touched the speaker or the track, before it folds back on its own.
const volumeLinger = 4 * time.Second

// New prepares the player. Call Open with a title and episode to start it.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	if deps.OpenVideo == nil {
		deps.OpenVideo = video.OpenURL
	}
	p := &Page{deps: deps, index: -1, muted: true, volume: defaultVolume, restore: defaultVolume, rate: 1, playing: true, danmaku: true}
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	return p
}

// Open resets the player to the chosen episode and resolves it.
func (p *Page) Open(detail source.Detail, episodes []source.Episode, index int) {
	p.releasePlayer()
	p.detail, p.episodes, p.index = detail, episodes, index
	p.lineID, p.playback = "", source.Playback{}
	p.loading, p.err, p.notice, p.started = false, nil, noticeNone, false
	p.playing, p.muted, p.showSubtitle, p.danmaku, p.panelHidden, p.cinema = true, true, false, true, true, false
	p.expanded, p.tab, p.scrub, p.scrubbing = false, tabEpisodes, 0, false
	p.Start()
}

// Start resolves the current episode once.
func (p *Page) Start() {
	if p.started || len(p.episodes) == 0 {
		return
	}
	p.started = true
	p.resolve()
}

func (p *Page) current() (source.Episode, bool) {
	if p.index < 0 || p.index >= len(p.episodes) {
		return source.Episode{}, false
	}
	return p.episodes[p.index], true
}

func (p *Page) resolve() {
	if p.deps.Source == nil {
		p.err, p.loading = errNoSource, false
		p.invalidate()
		return
	}
	episode, ok := p.current()
	if !ok {
		return
	}
	p.gen++
	gen := p.gen
	src := p.deps.Source
	line := p.lineID
	p.loading, p.err, p.notice = true, nil, noticeNone
	p.invalidate()
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		playback, err := src.Resolve(ctx, episode, line)
		if err == nil && !playableURL(playback.URL) {
			// Some lines answer with a poster image or an empty address while
			// claiming mp4; playableURL catches that, and the first other line
			// that answers with media takes over so the viewer sees a picture
			// instead of buffering forever.
			playback, err = firstPlayable(ctx, src, episode, playback)
		}
		p.deps.Run(func() {
			if gen != p.gen {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err
				p.releasePlayer()
			} else {
				p.playback = playback
				p.appliedLine = playback.Line.ID
				if p.lineID == "" {
					p.lineID = playback.Line.ID
				}
				p.open(playback.URL)
				p.record(episode, playback)
			}
			p.invalidate()
		})
	}()
}

// playableURL reports whether a resolved address is a media stream the player
// can open: an HLS playlist, a progressive file, or a plainly ranged endpoint.
// A poster image, a blank address and a web page all fail here.
func playableURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	full := strings.ToLower(parsed.Path)
	switch {
	case strings.HasSuffix(full, ".m3u8"), strings.Contains(strings.ToLower(parsed.RawQuery), "m3u8"):
		return true
	case strings.HasSuffix(full, ".mp4"), strings.HasSuffix(full, ".m4v"), strings.HasSuffix(full, ".ts"):
		return true
	}
	switch path.Ext(full) {
	case ".jpg", ".jpeg", ".png", ".webp", ".gif", ".svg", ".avif":
		return false
	}
	// Signed CDN endpoints carry no extension; a picture CDN does, and its
	// host names the image pipeline.
	host := strings.ToLower(parsed.Host)
	if strings.Contains(host, "image") || strings.Contains(host, "img") || strings.Contains(host, "pic") {
		return false
	}
	return true
}

// firstPlayable walks the lines the source reported and resolves the first one
// that answers with media, keeping the original playback when none does.
func firstPlayable(ctx context.Context, src source.Source, episode source.Episode, current source.Playback) (source.Playback, error) {
	for _, line := range current.Lines {
		if line.ID == "" || line.ID == current.Line.ID {
			continue
		}
		next, err := src.Resolve(ctx, episode, line.ID)
		if err != nil || !playableURL(next.URL) {
			continue
		}
		return next, nil
	}
	return current, nil
}

// open starts a system player for url, or records why it could not.
func (p *Page) open(url string) {
	p.releasePlayer()
	if url == "" {
		p.notice = noticeUnavailable
		return
	}
	if p.deps.Cache != nil {
		url = p.deps.Cache.ProxyURL(url)
	}
	system, err := p.deps.OpenVideo(url)
	if err != nil {
		p.notice = noticeUnsupported
		return
	}
	p.player = system
	system.SetVolume(p.volume)
	system.SetRate(p.rate)
	system.SetMuted(p.muted)
	if p.playing {
		system.Play()
	} else {
		system.Pause()
	}
}

func (p *Page) releasePlayer() {
	if p.player != nil {
		p.player.Close()
		p.player = nil
	}
}

// record keeps the viewer's place in the library for the continue row.
func (p *Page) record(episode source.Episode, playback source.Playback) {
	if p.deps.Library == nil {
		return
	}
	_ = p.deps.Library.RecordProgress(library.Progress{
		Media: p.detail.Ref, Title: p.detail.Title, PosterURL: p.detail.PosterURL,
		Episode: episode, UpdatedAt: time.Now().UTC(),
	})
	_ = playback
}

// TogglePlay pauses or resumes the video.
func (p *Page) TogglePlay() {
	p.playing = !p.playing
	if p.player != nil {
		if p.playing {
			p.player.Play()
		} else {
			p.player.Pause()
		}
	}
	p.invalidate()
}

// SeekFraction jumps to a fraction of the video.
func (p *Page) SeekFraction(f float64) {
	if p.player == nil {
		return
	}
	d := p.player.Duration()
	if d <= 0 {
		return
	}
	p.player.Seek(time.Duration(float64(d) * clampF(f, 0, 1)))
}

// SelectEpisode switches to episode i, resolving it.
func (p *Page) SelectEpisode(i int) {
	if i < 0 || i >= len(p.episodes) || i == p.index {
		return
	}
	p.index = i
	p.playback = source.Playback{}
	p.notice, p.err = noticeNone, nil
	p.playing = true
	p.started = false
	p.Start()
}

func (p *Page) Next()     { p.SelectEpisode(p.index + 1) }
func (p *Page) Previous() { p.SelectEpisode(p.index - 1) }

// SelectLine re-resolves the current episode on another playback line.
func (p *Page) SelectLine(id string) {
	if id == "" || id == p.lineID {
		return
	}
	p.lineID = id
	p.started = false
	p.Start()
}

// applyLine resolves the line the viewer just picked, once its bound value has
// changed. The view calls it after the line selector updates.
func (p *Page) applyLine() {
	if p.lineID == "" || p.lineID == p.appliedLine {
		return
	}
	p.appliedLine = p.lineID
	p.started = false
	p.Start()
}

// CycleRate steps playback speed through the offered rates.
func (p *Page) CycleRate() {
	next := rates[0]
	for i, r := range rates {
		if r == p.rate {
			next = rates[(i+1)%len(rates)]
			break
		}
	}
	p.rate = next
	if p.player != nil {
		p.player.SetRate(next)
	}
	p.invalidate()
}

// ToggleMute mutes or unmutes the video. Unmuting a slider left at zero brings
// the level back to the last audible one, so the speaker always restores sound.
func (p *Page) ToggleMute() {
	if p.muted {
		p.muted = false
		if p.volume <= 0 {
			level := p.restore
			if level <= 0 {
				level = defaultVolume
			}
			p.volume = clampF(level, 0, 1)
		}
	} else {
		p.muted = true
		if p.volume > 0 {
			p.restore = p.volume
		}
	}
	if p.player != nil {
		p.player.SetVolume(p.volume)
		p.player.SetMuted(p.muted)
	}
	p.invalidate()
}

// SetVolume applies the volume the viewer picks in the slider. The slider owns
// the level only: silence follows the level, and the speaker stays the one place
// that mutes and restores sound.
func (p *Page) SetVolume(v float64) {
	p.volume = clampF(v, 0, 1)
	if p.volume > 0 {
		p.muted = false
		p.restore = p.volume
	} else {
		p.muted = true
	}
	if p.player != nil {
		p.player.SetVolume(p.volume)
		p.player.SetMuted(p.muted)
	}
	p.invalidate()
}

// Audible reports whether sound is playing at all: the speaker dims both when it
// was silenced and when the slider sits at zero.
func (p *Page) Audible() bool { return !p.muted && p.volume > 0 }

// VolumeOpen reports whether the speaker has its level track out.
func (p *Page) VolumeOpen() bool { return p.volumeOpen }

// OpenVolume unfolds the level track beside the speaker.
func (p *Page) OpenVolume() {
	if p.volumeOpen {
		return
	}
	p.volumeOpen = true
	p.volumeUntil = time.Now().Add(volumeLinger)
	p.invalidate()
}

// CloseVolume folds the level track back beside the speaker.
func (p *Page) CloseVolume() {
	if !p.volumeOpen {
		return
	}
	p.volumeOpen, p.volumeUntil = false, time.Time{}
	p.invalidate()
}

// ToggleVolumeSpeaker is the press on the speaker itself: it unfolds the level
// track while it is away, and silences the video while it is out, folding the
// track in at once so a second press never leaves the track open by accident.
func (p *Page) ToggleVolumeSpeaker() {
	if !p.volumeOpen {
		p.OpenVolume()
		return
	}
	p.volumeOpen, p.volumeUntil = false, time.Time{}
	p.ToggleMute()
	p.invalidate()
}

// KeepVolumeOpen holds the level track out for a few idle seconds, and folds it
// back when they run out. The view calls it while the pointer rests on the
// speaker or the track.
func (p *Page) KeepVolumeOpen(now time.Time) {
	if !p.volumeOpen {
		return
	}
	deadline := now.Add(volumeLinger)
	if deadline.After(p.volumeUntil) {
		p.volumeUntil = deadline
	}
}

// FoldVolume closes the level track once its idle time has passed. The view
// calls it on the frames it asked for with After.
func (p *Page) FoldVolume(now time.Time) {
	if !p.volumeOpen || p.volumeUntil.IsZero() || now.Before(p.volumeUntil) {
		return
	}
	p.volumeOpen, p.volumeUntil = false, time.Time{}
	p.invalidate()
}

// ToggleDanmaku shows or hides the comment layer over the picture.
func (p *Page) ToggleDanmaku() {
	p.danmaku = !p.danmaku
	p.invalidate()
}

// SeekBy moves the playhead by delta, clamped inside the clip.
func (p *Page) SeekBy(delta time.Duration) {
	if p.player == nil {
		return
	}
	target := p.player.Position() + delta
	if target < 0 {
		target = 0
	}
	if d := p.player.Duration(); d > 0 && target > d {
		target = d
	}
	p.player.Seek(target)
	p.invalidate()
}

func (p *Page) ToggleSubtitle() { p.showSubtitle = !p.showSubtitle; p.invalidate() }

// TogglePanel hides or shows the now-playing side panel, letting the video fill
// the window and back. A player opens with the panel away, so the picture owns
// the screen until the viewer asks for the episode list.
func (p *Page) TogglePanel() {
	p.panelHidden = !p.panelHidden
	p.invalidate()
}

// CollapsePanel puts the side panel away; the picture keeps a reveal control.
func (p *Page) CollapsePanel() { p.setPanelHidden(true) }

// RevealPanel brings the side panel back beside the picture.
func (p *Page) RevealPanel() { p.setPanelHidden(false) }

func (p *Page) setPanelHidden(hidden bool) {
	if p.panelHidden == hidden {
		return
	}
	p.panelHidden = hidden
	p.invalidate()
}
func (p *Page) ToggleCinema()   { p.cinema = !p.cinema; p.invalidate() }
func (p *Page) ToggleExpanded() { p.expanded = !p.expanded; p.invalidate() }

func (p *Page) SelectTab(i int) {
	if i != tabEpisodes && i != tabComments {
		return
	}
	p.tab = i
	p.invalidate()
}

// Back leaves the player and returns what it opened.
func (p *Page) Back() {
	p.releasePlayer()
	if p.deps.OnBack != nil {
		p.deps.OnBack()
	}
}

// position and duration read the player safely.
func (p *Page) position() time.Duration {
	if p.player == nil {
		return 0
	}
	return p.player.Position()
}

func (p *Page) duration() time.Duration {
	if p.player == nil {
		return 0
	}
	return p.player.Duration()
}

func (p *Page) fraction() float64 {
	if p.scrubbing {
		return clampF(p.scrub, 0, 1)
	}
	d := p.duration()
	if d <= 0 {
		return 0
	}
	return clampF(float64(p.position())/float64(d), 0, 1)
}

func (p *Page) invalidate() {
	if p.redraw != nil {
		p.redraw()
	}
}

func clampF(v, low, high float64) float64 {
	if v < low {
		return low
	}
	if v > high {
		return high
	}
	return v
}
