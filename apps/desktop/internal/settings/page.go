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
	// SecretNames lists the variables the loaded rules ask for by name, and
	// Secrets returns the values stored for them. A rule signs its requests
	// with one, and an application started from the Finder or the Start menu
	// inherits no shell profile, so the page offers a field per name: a
	// reader supplies the value here instead of exporting it in a terminal.
	SecretNames func() []string
	Secrets     func() map[string]string
	SetSecrets  func(map[string]string)
}

// settingControlRadius and settingControlPaddingX are the shape every control
// on the page shares, whatever it changes.
const (
	settingControlRadius   float32 = 11
	settingControlPaddingX float32 = 14
	settingControlHeight   float32 = 38
	// settingControlWidth is the value column, shared by the drop-downs and the
	// access key fields so a card's right edge is one line.
	settingControlWidth float32 = 200
)

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
	// keys holds what the fields show, one per name in secretNames, and
	// keysSynced is what was last loaded from or written to the store.
	keys       []string
	keysSynced []string
	// editing is the index of the field that was last typed in, or -1, and
	// focusedField is the field that held focus in the last frame, or -1.
	// Both are needed: a value is stored when the field the reader typed in
	// is no longer the focused one.
	editing      int
	focusedField int
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
	p.editing = -1
	p.focusedField = -1
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

