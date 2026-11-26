// Package player is the playback screen: a video stage resolved from a source
// rule, its overlay controls, and a now-playing side panel listing episodes and
// playback lines. It records progress to the local library and owns no video of
// its own beyond the system player it opens.
package player

import (
	"context"
	"errors"
	"log"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/egoist/mygo/ui/video"

	"luvia/apps/desktop/internal/danmaku"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/videocache"
)

const requestTimeout = 30 * time.Second

// frameTimeout bounds how long an opened stream may hold the stage without a
// picture, and how long a picture that came may stand still before it is taken
// for the same stall. An engine that never delivers its first frame, or that
// stops delivering them, reports no error of its own, so the stage is the one
// that has to say so, rather than wait for good.
const frameTimeout = 10 * time.Second

var (
	errNoSource = errors.New("no source rule is configured")
	errNoFrame  = errors.New("the stream opened but gave no picture")
	errNoURL    = errors.New("the line gave no address")
)

// notice classifies the failure a toolbar or panel reports.
type notice int

const (
	noticeNone notice = iota
	noticeUnavailable
	noticeUnsupported
	noticeFailed
	noticeNoFrame
)

// commentWhy says why the layer over the picture carries what it carries.
type commentWhy int

const (
	// commentLoading is the state before the first answer arrives.
	commentLoading commentWhy = iota
	// commentReady means the layer has comments to draw.
	commentReady
	// commentNoMatch means nothing outside looked like this title.
	commentNoMatch
	// commentNoEpisode means the title was found but not this episode.
	commentNoEpisode
	// commentEmpty means the episode carries no comments of its own.
	commentEmpty
	// commentFailed means the lookup could not be completed.
	commentFailed
	// commentUnsupported means neither the rule nor a lookup offers comments.
	commentUnsupported
)

// Tab under the now-playing header.
const (
	tabEpisodes = iota
	// tabDanmaku is the readable copy of the layer that flies over the
	// picture: the same entries, listed, for a viewer who would rather read
	// them. The tab says danmaku because that is what the list holds, a
	// danmaku feed, not the source's review section.
	tabDanmaku
)

// Deps are the collaborators the player reads and calls. OpenVideo is injectable
// so tests can run without a system media framework; production uses video.OpenURL.
type Deps struct {
	Source source.Source
	// Danmaku finds comments for a title on a site that pins them to a moment.
	// A source rule's own comments are usually reviews with no moment at all,
	// which cannot cross the picture; this is what feeds that layer when it is
	// set. A nil lookup leaves the layer to the rule's comments alone.
	Danmaku   danmaku.Source
	Library   *library.Store
	Images    *media.Loader
	Cache     *videocache.Cache
	Run       func(func())
	OnBack    func()
	OpenVideo func(string) (*ui.VideoPlayer, error)
	// SetFullScreen takes the window in and out of full screen. The picture
	// fills the window either way; the player's full-screen button hands the
	// screen back by asking the window for it, not by hiding its own chrome.
	SetFullScreen func(bool)
	// SettingsPath is where the comment layer's settings are kept: every change
	// is written there, and the player opens with what was written. An empty
	// path keeps them for the session only.
	SettingsPath string
}

// Page is the player state. Everything here is touched on the main thread.
type Page struct {
	deps Deps

	detail      source.Detail
	episodes    []source.Episode
	index       int
	lineID      string
	appliedLine string
	// passed are the lines this episode opened without a picture, which the next
	// resolve skips, so the viewer is not shown the same failure twice.
	passed []string
	// lastFailure is what the engine said about the line it failed on, which the
	// stage reports when no other line answers the episode.
	lastFailure error

	player   *ui.VideoPlayer
	playback source.Playback

	// opened and buffered time the two waits a viewer sits through: the line
	// being resolved, and the engine holding a stream it has opened but not yet
	// drawn. The stage reports how long they last and the cache how fast the
	// bytes are coming. A wait after the picture came is timed from stallAt.
	opened   time.Time
	buffered time.Time

	// stallAt is when the playhead of a stream that already drew was last seen
	// to move, and stallPos where it stood then. They are zero until the stage
	// sees that stream at all, and a stream that stands still past them is the
	// same stall as one that never drew: the engine will not say so by itself,
	// so the stage is what says it.
	stallAt  time.Time
	stallPos time.Duration

	// openSeq is the open the page last asked for, on its own thread: a stream
	// that opened for an earlier one is closed rather than taken up.
	openSeq uint64

	loading bool
	err     error
	notice  notice
	started bool
	gen     uint64

	playing      bool
	muted        bool
	volume       float64
	restore      float64 // level to return to when unmuting
	rate         float64
	showSubtitle bool
	danmaku      bool
	// danmakuSettings is the comment layer's own setup, which outlives the
	// player, and danmakuOpen keeps its box unfolded while it is adjusted.
	danmakuSettings Settings
	// danmakuSet is true once the settings are a real choice, whether loaded
	// from the file or made in the controls. A page built for a test, or a
	// player whose file could not be read, starts with the defaults instead of
	// a zero value that would draw no lanes at all.
	danmakuSet  bool
	danmakuOpen bool
	// danmakuDirty is set by the controls while the viewer is changing a
	// setting, and cleared when the page has written it out, so a dragged
	// slider writes the file once it settles instead of at every step.
	danmakuDirty bool
	comments     []source.Danmaku
	commentGen   uint64
	// layer is where the layer over the picture believes the playhead is, which
	// is between the samples the player gives, so a comment glides on every
	// frame instead of stepping on each one. It is set aside while the picture
	// is paused or the layer off, because neither is a straight run of time.
	layer layerClock
	// commentWhy says why the layer over the picture is empty, so the panel
	// can name the reason instead of showing an empty list.
	commentWhy  commentWhy
	panelHidden bool
	cinema      bool
	expanded    bool
	tab         int
	scrub       float64 // 0..1 while the viewer drags the track
	scrubbing   bool
	scrubHandle ui.Handle
	scrubW      float32
	redrawMu    sync.Mutex
	redraw      func()
	// now reads the monotonic clock the layer is timed by; a test puts its own
	// here so it can walk time frame by frame instead of sleeping.
	now func() time.Time
}

