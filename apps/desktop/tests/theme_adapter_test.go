package tests

import (
	"github.com/egoist/mygo"
	"luvia/apps/desktop/internal/preferences"
	"os"
	"path/filepath"
	"testing"
)

func TestThemePlatformAdapter(t *testing.T) {
	source := mygo.ThemeDark
	apply := func(value mygo.ThemeSource) { source = value }
	path := filepath.Join(t.TempDir(), "theme.json")
	preferences.RestoreTheme(path, apply)
	if source != mygo.ThemeSystem {
		t.Fatal("missing preference must follow system")
	}
	preferences.ChangeTheme(path, mygo.ThemeDark, apply)
	if source != mygo.ThemeDark {
		t.Fatal("theme not applied")
	}
	source = mygo.ThemeLight
	preferences.RestoreTheme(path, apply)
	if source != mygo.ThemeDark {
		t.Fatal("saved theme was not restored")
	}
	if err := os.WriteFile(path, []byte("bad"), 0600); err != nil {
		t.Fatal(err)
	}
	preferences.RestoreTheme(path, apply)
	if source != mygo.ThemeSystem {
		t.Fatal("corrupt preference did not fall back")
	}
	preferences.ChangeTheme(filepath.Join(path, "blocked.json"), mygo.ThemeLight, apply)
	if source != mygo.ThemeLight {
		t.Fatal("failed persistence must still allow a session theme")
	}
}
