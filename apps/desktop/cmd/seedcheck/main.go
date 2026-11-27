// Command seedcheck opens the seed a built package carries, using the same
// salt the build embedded, and exits non-zero when it cannot. It is the release
// workflow's answer to the fault that shipped a package nobody could use: the
// packer sealed the key under one salt while the binary held another, and every
// build stayed green because nothing opened the file the way the app would.
//
// It reads the directory from LUVIA_SEED_ROOT and nothing else: no environment
// variable stands in for the salt, so the check can only pass when the build
// itself carries it.
package main

import (
	"fmt"
	"os"
	"strings"

	"luvia/apps/desktop/internal/preferences"
)

func main() {
	root := os.Getenv("LUVIA_SEED_ROOT")
	if root == "" {
		fmt.Fprintln(os.Stderr, "seedcheck: LUVIA_SEED_ROOT is required")
		os.Exit(2)
	}
	value, problem := preferences.InspectSeed(root)
	if problem != preferences.SeedMissing {
		fmt.Fprintf(os.Stderr, "seedcheck: %s: %s\n", root, problem)
		os.Exit(1)
	}
	// The value is the upstream key, so only its shape is reported.
	if !strings.HasPrefix(value, "ums_") || len(value) < 32 {
		fmt.Fprintf(os.Stderr, "seedcheck: the seed carries %d bytes, which is not a key\n", len(value))
		os.Exit(1)
	}
	fmt.Printf("seedcheck: %s opened the seed (%d bytes)\n", root, len(value))
}