var rates = []float64{1, 1.25, 1.5, 2, 0.75, 0.5}

// defaultVolume is the level a new player keeps in reserve: playback starts
// muted, and unmuting restores this level.
const defaultVolume = 0.7

// New prepares the player. Call Open with a title and episode to start it.
func New(deps Deps) *Page {
	if deps.Run == nil {
		deps.Run = mygo.RunOnMain
	}
	if deps.OpenVideo == nil {
		deps.OpenVideo = video.OpenURL
	}
	// The comment layer opens off: a viewer who has not asked for comments
	// gets the picture alone, and the danmaku button is one press away.
	p := &Page{deps: deps, index: -1, muted: true, volume: defaultVolume, restore: defaultVolume, rate: 1, playing: true, now: time.Now}
	// The layer opens on the viewer's own settings when the file holds one,
	// and on the defaults otherwise, so a player nobody has set up yet draws
	// comments instead of nothing.
	p.danmakuSettings, _ = LoadDanmakuSettings(deps.SettingsPath)
	if deps.Images != nil {
		deps.Images.SetChanged(func() { p.invalidate() })
	}
	return p
}

// Open resets the player to the chosen episode and resolves it.
func (p *Page) Open(detail source.Detail, episodes []source.Episode, index int) {
	p.cancelOpen()
	p.releasePlayer()
	p.detail, p.episodes, p.index = detail, episodes, index
	p.lineID, p.playback = "", source.Playback{}
	p.passed, p.lastFailure = nil, nil
	p.loading, p.err, p.notice, p.started = false, nil, noticeNone, false
	p.playing, p.muted, p.showSubtitle, p.danmaku, p.panelHidden, p.cinema = true, true, false, false, true, false
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
	// The lines this resolve abandons are passed over by the one that takes
	// over, for the rest of the episode: they already gave no picture.
	passed := p.passed
	known := p.playback
	p.loading, p.err, p.notice = true, nil, noticeNone
	p.opened, p.buffered = time.Now(), time.Time{}
	p.invalidate()
	p.cancelOpen()
	openSeq := p.openSeq
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		// The line the page asked for is resolved, unless the page is passing one
		// over: the lines it already knows are enough to pick the next.
		playback := known
		var err error
		if line != "" || len(passed) == 0 {
			playback, err = src.Resolve(ctx, episode, line)
			if source.IsKind(err, source.ErrNotFound) && line != "" {
				// A line id is bound to one episode's listing, so the line the
				// previous episode was watched on is gone here. Falls back to the
				// episode's own default line instead of failing the switch.
				playback, err = src.Resolve(ctx, episode, "")
				line = ""
			}
		}
		if err == nil && (len(passed) > 0 || !mediaOffers(ctx, playback.URL)) {
			// Some lines answer with a poster image or an empty address while
			// claiming mp4, and one may have just opened without a picture. Their
			// URLs often look plain, so the next line that answers with media
			// takes over, so the viewer sees a picture instead of buffering
			// forever.
			if passesOver(passed, playback.Line.ID) {
				playback = source.Playback{Lines: playback.Lines}
			}
			playback, err = firstPlayable(ctx, src, episode, playback, passed)
		}
		p.deps.Run(func() {
			if gen != p.gen {
				return
			}
			p.loading = false
			if err != nil {
				p.err = err
				p.releasePlayer()
				p.invalidate()
				return
			}
			if playback.URL == "" && len(passed) > 0 {
				// No other line answered: the lines that gave no picture are the
				// episode's last word, and the stage says what the engine said about
				// them, or that they drew none. The lines stay in the panel, so a
				// viewer can pick another one by hand.
				p.playback = playback
				p.reportFailure()
				return
			}
			p.playback = playback
			p.appliedLine = playback.Line.ID
			// The selector names the line the picture came from, whatever was
			// asked for: a source that answers on another line (its ids moved
			// on, or the line it was asked for now serves a neighbour) would
			// otherwise leave the panel pointing at a line that is not playing.
			p.lineID = playback.Line.ID
			p.record(episode, playback)
			p.loadComments(episode, gen)
			p.warmNext(episode, line, gen)
			// The stream opens off this thread: an engine that opened and never
			// drew would otherwise hold the page, and the wait it owes the viewer
			// would never be drawn at all. The address it opens is worked out
			// here, so the goroutine is handed a string instead of reaching into
			// the page for the cache and the opener.
			url := p.streamURL(playback.URL)
			open, run := p.deps.OpenVideo, p.deps.Run
			go func() {
				system, err := openStream(url, open)
				run(func() { p.takePlayer(system, err, openSeq) })
			}()
			p.invalidate()
		})
	}()
}

