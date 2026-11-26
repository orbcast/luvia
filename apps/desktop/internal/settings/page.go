package settings

import (
	"strings"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/icons"
	"luvia/apps/desktop/locales"
)

// Deps supplies the stored preferences. The page only reads them: a choice
// goes back through SetTheme, SetLocale or SetSource, which own persistence.
type Deps struct {
	Theme     func() mygo.ThemeSource
	SetTheme  func(mygo.ThemeSource)
	Locale    func() string
	SetLocale func(string)
	// Sources lists the source ids a viewer may choose, and Source is the
	// one in force. The drop-down is drawn only when there is a choice, so
	// a single source adds no row.
	Sources   func() []string
	Source    func() string
	SetSource func(string)
}

// themeSources is the order of the appearance drop-down's choices.
var themeSources = [...]mygo.ThemeSource{mygo.ThemeSystem, mygo.ThemeLight, mygo.ThemeDark}

// localeTags is the order of the language drop-down's choices, shared with
// the menu so the two never disagree. The follow system choice stays
// reachable from the menu: when it is in force the drop-down shows the
// language now in effect.
var localeTags = appshell.LocaleTags()

// Page is the settings view. It holds no preference of its own; the
// controls write the choice back, so the menu and the page always agree.
type Page struct {
	deps Deps

	// theme and locale are the indices the drop-downs show; the synced
	// ones are what the stored preference last said, so a choice made here
	// is told apart from one made elsewhere.
	theme        int
	themeSynced  int
	locale       int
	localeSynced int
	source       int
	sourceSynced int
}

// New builds the page over the stored preferences.
func New(deps Deps) *Page {
	p := &Page{deps: deps}
	p.theme = themeIndex(deps.Theme())
	p.themeSynced = p.theme
	p.locale = localeIndex(deps.Locale())
	// The language drawn comes from the frame's locale, so the follow
	// system choice settles on the first frame.
	p.localeSynced = -1
	p.source = sourceIndex(p.sources(), p.current())
	p.sourceSynced = p.source
	return p
}

// sourceIndex is where id sits in the offered sources, or 0 when it is not
// among them, as before the stored choice was made or after it stopped
// loading.
func sourceIndex(ids []string, id string) int {
	for i, it := range ids {
		if it == id {
			return i
		}
	}
	return 0
}

// sources are the ids on offer, asked again on every frame so a rule that
// was added or dropped shows up without a restart.
func (p *Page) sources() []string {
	if p.deps.Sources == nil {
		return nil
	}
	return p.deps.Sources()
}

// current is the id in force, or "" when no source answered.
func (p *Page) current() string {
	if p.deps.Source == nil {
		return ""
	}
	return p.deps.Source()
}

func themeIndex(source mygo.ThemeSource) int {
	for i, it := range themeSources {
		if it == source {
			return i
		}
	}
	return 0
}

// localeNames names the languages of the drop-down in the language of m,
// as the menu does, so the choice stays findable whatever is shown now.
func localeNames(m locales.Messages) []string {
	names := make([]string, 0, len(localeTags))
	for _, tag := range localeTags {
		names = append(names, appshell.LocaleName(m, tag))
	}
	return names
}

func localeIndex(tag string) int {
	for i, it := range localeTags {
		if tag == it || strings.HasPrefix(tag, it) {
			return i
		}
	}
	return 0
}

// View draws the page: a title and one drop-down row per preference, set
// on the page's background with room around them rather than in a card.
func (p *Page) View(c *ui.Context) ui.Element {
	theme := design.Theme(c.Theme())
	theme = *theme.WithPreferences(c.Preferences())
	c.SetTheme(&theme)
	m := locales.In(c.Locale())

	// A change made elsewhere, as from the menu, reaches the drop-down
	// before the frame draws it.
	if want := themeIndex(p.deps.Theme()); want != p.themeSynced {
		p.theme, p.themeSynced = want, want
	}
	tag := p.deps.Locale()
	if tag == "" {
		// Follow system: show the language in effect.
		tag = c.Locale()
	}
	if want := localeIndex(tag); want != p.localeSynced {
		p.locale, p.localeSynced = want, want
	}
	ids := p.sources()
	if want := sourceIndex(ids, p.current()); want != p.sourceSynced {
		p.source, p.sourceSynced = want, want
	}

	width, _ := c.Size()
	inset := design.Inset(width)
	column := ui.Column(c).Fill().Background(theme.Background).
		Padding(56, inset, 40+design.BottomClearance(width), inset).Gap(34)
	column.Children(func() {
		ui.Column(c).MaxWidth(560).Gap(30).Children(func() {
			ui.Text(c, m.Settings()).FontSize(26).Bold().TextColor(theme.Text)
			p.row(c, &theme, m.Appearance(), []string{m.FollowSystem(), m.Light(), m.Dark()}, &p.theme)
			p.row(c, &theme, m.Language(), localeNames(m), &p.locale)
			// A second source is the first moment choosing one means
			// anything, so the row appears with it. The ids are the rule
			// directory names, which is also what a viewer recognises.
			if len(ids) > 1 {
				p.row(c, &theme, m.DataSource(), ids, &p.source)
			}
		})
	})

	// The drop-downs wrote a new choice into the indices: hand it over.
	if p.theme != p.themeSynced {
		p.deps.SetTheme(themeSources[p.theme])
		p.themeSynced = p.theme
	}
	if p.locale != p.localeSynced {
		p.deps.SetLocale(localeTags[p.locale])
		p.localeSynced = p.locale
	}
	if p.source != p.sourceSynced && p.source < len(ids) && p.deps.SetSource != nil {
		p.deps.SetSource(ids[p.source])
		p.sourceSynced = p.source
	}
	return column
}

// row is a setting: its name, and the drop-down choosing it, on one line.
func (p *Page) row(c *ui.Context, theme *ui.Theme, label string, options []string, selected *int) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(20).Children(func() {
		ui.Text(c, label).FontSize(15).TextColor(theme.Text).Grow(1)
		selectBase := ui.SelectBase(c, selected)
		selectBase.Trigger.Width(200).Height(38).Radius(12).PaddingX(14).
			AlignItems(ui.Center).Gap(10).Material(glass.Glass{}).Label(label)
		selectBase.Trigger.Children(func() {
			ui.Text(c, options[*selected]).FontSize(14).TextColor(theme.Text).Grow(1)
			ui.Icon(c, icons.Parse("alt-arrow-down-linear")).Size(14, 14).TextColor(theme.TextMuted)
		})
		selectBase.Popup(func(panel ui.Element) {
			panel.Padding(6).Radius(14).Material(glass.Glass{})
			for i, option := range options {
				item := selectBase.Item(i).Padding(8, 12).Radius(9)
				if item.Highlighted() {
					item.Background(design.NavigationTint())
				}
				item.Children(func() {
					ui.Text(c, option).FontSize(14).TextColor(theme.Text)
				})
			}
		})
	})
}
