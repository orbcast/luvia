// Package screens assembles the app's destinations: the title page and the
// player open as screens over the navigation pages, and Back returns to where
// the viewer came from. It is pure assembly; the pages own their own state.
package screens

import (
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/detail"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/player"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/videocache"
)

// mode is which screen the router currently shows.
const (
	modeNav = iota
	modeDetail
	modePlayer
)

// Deps are the router's collaborators. Base draws the selected navigation page;
// OpenVideo lets tests replace the system media framework.
type Deps struct {
	Source    source.Source
	Library   *library.Store
	Images    *media.Loader
	Cache     *videocache.Cache
	Base      func(*ui.Context) ui.Element
	Refresh   func()
	Run       func(func())
	Immersive func(bool)
	// WindowButtons hides the window's own buttons on screens that bring
	// none, such as the player.
	WindowButtons func(bool)
	// SetFullScreen takes the window in and out of full screen, and
	// WindowFullScreen reports where the desktop last put it.
	SetFullScreen    func(bool)
	WindowFullScreen func() bool
	Intro            func()
	OpenVideo        func(string) (*ui.VideoPlayer, error)
}

// Router owns the title and player screens and switches between them and the
// navigation pages.
type Router struct {
	deps   Deps
	detail *detail.Page
	player *player.Page
	mode   int
}

// New wires the screens. Callbacks reference the router itself, which is safe
// because they only run once a viewer acts.
func New(deps Deps) *Router {
	r := &Router{deps: deps}
	r.detail = detail.New(detail.Deps{
		Source: deps.Source, Library: deps.Library, Images: deps.Images, Run: deps.Run,
		OnPlay: r.openPlayer, OnBack: r.backFromDetail,
	})
	r.player = player.New(player.Deps{
		Source: deps.Source, Library: deps.Library, Images: deps.Images, Cache: deps.Cache, Run: deps.Run,
		OnBack: r.backFromPlayer, OpenVideo: deps.OpenVideo,
		SetFullScreen: deps.SetFullScreen,
	})
	return r
}

// View draws the current screen.
func (r *Router) View(c *ui.Context) ui.Element {
	switch r.mode {
	case modeDetail:
		return r.detail.View(c)
	case modePlayer:
		// The window may have left full screen without the player asking, as
		// Escape does, so the player reads the window before it draws.
		if r.deps.WindowFullScreen != nil {
			r.player.SyncFullScreen(r.deps.WindowFullScreen())
		}
		return r.player.View(c)
	default:
		r.setImmersive(false)
		r.setWindowButtons(false)
		if r.deps.Base != nil {
			return r.deps.Base(c)
		}
		return ui.Box(c).Fill()
	}
}

// OpenDetail opens the title page for ref.
func (r *Router) OpenDetail(ref source.Ref) {
	r.mode = modeDetail
	r.setImmersive(true)
	r.setWindowButtons(false)
	r.detail.Open(ref)
	r.refresh()
}

func (r *Router) openPlayer(d source.Detail, episodes []source.Episode, index int) {
	r.mode = modePlayer
	r.setWindowButtons(true)
	if r.deps.Intro != nil {
		r.deps.Intro()
	}
	r.player.Open(d, episodes, index)
	r.refresh()
}

func (r *Router) backFromDetail() {
	r.mode = modeNav
	r.setImmersive(false)
	r.setWindowButtons(false)
	r.refresh()
}

func (r *Router) backFromPlayer() {
	r.mode = modeDetail
	r.setWindowButtons(false)
	r.refresh()
}

// Reset returns to the navigation pages when the viewer picks a destination.
func (r *Router) Reset() {
	r.mode = modeNav
	r.setImmersive(false)
	r.setWindowButtons(false)
}

func (r *Router) setImmersive(on bool) {
	if r.deps.Immersive != nil {
		r.deps.Immersive(on)
	}
}

// setWindowButtons hides the window's minimize, maximize and close buttons
// while a screen that brings its own controls owns the window.
func (r *Router) setWindowButtons(hidden bool) {
	if r.deps.WindowButtons != nil {
		r.deps.WindowButtons(hidden)
	}
}

func (r *Router) refresh() {
	if r.deps.Refresh != nil {
		r.deps.Refresh()
	}
}