// warmNext warms the cache for the episode a viewer is most likely to open
// next, in the background, so switching to it starts on stored bytes instead of
// a cold fetch. The episode is resolved on the line just used, which is the line
// the viewer would pick for it; only its leading segments are fetched. A viewer
// who has already moved on is not warmed for: the generation they moved past
// means the episode they left is no longer what comes next.
func (p *Page) warmNext(current source.Episode, line string, gen uint64) {
	if p.deps.Cache == nil || p.deps.Source == nil {
		return
	}
	// The episode and the line are worked out here on the main thread: the
	// goroutine starts from values it was handed, and asks the page nothing.
	next, ok := p.adjacent(current, 1)
	if !ok {
		return
	}
	src, cache, run := p.deps.Source, p.deps.Cache, p.deps.Run
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		playback, err := src.Resolve(ctx, next, line)
		if err != nil {
			return
		}
		// The generation is read in the callback, never before it: gen is the
		// page's own counter and only the main thread reads it.
		run(func() {
			if gen != p.gen {
				return
			}
			cache.Prefetch(playback.URL)
		})
	}()
}

// adjacent returns the episode offset away from current, matched by identity
// rather than by the page's own index, because current may be the episode the
// player was told to open.
func (p *Page) adjacent(current source.Episode, offset int) (source.Episode, bool) {
	for i, episode := range p.episodes {
		if episode.ID != current.ID || episode.Media != current.Media {
			continue
		}
		if i+offset < 0 || i+offset >= len(p.episodes) {
			return source.Episode{}, false
		}
		return p.episodes[i+offset], true
	}
	return source.Episode{}, false
}

// loadComments fills the layer over the picture. Three things can feed it: the
// comments the source itself publishes on a timeline, its review list, and a
// lookup on a site that pins comments to a moment.
//
// The order is deliberate. A source that publishes its own dated comments is
// describing this very copy of the title, so it wins outright. A lookup from
// outside comes next, because a comment with a moment is something the stage
// can lay over the picture, where a rule's review list usually has no moment
// at all and can only be spread across the episode. The rule's own list is the
// last resort. Either way the panel is told why the layer is what it is, so an
// empty layer never looks like a broken one.
//
// It runs on its own context: the caller resolves the playback and drops its
// deadline as soon as it has an address, which would cancel a request that has
// not been sent yet.
func (p *Page) loadComments(episode source.Episode, gen uint64) {
	p.commentGen++
	epoch := p.commentGen
	p.comments, p.commentWhy = nil, commentLoading
	run := p.deps.Run
	title := p.detail.Title
	// The source's own answers are taken here rather than in the callback, so
	// the goroutine holds no page of the app: the page is main-thread state, and
	// a background thread that reads it is a race however soon it reads.
	src := p.deps.Source
	lookup := p.deps.Danmaku
	commenter, canComment := src.(source.Commenter)
	publisher, publishes := src.(source.Manifest)
	if lookup == nil && !canComment && !publishes {
		p.commentWhy = commentUnsupported
		return
	}
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), requestTimeout)
		defer cancel()
		items, why := p.commentsFor(ctx, lookup, commenter, canComment, publisher, publishes, title, episode)
		// Every read of the page, the generations included, happens in the
		// callback: another resolve may have bumped them while this one was out
		// fetching, and only the main thread is allowed to look.
		run(func() {
			if epoch != p.commentGen || gen != p.gen {
				return
			}
			p.comments, p.commentWhy = items, why
			p.invalidate()
		})
	}()
}

