package appshell

import (
	"strconv"
	"time"

	"github.com/egoist/mygo"
	mglass "github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/glass"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/locales"
)

//go:generate python3 -B ../../../../scripts/generate-desktop-icons.py

// playerTopEdge is how much of the window's top the player's own buttons
// leave free to drag the window by: its cluster hangs below the title bar's
// line, and only the line itself, where no button sits, stays a drag strip.
const playerTopEdge float32 = 9

// Page identifies an application-level destination. Its content is not implemented yet.
type Page int

const (
	Discover Page = iota
	Library
	Following
	Search
	Settings
)

var pages = [...]Page{Discover, Library, Following, Search, Settings}

// navigationIcons names each destination's outline and filled figure: the
// selected destination takes the filled one.
var navigationIcons = [...][2]string{
	{"home-linear", "home-bold"},
	{"clapperboard-linear", "clapperboard-bold"},
	{"heart-linear", "heart-bold"},
	{"magnifer-linear", "magnifer-bold"},
	{"settings-linear", "settings-bold"},
}

// Pages returns the fixed navigation order without exposing shared mutable state.
func Pages() [5]Page { return pages }
func (p Page) Label(m locales.Messages) string {
	switch p {
	case Discover:
		return m.Discover()
	case Library:
		return m.Library()
	case Following:
		return m.Following()
	case Search:
		return m.Search()
	case Settings:
		return m.Settings()
	default:
		return ""
	}
}

// Shell owns the AppShell navigation state, never frame-scoped UI elements.
type Shell struct {
	selected   Page
	icons      [5][2]*ui.SVG
	onNavigate func(Page)
	content    func(*ui.Context) ui.Element

	// docked is the layout the bar shows, and lastWidth the window width
	// the frame before: a change of layout waits for the first frame the
	// window size held still in, since resizing a window does not animate
	// and the column-to-dock move is a layout change like any other.
	docked    bool
	lastWidth float32
	pending   bool
	// navHidden hides the navigation chrome on immersive screens such as the
	// title page and the player, which cover the whole window. The title page
	// keeps the window's buttons; the player hides them.
	navHidden bool
	// controlsHidden hides the window buttons the shell draws while the player
	// covers the window: the picture is the interface there, and a minimize or
	// close beside its own controls reads as one more player button. A platform
	// that draws its own buttons keeps them there.
	controlsHidden bool

	// controls are the window buttons the shell draws on a window that hides
	// the system's, and window is the window they act on. layout says where the
	// desktop puts them, and an empty layout means it asks for none, as under a
	// tiling compositor or where the platform draws its own, as macOS does.
	controls map[string]WindowControl
	window   *mygo.Window
	layout   WindowButtons
}

func New() *Shell {
	s := &Shell{lastWidth: -1, controls: make(map[string]WindowControl, len(windowControls))}
	for i, names := range navigationIcons {
		for j, name := range names {
			s.icons[i][j] = icons.Parse(name)
		}
	}
	for _, control := range windowControls {
		s.controls[control.key] = control
	}
	return s
}

// SetWindow installs the window the drawn window controls act on. Without it
// the controls only draw.
func (s *Shell) SetWindow(w *mygo.Window) { s.window = w }

// SetWindowButtons installs the desktop's title bar layout: where it puts the
// window's buttons and which it has. An empty layout draws none, for a window
// under a desktop that decorates them itself.
func (s *Shell) SetWindowButtons(layout WindowButtons) { s.layout = layout }

// WindowButtons is the title bar layout of a window's controls: the names
// "minimize", "maximize" and "close", at the title bar's start or its end.
type WindowButtons struct {
	Start, End []string
}

// Empty reports that the layout has no buttons at either end.
func (b WindowButtons) Empty() bool { return len(b.Start) == 0 && len(b.End) == 0 }

// Has reports that the layout puts button at one of its ends.
func (b WindowButtons) Has(button string) bool {
	for _, name := range append(b.Start, b.End...) {
		if name == button {
			return true
		}
	}
	return false
}

func (s *Shell) Selected() Page                  { return s.selected }
func (s *Shell) SetNavigationHidden(hidden bool) { s.navHidden = hidden }

// SetWindowControlsHidden hides the window buttons the shell draws while the
// player owns the window, and brings them back after. The window still drags by
// the strip the buttons leave. Where the platform draws its own buttons, as
// macOS does, there is nothing here to hide.
func (s *Shell) SetWindowControlsHidden(hidden bool) { s.controlsHidden = hidden }
func (s *Shell) SetOnNavigate(fn func(Page))         { s.onNavigate = fn }

