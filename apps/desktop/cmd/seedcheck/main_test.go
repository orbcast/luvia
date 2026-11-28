package main

import (
	"bytes"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"luvia/apps/desktop/internal/preferences"
)

func TestSeedcheckMissingRoot(t *testing.T) {
	var out, errBuf bytes.Buffer
	code := run(func(string) string { return "" }, &out, &errBuf)
	if code != 2 {
		t.Fatalf("code = %d, want 2", code)
	}
	if !strings.Contains(errBuf.String(), "LUVIA_SEED_ROOT is required") {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}
}

func TestSeedcheckMissingSeedAndEnv(t *testing.T) {
	t.Setenv(preferences.SeedVariable(), "")
	dir := t.TempDir()
	var out, errBuf bytes.Buffer
	code := run(func(k string) string {
		if k == "LUVIA_SEED_ROOT" {
			return dir
		}
		return t.TempDir()
	}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "no seed") {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}
}

func TestSeedcheckMalformedSeedAndEnvSuccess(t *testing.T) {
	t.Setenv(preferences.SeedVariable(), "")
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "kanju.seed"), []byte("bad"), 0o600); err != nil {
		t.Fatal(err)
	}
	var out, errBuf bytes.Buffer
	code := run(func(k string) string {
		if k == "LUVIA_SEED_ROOT" {
			return dir
		}
		return t.TempDir()
	}, &out, &errBuf)
	if code != 1 {
		t.Fatalf("code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), string(preferences.SeedForeign)) {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}

	t.Setenv(preferences.SeedVariable(), "secret-val")
	out.Reset()
	errBuf.Reset()
	code = run(func(k string) string {
		if k == "LUVIA_SEED_ROOT" {
			return dir
		}
		return t.TempDir()
	}, &out, &errBuf)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "reached the environment") {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}
