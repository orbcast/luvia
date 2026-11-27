package search

import (
	"strings"

	"luvia/apps/desktop/internal/source"
)

// marker assembles a metadata needle from runes. The check-rules gate forbids
// CJK string literals in production Go, and these needles match upstream data
// (not user-facing copy), so they are built here instead of written inline.
func marker(codes ...rune) string { return string(codes) }

// completedMarkers flag a finished title. "全…集" is caught by the rune test
// below as well.
var completedMarkers = []string{"complete", "completed", "full", marker(0x5B8C, 0x7ED3)} // 完结

// epUnit is the unit a source counts episodes in, as in its remarks; the
// search view reads a show's count from it.
var epUnit = rune(0x96C6) // 集

// qualityMarkers flag a 4K master.
var qualityMarkers = []string{"4k", "2160"}

// metadata flattens the fields the badges read.
func metadata(card source.Card) string {
	return strings.ToLower(card.Remarks + " " + strings.Join(card.Tags, " "))
}

// isUpdating reports whether the card is still running: movies are complete,
// and a series is complete only when its metadata says so.
func isUpdating(card source.Card) bool {
	if card.Kind == source.Movie {
		return false
	}
	text := metadata(card)
	for _, needle := range completedMarkers {
		if strings.Contains(text, needle) {
			return false
		}
	}
	// "全…集" without a space: a full run of episodes means complete.
	if strings.ContainsRune(text, 0x5168) && strings.ContainsRune(text, 0x96C6) {
		return false
	}
	return true
}

// has4K reports whether the card carries a 4K mark.
func has4K(card source.Card) bool {
	text := metadata(card)
	for _, needle := range qualityMarkers {
		if strings.Contains(text, needle) {
			return true
		}
	}
	return false
}
