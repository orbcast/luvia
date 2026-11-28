package preferences

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/egoist/mygo"
)

// Only the theme preference is app-owned. MyGo owns locale and window state.
// Dark is the theme of a first launch: the app is designed dark first, and a
// reader can still choose to follow the system in the settings page or menu.
const defaultTheme = mygo.ThemeDark

// DefaultTheme is the theme used before any preference is stored.
func DefaultTheme() mygo.ThemeSource { return defaultTheme }

type themePreference struct {
	Source mygo.ThemeSource `json:"source"`
}

func LoadTheme(path string) (mygo.ThemeSource, error) {
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return defaultTheme, nil
	}
	if err != nil {
		return defaultTheme, err
	}
	var pref themePreference
	if err := json.Unmarshal(data, &pref); err != nil {
		return defaultTheme, err
	}
	if !validTheme(pref.Source) {
		return defaultTheme, fmt.Errorf("invalid theme source %q", pref.Source)
	}
	return pref.Source, nil
}
func validTheme(source mygo.ThemeSource) bool {
	return source == mygo.ThemeSystem || source == mygo.ThemeLight || source == mygo.ThemeDark
}
func SaveTheme(path string, source mygo.ThemeSource) error {
	if !validTheme(source) {
		return fmt.Errorf("invalid theme source %q", source)
	}
	data, err := json.Marshal(themePreference{Source: source})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(filepath.Dir(path), ".theme-*")
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
