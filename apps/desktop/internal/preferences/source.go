package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// sourcePreference is the source the viewer last chose, named by its rule
// directory. No file, or an empty field, means no choice was made and the app
// picks the source it prefers.
type sourcePreference struct {
	Source string `json:"source"`
}

// LoadSource reports the stored source id, or "" when nothing was stored. A
// missing or unreadable file is not an error: the app falls back on its own
// choice, so a broken preference never keeps the window from opening.
func LoadSource(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pref sourcePreference
	if err := json.Unmarshal(data, &pref); err != nil {
		return ""
	}
	return pref.Source
}

// SaveSource stores the chosen source id. An empty id is not a choice, so
// there is nothing to write.
func SaveSource(path string, id string) error {
	if id == "" {
		return nil
	}
	data, err := json.Marshal(sourcePreference{Source: id})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".source-*")
	if err != nil {
		return err
	}
	defer os.Remove(file.Name())
	if _, err := file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err := file.Close(); err != nil {
		return err
	}
	return os.Rename(file.Name(), path)
}
