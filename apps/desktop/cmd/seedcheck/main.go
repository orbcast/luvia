// Command seedcheck reads a packaged directory the way a fresh install would:
// it lets ApplyAccessKeys run and then reads the upstream key back out of the
// process environment. Exiting zero means this build opened its seed and the key
// reached the code that signs requests.
//
// It exists because a green build is not the same as a usable package. Twice the
// release shipped a seed sealed under a salt the binary did not hold, and no
// check noticed: inspecting the file's bytes was not the same as reading it the
// way the app does.
//
// Only LUVIA_SEED_ROOT is read from the environment. The salt came in at link
// time, so the check can pass only when the build carries it.
package main

import (
	"fmt"
	"io"
	"os"

	"luvia/apps/desktop/internal/preferences"
)

func main() {
	os.Exit(run(os.Getenv, os.Stdout, os.Stderr))
}

func run(getenv func(string) string, stdout, stderr io.Writer) int {
	root := getenv("LUVIA_SEED_ROOT")
	if root == "" {
		fmt.Fprintln(stderr, "seedcheck: LUVIA_SEED_ROOT is required")
		return 2
	}
	preferences.ApplyAccessKeys(getenv("LUVIA_SCRATCH_HOME"), root)
	name := preferences.SeedVariable()
	if value := os.Getenv(name); value != "" {
		fmt.Fprintf(stdout, "seedcheck: %s reached the environment (%d bytes)\n", name, len(value))
		return 0
	}
	// ApplyAccessKeys only logs why a seed did not land, so ask it directly to
	// name the fault.
	_, problem := preferences.InspectSeed(root)
	if problem == preferences.SeedMissing {
		fmt.Fprintf(stderr, "seedcheck: %s: no seed, and no %s in the environment\n", root, name)
		return 1
	}
	fmt.Fprintf(stderr, "seedcheck: %s: %s\n", root, problem)
	return 1
}
