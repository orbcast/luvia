package tests

import (
	"fmt"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/locales"
	"testing"
)

func TestMenuNavigationAndSynchronization(t *testing.T) {
	s := appshell.New()
	theme := mygo.ThemeSystem
	locale := ""
	actions := appshell.MenuActions{Theme: func() mygo.ThemeSource { return theme }, SetTheme: func(v mygo.ThemeSource) { theme = v }, Locale: func() string { return locale }, SetLocale: func(v string) { locale = v }}
	menu := s.Menu(english, actions)
	tt := ui.NewTester(s.View, 960, 640)
	tt.SetLocale("en")
	calls := 0
	s.SetOnNavigate(func(appshell.Page) { s.SyncMenu(menu); calls++ })
	for _, p := range appshell.Pages() {
		item := menu.ItemByID(fmt.Sprintf("nav-%d", p))
		item.Click(item, nil)
		tt.Frame()
		node, ok := tt.Access(p.Label(english))
		if !ok || node.Current != ui.CurrentPage {
			t.Fatalf("menu did not navigate to %v", p)
		}
		if err := tt.Click(appshell.Discover.Label(english)); err != nil {
			t.Fatal(err)
		}
		if !menu.ItemByID("nav-0").IsChecked() {
			t.Fatal("click did not synchronize menu")
		}
		for _, other := range appshell.Pages() {
			if other != appshell.Discover && menu.ItemByID(fmt.Sprintf("nav-%d", other)).IsChecked() {
				t.Fatal("multiple destinations remain checked")
			}
		}
	}
	settingsItem := menu.ItemByID("settings")
	settingsItem.Click(settingsItem, nil)
	if s.Selected() != appshell.Settings {
		t.Fatal("settings item did not navigate")
	}
	previousCalls := calls
	settingsItem.Click(settingsItem, nil)
	if calls != previousCalls+1 {
		t.Fatal("selected menu destination must still invoke activation")
	}
	for _, source := range []mygo.ThemeSource{mygo.ThemeDark, mygo.ThemeLight, mygo.ThemeSystem} {
		item := menu.ItemByID("theme-" + string(source))
		item.Click(item, nil)
		rebuilt := s.Menu(english, actions)
		if theme != source || !rebuilt.ItemByID("theme-"+string(source)).IsChecked() {
			t.Fatalf("theme %s did not roundtrip", source)
		}
	}
	for _, tag := range []string{"zh-Hans", "en", ""} {
		item := menu.ItemByID("locale-" + tag)
		item.Click(item, nil)
		rebuilt := s.Menu(english, actions)
		if locale != tag || !rebuilt.ItemByID("locale-"+tag).IsChecked() {
			t.Fatalf("locale %s did not roundtrip", tag)
		}
	}
	s.SyncMenu(mygo.NewMenu(nil))
}

func TestMenuLocalizationAndRoles(t *testing.T) {
	t.Cleanup(func() { i18n.SetLocaleFunc(mygo.App.Locale) })
	s := appshell.New()
	actions := appshell.MenuActions{Theme: func() mygo.ThemeSource { return mygo.ThemeSystem }, Locale: func() string { return "" }}
	for _, tag := range []string{"en", "zh-Hans"} {
		i18n.SetLocaleFunc(func() string { return tag })
		m := locales.In(tag)
		menu := s.Menu(m, actions)
		items := menu.Items()
		if len(items) != 4 || items[0].Role != mygo.RoleAppMenu || items[1].Role != mygo.RoleEditMenu || items[3].Role != mygo.RoleWindowMenu {
			t.Fatal("missing standard menus")
		}
		for _, p := range appshell.Pages() {
			item := menu.ItemByID(fmt.Sprintf("nav-%d", p))
			if item.Label != p.Label(m) || item.Accelerator != fmt.Sprintf("CmdOrCtrl+%d", p+1) {
				t.Fatalf("incorrect navigation item %+v", item)
			}
		}
		if menu.ItemByID("view").Label != m.ViewMenu() || menu.ItemByID("settings").Label != m.SettingsMenu() {
			t.Fatal("stale menu locale")
		}
	}
	if appshell.Page(-1).Label(english) != "" {
		t.Fatal("invalid page has label")
	}
}

// TestMenuNamesLanguagesInTheirOwnLanguage keeps the language submenu
// readable to a reader who cannot read the language shown: each item says
// its language's own name.
func TestMenuNamesLanguagesInTheirOwnLanguage(t *testing.T) {
	s := appshell.New()
	actions := appshell.MenuActions{
		Theme:  func() mygo.ThemeSource { return mygo.ThemeSystem },
		Locale: func() string { return "zh-Hans" }, SetLocale: func(string) {},
	}
	for _, shown := range []string{"en", "zh-Hans"} {
		m := locales.In(shown)
		menu := s.Menu(m, actions)
		if got := menu.ItemByID("locale-en").Label; got != "English" {
			t.Fatalf("en item in %s: %q", shown, got)
		}
		if got := menu.ItemByID("locale-zh-Hans").Label; got != "简体中文" {
			t.Fatalf("zh-Hans item in %s: %q", shown, got)
		}
		if got := menu.ItemByID("locale-").Label; got != m.FollowSystem() {
			t.Fatalf("follow system item in %s: %q", shown, got)
		}
		if !menu.ItemByID("locale-zh-Hans").IsChecked() || menu.ItemByID("locale-en").IsChecked() {
			t.Fatalf("checked language is wrong in %s", shown)
		}
	}
}
