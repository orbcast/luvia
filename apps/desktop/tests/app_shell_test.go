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
				// The column starts on the window's one chrome line, which
				// keeps the system title bar's room first.
				top := design.ChromeTop(28) + design.ShellPadding
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

// TestChromeTopClearsTheSystemTitleBar covers where the window's chrome starts:
// on the shell's top margin where no title bar holds the system's controls, and
// under that bar's room plus the same margin where one does, so the column never
// tucks under the buttons.
func TestChromeTopClearsTheSystemTitleBar(t *testing.T) {
	if got := design.ChromeTop(0); got != design.ShellTop {
		t.Fatalf("chrome starts at %v without a title bar, not at %v", got, design.ShellTop)
	}
	if got, want := design.ChromeTop(28), float32(28+design.ShellTop); got != want {
		t.Fatalf("chrome starts at %v under a 28 title bar, not %v", got, want)
	}
}

// TestAppShellNativeWindowButtons covers where the platform draws the window's
// buttons itself: macOS keeps its traffic lights, and every other desktop gets
// the capsule the shell draws.
func TestAppShellNativeWindowButtons(t *testing.T) {
	for goos, native := range map[string]bool{"darwin": true, "windows": false, "linux": false} {
		if got := appshell.NativeWindowButtons(goos); got != native {
			t.Errorf("on %s the platform draws the window buttons: %v, want %v", goos, got, native)
		}
	}
}

// TestAppShellDrawsWindowButtons covers the window controls the shell draws in
// place of the system's: the desktop's layout decides which it has and where,
// and a desktop that asks for none gets none.
func TestAppShellDrawsWindowButtons(t *testing.T) {
	buttons := func(s *appshell.Shell) (names []string, bounds ui.Rect) {
		tt := ui.NewTester(s.View, 1100, 720)
		tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})
		tt.SetLocale("en")
		for _, node := range tt.AccessAll() {
			if node.Role != ui.RoleButton || !strings.HasSuffix(node.Name, "window") {
				continue
			}
			names = append(names, node.Name)
			box := node.Bounds
			if bounds.W == 0 {
				bounds = box
				continue
			}
			left, top := min(bounds.X, box.X), min(bounds.Y, box.Y)
			bounds = ui.Rect{X: left, Y: top, W: max(bounds.X+bounds.W, box.X+box.W) - left, H: max(bounds.Y+bounds.H, box.Y+box.H) - top}
		}
		return names, bounds
	}
	capsule := func(names int) (w, h float32) {
		n := float32(names)
		return n*design.WindowControlWidth + (n-1)*design.WindowControlGap + 2*design.WindowControlPadding,
			design.WindowControlHeight + 2*design.WindowControlPadding
	}

	s := appshell.New()
	s.SetWindowButtons(appshell.WindowButtons{End: []string{"minimize", "maximize", "close"}})
	names, bounds := buttons(s)
	// The capsule floats below the desktop's title bar, one gap under the room
	// the system's own controls take, the buttons in its padding.
	ty := design.ChromeTop(28) + design.WindowControlPadding
	if len(names) != 3 {
		t.Fatalf("a desktop with three window buttons got %v", names)
	}
	if names[0] != "Minimize window" || names[2] != "Close window" {
		t.Fatalf("the buttons read %v, not a title bar's order", names)
	}
	// The capsule floats one inset in from the window's edge; the buttons sit
	// in its padding, so together they are the capsule's width less that.
	w, _ := capsule(3)
	w -= 2 * design.WindowControlPadding
	// The capsule's hairline border takes a pixel of the room its padding
	// asked for, so the buttons end within one of it.
	edge := float32(1100 - design.WindowControlInset - design.WindowControlPadding)
	if d := bounds.X + bounds.W - edge; d < -2 || d > 1 {
		t.Fatalf("the window controls end at %v, not %v", bounds.X+bounds.W, edge)
	}
	if bounds.W != w || bounds.H != design.WindowControlHeight {
		t.Fatalf("the window controls are %vx%v, not %vx%v", bounds.W, bounds.H, w, design.WindowControlHeight)
	}
	if d := bounds.Y - ty; d < 0 || d > 1 {
		t.Fatalf("the buttons start at %v, not at the capsule's %v", bounds.Y, ty)
	}

	bare := appshell.New()
	bare.SetWindowButtons(appshell.WindowButtons{})
	if got, _ := buttons(bare); len(got) != 0 {
		t.Fatalf("a desktop that asks for no buttons still got %v", got)
	}
}

