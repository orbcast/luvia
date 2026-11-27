package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"luvia/apps/desktop/internal/preferences"
)

// TestSeedRoundTripCoversTheKey pins the release bundle's shape: what the
// packaging step writes is what the app opens, and the key is not in the file as
// text. The second half is the only claim worth testing — the sealing buys the
// cost of lifting the key, not its safety.
func TestSeedRoundTripCoversTheKey(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kanju.seed")
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := preferences.WriteSeedFile(path, map[string]string{"KANJU_SECRET": key}); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if bytes.Contains(data, []byte(key)) {
		t.Fatal("the sealed seed carries the key as text")
	}
	if len(data) < 28 {
		t.Fatalf("the seed is too small to hold a nonce and a tag: %d bytes", len(data))
	}
	values, err := preferences.ReadSeedFile(path)
	if err != nil {
		t.Fatalf("read seed back: %v", err)
	}
	if got := values["KANJU_SECRET"]; got != key {
		t.Fatalf("the seed gave %d bytes, want %d", len(got), len(key))
	}
}

// TestSeedSurvivesAnEmptyResourceDir covers the startup path's tolerance: a
// development build has no seed, and the app must start asking for the key
// instead of failing to open a file that was never packaged.
func TestSeedSurvivesAnEmptyResourceDir(t *testing.T) {
	if got := preferences.SeedValue(""); got != "" {
		t.Fatalf("an empty resource dir gave a key of %d bytes", len(got))
	}
	if got := preferences.SeedValue(filepath.Join(t.TempDir(), "absent")); got != "" {
		t.Fatalf("a missing seed gave a key of %d bytes", len(got))
	}
	// A file that is not a seed, or one from another release's salt, must not
	// half-open into garbage the engine would then sign with.
	foreign := filepath.Join(t.TempDir(), "resources")
	if err := os.MkdirAll(foreign, 0o755); err != nil {
		t.Fatal(err)
	}
	body := []byte{1, 'n', 'o', 't', 'a', 'n', 'o', 'n', 'c', 'e', 'x', 'x', 'x', 'x', 'x', 'x',
		'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x', 'x'}
	if err := os.WriteFile(filepath.Join(foreign, "kanju.seed"), body, 0o644); err != nil {
		t.Fatal(err)
	}
	if got := preferences.SeedValue(foreign); got != "" {
		t.Fatalf("a foreign seed opened into %d bytes", len(got))
	}
}
