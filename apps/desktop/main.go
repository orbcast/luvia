package main

import (
	"log"
	"net/http"
	"path/filepath"
	"runtime"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/appwindow"
	"luvia/apps/desktop/internal/catalog"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/home"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/media"
	"luvia/apps/desktop/internal/playintro"
	"luvia/apps/desktop/internal/preferences"
	"luvia/apps/desktop/internal/screens"
	"luvia/apps/desktop/internal/settings"
	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/loader"
	"luvia/apps/desktop/locales"
)

func main() {
	shell := appshell.New()
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
		preferences.RestoreTheme(themePath, mygo.Theme.SetSource)

		profile := openProfile(userData, cache)
		var introCtrl *playintro.Controller
		window := appwindow.New(func() appwindow.Window {
			return mygo.NewWindow(mygo.WindowOptions{
				Title: "Luvia", Width: 1100, Height: 720, MinWidth: 400, MinHeight: 400,
				StateKey: "main", TitleBarStyle: mygo.TitleBarHidden,
				Content: ui.View(func(c *ui.Context) {
					shell.View(c)
					introCtrl.View(c)
				}),
			})
		})
		introCtrl = playintro.New(func() { window.Refresh() })

		actions := appshell.MenuActions{
			Theme:    mygo.Theme.Source,
			SetTheme: func(source mygo.ThemeSource) { preferences.ChangeTheme(themePath, source, mygo.Theme.SetSource) },
			Locale:   mygo.App.ChosenLocale, SetLocale: mygo.App.SetLocale,
		}
		var router *screens.Router
		openTitle := func(ref source.Ref) { router.OpenDetail(ref) }
		discover := home.New(home.Deps{
			Source:   profile.source,
			Library:  profile.library,
			Images:   profile.images,
			Interval: 6 * time.Second,
			OnPlay:   openTitle,
			OnDetail: openTitle,
			OnAll:    func() { shell.Navigate(appshell.Library) },
		})
		catalogPage := catalog.New(catalog.Deps{
			Source:   profile.source,
			Library:  profile.library,
			Images:   profile.images,
			OnPlay:   openTitle,
			OnDetail: openTitle,
		})
		settingsPage := settings.New(settings.Deps{
			Theme: actions.Theme, SetTheme: actions.SetTheme,
			Locale: actions.Locale, SetLocale: actions.SetLocale,
		})
		profile.images.SetChanged(func() { window.Refresh() })
		router = screens.New(screens.Deps{
			Source:    profile.source,
			Library:   profile.library,
			Images:    profile.images,
			Refresh:   window.Refresh,
			Immersive: shell.SetNavigationHidden,
			Intro:     func() { introCtrl.Start(nil) },
			Base: func(c *ui.Context) ui.Element {
				switch shell.Selected() {
				case appshell.Discover:
					return discover.View(c)
				case appshell.Library:
					return catalogPage.View(c)
				case appshell.Settings:
					return settingsPage.View(c)
				}
				return placeholder(c, shell.Selected())
			},
		})
		shell.SetContent(router.View)
		rebuildMenu := func() { mygo.App.SetMenu(shell.Menu(locales.In(mygo.App.Locale()), actions)) }
		shell.SetOnNavigate(func(appshell.Page) {
			router.Reset()
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
}

// profile is the local state the window needs before any source answers.
type profile struct {
	source  source.Source
	library *library.Store
	images  *media.Loader
}

// openProfile opens the configured rule directory, the local library and the
// artwork cache. A missing source is not fatal: the Discover page explains it.
func openProfile(userData, cache string) profile {
	libraryPath := filepath.Join(userData, "library.json")
	store, err := library.Open(libraryPath)
	if err != nil {
		log.Printf("library %s: %v", libraryPath, err)
	}
	images := media.New(filepath.Join(cache, "artwork"), http.DefaultClient)

	rules := loader.DefaultDir(filepath.Join(userData, "rules"), filepath.Join(resourceDir(), "luvia-rules"))
	set, err := loader.Load(rules, engine.Options{Client: http.DefaultClient})
	if err != nil {
		log.Printf("rules %s: %v", rules, err)
		return profile{library: store, images: images}
	}
	for _, diagnostic := range set.Diagnostics() {
		log.Printf("rule skipped: %v", diagnostic)
	}
	return profile{source: preferredSource(set), library: store, images: images}
}

func preferredSource(set *loader.Set) source.Source {
	if found, ok := set.Get("kanju"); ok {
		return found
	}
	if sources := set.Sources(); len(sources) > 0 {
		return sources[0]
	}
	return nil
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
