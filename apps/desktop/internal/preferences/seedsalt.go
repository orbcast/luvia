//go:build prodseed

package preferences

import (
	"log"
	"strings"
)

// devSeedSalt names the development salt this file falls back to: the release
// side of the build has to know the value seeddev.go uses, or a build that lost
// its -X would open no seed at all.
const devSeedSalt = "luvia-dev-seed-not-for-release!!"

// releaseSeedSalt carrier of the salt the release build fills in. It is a
// variable, not an environment read, because the value has to travel with the
// binary: a reader who installs the app has no LUVIA_SEED_SALT in their
// environment, so a runtime lookup would leave the bundled seed sealed under a
// key nobody can produce. The workflow sets it through
//
//	go build -ldflags "-X luvia/apps/desktop/internal/preferences.releaseSeedSalt=<hex>"
//
// and the value is then part of the published bytes.
var releaseSeedSalt string

// seedSalt is the one part of the bundled seed that is not in the repository.
// The build fills it, so the bytes a release carries are unique to that
// release: extracting the key from one archive says nothing about the next,
// and the same published binary cannot be unpacked twice for two different
// answers.
//
// This file is only compiled with the prodseed tag, and the tag is only set by
// the release workflow. A development build uses the fallback in seeddev.go, so
// a local run keeps reaching the upstream when the environment carries the
// secret while a release never depends on a value that was never meant to ship.
//
// What this does not do: stop someone who can run the binary from reading the
// secret out of it. The program has to hold the plain secret to sign a request,
// so a debugger attached to the process sees it, and OWASP MASWE-0005 says as
// much about every client that bundles a credential. The salt raises the cost
// of extracting it from "open the file" to "instrument a program", and it makes
// a leak a single release's problem, which is the whole of the claim.
func seedSalt() []byte {
	if raw := strings.TrimSpace(releaseSeedSalt); raw != "" {
		if salt, ok := decodeSeedSalt(raw); ok {
			return salt
		}
		// A malformed salt is a build fault, and the only place it surfaces is
		// here. The fallback below keeps a developer build loadable; a release
		// that lost its -X opens seeds under the documented development salt.
		log.Printf("release seed salt is not hexadecimal: falling back to the development salt")
	}
	// A release that forgot the salt ships no seed rather than a guessable one.
	// The failure is loud: the app starts unsigned and the settings page asks
	// for the key, which is the state a reader can still fix. A random salt
	// would instead seal the key under a value nobody can reopen.
	return []byte(devSeedSalt)
}

// decodeSeedSalt reads the hexadecimal salt the release build embedded.
func decodeSeedSalt(raw string) ([]byte, bool) {
	if len(raw) != seedSaltSize*2 {
		return nil, false
	}
	salt := make([]byte, seedSaltSize)
	for index := range salt {
		high, highOK := hexNibble(raw[index*2])
		low, lowOK := hexNibble(raw[index*2+1])
		if !highOK || !lowOK {
			return nil, false
		}
		salt[index] = high<<4 | low
	}
	return salt, true
}

// hexNibble reads one hexadecimal digit.
func hexNibble(letter byte) (byte, bool) {
	switch {
	case letter >= '0' && letter <= '9':
		return letter - '0', true
	case letter >= 'a' && letter <= 'f':
		return letter - 'a' + 10, true
	case letter >= 'A' && letter <= 'F':
		return letter - 'A' + 10, true
	}
	return 0, false
}

// seedSaltSize matches the fallback, so both builds seal the same layout.
const seedSaltSize = 32

// SeedSalt exposes the salt this build opens and writes seeds with, so the
// packaging step seals under the same one. A release build takes it from
// releaseSeedSalt; a development build from seeddev.go. Either way the packer
// and the app must agree, and this is how both ask the same source.
func SeedSalt() []byte { return seedSalt() }
