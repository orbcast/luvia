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
	"os"

	"luvia/apps/desktop/internal/preferences"
)

func main() {
	root := os.Getenv("LUVIA_SEED_ROOT")
	if root == "" {
		fmt.Fprintln(os.Stderr, "seedcheck: LUVIA_SEED_ROOT is required")
		os.Exit(2)
	}
	preferences.ApplyAccessKeys(os.Getenv("LUVIA_SCRATCH_HOME"), root)
	name := preferences.SeedVariable()
	if value := os.Getenv(name); value != "" {
		fmt.Printf("seedcheck: %s reached the environment (%d bytes)\n", name, len(value))
		return
	}
	// ApplyAccessKeys only logs why a seed did not land, so ask it directly to
	// name the fault.
	_, problem := preferences.InspectSeed(root)
	if problem == preferences.SeedMissing {
		fmt.Fprintf(os.Stderr, "seedcheck: %s: no seed, and no %s in the environment\n", root, name)
		os.Exit(1)
	}
	fmt.Fprintf(os.Stderr, "seedcheck: %s: %s\n", root, problem)
	os.Exit(1)
}
