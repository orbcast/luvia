package main

import (
	"context"
	"path/filepath"
	"time"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/appwindow"
	"luvia/apps/desktop/internal/catalog"
	"luvia/apps/desktop/internal/danmaku"
	"luvia/apps/desktop/internal/home"
	"luvia/apps/desktop/internal/playintro"
	"luvia/apps/desktop/internal/screens"
	"luvia/apps/desktop/internal/search"
	"luvia/apps/desktop/internal/settings"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/loader"
)

// pageSet is every destination that reads the chosen source. A switch builds a
// new set instead of handing the old pages a new source: what a page shows came
// from the source that answered it, and an answer still in flight lands in the
// set that is no longer drawn.
type pageSet struct {
	discover *home.Page
	catalog  *catalog.Page
	search   *search.Page
	settings *settings.Page
	router   *screens.Router
}

// pageDeps is what a set is built from: the profile holding the chosen source,
// and the shell, window and intro the pages call back into.
type pageDeps struct {
	profile profile
	// userData is where the player keeps its own preferences, beside the
	// theme and the source list.
	userData string
	settings *settings.Page
	shell    *appshell.Shell
	window   *appwindow.Main
	intro    *playintro.Controller
}

// newPageSet wires the navigation pages and the router over one source.
func newPageSet(deps pageDeps) *pageSet {
	set := &pageSet{settings: deps.settings}
	openTitle := func(ref source.Ref) { set.router.OpenDetail(ref) }
	set.discover = home.New(home.Deps{
		Source:  deps.profile.source,
		Library: deps.profile.library,
		Images:  deps.profile.images,
		// The week strip reads the source's own schedule when the rule has
		// one; without it the page counts the viewer's follows only.
		Updates:  deps.profile.updates,
		Interval: 6 * time.Second,
		OnPlay:   openTitle,
		OnDetail: openTitle,
		OnAll:    func() { deps.shell.Navigate(appshell.Library) },
	})
	set.catalog = catalog.New(catalog.Deps{
		Source:   deps.profile.source,
		Library:  deps.profile.library,
		Images:   deps.profile.images,
		OnPlay:   openTitle,
		OnDetail: openTitle,
	})
	set.search = search.New(search.Deps{
		Source:   deps.profile.source,
		Library:  deps.profile.library,
		Images:   deps.profile.images,
		OnPlay:   openTitle,
		OnDetail: openTitle,
	})
	set.router = screens.New(screens.Deps{
		Source: deps.profile.source,
		// The comments a rule carries are usually reviews, which have no
		// moment to be laid over the picture at. The lookup is what gives the
		// layer something to draw.
		Danmaku:   danmaku.NewBilibili(nil),
		Library:   deps.profile.library,
		Images:    deps.profile.images,
		Cache:     deps.profile.video,
		Refresh:   deps.window.Refresh,
		Immersive: deps.shell.SetNavigationHidden,
		// The player brings its own controls, so the window's stay away
		// there and return with the navigation pages.
		WindowButtons: deps.shell.SetWindowControlsHidden,
		// The player's full-screen button takes the window itself in and
		// out of full screen; the desktop can also do it on its own, so
		// the router reads the window back before every player frame.
		SetFullScreen: func(on bool) {
			if w := deps.window.Current(); w != nil {
				w.SetFullScreen(on)
			}
		},
		WindowFullScreen: func() bool {
			if w := deps.window.Current(); w != nil {
				return w.IsFullScreen()
			}
			return false
		},
		// The player's own preferences sit beside the theme and the source
		// list, in the user data directory.
		PlayerSettingsPath: filepath.Join(deps.userData, "player.json"),
		Intro:              func() { deps.intro.Start(nil) },
		Base:               func(c *ui.Context) ui.Element { return set.navView(c, deps.shell.Selected()) },
	})
	return set
}

// navView draws the navigation page the shell selected.
func (s *pageSet) navView(c *ui.Context, page appshell.Page) ui.Element {
	switch page {
	case appshell.Discover:
		return s.discover.View(c)
	case appshell.Library:
		return s.catalog.View(c)
	case appshell.Search:
		return s.search.View(c)
	case appshell.Settings:
		return s.settings.View(c)
	}
	return placeholder(c, page)
}

// sourceID is the id of the source in force, or "" when none loaded.
func sourceID(src source.Source) string {
	if src == nil {
		return ""
	}
	return src.ID()
}

// sourceIDs lists the sources a viewer may switch between, in the order the
// rules loaded.
func sourceIDs(set *loader.Set) []string {
	if set == nil {
		return nil
	}
	sources := set.Sources()
	ids := make([]string, len(sources))
	for i, src := range sources {
		ids[i] = src.ID()
	}
	return ids
}

// chooseSource re-points the profile at the wanted source and asks whether it
// carries a schedule, which the Discover page needs to know before it draws.
// The source already in force is returned untouched, so the first build costs
// no second request.
func chooseSource(p profile, wanted string) profile {
	if p.rules == nil || (p.source != nil && p.source.ID() == wanted) {
		return p
	}
	chosen := preferredSource(p.rules, wanted)
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	p.source, p.updates = chosen, hasUpdates(ctx, chosen)
	return p
}
