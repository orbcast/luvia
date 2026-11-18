package tests

import (
	"strings"

	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/locales"
	"testing"
	"time"
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
				// Let the bar finish travelling between layouts.
				tt.Advance(600 * time.Millisecond)
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
				if size[0] < design.NarrowWidth {
					// Docked: one row at the bottom edge, centered across it.
					if first.X < 8 || last.X+last.W > float32(size[0])-8 || last.Y+last.H > float32(size[1]) {
						t.Fatalf("clipped dock: %+v %+v", first, last)
					}
					if first.Y < float32(size[1])/2 {
						t.Fatalf("dock is not at the bottom: %+v", first)
					}
					if d := (first.X+last.X+last.W)/2 - float32(size[0])/2; d < -1 || d > 1 {
						t.Fatalf("dock is not centered: %v", d)
					}
					continue
				}
				if first.X < 8 || first.Y < 28 || last.Y+last.H > float32(size[1]) {
					t.Fatalf("clipped navigation: %+v %+v", first, last)
				}
				top := float32(28 + design.NavigationTopGap + design.ShellPadding)
				if d := first.Y - top; d < -1 || d > 1 {
					t.Fatalf("column does not start under the title bar: %v", d)
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

func TestAppShellSplitsColumnAroundPinnedDestinations(t *testing.T) {
	s := appshell.New()
	tt := ui.NewTester(s.View, 1100, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})
	tt.SetLocale("en")
	tt.Advance(600 * time.Millisecond)

	var top, bottom []ui.AccessInfo
	for _, node := range tt.AccessAll() {
		if node.Role != ui.RoleButton {
			continue
		}
		switch node.Name {
		case "Search", "Settings":
			bottom = append(bottom, node)
		default:
			top = append(top, node)
		}
	}
	if len(top) != 3 || len(bottom) != 2 {
		t.Fatalf("split the column as %d top and %d bottom", len(top), len(bottom))
	}
	if bottom[0].Bounds.X != top[0].Bounds.X {
		t.Fatalf("groups are not in one column: %+v %+v", top[0].Bounds, bottom[0].Bounds)
	}
	gap := bottom[0].Bounds.Y - (top[2].Bounds.Y + top[2].Bounds.H)
	if gap < design.NavigationHeight {
		t.Fatalf("groups are not apart: %v", gap)
	}
	if edge := float32(720) - design.ShellInset; bottom[1].Bounds.Y+bottom[1].Bounds.H > edge+1 {
		t.Fatalf("settings does not end at the bottom inset: %+v", bottom[1].Bounds)
	}
}

func TestAppShellMovesBetweenColumnAndDock(t *testing.T) {
	s := appshell.New()
	tt := ui.NewTester(s.View, 1100, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})

	column := func() []ui.AccessInfo {
		var out []ui.AccessInfo
		for _, node := range tt.AccessAll() {
			if node.Role == ui.RoleButton {
				out = append(out, node)
			}
		}
		return out
	}
	nodes := column()
	if len(nodes) != 5 {
		t.Fatalf("got %d buttons", len(nodes))
	}
	if nodes[0].Bounds.X > 60 || nodes[4].Bounds.Y < nodes[0].Bounds.Y+100 {
		t.Fatalf("window did not start as a column: %+v", nodes[0].Bounds)
	}

	tt.SetSize(560, 700)
	tt.Frame()
	// The bar travels: partway through the spring it sits between the two
	// layouts instead of jumping to the dock.
	tt.Advance(80 * time.Millisecond)
	moving := column()
	if mid := moving[0].Bounds.Y; mid <= 58 || mid >= 632 {
		t.Fatalf("bar did not move between layouts: %+v", moving[0].Bounds)
	}
	tt.Advance(600 * time.Millisecond)
	nodes = column()
	if len(nodes) != 5 {
		t.Fatalf("got %d docked buttons", len(nodes))
	}
	row := nodes[0].Bounds
	for _, node := range nodes {
		if d := node.Bounds.Y - row.Y; d > 1 || d < -1 {
			t.Fatalf("dock is not one row: %+v vs %+v", node.Bounds, row)
		}
	}
	if row.Y < 300 || nodes[4].Bounds.X <= nodes[0].Bounds.X+100 {
		t.Fatalf("window did not dock at the bottom: %+v", row)
	}

	tt.SetSize(1100, 720)
	tt.Frame()
	tt.Advance(80 * time.Millisecond)
	if y := column()[0].Bounds.Y; y <= 58 || y >= 632 {
		t.Fatalf("bar did not travel back: %+v", column()[0].Bounds)
	}
	tt.Advance(600 * time.Millisecond)
	nodes = column()
	if nodes[0].Bounds.X > 60 {
		t.Fatalf("window did not return to the column: %+v", nodes[0].Bounds)
	}
}

// TestAppShellDrawsWindowButtons covers the window controls the shell draws in
// place of the system's: the desktop's layout decides which it has and where,
// and a desktop that asks for none gets none.
func TestAppShellDrawsWindowButtons(t *testing.T) {
	names := func(s *appshell.Shell) []string {
		tt := ui.NewTester(s.View, 1100, 720)
		tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})
		tt.SetLocale("en")
		var out []string
		for _, node := range tt.AccessAll() {
			if node.Role == ui.RoleButton && strings.HasPrefix(node.Name, "Close window") ||
				node.Role == ui.RoleButton && strings.HasPrefix(node.Name, "Minimize window") ||
				node.Role == ui.RoleButton && strings.HasPrefix(node.Name, "Maximize window") {
				out = append(out, node.Name)
			}
		}
		return out
	}

	s := appshell.New()
	s.SetWindowButtons(appshell.WindowButtons{End: []string{"minimize", "maximize", "close"}})
	if got := names(s); len(got) != 3 {
		t.Fatalf("a desktop with three window buttons got %v", got)
	}

	bare := appshell.New()
	bare.SetWindowButtons(appshell.WindowButtons{})
	if got := names(bare); len(got) != 0 {
		t.Fatalf("a desktop that asks for no buttons still got %v", got)
	}
}
