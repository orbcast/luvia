package appshell

import (
	"fmt"
	"github.com/egoist/mygo"
	"luvia/apps/desktop/locales"
)

// MenuActions supplies application-owned preference operations.
type MenuActions struct {
	Theme     func() mygo.ThemeSource
	SetTheme  func(mygo.ThemeSource)
	Locale    func() string
	SetLocale func(string)
}

// SyncMenu keeps native radio items consistent with navigation from other inputs.
func (s *Shell) SyncMenu(menu *mygo.Menu) {
	if item := menu.ItemByID(fmt.Sprintf("nav-%d", s.selected)); item != nil {
		item.SetChecked(true)
	}
}

func (s *Shell) Menu(m locales.Messages, actions MenuActions) *mygo.Menu {
	navigate := func(p Page) func(*mygo.MenuItem, *mygo.Window) {
		return func(_ *mygo.MenuItem, _ *mygo.Window) { s.Navigate(p) }
	}
	navigation := make([]*mygo.MenuItem, 0, 5)
	for i, p := range pages {
		navigation = append(navigation, &mygo.MenuItem{ID: fmt.Sprintf("nav-%d", p), Label: p.Label(m),
			Type: mygo.MenuItemRadio, Checked: s.selected == p, Accelerator: fmt.Sprintf("CmdOrCtrl+%d", i+1), Click: navigate(p)})
	}
	appearance := make([]*mygo.MenuItem, 0, 3)
	for i, source := range []mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark} {
		appearance = append(appearance, &mygo.MenuItem{ID: "theme-" + string(source),
			Label: []string{m.FollowSystem(), m.Light(), m.Dark()}[i], Type: mygo.MenuItemRadio, Checked: actions.Theme() == source,
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) { actions.SetTheme(source) }})
	}
	languages := make([]*mygo.MenuItem, 0, 3)
	for i, tag := range []string{"", "zh-Hans", "en"} {
		languages = append(languages, &mygo.MenuItem{ID: "locale-" + tag,
			Label: []string{m.FollowSystem(), m.SimplifiedChinese(), m.English()}[i], Type: mygo.MenuItemRadio, Checked: actions.Locale() == tag,
			Click: func(_ *mygo.MenuItem, _ *mygo.Window) { actions.SetLocale(tag) }})
	}
	view := append(navigation, mygo.Separator(),
		&mygo.MenuItem{ID: "appearance", Label: m.Appearance(), Submenu: appearance},
		&mygo.MenuItem{ID: "language", Label: m.Language(), Submenu: languages},
		mygo.Separator(), &mygo.MenuItem{Role: mygo.RoleToggleFullScreen})
	return mygo.NewMenu([]*mygo.MenuItem{
		{Role: mygo.RoleAppMenu, Submenu: []*mygo.MenuItem{
			{Role: mygo.RoleAbout}, mygo.Separator(),
			{ID: "settings", Label: m.SettingsMenu(), Accelerator: "CmdOrCtrl+,", Click: navigate(Settings)},
			mygo.Separator(), {Role: mygo.RoleServices}, mygo.Separator(),
			{Role: mygo.RoleHide}, {Role: mygo.RoleHideOthers}, {Role: mygo.RoleUnhide},
			mygo.Separator(), {Role: mygo.RoleQuit},
		}},
		{Role: mygo.RoleEditMenu},
		{ID: "view", Label: m.ViewMenu(), Submenu: view},
		{Role: mygo.RoleWindowMenu},
	})
}
