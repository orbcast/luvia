// Package icons parses the Solar SVGs the desktop app embeds, once each.
package icons

import (
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
