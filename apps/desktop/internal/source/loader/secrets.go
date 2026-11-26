package loader

import (
	"sort"
	"strings"

	"luvia/apps/desktop/internal/source/rule"
)

// envMarker opens a value the rule leaves to the environment, as the engine
// reads it in ${env:NAME}.
const envMarker = "${env:"

// SecretNames lists the environment variables the loaded rules ask for, sorted
// and without duplicates. A rule says ${env:NAME} instead of writing a secret
// into the file, and the engine reads NAME from the process environment, which
// an application started from the Finder or the Start menu does not inherit
// from a shell profile. The settings page offers a field per name here, so a
// reader can supply one without a terminal.
//
// Only the three fields the engine expands with expandEnv are searched: the
// signature secret and the signature and transport headers. A step's own path,
// query and body, and the exchange's path, are parsed as templates, where
// ${env:X} is read as a selector and the rule is rejected at load time, so a
// variable cannot hide there; a test pins that.
func (s *Set) SecretNames() []string {
	seen := map[string]bool{}
	for _, parsed := range s.rules {
		collectEnvNames(parsed, seen)
	}
	names := make([]string, 0, len(seen))
	for name := range seen {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}

// collectEnvNames walks the string values a rule sends that the engine expands
// from the environment.
func collectEnvNames(r *rule.Rule, seen map[string]bool) {
	if r.Sign != nil {
		addEnvNames(r.Sign.Secret, seen)
		for _, header := range r.Sign.Headers {
			addEnvNames(header.Value, seen)
		}
	}
	for _, header := range r.HTTP.Headers {
		addEnvNames(header.Value, seen)
	}
}

// addEnvNames appends every ${env:NAME} hole in text.
func addEnvNames(text string, seen map[string]bool) {
	for {
		start := strings.Index(text, envMarker)
		if start < 0 {
			return
		}
		rest := text[start+len(envMarker):]
		end := strings.IndexByte(rest, '}')
		if end < 0 {
			return
		}
		if name := strings.TrimSpace(rest[:end]); name != "" {
			seen[name] = true
		}
		text = rest[end+1:]
	}
}