// commentsFor asks the source's own timeline first, the lookup second and the
// rule's review list last, and reports what came back. A lookup that matched
// the title but carries nothing for this episode is a different answer from one
// that matched nothing at all, and the panel says which. Neither the lookup nor
// the review list is the source of truth: the source's own timeline is.
func (p *Page) commentsFor(ctx context.Context, lookup danmaku.Source, commenter source.Commenter, canComment bool, publisher source.Manifest, publishes bool, title string, episode source.Episode) ([]source.Danmaku, commentWhy) {
	if items, ok := p.publishedComments(ctx, publisher, publishes, episode); ok {
		return items, commentReady
	}
	if lookup == nil {
		return p.ruleComments(ctx, commenter, canComment, episode)
	}
	// The lookup's own finding is what the panel reports when the rule has
	// nothing either: it is the layer's intended source, so its answer is the
	// one that explains the empty list.
	why := commentNoMatch
	got, err := lookup.Lookup(ctx, title, episode.Number)
	switch {
	case err != nil:
		why = commentFailed
	case got.Reason == danmaku.ReasonReady && len(got.Items) > 0:
		return got.Items, commentReady
	case got.Reason == danmaku.ReasonNoEpisode:
		why = commentNoEpisode
	case got.Reason == danmaku.ReasonEmpty:
		why = commentEmpty
	}
	if items, ruled := p.ruleComments(ctx, commenter, canComment, episode); ruled == commentReady {
		return items, commentReady
	}
	return nil, why
}

// publishedComments asks the source for the comments it publishes on its own
// timeline. The bool is false when it has nothing to say, so the caller falls
// through to the outside lookup rather than settling for an empty layer.
func (p *Page) publishedComments(ctx context.Context, publisher source.Manifest, publishes bool, episode source.Episode) ([]source.Danmaku, bool) {
	if !publishes {
		return nil, false
	}
	items, err := publisher.Manifest(ctx, episode, 0)
	if err != nil || len(items) == 0 {
		return nil, false
	}
	return items, true
}

// ruleComments asks the source rule for the comments it carries. A rule
// without a comments block answers with an unsupported error, which is a
// different finding from an episode that simply has none.
func (p *Page) ruleComments(ctx context.Context, commenter source.Commenter, canComment bool, episode source.Episode) ([]source.Danmaku, commentWhy) {
	if !canComment {
		return nil, commentUnsupported
	}
	items, err := commenter.Comments(ctx, episode, 0)
	if err != nil {
		var problem *source.Error
		if errors.As(err, &problem) && problem.Kind == source.ErrUnsupported {
			return nil, commentUnsupported
		}
		return nil, commentEmpty
	}
	if len(items) == 0 {
		return nil, commentEmpty
	}
	return items, commentReady
}

// flying is one comment crossing the picture: its text, the kind of crossing it
// makes, the lane it takes, the playhead moment it entered at and the share of
// the crossing it had made when the layer last placed it. The stage works the
// share out again from entry and the playhead it is drawing, so the line keeps
// to the pace of the clock even on a frame that skipped one. A pinned comment's
// share is how long it still has to stay up.
type flying struct {
	text  string
	kind  int
	lane  int
	entry time.Duration
	slide float32
}

// laneShare is how many rows of the band each kind of comment may use. A
// scrolling line is one row tall and needs a row of its own, while a pinned one
// hangs in the middle of the band under a pill, so the two never share rows:
// scrolling lines start at the top of the band and count down, pinned-below
// ones count up from the bottom, and pinned-above ones hold the middle.
func laneShare(set Settings, rows int) (scroll, top, bottom int) {
	if rows < 1 {
		rows = 1
	}
	// The middle row, when there is one to hold: a band of one row has no middle
	// left over for a pinned line.
	centre := 0
	if set.Top && rows >= 3 {
		centre = 1
	}
	rest := rows - centre
	if !set.Scroll {
		rest = 0
		bottom = 0
	}
	if set.Bottom {
		// Pinned-below lines take the rows under the scrolling block, up to two
		// of them: a pillar of pinned lines under a busy picture reads as a wall.
		bottom = min(2, max(1, rest/4))
	}
	if set.Scroll {
		scroll = rest - bottom
		if scroll < 1 {
			scroll = 1
			bottom = max(0, rest-1)
		}
	}
	if !set.Bottom {
		bottom = 0
	}
	return scroll, centre, bottom
}

