package preferences

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// localePreference is the language the viewer last chose. No file, or an
// empty field, means no choice was made, and the app opens in the language
// its readers are most likely to want: Simplified Chinese, the language of
// the readers the app is made for, unless the system speaks another
// language of those the app is written in.
type localePreference struct {
	Locale string `json:"locale"`
}

// LoadLocale reports the stored language tag, or "" when nothing was
// stored. A missing or unreadable file is not an error: the app falls back
// on its own choice, so a broken preference never keeps the window from
// opening.
func LoadLocale(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	var pref localePreference
	if err := json.Unmarshal(data, &pref); err != nil {
		return ""
	}
	return strings.TrimSpace(pref.Locale)
}

// SaveLocale stores the chosen language tag. An empty tag is the follow
// system choice, which MyGo keeps on its own, so it is stored as given:
// writing it clears the app's choice for the next launch.
func SaveLocale(path, tag string) error {
	data, err := json.Marshal(localePreference{Locale: tag})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".locale-*")
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
