package tests

import (
	"github.com/egoist/mygo"
	"luvia/apps/desktop/internal/preferences"
	"os"
	"path/filepath"
	"testing"
)

func TestThemePreferenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences", "theme.json")
	source, err := preferences.LoadTheme(path)
	if err != nil || source != mygo.ThemeSystem {
		t.Fatalf("first launch: %v %v", source, err)
	}
	for _, want := range []mygo.ThemeSource{mygo.ThemeDark, mygo.ThemeLight, mygo.ThemeSystem} {
		if err := preferences.SaveTheme(path, want); err != nil {
			t.Fatal(err)
		}
		got, err := preferences.LoadTheme(path)
		if err != nil || got != want {
			t.Fatalf("roundtrip: %v %v", got, err)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			t.Fatalf("temporary files left behind: %v %v", entries, err)
		}
	}
}
func TestThemePreferenceFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "theme.json")
	for _, data := range []string{"{", "{}", `{"source":"unknown"}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		source, err := preferences.LoadTheme(path)
		if err == nil || source != mygo.ThemeSystem {
			t.Fatalf("bad file did not fall back: %s %v %v", data, source, err)
		}
	}
	if _, err := preferences.LoadTheme(dir); err == nil {
		t.Fatal("expected read failure")
	}
	if err := preferences.SaveTheme(path, mygo.ThemeSource("invalid")); err == nil {
		t.Fatal("invalid source accepted")
	}
	if err := preferences.SaveTheme(filepath.Join(path, "theme.json"), mygo.ThemeDark); err == nil {
		t.Fatal("expected mkdir failure")
	}
	if err := preferences.SaveTheme(dir, mygo.ThemeDark); err == nil {
		t.Fatal("expected rename failure")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("failed save left files: %v %v", entries, err)
	}
}