// flyingComments picks the comments crossing the picture at the playhead: the
// ones scrolling across, and the ones pinned to the top or the bottom of the
// band. It returns the rows the layer may use as well, so the stage can tell a
// lane that is free from one a long comment still holds.
//
// A lane is owned by a comment until that comment has left the lane, not until
// a fixed number of comments has gone by: a long line takes longer to clear it
// than a short one, and two lines in one lane overlap when the next one enters
// on the same beat. A lane a comment has just left stays quiet for a moment so
// the next one enters with a gap.
//
// now is where the layer is walking, the playhead carried between reports rather
// than the reports themselves: see layerPosition.
func (p *Page) flyingComments(now time.Duration, width, height float32) []flying {
	if !p.danmaku || len(p.comments) == 0 {
		return nil
	}
	set := p.danmakuSettings
	spread := !timedComments(p.comments)
	window := p.danmakuWindow()
	lead := window / 2
	area := laneWidth(width)
	// The band decides how many rows there are, so the layer counts rows the same
	// way the stage does.
	rows := p.danmakuRows(height)
	if rows < 1 {
		return nil
	}
	scrollLanes, centre, bottomLanes := laneShare(set, rows)
	// free says when each scrolling lane is free again, and held says which line
	// holds it until then. A line takes its lane when it enters and keeps it for
	// the whole crossing: deciding again on every frame would let a line that
	// comes later in the list take the lane a longer one is still crossing, which
	// is what stacks three lines on one row.
	free := make(map[int]time.Duration)
	held := make(map[int]int)
	// shown says until when each pinned lane is taken, so a pinned line holds its
	// row for as long as it stays up rather than leaving room for the next one to
	// land on it.
	shown := make(map[int]time.Duration)
	var out []flying
	for i, comment := range p.comments {
		kind := commentKind(i)
		if !set.allows(kind) {
			continue
		}
		at := comment.At
		if spread {
			at = p.spreadMoment(i, len(p.comments))
		}
		from := at - lead
		if now < from || now-from > window {
			continue
		}
		slide := float32(now-from) / float32(window)
		// A lane is free once the line before it has moved far enough in that the
		// new one enters behind its tail rather than on top of it. The moment to
		// judge is the new line's own entry, not this frame: lines that start on
		// the same beat would otherwise all see the frame they arrive in and claim
		// the one lane that is free.
		occupy := clampF((textWidth(comment.Text, set.Size)+64)/float64(area), 0.12, 0.9)
		lane := 0
		if kind == kindScroll {
			seat, mine := held[i]
			if !mine {
				seat = -1
				for l := 0; l < scrollLanes; l++ {
					if until, ok := free[l]; !ok || from >= until {
						seat = l
						break
					}
				}
				if seat < 0 {
					// Every lane is taken: the layer drops the line rather than
					// stacking two in one row, which is what makes a busy episode
					// unreadable.
					continue
				}
				held[i] = seat
				free[seat] = from + time.Duration(occupy*float64(window))
			}
			lane = seat
		} else if kind == kindTop {
			// The middle row holds one pinned line at a time.
			if centre == 0 {
				continue
			}
			lane = pinLane(shown, kindTop, centre, from, now, window)
		} else {
			lane = pinLane(shown, kindBottom, bottomLanes, from, now, window)
		}
		if lane < 0 {
			continue
		}
		out = append(out, flying{text: comment.Text, kind: kind, lane: lane, entry: from, slide: slide})
	}
	// The lane number is a place inside the kind's own block, and the stage turns
	// it into a row: scrolling lines fill the band from the top down, pinned-above
	// lines centre in it and pinned-below ones count up from its bottom.
	return out
}

// pinLane seats a pinned comment in one of its kind's rows, or reports -1 when
// all of them are still showing another. shown keeps the row numbers of the two
// pinned kinds apart by adding their lane count, so one map covers both.
func pinLane(shown map[int]time.Duration, kind, lanes int, from, now time.Duration, window time.Duration) int {
	base := 0
	if kind == kindBottom {
		base = 64
	}
	for l := 0; l < lanes; l++ {
		if until, ok := shown[base+l]; !ok || now >= until {
			shown[base+l] = from + window
			return l
		}
	}
	return -1
}

// commentKind says how the i-th comment of a list crosses: two in five are
// pinned, split between the top and the bottom, and the rest scroll. Danmaku
// arrives as one undifferentiated list here — the source rule's comments carry
// no mode — so the pattern stands in for the mix a site's own server would
// send, and it is stable for a given comment, so one does not change its kind
// between frames.
func commentKind(i int) int {
	switch i % 5 {
	case 2:
		return kindTop
	case 4:
		return kindBottom
	}
	return kindScroll
}

// allows reports whether the settings let this kind of comment through.
func (s Settings) allows(kind int) bool {
	switch kind {
	case kindTop:
		return s.Top
	case kindBottom:
		return s.Bottom
	}
	return s.Scroll
}

// danmakuWindow is how long a comment at the chosen speed takes to cross.
func (p *Page) danmakuWindow() time.Duration {
	return time.Duration(clampF(p.danmakuSettings.Speed, danmakuFastest, danmakuSlowest) * float64(time.Second))
}

