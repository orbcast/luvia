package main

import (
	"context"
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/appwindow"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/playintro"
	"luvia/apps/desktop/internal/preferences"
	"luvia/apps/desktop/internal/settings"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/loader"
	"luvia/apps/desktop/internal/videocache"
	"luvia/apps/desktop/locales"
)

func main() {
	shell := appshell.New()
	var videoCache *videocache.Cache
	mygo.App.WhenReady(func() {
		userData, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		cache, err := mygo.App.Path(mygo.PathCache)
		if err != nil {
			log.Fatal(err)
		}
		themePath := filepath.Join(userData, "theme.json")
		sourcePath := filepath.Join(userData, "source.json")
		preferences.RestoreTheme(themePath, mygo.Theme.SetSource)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		profile := openProfile(ctx, userData, cache, preferences.LoadSource(sourcePath))
		videoCache = profile.video
		var introCtrl *playintro.Controller
		window := appwindow.New(func() appwindow.Window {
			// The window hides the system's buttons (HideWindowButtons) and the
			// shell draws its own, so the title bar holds nothing but the app's
			// controls; shell gets the window to act on once it exists.
			w := mygo.NewWindow(mygo.WindowOptions{
				Title: "Luvia", Width: 1100, Height: 720, MinWidth: 400, MinHeight: 400,
				StateKey: "main", TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: true,
				Content: ui.View(func(c *ui.Context) {
					shell.View(c)
					introCtrl.View(c)
				}),
			})
			shell.SetWindow(w)
			// The desktop decides which window buttons its title bars have and
			// at which end; the shell draws those, and none where it asks for
			// none, as under a tiling compositor.
			order := mygo.Theme.WindowButtons()
			shell.SetWindowButtons(appshell.WindowButtons{Start: order.Start, End: order.End})
			return w
		})
		introCtrl = playintro.New(func() { window.Refresh() })

		actions := appshell.MenuActions{
			Theme:    mygo.Theme.Source,
			SetTheme: func(source mygo.ThemeSource) { preferences.ChangeTheme(themePath, source, mygo.Theme.SetSource) },
			Locale:   mygo.App.ChosenLocale, SetLocale: mygo.App.SetLocale,
		}

		// applySource points every source-bound page at id, and is how both the
		// first frame and a switch in the settings page reach the same build.
		var pages *pageSet
		var applySource func(string)
		settingsPage := settings.New(settings.Deps{
			Theme: actions.Theme, SetTheme: actions.SetTheme,
			Locale: actions.Locale, SetLocale: actions.SetLocale,
			// The drop-down lists the rules that loaded, so it fills in as soon
			// as another rule directory appears.
			Sources: func() []string { return sourceIDs(profile.rules) },
			Source:  func() string { return sourceID(profile.source) },
			SetSource: func(id string) {
				if sourceID(profile.source) == id {
					return
				}
				applySource(id)
				if err := preferences.SaveSource(sourcePath, id); err != nil {
					log.Printf("source preference %s: %v", sourcePath, err)
				}
				window.Refresh()
			},
		})
		applySource = func(id string) {
			profile = chooseSource(profile, id)
			pages = newPageSet(pageDeps{
				profile: profile, settings: settingsPage,
				shell: shell, window: window, intro: introCtrl,
			})
			// Artwork arriving for the pages just built redraws the window;
			// the loader keeps one callback, so the newest set owns it.
			profile.images.SetChanged(func() { window.Refresh() })
			shell.SetContent(pages.router.View)
		}
		applySource(sourceID(profile.source))

		rebuildMenu := func() { mygo.App.SetMenu(shell.Menu(locales.In(mygo.App.Locale()), actions)) }
		shell.SetOnNavigate(func(appshell.Page) {
			pages.router.Reset()
			shell.SyncMenu(mygo.App.Menu())
			window.Refresh()
		})
		rebuildMenu()
		mygo.App.OnLocaleChanged(func(string) { rebuildMenu() })
		if runtime.GOOS == "darwin" {
			mygo.App.OnWindowAllClosed(func() {})
		}
		mygo.App.OnActivate(func(visible bool) {
			if !visible {
				window.Open()
			}
		})
		window.Open()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
	if videoCache != nil {
		_ = videoCache.Close()
	}
}

// profile is the local state the window needs before any source answers.
type profile struct {
	source  source.Source
	library *library.Store
	images  *media.Loader
	video   *videocache.Cache
	// rules is the loaded rule set, kept so a switch can reach the source that
	// was not chosen this time without reading the directory again.
	rules *loader.Set
	// updates reports whether the chosen source carries an updates block, so
	// the Discover page knows whether the week strip can ask it for a day.
	updates bool
}

// openProfile opens the configured rule directory, the local library and the
// artwork cache. A missing source is not fatal: the Discover page explains it.
func openProfile(ctx context.Context, userData, cache, wanted string) profile {
	libraryPath := filepath.Join(userData, "library.json")
	store, err := library.Open(libraryPath)
	if err != nil {
		log.Printf("library %s: %v", libraryPath, err)
	}
	images := media.New(filepath.Join(cache, "artwork"), media.Options{})
	video, err := videocache.New(filepath.Join(cache, "video"), videocache.Options{})
	if err != nil {
		log.Printf("video cache %s: %v", filepath.Join(cache, "video"), err)
	}

	rules := loader.DefaultDir(filepath.Join(userData, "rules"), filepath.Join(resourceDir(), "luvia-rules"))
	set, err := loader.Load(rules, engine.Options{Client: http.DefaultClient})
	if err != nil {
		log.Printf("rules %s: %v", rules, err)
		return profile{library: store, images: images, video: video}
	}
	for _, diagnostic := range set.Diagnostics() {
		log.Printf("rule skipped: %v", diagnostic)
	}
	chosen := preferredSource(set, wanted)
	return profile{
		source: chosen, library: store, images: images, video: video,
		rules: set, updates: hasUpdates(ctx, chosen),
	}
}

// preferredSource is the stored choice when it still loads, otherwise the
// source the app opens with, otherwise whichever rule came first. A stored id
// names a rule directory that may have been removed since.
func preferredSource(set *loader.Set, wanted string) source.Source {
	if wanted != "" {
		if found, ok := set.Get(wanted); ok {
			return found
		}
	}
	if found, ok := set.Get("kanju"); ok {
		return found
	}
	if sources := set.Sources(); len(sources) > 0 {
		return sources[0]
	}
	return nil
}

// hasUpdates reports whether src answers Updates, which the engine decides by
// whether its rule declares an updates block. Probing the first weekday costs
// one request and tells a source without a schedule apart from one whose
// schedule failed.
func hasUpdates(ctx context.Context, src source.Source) bool {
	if src == nil {
		return false
	}
	_, err := src.Updates(ctx, source.Day{Weekday: 1})
	if err == nil {
		return true
	}
	kind, ok := source.KindOf(err)
	return ok && kind != source.ErrUnsupported
}

func resourceDir() string {
	dir, err := mygo.App.Path(mygo.PathResources)
	if err != nil {
		return ""
	}
	return dir
}

func placeholder(c *ui.Context, page appshell.Page) ui.Element {
	t := c.Theme()
	m := locales.In(c.Locale())
	width, _ := c.Size()
	inset := design.Inset(width)
	column := ui.Column(c).Fill().Background(t.Background).
		Padding(56, inset, 40+design.BottomClearance(width), inset).Gap(6)
	column.Children(func() {
		ui.Text(c, page.Label(m)).FontSize(20).Bold().TextColor(t.Text)
		ui.Text(c, m.PageComing()).FontSize(13).TextColor(t.TextMuted)
	})
	return column
}
