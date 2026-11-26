package tests

import (
	"github.com/egoist/mygo"
	"luvia/apps/desktop/internal/appshell"
	"luvia/apps/desktop/internal/preferences"
	"os"
	"path/filepath"
	"runtime"
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

func TestPreferenceSourceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences", "source.json")
	if got := preferences.LoadSource(path); got != "" {
		t.Fatalf("first launch: %q", got)
	}
	for _, want := range []string{"kanju", "hongguo"} {
		if err := preferences.SaveSource(path, want); err != nil {
			t.Fatal(err)
		}
		if got := preferences.LoadSource(path); got != want {
			t.Fatalf("roundtrip: %q, want %q", got, want)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			t.Fatalf("temporary files left behind: %v %v", entries, err)
		}
	}
}

func TestPreferenceSourceFailures(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "source.json")
	for _, data := range []string{"{", "", `{"source":""}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := preferences.LoadSource(path); got != "" {
			t.Fatalf("bad file %q read as %q", data, got)
		}
	}
	// A directory where the file belongs reads as no choice, as a missing file
	// does: the app opens with its own source either way.
	if got := preferences.LoadSource(dir); got != "" {
		t.Fatalf("directory read as %q", got)
	}
	// No id means no choice, so there is nothing to write and no file appears.
	empty := filepath.Join(dir, "empty", "source.json")
	if err := preferences.SaveSource(empty, ""); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(empty); !os.IsNotExist(err) {
		t.Fatalf("empty save wrote a file: %v", err)
	}
	if err := preferences.SaveSource(filepath.Join(path, "source.json"), "kanju"); err == nil {
		t.Fatal("expected mkdir failure")
	}
	if err := preferences.SaveSource(dir, "kanju"); err == nil {
		t.Fatal("expected rename failure")
	}
	if entries, err := os.ReadDir(dir); err != nil || len(entries) != 1 {
		t.Fatalf("failed save left files: %v %v", entries, err)
	}
}

func TestLocalePreferenceRoundTrip(t *testing.T) {
	path := filepath.Join(t.TempDir(), "preferences", "locale.json")
	if got := preferences.LoadLocale(path); got != "" {
		t.Fatalf("first launch: %q", got)
	}
	for _, want := range []string{"zh-Hans", "en", ""} {
		if err := preferences.SaveLocale(path, want); err != nil {
			t.Fatal(err)
		}
		if got := preferences.LoadLocale(path); got != want {
			t.Fatalf("roundtrip: %q, want %q", got, want)
		}
		entries, err := os.ReadDir(filepath.Dir(path))
		if err != nil || len(entries) != 1 {
			t.Fatalf("temporary files left behind: %v %v", entries, err)
		}
	}
}

func TestLocalePreferenceFailures(t *testing.T) {
	path := filepath.Join(t.TempDir(), "locale.json")
	for _, data := range []string{"{", `{"locale":42}`} {
		if err := os.WriteFile(path, []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := preferences.LoadLocale(path); got != "" {
			t.Fatalf("bad file did not fall back: %q %q", data, got)
		}
	}
}

// TestDefaultLocaleIsChinese keeps the first launch on a system whose
// language the app is not written in from showing English, the language of
// the catalogs: the app opens in Simplified Chinese instead.
func TestDefaultLocaleIsChinese(t *testing.T) {
	if appshell.DefaultLocale != "zh-Hans" {
		t.Fatalf("default locale: %q", appshell.DefaultLocale)
	}
}

// TestSecretsRoundTrip covers the file a machine without a shell profile
// needs: a rule asks for ${env:NAME}, the page stores NAME, and the next
// launch reads it back.
func TestSecretsRoundTrip(t *testing.T) {
	dir := t.TempDir()
	if got := preferences.LoadSecrets(dir); got != nil {
		t.Fatalf("first launch: %v", got)
	}
	want := map[string]string{"KANJU_SECRET": "abc123", "OTHER": "xyz"}
	if err := preferences.SaveSecrets(dir, want); err != nil {
		t.Fatal(err)
	}
	got := preferences.LoadSecrets(dir)
	if len(got) != len(want) {
		t.Fatalf("loaded %v, want %v", got, want)
	}
	for name, value := range want {
		if got[name] != value {
			t.Fatalf("%s = %q, want %q", name, got[name], value)
		}
	}
	// The file holds a secret: only the owner may read it. Windows decides
	// access with ACLs rather than permission bits, so the mode is only
	// meaningful where the platform has them.
	if runtime.GOOS != "windows" {
		info, err := os.Stat(filepath.Join(dir, "secrets.json"))
		if err != nil {
			t.Fatal(err)
		}
		if info.Mode().Perm() != 0600 {
			t.Fatalf("mode %v, want 0600", info.Mode().Perm())
		}
	}
	// An empty set removes the file rather than leaving an empty object.
	if err := preferences.SaveSecrets(dir, nil); err != nil {
		t.Fatal(err)
	}
	if got := preferences.LoadSecrets(dir); got != nil {
		t.Fatalf("after clearing: %v", got)
	}
}

// TestSecretsIgnoreBrokenFiles keeps a half-written file from stopping the
// application: the rules then ask for their variables the way they did
// before the file existed.
func TestSecretsIgnoreBrokenFiles(t *testing.T) {
	dir := t.TempDir()
	for _, data := range []string{"{", `{"KANJU_SECRET": 42}`, `{"": "x", "KANJU_SECRET": "  "}`} {
		if err := os.WriteFile(filepath.Join(dir, "secrets.json"), []byte(data), 0600); err != nil {
			t.Fatal(err)
		}
		if got := preferences.LoadSecrets(dir); got != nil {
			t.Fatalf("%q loaded as %v", data, got)
		}
	}
}

// TestApplySecretsLeavesTheEnvironmentInCharge keeps a shell that exports
// its own copy from being overridden by a stored value.
func TestApplySecretsLeavesTheEnvironmentInCharge(t *testing.T) {
	t.Setenv("LU-EXPORTED", "from-shell")
	if err := os.Unsetenv("LU-STORED"); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Unsetenv("LU-STORED") })
	preferences.ApplySecrets(map[string]string{"LU-EXPORTED": "from-file", "LU-STORED": "from-file"})
	if got := os.Getenv("LU-EXPORTED"); got != "from-shell" {
		t.Fatalf("exported value replaced with %q", got)
	}
	if got := os.Getenv("LU-STORED"); got != "from-file" {
		t.Fatalf("stored value not applied: %q", got)
	}
}