// View draws the page: its title above the preferences. The page is itself the
// card — the theme's surface inside a hairline, one body for a few decisions —
// and the groups are told apart by their names and the air between them, not by
// a stack of smaller cards inside a card.
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
	card := design.SettingCardSpec(&theme)
	column := ui.Column(c).Fill().Background(theme.Background).
		Padding(design.PageTopInset(c.TitleBar().Bottom), inset, 40+design.BottomClearance(width), inset).
		Gap(24)
	column.Children(func() {
		ui.Column(c).MaxWidth(560).Gap(20).Children(func() {
			ui.Text(c, m.Settings()).FontSize(26).Bold().TextColor(theme.Text)
			ui.Column(c).FillWidth().Radius(card.Radius).Background(card.Fill).Border(1, card.Border).
				Padding(22, 22, 14, 22).Gap(2).Children(func() {
				p.group(c, &theme, m.AppearanceAndLanguage(), func() {
					p.row(c, &theme, m.Appearance(), []string{m.FollowSystem(), m.Light(), m.Dark()}, &p.theme)
					p.rule(c, &theme)
					p.row(c, &theme, m.Language(), localeNames(m), &p.locale)
				})
			})
			// A second source is the first moment choosing one means
			// anything, so the row appears with it. The ids are the rule
			// directory names, which is also what a viewer recognises.
			names := p.secretNames()
			if len(ids) > 1 || len(names) > 0 {
				ui.Column(c).FillWidth().Radius(card.Radius).Background(card.Fill).Border(1, card.Border).
					Padding(22, 22, 14, 22).Gap(2).Children(func() {
					p.group(c, &theme, m.DataSourceAndKeys(), func() {
						if len(names) > 0 {
							p.keysRow(c, &theme, m)
							if len(ids) > 1 {
								p.rule(c, &theme)
							}
						}
						if len(ids) > 1 {
							p.row(c, &theme, m.DataSource(), ids, &p.source)
						}
					})
				})
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
	// A field hands its value over when it loses focus, so a reader is not
	// interrupted halfway through a secret, and the other fields keep what
	// they show. The fields report focus while they are drawn, so the frame
	// that follows a blur is the one that stores what was typed.
	if p.editing >= 0 && p.focusedField < 0 {
		p.commitKeys()
	}
	return column
}

// commitKeys stores what the fields hold. An empty field means the variable
// is left to the process environment, so it is dropped from the store.
func (p *Page) commitKeys() {
	if p.deps.SetSecrets == nil {
		p.keysSynced = append([]string{}, p.keys...)
		p.editing = -1
		return
	}
	if changed(p.keys, p.keysSynced) {
		values := make(map[string]string, len(p.keys))
		for i, name := range p.secretNames() {
			if i < len(p.keys) && strings.TrimSpace(p.keys[i]) != "" {
				values[name] = strings.TrimSpace(p.keys[i])
			}
		}
		p.deps.SetSecrets(values)
	}
	p.keysSynced = append([]string{}, p.keys...)
	p.editing = -1
}

// secretNames is the list the fields are built from.
func (p *Page) secretNames() []string {
	if p.deps.SecretNames == nil {
		return nil
	}
	return p.deps.SecretNames()
}

// changed reports whether two lists differ.
func changed(a, b []string) bool {
	if len(a) != len(b) {
		return true
	}
	for i := range a {
		if a[i] != b[i] {
			return true
		}
	}
	return false
}

// group is one group of settings: its name over the rows it holds. Its name is
// small and quiet, and the space above it separates it from the group before,
// so a card of several groups reads as a card of several topics.
func (p *Page) group(c *ui.Context, theme *ui.Theme, title string, body func()) {
	ui.Column(c).FillWidth().Gap(0).Children(func() {
		ui.Text(c, title).FontSize(12).Bold().LetterSpacing(0.4).TextColor(theme.TextMuted).
			Height(34).Padding(8, 0, 0, 0)
		body()
	})
}

// rule separates two rows inside a card, inset to the card's padding so it
// reads as a division between rows and not as the card's own edge.
func (p *Page) rule(c *ui.Context, theme *ui.Theme) {
	ui.Box(c).FillWidth().Height(1).Background(theme.Border)
}

// keysRow draws one field per variable the loaded rules ask for. A source
// without such a variable adds nothing to its card.
func (p *Page) keysRow(c *ui.Context, theme *ui.Theme, m locales.Messages) {
	names := p.secretNames()
	if len(names) == 0 || p.deps.Secrets == nil {
		p.keys = nil
		p.keysSynced = nil
		return
	}
	p.focusedField = -1
	stored := p.deps.Secrets()
	fresh := make([]string, len(names))
	for i, name := range names {
		fresh[i] = stored[name]
	}
	// Three cases, and no fourth: a field list that does not exist yet is
	// built from the store; one being typed in keeps what the reader typed;
	// one sitting idle follows the store, so a value written elsewhere or
	// cleared in the file shows up without a restart. The idle case only
	// applies once the list has a length, because a store with no value and
	// an empty field look the same and must not fight each other.
	// The slice the inputs edit is kept for its own lifetime: rebuilding it
	// every frame would hand the fields a new home mid-typing and lose what
	// was typed. It is rebuilt only when the set of names changes.
	if len(p.keys) != len(names) {
		p.keys = fresh
		p.keysSynced = append([]string{}, fresh...)
	} else if p.editing < 0 && changed(fresh, p.keysSynced) {
		copy(p.keys, fresh)
		p.keysSynced = append([]string{}, fresh...)
	}

	ui.Column(c).FillWidth().Gap(0).Children(func() {
		ui.Column(c).Gap(2).Padding(0, 0, 10, 0).Children(func() {
			ui.Text(c, m.AccessKeys()).FontSize(15).Bold().TextColor(theme.Text)
			ui.Text(c, m.AccessKeyHint()).FontSize(12.5).TextColor(theme.TextMuted)
		})
		for i, name := range names {
			i := i
			ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(20).Height(design.SettingRowHeight).Children(func() {
				ui.Text(c, name).FontSize(14).TextColor(theme.Text).Grow(1).
					Height(settingControlHeight).AlignSelf(ui.Center)
				// The field is dressed like the drop-downs beside it: the same
				// height, radius and body, with the hairline that turns into
				// the accent while it is being typed in. Two kinds of control
				// on one page should not read as two design languages.
				input := ui.TextInputBase(c, &p.keys[i]).Password().Width(settingControlWidth).
					Height(settingControlHeight).Radius(settingControlRadius).PaddingX(settingControlPaddingX).
					Background(theme.Surface).Border(1, theme.Border).
					TextColor(theme.Text).PlaceholderColor(theme.TextMuted).
					Placeholder(m.AccessKeyMissing()).Label(name)
				// The field keeps what was typed in its own value, so the
				// frame that follows typing draws the text rather than the
				// placeholder it started from.
				if input.Focused() {
					p.editing = i
					p.focusedField = i
					input.Border(1, theme.Accent)
				}
			})
		}
	})
}

// row is a setting: its name, and the drop-down choosing it, on one line.
func (p *Page) row(c *ui.Context, theme *ui.Theme, label string, options []string, selected *int) {
	ui.Row(c).FillWidth().AlignItems(ui.Center).Gap(20).Height(design.SettingRowHeight).Children(func() {
		ui.Text(c, label).FontSize(14).TextColor(theme.Text).Grow(1)
		// The drop-down and the key fields share one shape: the theme's surface
		// inside a hairline, the same radius and height, so a settings card is
		// cut from one set of controls.
		selectBase := ui.SelectBase(c, selected)
		selectBase.Trigger.Width(settingControlWidth).Height(settingControlHeight).Radius(settingControlRadius).
			PaddingX(settingControlPaddingX).Background(theme.Surface).Border(1, theme.Border).
			AlignItems(ui.Center).Gap(10).Label(label)
		selectBase.Trigger.Children(func() {
			// The name gets the row, the value control keeps its own width and
			// takes only what it needs: a long label in the narrow window wraps
			// instead of squeezing the drop-down to its text.
			ui.Text(c, options[*selected]).FontSize(14).TextColor(theme.Text).Grow(1).MaxLines(1)
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
