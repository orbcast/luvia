package main

import (
	"log"
	"path/filepath"
	"runtime"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/appwindow"
	"luvia/apps/desktop/internal/preferences"
	"luvia/apps/desktop/locales"
)

func main() {
	shell := appshell.New()
	mygo.App.WhenReady(func() {
		dir, err := mygo.App.Path(mygo.PathUserData)
		if err != nil {
			log.Fatal(err)
		}
		themePath := filepath.Join(dir, "theme.json")
		preferences.RestoreTheme(themePath, mygo.Theme.SetSource)
		window := appwindow.New(func() appwindow.Window {
			return mygo.NewWindow(mygo.WindowOptions{
				Title: "Luvia", Width: 1100, Height: 720, MinWidth: 640, MinHeight: 420,
				StateKey: "main", TitleBarStyle: mygo.TitleBarHidden, Content: ui.View(shell.View),
			})
		})
		actions := appshell.MenuActions{
			Theme:    mygo.Theme.Source,
			SetTheme: func(source mygo.ThemeSource) { preferences.ChangeTheme(themePath, source, mygo.Theme.SetSource) },
			Locale:   mygo.App.ChosenLocale, SetLocale: mygo.App.SetLocale,
		}
		rebuildMenu := func() { mygo.App.SetMenu(shell.Menu(locales.In(mygo.App.Locale()), actions)) }
		shell.SetOnNavigate(func(appshell.Page) {
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
