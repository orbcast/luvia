package tests

import (
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
}

func (p *fakePreferences) deps() settings.Deps {
	return settings.Deps{
		Theme:     func() mygo.ThemeSource { return p.theme },
		SetTheme:  func(source mygo.ThemeSource) { p.theme = source },
		Locale:    func() string { return p.locale },
		SetLocale: func(tag string) { p.locale = tag },
		Sources:   func() []string { return p.sources },
		Source:    func() string { return p.source },
		SetSource: func(id string) { p.source = id },
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
	choose(t, tt, "English", "Simplified Chinese")
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
	choose(t, tt, "简体中文", "英文")
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
	choose(t, tt, "简体中文", "英文")
	if prefs.locale != "en" {
		t.Fatalf("locale = %q, want en", prefs.locale)
	}
}