// SetContent installs the page that fills the window behind the navigation
// column. The navigation column and the window drag strip stay above it.
func (s *Shell) SetContent(fn func(*ui.Context) ui.Element) { s.content = fn }
func (s *Shell) Navigate(p Page) {
	if p < Discover || p > Settings {
		return
	}
	s.selected = p
	if s.onNavigate != nil {
		s.onNavigate(p)
	}
}

// navigationMotion carries the AppShell between the side column and the
// bottom dock: the bar and its buttons share one transition, so they arrive
// together instead of jumping to the new layout.
var navigationMotion = ui.ElementTransition{
	Duration:  260 * time.Millisecond,
	Animation: &ui.Snappy,
	// Naming a property takes the others off, so Position and Size are
	// listed with Colors: the bar and its buttons carry their place, their
	// size and their tint across the column-to-dock move.
	Position: true,
	Size:     true,
	Colors:   true,
}

// pageMotion fades a page in as it takes the content area, so switching
// tabs reads as a change of place rather than a jump cut.
var pageMotion = ui.ElementTransition{
	Duration: 220 * time.Millisecond,
	Enter:    &ui.Motion{Opacity: 0, Y: 10},
	Exit:     &ui.Motion{Opacity: 0, Y: -6},
}

func (s *Shell) View(c *ui.Context) {
	theme := design.Theme(c.Theme())
	theme = *theme.WithPreferences(c.Preferences())
	c.SetTheme(&theme)
	m := locales.In(c.Locale())
	for i, p := range pages {
		c.OnShortcut(ui.Super, []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5}[i], func() { s.Navigate(p) })
	}
	c.OnShortcut(ui.Super, ui.KeyComma, func() { s.Navigate(Settings) })
	width, height := c.Size()
	bar := c.TitleBar()
	ui.Column(c).Fill().Background(theme.Background).Children(func() {
		if s.content != nil {
			ui.Box(c).Key("page:" + strconv.Itoa(int(s.selected))).Absolute().Left(0).Right(0).Top(0).Bottom(0).
				Transition(pageMotion).Children(func() {
				s.content(c)
			})
		}
		// The strip that drags the window by its title bar gives the player its
		// top edge: the player hangs its own cluster of buttons there, and a
		// strip laid over it would take their presses and move the window
		// instead. The player is the one screen that fills the window and keeps
		// the whole title line a drag strip of its own, so the shell leaves it
		// the room its buttons need.
		strip := max(design.TitleBarHeight, bar.Bottom)
		if s.navHidden {
			strip = playerTopEdge
		}
		// The strip gives way to the window buttons: it only covers the title
		// line where nothing else does, so it still drags the window by the
		// room around the capsule, and a press on a window button at the top
		// left of a page that hides the navigation, as the title page does,
		// still reaches that button. The page's own top-left control takes its
		// own presses: nothing is laid over it.
		s.dragStrip(c, s.TitleStrip(width, strip), s.controlsHidden, bar)
		if !s.controlsHidden {
			s.windowControls(c, m, bar, width)
		}
		// The AppShell is placed by the window width: a dock along the bottom
		// when it narrows, or, at the left edge, two panels split around the
		// destinations that belong at the bottom. Panels and buttons are
		// siblings so a button keeps its identity as it moves between layouts
		// and the transition carries it.
		docked := s.columnOrDock(c, width)
		panels := navPanels(docked, width, height, bar.Bottom)
		if s.navHidden {
			panels = nil
		}
		for _, panel := range panels {
			ui.Box(c).Key(panel.key).Absolute().Left(panel.left).Top(panel.top).
				Width(panel.width).Height(panel.height).Radius(design.ShellRadius).
				Role(ui.RoleNavigation).Label(m.Navigation()).
				Material(mglass.Glass{}).ZIndex(10).Transition(navigationMotion)
		}
		for _, panel := range panels {
			for i, p := range panel.pages {
				x, y := panel.left+design.ShellPadding, panel.top+design.ShellPadding
				if panel.stacked {
					y += float32(i) * (design.NavigationHeight + design.ShellGap)
				} else {
					x += float32(i) * (design.NavigationWidth + design.ShellGap)
				}
				s.navigationButton(c, &theme, m, p, x, y)
			}
		}
	})
}