// TestAppShellWindowControlsTakeTheirPlaces covers the capsule's inner layout:
// one hit area per button, apart by the gap the design sets, and at the start
// of the title bar where the desktop puts them there.
func TestAppShellWindowControlsTakeTheirPlaces(t *testing.T) {
	s := appshell.New()
	s.SetWindowButtons(appshell.WindowButtons{Start: []string{"close", "minimize", "maximize"}})

	tt := ui.NewTester(s.View, 1100, 720)
	tt.SetTitleBar(ui.TitleBar{Height: 28, Bottom: 28, Left: 78})
	tt.SetLocale("en")

	boxes := make(map[string]ui.Rect, 3)
	for _, node := range tt.AccessAll() {
		if node.Role == ui.RoleButton && strings.HasSuffix(node.Name, "window") {
			boxes[node.Name] = node.Bounds
		}
	}
	reset, ok := boxes["Close window"]
	if !ok {
		t.Fatalf("the close button is missing among %v", boxes)
	}
	wantStart := float32(design.WindowControlInset + design.WindowControlPadding)
	if d := reset.X - wantStart; d < 0 || d > 1 {
		t.Fatalf("the close button starts at %v, not at the capsule's start %v", reset.X, wantStart)
	}
	for _, name := range []string{"Minimize window", "Maximize window"} {
		box, ok := boxes[name]
		if !ok {
			t.Fatalf("%s is missing among %v", name, boxes)
		}
		if box.W != design.WindowControlWidth || box.H != design.WindowControlHeight {
			t.Fatalf("%s is %vx%v, not %vx%v", name, box.W, box.H, design.WindowControlWidth, design.WindowControlHeight)
		}
		if box.Y != reset.Y {
			t.Fatalf("%s sits at %v, not beside the close button at %v", name, box.Y, reset.Y)
		}
	}
	prev := reset
	for _, name := range []string{"Minimize window", "Maximize window"} {
		box := boxes[name]
		if gap := box.X - (prev.X + prev.W); gap != design.WindowControlGap {
			t.Fatalf("%s follows %v apart, not %v", name, gap, design.WindowControlGap)
		}
		prev = box
	}
}

// TestAppShellKeepsThePlayersTopButtonsClickable covers the strip that drags
// the window by its title bar: the player hangs its own cluster of buttons at
// the top of the picture, and a strip laid over it took their presses and moved
// the window instead, so the panel toggle did nothing while the player filled
// the screen.
func TestAppShellKeepsThePlayersTopButtonsClickable(t *testing.T) {
	s := appshell.New()
	pressed := 0
	// The player's top cluster, at the place the player puts it: the pill
	// starts below the marker's line, inside the title bar's height.
	s.SetContent(func(c *ui.Context) ui.Element {
		button := ui.ButtonBase(c).Key("probe").Absolute().Right(24).Top(9.5).Size(34, 34).
			Label("Show panel").Background(ui.Transparent)
		button.Children(func() { ui.Text(c, "Show panel").FontSize(1) })
		if button.Clicked() {
			pressed++
		}
		return ui.Box(c).Fill()
	})

	tt := ui.NewTester(s.View, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	// The player owns the window: its own buttons take the presses at the top.
	s.SetNavigationHidden(true)
	tt.Frame()
	if err := tt.Click("Show panel"); err != nil {
		t.Fatalf("the player's top button is unreachable: %v", err)
	}
	tt.Frame()
	if pressed != 1 {
		t.Fatalf("the strip took the press meant for the player's button: %d clicks", pressed)
	}
}
