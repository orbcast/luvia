package main

import (
	"bytes"
	"path/filepath"
	"strings"
	"testing"

	"luvia/apps/desktop/internal/preferences"
)

func TestSeedpackValidation(t *testing.T) {
	var out, errBuf bytes.Buffer
	if code := run(nil, func(string) string { return "" }, func() []byte { return []byte("salt") }, &out, &errBuf); code != 2 {
		t.Fatalf("missing -out code = %d, want 2", code)
	}

	errBuf.Reset()
	if code := run([]string{"-out", filepath.Join(t.TempDir(), "kanju.seed")}, func(string) string { return "" }, func() []byte { return []byte("salt") }, &out, &errBuf); code != 1 {
		t.Fatalf("missing env code = %d, want 1", code)
	}

	errBuf.Reset()
	if code := run([]string{"-out", filepath.Join(t.TempDir(), "kanju.seed"), "-names", "K,"}, func(k string) string { return "v" }, func() []byte { return nil }, &out, &errBuf); code != 1 {
		t.Fatalf("empty salt code = %d, want 1", code)
	}
	if !strings.Contains(errBuf.String(), "seed salt is not configured") {
		t.Fatalf("unexpected stderr: %q", errBuf.String())
	}
}

func TestSeedpackSuccess(t *testing.T) {
	outPath := filepath.Join(t.TempDir(), "kanju.seed")
	var out, errBuf bytes.Buffer
	code := run(
		[]string{"-out", outPath, "-names", preferences.SeedVariable()},
		func(string) string { return "top-secret" },
		func() []byte { return []byte("0123456789abcdef") },
		&out,
		&errBuf,
	)
	if code != 0 {
		t.Fatalf("code = %d, want 0 (stderr=%q)", code, errBuf.String())
	}
	if !strings.Contains(out.String(), "sealed 1 value(s)") {
		t.Fatalf("unexpected stdout: %q", out.String())
	}
}
