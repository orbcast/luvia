package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/settings"
)

// TestSettingsGroupsItsRows covers the page's shape: the preferences sit in
// named groups on one card rather than loose on the canvas, and the access key
// field — when the loaded rules ask for one — shares the drop-downs' body
// instead of reading as a control from another design.
func TestSettingsGroupsItsRows(t *testing.T) {
	prefs := newPreferences()
	prefs.names = []string{"KANJU_SECRET"}
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 760)
	tt.SetLocale("en")
	tt.Frame()

	for _, want := range []string{
		"Settings",
		"Appearance and language",
		"Appearance", "Language",
		"Source and access keys",
		"Access keys", "KANJU_SECRET",
	} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}

	// The field and the drop-down that changes the language are one control
	// body: same height, and both inside the card's right-hand column.
	// The name sits in a box as tall as the page's controls, and that box ends
	// with the row's other text: an input taller than its row would swallow the
	// air the groups are told apart by.
	name, ok := tt.Find("KANJU_SECRET")
	if !ok {
		t.Fatalf("the field is missing in %v", tt.Texts())
	}
	field := name
	language, ok := tt.Find("English")
	if !ok {
		t.Fatalf("the language drop-down is missing in %v", tt.Texts())
	}
	if want := float32(38); field.H != want {
		t.Fatalf("the access key field is %v high, want %v: it does not match the drop-downs", field.H, want)
	}
	// The field's box shares the page's right-hand column with the drop-downs'
	// triggers: it stops where they stop, short of the card's padding.
	if field.X+field.W > language.X+language.W+0.5 {
		t.Fatalf("the field ends at %v, past the drop-down's %v: it is not in the controls' column",
			field.X+field.W, language.X+language.W)
	}
	if field.X < 100 {
		t.Fatalf("the field starts at %v: the name is not the row's first column", field.X)
	}
}

// TestSettingsPageStartsOnThePageLine covers the shared top rhythm: the page
// title starts on the same line the other navigation pages start their own.
func TestSettingsPageStartsOnThePageLine(t *testing.T) {
	prefs := newPreferences()
	page := settings.New(prefs.deps())
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 900, 700)
	tt.SetLocale("en")
	tt.Frame()

	title, ok := tt.Find("Settings")
	if !ok {
		t.Fatalf("no title in %v", tt.Texts())
	}
	if want := design.PageTopInset(0); title.Y != want {
		t.Fatalf("the title starts at y=%v, want %v: the page does not share the page line", title.Y, want)
	}
}