// WindowControl is one button of the window controls Luvia draws on a window
// that hides the system's (mygo.WindowOptions.HideWindowButtons): these take
// their place. macOS draws its own, so nothing here is drawn there.
type WindowControl struct {
	key   string
	label func(locales.Messages) string
	act   func(*mygo.Window)
}

// NativeWindowButtons reports whether the platform draws the window's close,
// minimize and maximize buttons itself, over the page: macOS keeps its traffic
// lights at the window's top-left, and its own gesture and menu behavior with
// them. Where it does, the window keeps the system's buttons and the shell
// draws none; where it does not, as on Windows and Linux, the window hides them
// and the shell draws the set the desktop would have put in its title bars.
func NativeWindowButtons(goos string) bool { return goos == "darwin" }

// windowControls are the buttons a title bar asks Luvia for by name, in the
// order a title bar reads them: the minimize dash, the maximize square, and
// the close cross at the end.
var windowControls = [3]WindowControl{
	{key: "minimize", label: locales.Messages.MinimizeWindow,
		act: func(w *mygo.Window) {
			if w != nil {
				w.Minimize()
			}
		}},
	{key: "maximize", label: locales.Messages.MaximizeWindow,
		act: func(w *mygo.Window) {
			if w != nil {
				w.ToggleMaximize()
			}
		}},
	{key: "close", label: locales.Messages.CloseWindow,
		act: func(w *mygo.Window) {
			if w != nil {
				w.Close()
			}
		}},
}

// windowControlGlyph is the figure of each window button Solar draws: the dash
// of minimize and the cross of close. The maximize square has no figure in the
// collection that is a bare square, so windowControlSquare draws it.
var windowControlGlyph = map[string]string{
	"minimize": "minus-linear",
	"close":    "close-linear",
}

// windowControlSquare draws the maximize figure a title bar uses, reading the
// window itself: a square alone while it is unmaximized, and the same square
// with a second one stepped behind it at the bottom right while it is
// maximized, the figure that restores it, as every desktop draws the pair. It
// paints after the button's children, so the window's state it reads is the
// one the click just left, and none of the button's figure shows on top of it.
func windowControlSquare(p *ui.Painter, r ui.Rect, w *mygo.Window) {
	maximized := w != nil && w.IsMaximized()
	for _, box := range windowControlSquareFrames(r, maximized) {
		p.Stroke(box, design.WindowControlInk(), 1.5, 1.3)
	}
}

// windowControlSquareFrames are the squares of windowControlSquare, as boxes
// to stroke on a button of box r.
func windowControlSquareFrames(r ui.Rect, maximized bool) []ui.Rect {
	const (
		size = 10
		step = 3
	)
	left := r.X + (r.W-size-step)/2
	top := r.Y + (r.H-size-step)/2
	if maximized {
		return []ui.Rect{
			{X: left + step, Y: top + step, W: size, H: size},
			{X: left, Y: top, W: size, H: size},
		}
	}
	return []ui.Rect{{X: left + step/2, Y: top + step/2, W: size, H: size}}
}

// TitleStrip is the band the window drags by across the top of the window: the
// title bar's height, or the player's own top edge.
func (s *Shell) TitleStrip(width, strip float32) ui.Rect {
	return ui.Rect{X: 0, Y: 0, W: width, H: strip}
}

// dragStrip lays the window's drag handle over the title band, minus the
// capsule of window buttons that shares the band with it: the strip above the
// capsule, the band below it, and the room beside it on either end still drag
// the window, so a press on a window button reaches that button while the
// title line around it keeps moving the window.
func (s *Shell) dragStrip(c *ui.Context, band ui.Rect, controlsHidden bool, bar ui.TitleBar) {
	var hole ui.Rect
	skip := false
	if !controlsHidden {
		hole, skip = s.WindowControlCapsule(bar, band.W)
	}
	segments := []ui.Rect{band}
	if skip {
		segments = CutRect(band, hole)
	}
	for _, seg := range segments {
		if seg.W <= 0 || seg.H <= 0 {
			continue
		}
		ui.Row(c).Absolute().Left(seg.X).Top(seg.Y).Width(seg.W).Height(seg.H).
			DragWindow().ZIndex(20)
	}
}

