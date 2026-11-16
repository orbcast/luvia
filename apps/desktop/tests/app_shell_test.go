package tests

import (
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/locales"
	"testing"
)

var english = locales.In("en")

func TestAppShellNavigation(t *testing.T) {
	s := appshell.New()
	tt := ui.NewTester(s.View, 960, 640)
	tt.SetLocale("en")
	if s.Selected() != appshell.Discover {
		t.Fatal("startup must show Discover")
	}
	for _, p := range appshell.Pages() {
		if err := tt.Click(p.Label(english)); err != nil {
			t.Fatal(err)
		}
		if s.Selected() != p {
			t.Fatalf("selected %v, want %v", s.Selected(), p)
		}
		for _, candidate := range appshell.Pages() {
			node, ok := tt.Access(candidate.Label(english))
			if !ok || (node.Current == ui.CurrentPage) != (candidate == p) {
				t.Fatalf("incorrect current state for %v: %+v", candidate, node)
			}
		}
	}
	for _, node := range tt.AccessAll() {
		if node.Role == ui.RoleText {
			t.Fatal("business content must stay empty")
		}
	}
}

func TestAppShellShortcuts(t *testing.T) {
	s := appshell.New()
	tt := ui.NewTester(s.View, 960, 640)
	for i, key := range []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5} {
		tt.Key(ui.Super, key)
		if s.Selected() != appshell.Pages()[i] {
			t.Fatalf("shortcut %d selected %v", i+1, s.Selected())
		}
	}
	s.Navigate(appshell.Page(99))
	s.Navigate(appshell.Page(-1))
	if s.Selected() != appshell.Settings {
		t.Fatal("invalid navigation changed selection")
	}
	tt.Key(ui.Super, ui.Key1)
	tt.Key(ui.Super, ui.KeyComma)
	if s.Selected() != appshell.Settings {
		t.Fatal("settings shortcut did not navigate")
	}
}

func TestAppShellLayoutAndLocale(t *testing.T) {
	for _, size := range [][2]int{{960, 640}, {640, 420}, {1200, 800}} {
		s := appshell.New()
		tt := ui.NewTester(s.View, size[0], size[1])
		tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})
		for _, tag := range []string{"en", "zh-Hans"} {
			tt.SetLocale(tag)
			for _, dark := range []bool{false, true} {
				tt.SetDark(dark)
				var nodes []ui.AccessInfo
				for _, node := range tt.AccessAll() {
					if node.Role == ui.RoleButton {
						nodes = append(nodes, node)
					}
				}
				if len(nodes) != 5 {
					t.Fatalf("got %d buttons", len(nodes))
				}
				first, last := nodes[0].Bounds, nodes[4].Bounds
				if first.X < 8 || first.Y < 28 || last.Y+last.H > float32(size[1]) {
					t.Fatalf("clipped navigation: %+v %+v", first, last)
				}
				if d := (first.Y+last.Y+last.H)/2 - float32(size[1])/2; d < -1 || d > 1 {
					t.Fatalf("not vertically centered: %v", d)
				}
				for _, p := range appshell.Pages() {
					if _, ok := tt.Access(p.Label(locales.In(tag))); !ok {
						t.Fatalf("missing localized %v", p)
					}
				}
			}
		}
	}
}
