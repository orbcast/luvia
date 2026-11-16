package appshell

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
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
	icons      [5]*ui.SVG
	onNavigate func(Page)
}

func New() *Shell {
	s := &Shell{}
	for i, source := range solarSources {
		s.icons[i] = ui.MustParseSVG([]byte(source))
	}
	return s
}
func (s *Shell) Selected() Page              { return s.selected }
func (s *Shell) SetOnNavigate(fn func(Page)) { s.onNavigate = fn }
func (s *Shell) Navigate(p Page) {
	if p < Discover || p > Settings {
		return
	}
	s.selected = p
	if s.onNavigate != nil {
		s.onNavigate(p)
	}
}

func (s *Shell) View(c *ui.Context) {
	theme := desktopTheme(c.Theme())
	theme = *theme.WithPreferences(c.Preferences())
	c.SetTheme(&theme)
	m := locales.In(c.Locale())
	for i, p := range pages {
		c.OnShortcut(ui.Super, []ui.Key{ui.Key1, ui.Key2, ui.Key3, ui.Key4, ui.Key5}[i], func() { s.Navigate(p) })
	}
	c.OnShortcut(ui.Super, ui.KeyComma, func() { s.Navigate(Settings) })
	_, height := c.Size()
	bar := c.TitleBar()
	ui.Column(c).Fill().Background(theme.Background).Children(func() {
		ui.Row(c).Absolute().Top(0).Left(bar.Left).Right(bar.Right).
			Height(max(titleBarHeight, bar.Bottom)).DragWindow()
		ui.Column(c).Key("app-shell").Absolute().Left(shellInset).
			Top(max(bar.Bottom+titleBarGap, (height-shellHeight)/2)).Width(shellWidth).Padding(shellPadding).Gap(shellGap).
			Radius(shellRadius).Role(ui.RoleNavigation).Label(m.Navigation()).Material(glass.Glass{}).Children(func() {
			for _, p := range pages {
				label := p.Label(m)
				button := ui.ButtonBase(c).Key(int(p)).Size(navigationWidth, navigationHeight).Radius(navigationRadius).
					Label(label).Tooltip(label).TextColor(theme.TextMuted).Center()
				if p == s.selected {
					button.Background(theme.Inverse).TextColor(theme.InverseText).Current(ui.CurrentPage)
					if button.Hovered() || button.Pressed() {
						button.Background(theme.Inverse.Mix(theme.InverseText, 0.10))
					}
				} else if button.Pressed() {
					button.Background(theme.SurfacePressed).TextColor(theme.Text)
				} else if button.Hovered() {
					button.Background(theme.SurfaceHover).TextColor(theme.Text)
				}
				button.Children(func() { ui.Icon(c, s.icons[p]).Size(navigationIconSize, navigationIconSize) })
				if button.Clicked() {
					s.Navigate(p)
				}
			}
		})
	})
}
