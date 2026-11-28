package preferences

import (
	"github.com/egoist/mygo"
	"log"
)

// RestoreTheme applies a saved preference, falling back to the default theme
// (dark) when none is stored or the file cannot be read.
func RestoreTheme(path string, apply func(mygo.ThemeSource)) {
	source, err := LoadTheme(path)
	if err != nil {
		log.Print("theme_load_failed")
	}
	apply(source)
}

// ChangeTheme applies the session choice even when persistence is unavailable.
func ChangeTheme(path string, source mygo.ThemeSource, apply func(mygo.ThemeSource)) {
	if err := SaveTheme(path, source); err != nil {
		log.Print("theme_save_failed")
	}
	apply(source)
}
