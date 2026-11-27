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
func seedKey(salt []byte) []byte {
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
	values, _, _ := readSeed(resources)
	return strings.TrimSpace(values[seedVariable])
}

// SeedProblem says why the bundled seed, when the package carries one, did not
// supply the key. It is empty when there is no seed at all, which is the normal
// state of a development checkout rather than a fault.
//
// This exists because every failure of the seed is silent by design: the app
// starts and asks for the key instead of refusing to launch, so a release whose
// salt was rotated without a rebuild looks exactly like one that shipped
// without a seed. A line in the log is the only place that difference shows.
type SeedProblem string

const (
	// SeedMissing is reported when the resources directory has no seed file.
	SeedMissing SeedProblem = ""
	// SeedUnreadable covers a seed that exists but cannot be read as bytes.
	SeedUnreadable SeedProblem = "the bundled seed cannot be read"
	// SeedForeign covers a seed sealed under another build's salt, which is what
	// a rotated LUVIA_SEED_SALT without a new release produces.
	SeedForeign SeedProblem = "the bundled seed was sealed by another build"
	// SeedMalformed covers a file that is not a seed at all.
	SeedMalformed SeedProblem = "the bundled seed is not in a form this build reads"
	// SeedEmpty covers a seed that opens but carries no key, which is what a
	// packaging step without KANJU_SECRET leaves behind.
	SeedEmpty SeedProblem = "the bundled seed carries no " + seedVariable
)

// InspectSeed reports what the bundled seed in resources gave, and why it gave
// nothing when it did. Neither return value contains the key.
func InspectSeed(resources string) (string, SeedProblem) {
	values, present, problem := readSeed(resources)
	if problem != SeedMissing {
		return "", problem
	}
	if !present {
		return "", SeedMissing
	}
	key := strings.TrimSpace(values[seedVariable])
	if key == "" {
		return "", SeedEmpty
	}
	return key, SeedMissing
}

// readSeed is the one place the seed file is opened; present reports whether the
// package carried one at all, which tells a checkout apart from a broken
// release.
func readSeed(resources string) (values map[string]string, present bool, problem SeedProblem) {
	if resources == "" {
		return nil, false, SeedMissing
	}
	data, err := os.ReadFile(filepath.Join(resources, seedFile))
	if err != nil {
		if os.IsNotExist(err) {
			return nil, false, SeedMissing
		}
		return nil, true, SeedUnreadable
	}
	present = true
	plain, err := openPayload(data, seedSalt())
	if err != nil {
		if seedKey(seedSalt()) == nil {
			// The build carries no salt, so no seed could ever open: reporting a
			// foreign seal would send whoever reads the log after the wrong file.
			return nil, true, SeedUnreadable
		}
		return nil, true, SeedForeign
	}
	var document seedPayload
	if err := json.Unmarshal(plain, &document); err != nil {
		return nil, true, SeedMalformed
	}
	return document.Secrets, true, SeedMissing
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
//
// salt is passed in rather than looked up here: the packer runs as its own
// process and the app embeds the salt at link time, so the two agree only when
// the caller hands over the value the build embedded. See cmd/seedpack.
func WriteSeedFile(path string, values map[string]string, salt []byte) error {
	document, err := json.Marshal(seedPayload{Secrets: values})
	if err != nil {
		return err
	}
	sealed, err := sealPayload(document, salt)
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
	plain, err := openPayload(data, seedSalt())
	if err != nil {
		return nil, err
	}
	var document seedPayload
	if err := json.Unmarshal(plain, &document); err != nil {
		return nil, err
	}
	return document.Secrets, nil
}

// OpenSeedForTest opens a seed under a caller-supplied salt, which is how a
// test checks that one release's build cannot open another release's seed.
func OpenSeedForTest(path string, salt []byte) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	plain, err := openPayload(data, salt)
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
func sealPayload(plain []byte, salt []byte) ([]byte, error) {
	secret := seedKey(salt)
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

func openPayload(data []byte, salt []byte) ([]byte, error) {
	if len(data) < 2 || data[0] != 1 {
		return nil, errors.New("seed has an unknown format")
	}
	secret := seedKey(salt)
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
