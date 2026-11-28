package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/appwindow"
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
		localePath := filepath.Join(userData, "locale.json")
		preferences.RestoreTheme(themePath, mygo.Theme.SetSource)
		preferences.ApplyAccessKeys(userData, resourceDir())
		// The app is made for Simplified Chinese readers, but MyGo falls back
		// on the language the catalogs are written in, English, whenever the
		// system speaks none of the app's languages. Choosing the language
		// here, before the window is built, keeps that fallback from ever
		// showing, and MyGo keeps the choice for the next launch.
		if chosen := preferences.LoadLocale(localePath); chosen != "" {
			mygo.App.SetLocale(chosen)
		} else if tag := mygo.App.Locale(); tag == "en" {
			mygo.App.SetLocale(appshell.DefaultLocale)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		profile := openProfile(ctx, userData, cache, preferences.LoadSource(sourcePath))
		videoCache = profile.video
		var introCtrl *playintro.Controller
		window := appwindow.New(func() appwindow.Window {
			// macOS keeps its own traffic lights over the page, so the window
			// shows them and the shell draws none; other desktops let the
			// window hide theirs and the shell draws the set their title bars
			// would hold. Shell gets the window to act on as it is.
			native := appshell.NativeWindowButtons(runtime.GOOS)
			w := mygo.NewWindow(mygo.WindowOptions{
				Title: "Luvia", Width: 1100, Height: 720, MinWidth: 400, MinHeight: 400,
				StateKey: "main", TitleBarStyle: mygo.TitleBarHidden, HideWindowButtons: !native,
				Content: ui.View(func(c *ui.Context) {
					shell.View(c)
					introCtrl.View(c)
				}),
			})
			shell.SetWindow(w)
			// The desktop decides which window buttons its title bars have and
			// at which end; the shell draws those, and none where it asks for
			// none, as under a tiling compositor. Where the platform draws them
			// itself, the shell is left with no buttons to draw.
			if !native {
				order := mygo.Theme.WindowButtons()
				shell.SetWindowButtons(appshell.WindowButtons{Start: order.Start, End: order.End})
			}
			return w
		})
		introCtrl = playintro.New(func() { window.Refresh() })

		actions := appshell.MenuActions{
			Theme:    mygo.Theme.Source,
			SetTheme: func(source mygo.ThemeSource) { preferences.ChangeTheme(themePath, source, mygo.Theme.SetSource) },
			Locale:   mygo.App.ChosenLocale,
			SetLocale: func(tag string) {
				mygo.App.SetLocale(tag)
				if err := preferences.SaveLocale(localePath, tag); err != nil {
					log.Printf("locale preference %s: %v", localePath, err)
				}
			},
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
			// The variables the loaded rules ask for by name. A value typed
			// here is stored and applied to this process, so the pages can be
			// built again without a restart.
			SecretNames: func() []string { return profile.secretNames },
			Secrets:     func() map[string]string { return preferences.LoadSecrets(userData) },
			HasEnv:      func(name string) bool { return strings.TrimSpace(os.Getenv(name)) != "" },
			SetSecrets: func(values map[string]string) {
				if err := preferences.SaveSecrets(userData, values); err != nil {
					log.Printf("secrets %s: %v", userData, err)
					return
				}
				preferences.ApplySecrets(values)
				// A newly supplied secret is what a failed request needed, so
				// the pages are built again rather than left on the error.
				applySource(sourceID(profile.source))
				window.Refresh()
			},
		})
		applySource = func(id string) {
			profile = chooseSource(profile, id)
			pages = newPageSet(pageDeps{
				profile: profile, userData: userData, settings: settingsPage,
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
	// secretNames lists the variables the loaded rules ask for by name, in
	// the order they appear, so the settings page can offer a field for each.
	secretNames []string
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
	set, err := loader.Load(rules, engine.Options{
		Client: http.DefaultClient,
		// A claimed upstream session lives beside the library and the
		// reader's secrets: it is this installation's identity, not the
		// cached response of this run.
		SessionsDir: userData,
	})
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
		secretNames: set.SecretNames(),
		rules:       set, updates: hasUpdates(ctx, chosen),
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