// danmakuRows is how many rows of the layer fit in a stage of this height, which
// the stage works out with rowsIn: the layer seats its lines against the same
// count so a seat is always a row that is really there.
func (p *Page) danmakuRows(height float32) int {
	set := p.danmakuSettings
	top, bottom := design.DanmakuBand(set.Area)
	from := height*top + height*design.DanmakuBandTop*(1-top)
	to := height*bottom - height*design.DanmakuBandBottom*(1-bottom)
	if to < from+design.DanmakuLaneHeight {
		from, to = height*design.DanmakuBandTop, height-design.DanmakuBandBottom*height
	}
	return rowsIn(from, to)
}

// laneWidth is the distance a comment travels across a stage width wide.
func laneWidth(width float32) float64 {
	if width < 240 {
		width = 240
	}
	return float64(width) + 240
}

// textWidth estimates how wide a line of text is at size, which is what decides
// how long a comment holds its lane. The layer is drawn with real text, but a
// measurement here would need a painter the page does not own, and a lane is
// only ever a little too long or a little too short by this estimate.
func textWidth(text string, size float32) float64 {
	runes := 0
	for range text {
		runes++
	}
	return float64(runes) * float64(size) * 0.95
}

// timedComments reports whether any comment carries a moment of its own. A
// source that only ships a list of reviews leaves every moment at zero.
func timedComments(comments []source.Danmaku) bool {
	for _, comment := range comments {
		if comment.At > 0 {
			return true
		}
	}
	return false
}

// spreadMoment places an untimed comment evenly across the episode, so a list
// of reviews drifts over the picture instead of piling up in the first seconds.
// The step never drops below one crossing, which keeps two comments from
// entering at once when the episode is short or the list is long.
func (p *Page) spreadMoment(index, count int) time.Duration {
	span := p.duration()
	if span <= 0 {
		span = time.Duration(count) * p.danmakuWindow()
	}
	step := span / time.Duration(count)
	if step < p.danmakuWindow() {
		step = p.danmakuWindow()
	}
	return time.Duration(index)*step + step/2
}

// openStream starts a system player for url with open, off the page's thread,
// which the engine may take long to return from, or never return from at all. It
// returns what open gave, for the page's thread to take up with takePlayer.
//
// It takes the opener rather than the page so the goroutine that runs it holds
// nothing of the page: the page is main-thread state, and an address that came
// back empty is the one failure this side can raise on its own.
func openStream(url string, open func(string) (*ui.VideoPlayer, error)) (*ui.VideoPlayer, error) {
	if url == "" {
		return nil, errNoURL
	}
	return open(url)
}

// streamURL is the address the engine is asked for: the source's own, routed
// through the cache when one is set. It is worked out on the main thread, so the
// goroutine that opens the stream carries no reference to the page with it.
func (p *Page) streamURL(url string) string {
	if url == "" {
		return ""
	}
	if p.deps.Cache != nil {
		url = p.deps.Cache.ProxyURL(url)
	}
	return url
}

// takePlayer takes up the stream open started for the open of seq, or drops it
// when the page has moved on to another one, or closed: its player would
// otherwise play a stream nobody asked for, and never be closed.
func (p *Page) takePlayer(system *ui.VideoPlayer, err error, seq uint64) {
	if seq != p.openSeq {
		if system != nil {
			system.Close()
		}
		return
	}
	p.releasePlayer()
	if err != nil {
		// An engine that refused the stream gives way to another line, as one
		// that opened and drew nothing does: the failure is the stage's word only
		// once no other line answers.
		p.lastFailure = err
		if !p.RetryOtherLine() {
			p.reportFailure()
		}
		return
	}
	// The wait the stage shows is the stream's: it started when opening did, and
	// the engine holds it until it draws.
	p.buffered = time.Now()
	p.player = system
	system.SetVolume(p.volume)
	system.SetRate(p.rate)
	system.SetMuted(p.muted)
	if p.playing {
		system.Play()
	} else {
		system.Pause()
	}
	p.invalidate()
}

// cancelOpen tells the open in flight, if any, that the page no longer wants
// the stream it started: its player is closed when its open returns.
func (p *Page) cancelOpen() { p.openSeq++ }

func (p *Page) releasePlayer() {
	if p.player != nil {
		p.player.Close()
		p.player = nil
	}
	p.buffered = time.Time{}
	p.stallAt, p.stallPos = time.Time{}, 0
}

