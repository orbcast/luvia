package tests

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
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
	if err := preferences.WriteSeedFile(path, map[string]string{"KANJU_SECRET": key}, preferences.SeedSalt()); err != nil {
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

// TestSeedProblemNamesTheFault pins the reporting that keeps a silent seed from
// looking like a healthy one: a rotated salt reports a foreign seal, a file that
// is not a seed reports a malformed one, a seed without the key reports the
// missing name, and a checkout with no seed reports nothing at all. None of the
// messages may carry the key.
func TestSeedProblemNamesTheFault(t *testing.T) {
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	resources := t.TempDir()
	if got, problem := preferences.InspectSeed(resources); got != "" || problem != preferences.SeedMissing {
		t.Fatalf("a checkout without a seed reported %q / %q", got, problem)
	}

	good := filepath.Join(resources, "kanju.seed")
	if err := preferences.WriteSeedFile(good, map[string]string{"KANJU_SECRET": key}, preferences.SeedSalt()); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	if got, problem := preferences.InspectSeed(resources); got != key || problem != preferences.SeedMissing {
		t.Fatalf("a healthy seed reported %d bytes / %q", len(got), problem)
	}

	// A flip in the ciphertext is what a seed from another build's salt looks
	// like: authenticated encryption cannot tell the two apart, and both mean
	// the same thing to whoever reads the log.
	sealed, err := os.ReadFile(good)
	if err != nil {
		t.Fatal(err)
	}
	sealed[len(sealed)-1] ^= 0xFF
	if err := os.WriteFile(good, sealed, 0o644); err != nil {
		t.Fatal(err)
	}
	_, problem := preferences.InspectSeed(resources)
	if problem != preferences.SeedForeign {
		t.Fatalf("a seed from another seal reported %q", problem)
	}
	if text := string(problem); text == "" || strings.Contains(text, key[:8]) {
		t.Fatalf("the report is empty or leaks the key: %q", text)
	}

	if err := os.WriteFile(good, []byte{9, 9, 9}, 0o644); err != nil {
		t.Fatal(err)
	}
	if _, problem := preferences.InspectSeed(resources); problem != preferences.SeedForeign {
		t.Fatalf("a file that is not a seed reported %q", problem)
	}

	if err := preferences.WriteSeedFile(good, map[string]string{"OTHER": key}, preferences.SeedSalt()); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	if got, problem := preferences.InspectSeed(resources); got != "" || problem != preferences.SeedEmpty {
		t.Fatalf("a seed without the key reported %q / %q", got, problem)
	}
}

// TestApplyAccessKeysPrefersTheReaderOverThePackage pins the startup order the
// release depends on: a key the environment or the settings page supplies wins
// over the bundled one, so shipping a seed never takes the choice away. The
// seed supplies the value only when neither of the other two has one.
func TestApplyAccessKeysPrefersTheReaderOverThePackage(t *testing.T) {
	const packaged = "packaged-key-do-not-use"
	const typed = "typed-key-do-not-use"
	resources := t.TempDir()
	if err := preferences.WriteSeedFile(filepath.Join(resources, "kanju.seed"),
		map[string]string{"KANJU_SECRET": packaged}, preferences.SeedSalt()); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	name := preferences.SeedVariable()
	userData := t.TempDir()

	// Nothing supplied: the package's key is what the engine sees.
	for _, kept := range []string{"", packaged} {
		t.Setenv(name, kept)
		if kept == "" {
			os.Unsetenv(name)
		}
		preferences.ApplyAccessKeys(userData, resources)
		if got := os.Getenv(name); got != packaged {
			t.Fatalf("without a reader's key the engine got %q", got)
		}
	}

	// The settings page wins: a value it saved must not be replaced by the seed.
	if err := preferences.SaveSecrets(userData, map[string]string{name: typed}); err != nil {
		t.Fatalf("save secrets: %v", err)
	}
	os.Unsetenv(name)
	preferences.ApplyAccessKeys(userData, resources)
	if got := os.Getenv(name); got != typed {
		t.Fatalf("the settings page lost to the package: engine got %q", got)
	}

	// The environment wins over both.
	t.Setenv(name, "exported-key-do-not-use")
	preferences.ApplyAccessKeys(userData, resources)
	if got := os.Getenv(name); got != "exported-key-do-not-use" {
		t.Fatalf("the environment lost: engine got %q", got)
	}
}

// TestSeedSaltTravelsWithTheBuild pins the fault that made every shipped
// package ask for a key: the salt used to be read from the environment at run
// time, so a reader's machine — which has no LUVIA_SEED_SALT — opened nothing.
// The salt now belongs to the build, and the packer asks the same source, so a
// seed this build writes is a seed this build opens.
func TestSeedSaltTravelsWithTheBuild(t *testing.T) {
	salt := preferences.SeedSalt()
	if len(salt) == 0 {
		t.Fatal("the build carries no salt, so no bundled seed could ever open")
	}
	path := filepath.Join(t.TempDir(), "kanju.seed")
	const key = "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	if err := preferences.WriteSeedFile(path, map[string]string{"KANJU_SECRET": key}, salt); err != nil {
		t.Fatalf("write seed: %v", err)
	}
	values, err := preferences.ReadSeedFile(path)
	if err != nil {
		t.Fatalf("read seed: %v", err)
	}
	if values["KANJU_SECRET"] != key {
		t.Fatalf("the seed got back %d bytes", len(values["KANJU_SECRET"]))
	}
	// A second build's salt is what a seed from another release looks like: it
	// must not open, or one leak would cover every release.
	other := append([]byte(nil), salt...)
	other[0] ^= 0xFF
	if _, err := preferences.OpenSeedForTest(path, other); err == nil {
		t.Fatal("a seed sealed under another salt opened")
	}
}
