package preferences

import (
	"crypto/rand"
	"io"
)

// fillRandom draws bytes for a nonce. A source that fails is not replaced with
// anything guessable: a seed sealed under a predictable nonce would be worth
// even less than the file it sits in, so the caller is told and ships nothing.
func fillRandom(target []byte) error {
	if _, err := io.ReadFull(rand.Reader, target); err != nil {
		return err
	}
	return nil
}
