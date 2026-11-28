// Command seedpack writes the sealed access-key bundle a release carries beside
// its rules. It takes the named variables from the environment, seals them, and
// writes the result where the app reads it at startup.
//
// The file it writes is not a secret in the cryptographic sense. It keeps the
// key out of the archive as plain text, so lifting it takes instrumenting the
// program rather than opening a file, and its salt comes from the build, so a
// leak belongs to one release. See internal/preferences/seed.go for the full
// account of what this does and does not buy.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"strings"

	"luvia/apps/desktop/internal/preferences"
)

func main() {
	os.Exit(run(os.Args[1:], os.Getenv, preferences.SeedSalt, os.Stdout, os.Stderr))
}

func run(args []string, getenv func(string) string, seedSalt func() []byte, stdout, stderr io.Writer) int {
	fs := flag.NewFlagSet("seedpack", flag.ContinueOnError)
	fs.SetOutput(stderr)
	out := fs.String("out", "", "where to write the sealed seed")
	names := fs.String("names", preferences.SeedVariable(), "comma-separated variable names to seal")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if *out == "" {
		fmt.Fprintln(stderr, "seedpack: -out is required")
		return 2
	}
	values := map[string]string{}
	for _, name := range strings.Split(*names, ",") {
		if name == "" {
			continue
		}
		value := getenv(name)
		if value == "" {
			// A release with no key would look packaged and fail on the reader's
			// machine; stopping here keeps the failure where it was caused.
			fmt.Fprintf(stderr, "seedpack: %s is not set\n", name)
			return 1
		}
		values[name] = value
	}
	// The salt comes from the same place the app reads it: preferences owns the
	// hex decoding, so the packer and the shipped binary cannot end up with two
	// different keys for the same value.
	salt := seedSalt()
	if len(salt) == 0 {
		// Without a salt the packer would seal under a key the app cannot
		// rebuild, and the failure would only show on a reader's machine.
		fmt.Fprintln(stderr, "seedpack: seed salt is not configured for this build")
		return 1
	}
	if err := preferences.WriteSeedFile(*out, values, salt); err != nil {
		fmt.Fprintf(stderr, "seedpack: %v\n", err)
		return 1
	}
	fmt.Fprintf(stdout, "seedpack: sealed %d value(s) into %s\n", len(values), *out)
	return 0
}
