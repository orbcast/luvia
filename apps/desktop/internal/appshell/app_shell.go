package appshell

import (
	"strconv"
	"time"

	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/locales"
)

//go:generate python3 -B ../../../../scripts/generate-desktop-icons.py

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
	// title page and the player, which cover the whole window.
	navHidden bool
}

func New() *Shell {
	s := &Shell{lastWidth: -1}
	for i, names := range navigationIcons {
		for j, name := range names {
			s.icons[i][j] = icons.Parse(name)
		}
	}
	return s
}
func (s *Shell) Selected() Page                  { return s.selected }
func (s *Shell) SetNavigationHidden(hidden bool) { s.navHidden = hidden }
func (s *Shell) SetOnNavigate(fn func(Page))     { s.onNavigate = fn }

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
		ui.Row(c).Absolute().Top(0).Left(bar.Left).Right(bar.Right).
			Height(max(design.TitleBarHeight, bar.Bottom)).DragWindow().ZIndex(20)
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
				Material(glass.Glass{}).ZIndex(10).Transition(navigationMotion)
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
	top := barBottom + design.NavigationTopGap
	if top < barBottom+design.TitleBarGap {
		top = barBottom + design.TitleBarGap
	}
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
		button.Material(glass.Glass{Tint: design.NavigationTint()}).TextColor(theme.Text).Current(ui.CurrentPage)
		if button.Hovered() || button.Pressed() {
			button.Material(glass.Glass{Tint: design.NavigationTint(), Interactive: true})
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