// watchEngine reports what the engine of the current stream failed at: an error
// it raised, a stream that held the stage without a picture for longer than an
// opening may take, or one that drew and then stopped moving for as long. All
// three become the stage's own error, which is the one thing a viewer can see,
// close and try again from. An engine that waits for data without ever drawing,
// or without ever moving again, is the same stall as one that never reports
// anything: neither will wake the page again on its own, so the stage is what
// says so.
func (p *Page) watchEngine() {
	if p.player == nil {
		return
	}
	if err := p.player.Err(); err != nil {
		p.lastFailure = err
		if !p.RetryOtherLine() {
			p.reportFailure()
		}
		return
	}
	if p.player.Ready() {
		if !p.stalled() {
			return
		}
	} else if p.buffered.IsZero() || time.Since(p.buffered) < frameTimeout {
		return
	}
	// A line that opened without a picture, or that stopped giving them, gives
	// way to the next one. The stage is told so only when no line answers, which
	// is what the wait's last resort leaves the viewer with.
	p.lastFailure = nil
	if !p.RetryOtherLine() {
		p.reportFailure()
	}
}

// standing reports whether the playhead of a stream that drew has stood still
// since the stage last looked, recording where it stood. It is the only sign an
// engine gives that it has nothing to give: it goes on calling the stream
// buffering long after the picture moves again, and goes on calling it playing
// while the picture stands still, so neither of its own words is worth the wait
// marker. A stream whose playhead reaches its end played out rather than stood
// still, and one a viewer paused stands still by choice, so neither is one.
func (p *Page) standing() bool {
	if p.player == nil || !p.player.Ready() || !p.player.Playing() {
		return false
	}
	at := p.position()
	if total := p.duration(); total > 0 && at >= total {
		return false
	}
	if p.stallAt.IsZero() || at != p.stallPos {
		p.stallAt, p.stallPos = time.Now(), at
		return false
	}
	return true
}

// stalled reports whether the playhead of a stream that drew has stood still for
// longer than a wait may take, which the engine will not say by itself.
func (p *Page) stalled() bool {
	return p.standing() && time.Since(p.stallAt) >= frameTimeout
}

// RetryOtherLine gives up on the line the episode opened and moves it to the
// next one that answers with media, for a stream that failed or gave no picture.
// When the episode has no other line, the stage reports what the engine said
// about this one, or that it drew no picture. It reports whether it acted, which
// it cannot when no line is known yet.
func (p *Page) RetryOtherLine() bool {
	line := p.lineID
	if line == "" {
		line = p.appliedLine
	}
	if line == "" || passesOver(p.passed, line) {
		return false
	}
	// The lines are copied out: a resolve in flight holds the slice it was
	// handed, and must not see this one grow under it.
	passed := append(append([]string(nil), p.passed...), line)
	p.passed = passed
	// The stream that failed goes with its line: kept, the stage would see it
	// fail again on the next frame, and ask for another line every time.
	p.releasePlayer()
	if !p.offersAnotherLine(passed, line) {
		p.reportFailure()
		return true
	}
	p.lineID = ""
	p.started = false
	p.Start()
	return true
}

// offersAnotherLine reports whether the episode has a line left to try, past the
// ones it already gave up on.
func (p *Page) offersAnotherLine(passed []string, line string) bool {
	for _, offered := range p.playback.Lines {
		if offered.ID == "" || offered.ID == line || passesOver(passed, offered.ID) {
			continue
		}
		return true
	}
	return false
}

// reportFailure puts the failure on the stage in the engine's own words, or as a
// stream that gave no picture when the engine said nothing. It is the last word
// once no line is left to try.
func (p *Page) reportFailure() {
	p.releasePlayer()
	if p.lastFailure != nil {
		p.notice, p.err = noticeFailed, p.lastFailure
	} else {
		p.notice, p.err = noticeNoFrame, errNoFrame
	}
	p.invalidate()
}

