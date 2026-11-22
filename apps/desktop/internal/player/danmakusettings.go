package player

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
)

// The comment layer is set the way the sites that made danmaku popular set it:
// the screen area it may use, how opaque and how big the text is, how fast it
// crosses, which kinds of comment are let through, how many may be on screen at
// once. The settings belong to the viewer, not to the episode, so they outlive
// the player and are written beside the theme preference.
//
// Zero values here are never "off": Area 0 is the whole picture and Opacity 0
// means "not chosen yet", so a settings file that is missing, half written or
// from an older build still reads as something the viewer can watch.

// The screen areas the layer may use, in the order the setting offers them.
const (
	areaFull = iota
	areaTop
	areaMiddle
	areaBottom
)

// The kind of crossing a comment makes: scrolled from the right, or pinned to
// the top or the bottom of the layer's band for a few seconds.
const (
	kindScroll = iota
	kindTop
	kindBottom
)

// DanmakuSpeed is how long one comment takes to cross the picture, as the scale
// from the slowest to the fastest. A stored speed is clamped back into it.
const (
	danmakuSlowest = 14.0
	danmakuFastest = 4.0
)

// Settings are the comment layer's preferences, shared by every episode.
type Settings struct {
	// Area is the band of the picture the layer uses: areaFull, areaTop,
	// areaMiddle or areaBottom.
	Area int
	// Opacity is the text's, 0.1 to 1.
	Opacity float32
	// Size is the text's size in logical pixels.
	Size float32
	// Bold draws every comment heavier, which reads better on a small screen.
	Bold bool
	// Speed is how long a comment takes to cross, in seconds.
	Speed float64
	// Lanes is how many rows may be used at once.
	Lanes int
	// Scroll, Top and Bottom switch those kinds of comment on; off hides them.
	Scroll, Top, Bottom bool
}

// DefaultSettings is what a viewer who has never opened the settings watches.
func DefaultSettings() Settings {
	return Settings{Area: areaFull, Opacity: 0.9, Size: 18, Speed: 8, Lanes: 8, Scroll: true, Top: true, Bottom: true}
}

// clamped returns s with every field inside the range the controls offer, so a
// file edited by hand or left by an older build cannot draw a layer nobody can
// read or a speed no comment finishes.
func (s Settings) clamped() Settings {
	fix := DefaultSettings()
	if s.Area < areaFull || s.Area > areaBottom {
		s.Area = fix.Area
	}
	if s.Opacity <= 0 || s.Opacity > 1 {
		s.Opacity = fix.Opacity
	}
	if s.Size < 12 || s.Size > 30 {
		s.Size = fix.Size
	}
	if s.Speed < danmakuFastest || s.Speed > danmakuSlowest {
		s.Speed = fix.Speed
	}
	if s.Lanes < 1 || s.Lanes > 14 {
		s.Lanes = fix.Lanes
	}
	return s
}

// LoadDanmakuSettings reads the stored settings. ok is false when there is
// nothing to read or the file cannot be parsed, which is not a failure: a
// preference is never worth keeping a viewer from playing, so the caller keeps
// the defaults instead of reporting anything.
func LoadDanmakuSettings(path string) (s Settings, ok bool) {
	if path == "" {
		return DefaultSettings(), false
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return DefaultSettings(), false
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return DefaultSettings(), false
	}
	return s.clamped(), true
}

// SaveDanmakuSettings writes the settings, replacing the file as a whole so a
// reader never sees half of one.
func SaveDanmakuSettings(path string, s Settings) error {
	if path == "" {
		return errors.New("no settings path")
	}
	data, err := json.Marshal(s.clamped())
	if err != nil {
		return err
	}
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(dir, ".danmaku-*")
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
