package tests

import (
	"errors"
	"testing"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"luvia/apps/desktop/internal/settings"
)

// fakePreferences records what the page writes, as the window would.
type fakePreferences struct {
	theme   mygo.ThemeSource
	locale  string
	sources []string
	source  string
	names   []string
	secrets map[string]string
}

func (p *fakePreferences) deps() settings.Deps {
	return settings.Deps{
		Theme:       func() mygo.ThemeSource { return p.theme },
		SetTheme:    func(source mygo.ThemeSource) { p.theme = source },
		Locale:      func() string { return p.locale },
		SetLocale:   func(tag string) { p.locale = tag },
		Sources:     func() []string { return p.sources },
		Source:      func() string { return p.source },
		SetSource:   func(id string) { p.source = id },
		SecretNames: func() []string { return p.names },
		Secrets:     func() map[string]string { return p.secrets },
		SetSecrets:  func(values map[string]string) { p.secrets = values },
	}
}

func newPreferences() *fakePreferences {
	return &fakePreferences{theme: mygo.ThemeSystem, locale: "en"}
}

// choose opens the drop-down that shows trigger and picks option in it.
func choose(t *testing.T, tt *ui.Tester, trigger, option string) {
	t.Helper()
	if err := tt.Click(trigger); err != nil {
		t.Fatalf("open %q: %v", trigger, err)
	}
	tt.Frame()
	if err := tt.Click(option); err != nil {
		t.Fatalf("choose %q in %q: %v", option, trigger, err)
	}
	tt.Frame()
}

func TestSettingsOffersAppearanceAndLanguage(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{"Settings", "Appearance", "Follow System", "Language", "English"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if err := tt.Click("Follow System"); err != nil {
		t.Fatalf("open appearance: %v", err)
	}
	tt.Frame()
	for _, want := range []string{"Light", "Dark"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q among %v", want, tt.Texts())
		}
	}
}

func TestSettingsThemeChoiceIsSaved(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	choose(t, tt, "Follow System", "Dark")
	if prefs.theme != mygo.ThemeDark {
		t.Fatalf("theme = %q, want dark", prefs.theme)
	}
	choose(t, tt, "Dark", "Light")
	if prefs.theme != mygo.ThemeLight {
		t.Fatalf("theme = %q, want light", prefs.theme)
	}
}

func TestSettingsShowsExternalThemeChange(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	// The menu changes the preference behind the page's back.
	prefs.theme = mygo.ThemeDark
	tt.Frame()
	choose(t, tt, "Dark", "Light")
	if prefs.theme != mygo.ThemeLight {
		t.Fatalf("page did not adopt the stored theme: %q", prefs.theme)
	}
}

func TestSettingsLanguageChoiceIsSaved(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	// The drop-down names each language in the language shown, so a reader
	// who cannot read English still finds their own.
	choose(t, tt, "English", "简体中文")
	if prefs.locale != "zh-Hans" {
		t.Fatalf("locale = %q, want zh-Hans", prefs.locale)
	}
	tt.SetLocale("zh-Hans")
	tt.Frame()
	for _, want := range []string{"设置", "外观", "语言", "简体中文"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	choose(t, tt, "简体中文", "English")
	if prefs.locale != "en" {
		t.Fatalf("locale = %q, want en", prefs.locale)
	}
}

func TestSettingsHidesTheSourceChoiceWithOneSource(t *testing.T) {
	prefs := newPreferences()
	prefs.sources, prefs.source = []string{"kanju"}, "kanju"
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	if tt.HasText("Data source") {
		t.Fatalf("a single source still drew the row: %v", tt.Texts())
	}
	prefs.sources = nil
	tt.Frame()
	if tt.HasText("Data source") {
		t.Fatalf("no source still drew the row: %v", tt.Texts())
	}
}

func TestSettingsSourceChoiceIsSaved(t *testing.T) {
	prefs := newPreferences()
	prefs.sources, prefs.source = []string{"kanju", "hongguo"}, "kanju"
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Data source") {
		t.Fatalf("source row missing with two sources: %v", tt.Texts())
	}
	choose(t, tt, "kanju", "hongguo")
	if prefs.source != "hongguo" {
		t.Fatalf("source = %q, want hongguo", prefs.source)
	}
}

func TestSettingsShowsTheSystemLanguageWhenFollowingIt(t *testing.T) {
	prefs := newPreferences()
	prefs.locale = ""
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	if !tt.HasText("简体中文") {
		t.Fatalf("follow system language not shown: %v", tt.Texts())
	}
	choose(t, tt, "简体中文", "English")
	if prefs.locale != "en" {
		t.Fatalf("locale = %q, want en", prefs.locale)
	}
}

// TestSettingsOffersAFieldPerAccessKey covers the machine that never sees a
// shell profile: a rule names its secret as ${env:NAME}, so the page offers
// one field per name and stores what is typed there.
func TestSettingsOffersAFieldPerAccessKey(t *testing.T) {
	prefs := newPreferences()
	prefs.names = []string{"KANJU_SECRET"}
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	if !tt.HasText("访问密钥") {
		t.Fatalf("access key section missing: %v", tt.Texts())
	}
	if !tt.HasText("KANJU_SECRET") {
		t.Fatalf("variable name missing: %v", tt.Texts())
	}
	// Click the field's own box: the name is on a label beside it.
	x, y, err := fieldCentre(t, tt, prefs.names)
	if err != nil {
		t.Fatalf("field: %v", err)
	}
	tt.ClickAt(x, y)
	tt.Type("s3cr3t-value")
	// The value is handed over when the field loses focus, so a reader is
	// not interrupted halfway through a secret.
	tt.ClickAt(860, 560)
	tt.Frame()
	if got := prefs.secrets["KANJU_SECRET"]; got != "s3cr3t-value" {
		t.Fatalf("stored %q, want s3cr3t-value", got)
	}
}

// TestSettingsHidesAccessKeySectionsWithoutNames keeps a source that asks for
// no variable from growing an empty section.
func TestSettingsHidesAccessKeySectionsWithoutNames(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.SetLocale("zh-Hans")
	tt.Frame()
	if tt.HasText("访问密钥") {
		t.Fatalf("empty access key section drawn: %v", tt.Texts())
	}
}

// fieldCentre finds the box of the access key field, which sits to the right
// of the row that names it: the page draws the name, then a 200 DIP input.
func fieldCentre(t *testing.T, tt *ui.Tester, names []string) (float32, float32, error) {
	t.Helper()
	row, ok := tt.Find(names[0])
	if !ok {
		return 0, 0, errFieldMissing
	}
	return row.X + row.W + 100, row.Y + row.H/2, nil
}

var errFieldMissing = errors.New("access key row not drawn")

// TestFieldCentreFindsTheInput pins the geometry the access key test relies
// on: the name sits in the row, the input 200 DIPs to its right.
func TestFieldCentreFindsTheInput(t *testing.T) {
	prefs := newPreferences()
	prefs.names = []string{"KANJU_SECRET"}
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 600)
	tt.Frame()
	x, y, err := fieldCentre(t, tt, prefs.names)
	if err != nil {
		t.Fatal(err)
	}
	if x < 0 || y < 0 {
		t.Fatalf("field at (%v, %v)", x, y)
	}
}