// CutRect returns what is left of seg once hole is taken out of it, as the
// bands above and below the hole plus the part of its row beside it.
func CutRect(seg, hole ui.Rect) []ui.Rect {
	// A hole clear of the segment leaves it whole.
	if hole.X >= seg.X+seg.W || seg.X >= hole.X+hole.W || hole.Y >= seg.Y+seg.H || seg.Y >= hole.Y+hole.H {
		return []ui.Rect{seg}
	}
	var out []ui.Rect
	if above := hole.Y - seg.Y; above > 0 {
		out = append(out, ui.Rect{X: seg.X, Y: seg.Y, W: seg.W, H: above})
	}
	if below := seg.Y + seg.H - (hole.Y + hole.H); below > 0 {
		out = append(out, ui.Rect{X: seg.X, Y: hole.Y + hole.H, W: seg.W, H: below})
	}
	top, bottom := max(seg.Y, hole.Y), min(seg.Y+seg.H, hole.Y+hole.H)
	if bottom > top {
		if left := hole.X - seg.X; left > 0 {
			out = append(out, ui.Rect{X: seg.X, Y: top, W: left, H: bottom - top})
		}
		if right := seg.X + seg.W - (hole.X + hole.W); right > 0 {
			out = append(out, ui.Rect{X: hole.X + hole.W, Y: top, W: right, H: bottom - top})
		}
	}
	return out
}

// windowControlNames is the buttons the desktop's layout asks for: the end of
// the title bar holding the cluster decides which end is filled.
func (s *Shell) windowControlNames() (names []string, atStart bool) {
	atStart = len(s.layout.Start) > 0
	if atStart {
		return s.layout.Start, true
	}
	return s.layout.End, false
}

// WindowControlCapsule is where the capsule of window buttons sits on a window
// of the given size: one inset from the window's edge, on ChromeTop below the
// title bar's room.
func (s *Shell) WindowControlCapsule(bar ui.TitleBar, width float32) (ui.Rect, bool) {
	names, atStart := s.windowControlNames()
	if len(names) == 0 {
		return ui.Rect{}, false
	}
	n := float32(len(names))
	capsuleWidth := n*design.WindowControlWidth + (n-1)*design.WindowControlGap + 2*design.WindowControlPadding
	top := design.ChromeTop(bar.Bottom)
	left := width - design.WindowControlInset - capsuleWidth
	if atStart {
		left = design.WindowControlInset
	}
	return ui.Rect{X: left, Y: top, W: capsuleWidth, H: design.WindowControlHeight + 2*design.WindowControlPadding}, true
}

// windowControls draws the window's buttons in the capsule the AppShell floats
// over the page: at the start of the title bar on Linux, at its end there and
// on Windows. It draws none under a desktop that decorates windows itself and
// asks for none, and none where the platform draws its own, as macOS does.
func (s *Shell) windowControls(c *ui.Context, m locales.Messages, bar ui.TitleBar, width float32) {
	capsule, ok := s.WindowControlCapsule(bar, width)
	if !ok {
		return
	}
	names, _ := s.windowControlNames()
	left, top := capsule.X, capsule.Y
	cluster := ui.Row(c).Key("window-controls").Absolute().Left(left).Top(top).Radius(design.WindowControlRadius).
		Padding(design.WindowControlPadding).Gap(design.WindowControlGap).AlignItems(ui.Center).ZIndex(21).
		Material(glass.Surface{
			Fill:      design.WindowControlFill(),
			Border:    design.WindowControlBorder(),
			Highlight: design.WindowControlHighlight(),
			Shadow:    design.WindowControlShadow(),
			Lift:      10, Softness: 28, Spread: -10,
			Blur: 14,
		})
	cluster.Children(func() {
		for _, name := range names {
			control, ok := s.controls[name]
			if !ok {
				continue
			}
			s.windowControl(c, m, control)
		}
	})
}

// windowControl draws one window button: a figure on the capsule that only
// lights as the pointer reaches it. The close button is the one that takes a
// color there, red, as leaving the window is the one promise a color can keep.
func (s *Shell) windowControl(c *ui.Context, m locales.Messages, control WindowControl) {
	label := control.label(m)
	button := ui.ButtonBase(c).Key("window-"+control.key).
		Size(design.WindowControlWidth, design.WindowControlHeight).Radius(design.WindowControlRadius).
		Label(label).Tooltip(label).TextColor(design.WindowControlInk()).Center()
	fill := ui.Transparent
	ink := design.WindowControlInk()
	if !button.Pressed() {
		if button.Hovered() {
			fill, ink = design.WindowControlHover(), design.WindowControlInkHover()
			if control.key == "close" {
				fill = design.CloseHover()
			}
		}
	}
	button.Background(fill).TextColor(ink)
	button.Children(func() {
		if control.key == "maximize" {
			ui.Box(c).Size(design.WindowControlIconSize, design.WindowControlIconSize).
				DrawOver(func(p *ui.Painter, r ui.Rect) { windowControlSquare(p, r, s.window) })
			return
		}
		ui.Icon(c, icons.Parse(windowControlGlyph[control.key])).Size(design.WindowControlIconSize, design.WindowControlIconSize)
	})
	if button.Clicked() {
		control.act(s.window)
	}
}