// watchOpening reports an open that has held the stage longer than an opening
// may take, the one wait the engine cannot report on: its open has not returned,
// so there is no player yet, and nothing else would end the wait. It says so and
// reports whether it did.
func (p *Page) watchOpening() bool {
	if p.player != nil || p.buffered.IsZero() {
		return false
	}
	if time.Since(p.buffered) < frameTimeout {
		return false
	}
	p.notice, p.err = noticeNoFrame, errNoFrame
	p.cancelOpen()
	p.invalidate()
	return true
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
	p.comments, p.commentWhy = nil, commentLoading
	p.notice, p.err = noticeNone, nil
	// The previous episode's engine is taken down with its own picture, so the
	// wait that follows starts from an empty stage instead of the last frame.
	p.playing = true
	p.started = false
	p.buffered = time.Time{}
	p.releasePlayer()
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

// Muted reports whether the video was silenced from the speaker.
func (p *Page) Muted() bool { return p.muted }

// Volume reports the level the track beside the speaker holds.
func (p *Page) Volume() float64 { return p.volume }

// ToggleDanmaku shows or hides the comment layer over the picture. The settings
// box is the viewer's own business and stays where it was: turning the comments
// off is not a request to close it, and turning them back on is not a request to
// open it.
func (p *Page) ToggleDanmaku() {
	p.danmaku = !p.danmaku
	p.invalidate()
}

// DanmakuOn reports whether the layer is drawing comments over the picture.
func (p *Page) DanmakuOn() bool { return p.danmaku }

// DanmakuOpen reports whether the layer's settings are unfolded.
func (p *Page) DanmakuOpen() bool { return p.danmakuOpen }

// SetDanmakuOpen unfolds or folds the layer's settings, which stay open while
// the viewer adjusts them so the picture answers each change at once.
func (p *Page) SetDanmakuOpen(open bool) {
	p.danmakuOpen = open
	p.invalidate()
}

// DanmakuSettings returns a copy of the layer's settings for the view.
func (p *Page) DanmakuSettings() Settings { return p.danmakuSettings }

// SetClock gives the comment layer the clock it times itself by, which a test
// moves on by hand: the layer carries the playhead across the frames between the
// player's reports, and that pace is only readable if the test owns the clock.
func (p *Page) SetClock(now func() time.Time) {
	if now == nil {
		p.now = time.Now
		return
	}
	p.now = now
}

// SetDanmakuSettings takes the settings the viewer just changed, fits them to
// the range the controls offer, draws the layer with them and keeps them for
// the next episode. Writing is small and happens on a change, not per frame.
func (p *Page) SetDanmakuSettings(s Settings) {
	p.danmakuSettings = s.clamped()
	p.danmakuDirty = true
	p.invalidate()
}

// editDanmakuSettings changes the settings in place and marks them to be
// written. The controls use it while the viewer is still dragging, so the
// picture answers at once and the file is written once the drag has settled.
func (p *Page) editDanmakuSettings(change func(*Settings)) {
	change(&p.danmakuSettings)
	p.danmakuSettings = p.danmakuSettings.clamped()
	p.danmakuDirty = true
	p.invalidate()
}

// flushDanmakuSettings writes the settings when they have changed since the
// last write. It runs at the end of a frame, after the controls have had their
// say, so one frame of dragging is one write at most and a settled drag is one
// write in all.
func (p *Page) flushDanmakuSettings() {
	if !p.danmakuDirty {
		return
	}
	p.danmakuDirty = false
	if p.deps.SettingsPath == "" {
		return
	}
	if err := SaveDanmakuSettings(p.deps.SettingsPath, p.danmakuSettings); err != nil {
		log.Printf("danmaku_settings_save_failed: %v", err)
	}
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

// PictureBox is the box a picture of the given ratio fills inside a stage of
// the given size once scaled to fit whole, centred: empty until a picture of a
// known ratio has come.
func (p *Page) PictureBox(width, height float32) ui.Rect {
	return p.pictureBox(width, height)
}

// PanelHidden reports whether the side panel is away and the picture owns the
// window.
func (p *Page) PanelHidden() bool { return p.panelHidden }

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

// ToggleFullScreen takes the window in and out of full screen. Cinema is the
// player's own reading of it: the panel goes away and the picture keeps the
// whole window while the screen is the viewer's.
func (p *Page) ToggleFullScreen() {
	p.setCinema(!p.cinema)
}

// SyncFullScreen follows the window when the desktop took it in or out of full
// screen on its own, as Escape does. The window already holds that state, so
// this only moves the player's own reading of it.
func (p *Page) SyncFullScreen(on bool) {
	if p.cinema == on {
		return
	}
	p.cinema = on
	p.invalidate()
}

func (p *Page) setCinema(on bool) {
	p.cinema = on
	if p.deps.SetFullScreen != nil {
		p.deps.SetFullScreen(on)
	}
	p.invalidate()
}
func (p *Page) ToggleExpanded() { p.expanded = !p.expanded; p.invalidate() }

func (p *Page) SelectTab(i int) {
	if i != tabEpisodes && i != tabDanmaku {
		return
	}
	p.tab = i
	p.invalidate()
}

// Back leaves the player and returns what it opened. Leaving full screen is
// part of leaving the player: the pages behind it want the window back.
func (p *Page) Back() {
	if p.cinema {
		p.setCinema(false)
	}
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

// invalidate asks for another frame. The redraw function is drawn on the
// thread that builds the view and called from the streams that answer the
// page, so both ends of it take the lock: a player opening a line would
// otherwise be the one thing a race detector sees first.
func (p *Page) invalidate() {
	p.redrawMu.Lock()
	redraw := p.redraw
	p.redrawMu.Unlock()
	if redraw != nil {
		redraw()
	}
}

// setRedraw points invalidate at the context building the current frame.
func (p *Page) setRedraw(redraw func()) {
	p.redrawMu.Lock()
	p.redraw = redraw
	p.redrawMu.Unlock()
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
