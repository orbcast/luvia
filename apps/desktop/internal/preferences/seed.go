package preferences

import (
	"crypto/aes"
	"crypto/cipher"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
)

// seedFile is the sealed bundle a release carries beside its rules. It holds
// the access key the bundled rule signs with, so a reader who installs the app
// can start without a settings step. The file is not a secret in the
// cryptographic sense and must not be treated as one: what it buys is a cost
// for whoever wants to lift the key out, not an impossibility. See SeedValue.
const seedFile = "kanju.seed"

// seedSalt is mixed into the key derivation, so a seed from one release cannot
// be opened with another one's build. Its value comes from the build, see
// seedsalt.go and seeddev.go.
func seedKey() []byte {
	salt := seedSalt()
	if len(salt) == 0 {
		return nil
	}
	// A short, stable derivation: this guards the file against being read, not
	// the key against being used by whoever reads it anyway.
	derived := make([]byte, 32)
	copy(derived, []byte("luvia.seed.v1"))
	for index, value := range salt {
		derived[index%len(derived)] ^= value
	}
	return derived
}

// SeedValue reads the key a release packaged beside its rules, if there is one
// and this build can open it. A missing file, a file from another release, or
// a build without a salt all report an empty string: the app then starts the
// way it did before the seed existed and asks for the key.
//
// The cost this pays for is extraction, not use. Whoever can run this binary
// with a debugger can read the key the moment it signs a request, and OWASP
// MASWE-0005 records that for every client that bundles a credential. The seed
// is written so that the key is not sitting in the archive as text.
func SeedValue(resources string) string {
	if resources == "" {
		return ""
	}
	data, err := os.ReadFile(filepath.Join(resources, seedFile))
	if err != nil {
		return ""
	}
	plain, err := openPayload(data)
	if err != nil {
		return ""
	}
	var document seedPayload
	if err := json.Unmarshal(plain, &document); err != nil {
		return ""
	}
	return strings.TrimSpace(document.Secrets[seedVariable])
}

// seedVariable is the one name the bundled seed is read for: the key the
// checked-in kanju rule signs with. A seed that carries other names keeps them
// for the settings page, not for the app to apply on its own.
const seedVariable = "KANJU_SECRET"

// SeedVariable names the value the bundled seed supplies, so the startup path
// can apply it without repeating the name.
func SeedVariable() string { return seedVariable }

// seedPayload is the document the release workflow hands the packer. Keeping it
// JSON, rather than a bare string, leaves room for the name of the variable a
// future release fills without changing the file's meaning.
type seedPayload struct {
	Secrets map[string]string `json:"secrets"`
}

// WriteSeedFile seals the named values into path, which is the form the release
// workflow writes into the bundled resources. It is the only writer of seedFile
// in the repository; the app itself only reads.
func WriteSeedFile(path string, values map[string]string) error {
	document, err := json.Marshal(seedPayload{Secrets: values})
	if err != nil {
		return err
	}
	sealed, err := sealPayload(document)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
		return err
	}
	return os.WriteFile(path, sealed, 0644)
}

// ReadSeedFile opens what WriteSeedFile wrote, and reports the values it holds.
func ReadSeedFile(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	plain, err := openPayload(data)
	if err != nil {
		return nil, err
	}
	var document seedPayload
	if err := json.Unmarshal(plain, &document); err != nil {
		return nil, err
	}
	return document.Secrets, nil
}

// sealPayload and openPayload are the layout WriteSeedFile and ReadSeedFile
// share, separate from SealSeed so a test can pin the file format without
// reaching into the app's resource directory.
func sealPayload(plain []byte) ([]byte, error) {
	secret := seedKey()
	if secret == nil {
		return nil, errors.New("seed salt is not configured for this build")
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	sealed, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	out := make([]byte, 1+sealed.NonceSize(), 1+sealed.NonceSize()+len(plain)+sealed.Overhead())
	out[0] = 1
	if err := fillRandom(out[1 : 1+sealed.NonceSize()]); err != nil {
		return nil, err
	}
	out = sealed.Seal(out, out[1:1+sealed.NonceSize()], plain, nil)
	return out, nil
}

func openPayload(data []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 1 {
		return nil, errors.New("seed has an unknown format")
	}
	secret := seedKey()
	if secret == nil {
		return nil, errors.New("seed salt is not configured for this build")
	}
	block, err := aes.NewCipher(secret)
	if err != nil {
		return nil, err
	}
	sealed, err := cipher.NewGCM(block)
	if err != nil {
		return nil, err
	}
	size := sealed.NonceSize()
	if len(data) < 1+size+sealed.Overhead() {
		return nil, errors.New("seed is truncated")
	}
	return sealed.Open(nil, data[1:1+size], data[1+size:], nil)
}