// navPanel is one piece of glass holding some destinations, either stacked in
// a column or laid out in a row.
type navPanel struct {
	key     string
	left    float32
	top     float32
	width   float32
	height  float32
	stacked bool
	pages   []Page
}

// navPanels lays the destinations out for the window: down the left edge as a
// top group and a bottom group, or along the bottom as a single row.
func navPanels(docked bool, width, height, barBottom float32) []navPanel {
	if docked {
		w := float32(len(pages))*design.NavigationWidth + float32(len(pages)-1)*design.ShellGap + 2*design.ShellPadding
		return []navPanel{{
			key:    "app-shell",
			left:   (width - w) / 2,
			top:    height - design.ShellInset - design.DockHeight,
			width:  w,
			height: design.DockHeight,
			pages:  pages[:],
		}}
	}
	// The column starts on the same line as the window controls' capsule in
	// the other corner, so the window's top reads as one row.
	top := design.ChromeTop(barBottom)
	// Search and settings sit at the bottom edge of the window, apart from
	// the destinations that open browsing.
	pinned := 2
	upper := len(pages) - pinned
	return []navPanel{
		{
			key:     "app-shell-top",
			left:    design.ShellInset,
			top:     top,
			width:   design.ShellWidth,
			height:  design.ShellGroupHeight(upper),
			stacked: true,
			pages:   pages[:upper],
		},
		{
			key:     "app-shell-bottom",
			left:    design.ShellInset,
			top:     height - design.ShellInset - design.ShellGroupHeight(pinned),
			width:   design.ShellWidth,
			height:  design.ShellGroupHeight(pinned),
			stacked: true,
			pages:   pages[upper:],
		},
	}
}

// columnOrDock reports the layout to draw this frame. The layout follows the
// width on the first frame at a new width; a switch waits one frame so the
// bar's move is animated rather than flattened by the resize.
func (s *Shell) columnOrDock(c *ui.Context, width float32) bool {
	settled := width == s.lastWidth
	s.lastWidth = width
	want := design.Docked(width)
	if want == s.docked {
		s.pending = false
		return s.docked
	}
	if settled || s.pending {
		s.docked, s.pending = want, false
		return s.docked
	}
	s.pending = true
	c.Invalidate()
	return s.docked
}

// navigationButton draws one square destination that reads the theme's fill
// as its selected state. Its offset is given, so the bar can lay the same
// buttons out as a column or a row and animate between them.
func (s *Shell) navigationButton(c *ui.Context, theme *ui.Theme, m locales.Messages, p Page, left, top float32) {
	label := p.Label(m)
	button := ui.ButtonBase(c).Key(int(p)).Absolute().Left(left).Top(top).
		Size(design.NavigationWidth, design.NavigationHeight).Radius(design.NavigationRadius).
		Label(label).Tooltip(label).TextColor(theme.TextMuted).Center().
		ZIndex(11).Transition(navigationMotion)
	if p == s.selected {
		// The selected destination wears transparent grey glass, and its
		// filled figure takes the theme's text color: white in dark, black
		// in light.
		button.Material(mglass.Glass{Tint: design.NavigationTint()}).TextColor(theme.Text).Current(ui.CurrentPage)
		if button.Hovered() || button.Pressed() {
			button.Material(mglass.Glass{Tint: design.NavigationTint(), Interactive: true})
		}
	} else if button.Pressed() {
		button.Background(theme.SurfacePressed).TextColor(theme.Text)
	} else if button.Hovered() {
		button.Background(theme.SurfaceHover).TextColor(theme.Text)
	}
	figure := s.icons[p][0]
	if p == s.selected {
		figure = s.icons[p][1]
	}
	button.Children(func() { ui.Icon(c, figure).Size(design.NavigationIconSize, design.NavigationIconSize) })
	if button.Clicked() {
		s.Navigate(p)
	}
}
