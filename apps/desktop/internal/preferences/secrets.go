package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// secretsFile is the file, in the user data directory, where the values a
// rule asks for by name are kept. A rule writes ${env:NAME} and the engine
// reads the process environment, which a desktop application started from
// a file manager does not inherit from a shell profile; this file is the
// way a reader supplies NAME on a machine that never sees a terminal.
const secretsFile = "secrets.json"

// secretsMode keeps the file readable only by its owner.
const secretsMode os.FileMode = 0600

// LoadSecrets reads the named values. A missing or unreadable file is not an
// error: a first launch has none, and a broken one only means the rule is
// asked for its variables the way it was before the file existed.
func LoadSecrets(dir string) map[string]string {
	data, err := os.ReadFile(filepath.Join(dir, secretsFile))
	if err != nil {
		return nil
	}
	var stored map[string]string
	if err := json.Unmarshal(data, &stored); err != nil {
		return nil
	}
	values := make(map[string]string, len(stored))
	for name, value := range stored {
		if name == "" || strings.TrimSpace(value) == "" {
			continue
		}
		values[name] = value
	}
	if len(values) == 0 {
		return nil
	}
	return values
}

// SaveSecrets writes the named values, replacing the file. An empty map
// removes it, which is what a reader who clears every value expects.
func SaveSecrets(dir string, values map[string]string) error {
	path := filepath.Join(dir, secretsFile)
	if len(values) == 0 {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	data, err := json.MarshalIndent(values, "", "  ")
	if err != nil {
		return err
	}
	temporary, err := os.CreateTemp(dir, ".secrets-*")
	if err != nil {
		return err
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return err
	}
	if err := temporary.Close(); err != nil {
		return err
	}
	if err := os.Chmod(name, secretsMode); err != nil {
		return err
	}
	return os.Rename(name, path)
}

// ApplySecrets puts the stored values into the process environment. Values
// the environment already defines win, so a shell that exports its own copy
// stays in charge, and a stored value is never silently preferred over it.
func ApplySecrets(values map[string]string) {
	for name, value := range values {
		if _, set := os.LookupEnv(name); set {
			continue
		}
		os.Setenv(name, value)
	}
}
