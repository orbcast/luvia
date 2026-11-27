//go:build prodseed

package preferences

import "os"

// seedSalt is the one part of the bundled seed that is not in the repository.
// The build fills it from LUVIA_SEED_SALT, so the bytes a release carries are
// unique to that release: extracting the key from one archive says nothing
// about the next, and the same published binary cannot be unpacked twice for
// two different answers.
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
	// A release that forgot the salt ships no seed rather than a guessable one.
	// The failure is loud: the app starts unsigned and the settings page asks
	// for the key, which is the state a reader can still fix. A random salt
	// would instead seal the key under a value nobody can reopen.
	raw := os.Getenv("LUVIA_SEED_SALT")
	if len(raw) != seedSaltSize*2 {
		return nil
	}
	salt := make([]byte, seedSaltSize)
	for index := range salt {
		high, highOK := hexNibble(raw[index*2])
		low, lowOK := hexNibble(raw[index*2+1])
		if !highOK || !lowOK {
			return nil
		}
		salt[index] = high<<4 | low
	}
	return salt
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
