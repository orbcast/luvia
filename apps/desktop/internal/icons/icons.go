// Package icons parses the Solar SVGs the desktop app embeds, once each.
package icons

import (
	"regexp"
	"strconv"
	"strings"
	"sync"

	"github.com/egoist/mygo/ui"
)

var parsed sync.Map

// Parse returns the SVG whose figure name is name, or nil when the app does
// not embed it. Call it on the main thread, where the UI is built.
func Parse(name string) *ui.SVG {
	if svg, ok := parsed.Load(name); ok {
		return svg.(*ui.SVG)
	}
	source, ok := Sources[name]
	if !ok {
		return nil
	}
	svg := ui.MustParseSVG([]byte(source))
	parsed.Store(name, svg)
	return svg
}

// The Solar masters draw their figures at 1.5; the player's controls ask for a
// 1.35 hairline, so their figures are redrawn at that weight once and kept here.
var (
	strokeWeight = regexp.MustCompile(`stroke-width="([0-9.]+)"`)
	weighed      sync.Map
)

// Weighed returns the SVG whose name is name drawn with every stroke weight set
// to weight, or nil when the app does not embed it. Call it on the main thread,
// where the UI is built.
func Weighed(name string, weight float32) *ui.SVG {
	key := name + "@" + strconv.FormatFloat(float64(weight), 'g', -1, 32)
	if svg, ok := weighed.Load(key); ok {
		return svg.(*ui.SVG)
	}
	base, ok := Sources[name]
	if !ok {
		return nil
	}
	value := strconv.FormatFloat(float64(weight), 'g', -1, 32)
	source := strokeWeight.ReplaceAllString(strings.ReplaceAll(base, "'", `"`), `stroke-width="`+value+`"`)
	svg := ui.MustParseSVG([]byte(source))
	weighed.Store(key, svg)
	return svg
}
