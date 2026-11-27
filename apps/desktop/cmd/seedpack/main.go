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
	"os"
	"strings"

	"luvia/apps/desktop/internal/preferences"
)

func main() {
	out := flag.String("out", "", "where to write the sealed seed")
	names := flag.String("names", preferences.SeedVariable(), "comma-separated variable names to seal")
	flag.Parse()
	if *out == "" {
		fmt.Fprintln(os.Stderr, "seedpack: -out is required")
		os.Exit(2)
	}
	values := map[string]string{}
	for _, name := range strings.Split(*names, ",") {
		if name == "" {
			continue
		}
		value := os.Getenv(name)
		if value == "" {
			// A release with no key would look packaged and fail on the reader's
			// machine; stopping here keeps the failure where it was caused.
			fmt.Fprintf(os.Stderr, "seedpack: %s is not set\n", name)
			os.Exit(1)
		}
		values[name] = value
	}
	if err := preferences.WriteSeedFile(*out, values); err != nil {
		fmt.Fprintf(os.Stderr, "seedpack: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("seedpack: sealed %d value(s) into %s\n", len(values), *out)
}
