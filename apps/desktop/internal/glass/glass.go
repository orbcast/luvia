// Package glass is the translucent surface the AppShell and the player lay over
// the content: the window's control capsule, the player's pill. MyGo's own
// plugins/glass draws Liquid Glass as macOS does, over what the frame under an
// element painted; a bar over a video or a cover image reads better as the
// plain equivalent, so this package composes one from what MyGo draws: a
// tinted fill over a blurred backdrop, a hairline border lit from above and a
// shadow lifting the pane off what it covers.
package glass

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
)

// Surface is a glass pane over content: its Fill over a backdrop blurred by
// Blur, its Border around it, its Highlight along the top rim and its Shadow
// under it. The element's Radius shapes all of them, so the pane's fill, its
// border and its shadow never disagree about its shape.
type Surface struct {
	// Fill is the tint over the blurred backdrop.
	Fill ui.Color
	// Border is the hairline around the pane; a transparent color draws none.
	Border ui.Color
	// Highlight is the inner light along the pane's top rim, as the tilt of a
	// glass edge throws it; a transparent color draws none.
	Highlight ui.Color
	// Shadow is the drop shadow under the pane: Lift DIPs down, softened by
	// Softness and grown by Spread; a transparent color draws none.
	Shadow                 ui.Color
	Lift, Softness, Spread float32
	// Blur is how much the backdrop behind the pane blurs, in DIPs.
	Blur float32
}

// BuildMaterial implements ui.MaterialBuilder: it turns the element it is built
// on into its fill, border and shadows, and blurs what shows under it, leaving
// the pane's shape to the element's Radius.
func (s Surface) BuildMaterial(e ui.Element) ui.Material {
	if s.Fill.A > 0 {
		e.Background(s.Fill)
	}
	if s.Border.A > 0 {
		e.Border(1, s.Border)
	}
	if s.Shadow.A > 0 {
		e.Shadow(0, s.Lift, s.Softness, s.Spread, s.Shadow)
	}
	if s.Highlight.A > 0 {
		e.InsetShadow(0, 1, 1, 0, s.Highlight)
	}
	return glass.Blur{Radius: s.Blur}
}

// PaintMaterial implements ui.Material: a Surface is only ever painted as what
// BuildMaterial makes of it, a blur, so this draws nothing.
func (Surface) PaintMaterial(*ui.Painter, ui.Rect, [4]float32) {}
