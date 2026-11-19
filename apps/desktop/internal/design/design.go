// Package design centralizes the tokens DESIGN.md defines for the desktop app,
// so the AppShell and every page draw from one palette and one geometry.
package design

import "github.com/egoist/mygo/ui"

// AppShell geometry in logical pixels; keep synchronized with DESIGN.md.
const (
	ShellInset         = 12
	ShellWidth         = 56
	ShellPadding       = 6
	ShellGap           = 6
	ShellRadius        = 22
	NavigationWidth    = 44
	NavigationHeight   = NavigationWidth
	NavigationRadius   = 13
	NavigationIconSize = 23
	ShellHeight        = 5*NavigationHeight + 4*ShellGap + 2*ShellPadding
	TitleBarHeight     = 28

	// WindowControlWidth and WindowControlHeight are the hit area of each window
	// button in the capsule the shell draws in place of the system's, and
	// WindowControlGap the space between them. WindowControlPadding is the room
	// between the buttons and the capsule's edge, WindowControlInset the room
	// between the capsule and the window's edge, and WindowControlTop where the
	// capsule starts.
	WindowControlWidth    = 38
	WindowControlHeight   = 32
	WindowControlGap      = 2
	WindowControlRadius   = 999
	WindowControlIconSize = 16
	WindowControlPadding  = 5
	WindowControlInset    = 12
	WindowControlTop      = 12

	// ContentInset is where page content starts, clear of the floating AppShell.
	ContentInset  = ShellInset + ShellWidth
	ContentMargin = 36
	// PageGutter keeps page copy a little clear of the floating column.
	PageGutter = 28
	PageInset  = ContentInset + PageGutter

	// NarrowWidth is the window width below which the AppShell leaves the
	// side for a dock and pages tighten their margins.
	NarrowWidth = 720
	// NarrowGutter is the page margin while the AppShell is docked.
	NarrowGutter = 20
	// DockHeight is the dock: one navigation row plus its padding.
	DockHeight = NavigationHeight + 2*ShellPadding
)

// NavigationTint is the grey wash the selected AppShell destination sits on.
func NavigationTint() ui.Color { return ui.RGBA(128, 128, 128, 0.22) }

// PlayIntroBackground is the dark backdrop the play-intro (启播) animation sits
// on. The intro is always dark, whatever the desktop appearance, so its blue
// mark and white wordmark stay legible.
func PlayIntroBackground() ui.Color { return ui.Hex("#0B0A0F") }

// The window controls are a capsule of glass floating over the page, so they
// keep one deep tint at either theme, as a title bar over artwork does.

// WindowControlFill is the capsule's tint over the blurred backdrop.
func WindowControlFill() ui.Color { return ui.RGBA(18, 20, 25, 0.55) }

// WindowControlBorder is the hairline around the capsule, and
// WindowControlHighlight the light along its top rim, as glass throws it.
func WindowControlBorder() ui.Color    { return ui.RGBA(255, 255, 255, 0.14) }
func WindowControlHighlight() ui.Color { return ui.RGBA(255, 255, 255, 0.10) }

// WindowControlShadow lifts the capsule off the page.
func WindowControlShadow() ui.Color { return ui.RGBA(0, 0, 0, 0.75) }

// WindowControlInk is the controls' figure, and WindowControlInkHover the one
// under the pointer; WindowControlHover is the wash a button takes there.
func WindowControlInk() ui.Color      { return ui.RGBA(255, 255, 255, 0.80) }
func WindowControlInkHover() ui.Color { return ui.RGBA(255, 255, 255, 1) }
func WindowControlHover() ui.Color    { return ui.RGBA(255, 255, 255, 0.11) }

// CloseHover is the one color the cluster gives a meaning: the close button
// turns red under the pointer, the one button that leaves the window.
func CloseHover() ui.Color { return ui.Hex("#E5484D") }

// ShellGroupHeight is the glass panel around count destinations stacked in
// a column: the padding on both ends and one gap between each pair.
func ShellGroupHeight(count int) float32 {
	if count <= 0 {
		return 2 * ShellPadding
	}
	return float32(count)*NavigationHeight + float32(count-1)*ShellGap + 2*ShellPadding
}

// ChromeTop is the one line the window's chrome starts on: the window
// controls' capsule, the AppShell column and a page's back button all begin
// there, so the top corners read as one row. A window whose title bar holds
// the system's controls keeps that bar's room first, as the capsule does.
func ChromeTop(barBottom float32) float32 { return max(barBottom, WindowControlTop) }

// Docked reports whether the AppShell docks at the bottom at this width.
func Docked(width float32) bool { return width < NarrowWidth }

// Inset is the page's left and right margin at this width.
func Inset(width float32) float32 {
	if Docked(width) {
		return NarrowGutter
	}
	return PageInset
}

// BottomClearance is the room page content leaves for the dock.
func BottomClearance(width float32) float32 {
	if Docked(width) {
		return DockHeight + 2*ShellInset
	}
	return 0
}

// Theme chooses the palette DESIGN.md defines for the given base theme. Desktop
// accessibility preferences are applied by the caller after choosing it.
func Theme(base *ui.Theme) ui.Theme {
	t := *ui.LightTheme()
	if base.Dark {
		t = *ui.DarkTheme()
	}
	if t.Dark {
		t.Background = ui.Hex("#0B0A0F")
		t.Surface = ui.Hex("#17151C")
		t.SurfaceHover = ui.Hex("#302A38")
		t.SurfacePressed = ui.Hex("#3B3345")
		t.Border = ui.Hex("#39343F")
		t.Text = ui.Hex("#F8F2EB")
		t.TextMuted = ui.Hex("#B9B3C1")
		t = *t.WithAccent(ui.Hex("#E6AA5A"))
		t.AccentText = ui.Hex("#241A0D")
		t.Inverse = ui.Hex("#F8F2EB")
		t.InverseText = ui.Hex("#221B33")
	} else {
		t.Background = ui.Hex("#F3EEF6")
		t.Surface = ui.Hex("#FFFBFE")
		t.SurfaceHover = ui.Hex("#E2DBEA")
		t.SurfacePressed = ui.Hex("#D8CFE3")
		t.Border = ui.Hex("#D8D0DF")
		t.Text = ui.Hex("#221B33")
		t.TextMuted = ui.Hex("#5D566B")
		t = *t.WithAccent(ui.Hex("#5A48CA"))
		t.AccentText = ui.Hex("#FFFFFF")
		t.Inverse = ui.Hex("#221B33")
		t.InverseText = ui.Hex("#FFFBFE")
	}
	t.Focus = t.Accent
	t.FocusRingWidth = 2
	t.FocusRingOffset = 2
	return t
}

// Artwork is what a hero image sits under: the scrims that carry it into
// the page, and the ink and control colors that stay legible on what
// remains of it.
type Artwork struct {
	// Dim washes the whole image, ScrimTop shades the band under the title
	// bar, ScrimSides reads the caption, and Deep is the middle of the fade
	// into the page.
	Dim, ScrimTop, ScrimSides, Deep ui.Color
	// Ink colors titles and figures, Muted the metadata and captions.
	Ink, Muted ui.Color
	// Veil, VeilHover and Border dress controls laid over the image.
	Veil, VeilHover, Border ui.Color
	// Track is the unfilled part of a progress bar over the image.
	Track ui.Color
	// SideMid and SideEnd place the side scrim's middle and clear stops;
	// FadeStart and FadePage place where the bottom fade begins and where it
	// has become the page background.
	SideMid, SideEnd, FadeStart, FadePage float32
}

// OnArtwork returns the palette for a hero image in this theme. The dark
// theme shades a dark scrim under white ink; the light one washes the
// image towards white and writes in the theme's own dark text, so the hero
// reads as part of a light page instead of a photograph behind glass.
func OnArtwork(t *ui.Theme) Artwork {
	if t.Dark {
		return Artwork{
			Dim:        ui.RGBA(0, 0, 0, 0.20),
			ScrimTop:   ui.RGBA(0, 0, 0, 0.30),
			ScrimSides: ui.RGBA(0, 0, 0, 0.90),
			Deep:       ui.RGBA(0, 0, 0, 0.55),
			Ink:        ui.RGBA(255, 255, 255, 0.96),
			Muted:      ui.RGBA(255, 255, 255, 0.72),
			Veil:       ui.RGBA(255, 255, 255, 0.16),
			VeilHover:  ui.RGBA(255, 255, 255, 0.26),
			Border:     ui.RGBA(255, 255, 255, 0.22),
			Track:      ui.RGBA(255, 255, 255, 0.30),
			SideMid:    0.55,
			SideEnd:    0.92,
			FadeStart:  0.52,
			FadePage:   0.88,
		}
	}
	return Artwork{
		// A light wash keeps the photo itself instead of bleaching it: only
		// the band the caption sits in is lifted to white, while the rest of
		// the image keeps its tone and the bottom still melts into the page.
		Dim:        ui.RGBA(255, 255, 255, 0.06),
		ScrimTop:   ui.RGBA(255, 255, 255, 0.28),
		ScrimSides: ui.RGBA(255, 255, 255, 0.92),
		Deep:       ui.RGBA(255, 255, 255, 0.50),
		Ink:        t.Text,
		Muted:      t.TextMuted,
		Veil:       ui.RGBA(255, 255, 255, 0.55),
		VeilHover:  ui.RGBA(255, 255, 255, 0.75),
		Border:     t.Border,
		Track:      ui.RGBA(0, 0, 0, 0.16),
		SideMid:    0.50,
		SideEnd:    0.80,
		FadeStart:  0.58,
		FadePage:   0.86,
	}
}

// MaskOpaque is the opaque stop of a mask that fades artwork out.
func MaskOpaque() ui.Color     { return ui.RGB(0, 0, 0) }
func TrackOnArtwork() ui.Color { return ui.RGBA(255, 255, 255, 0.30) }

// CategoryTint is the pill color of one resource-library category. The tints
// are fixed category colors, the same in both themes: the "all" tab is warm
// paper with ink text, the saturated ones keep white text.
type CategoryTint struct {
	Background ui.Color
	Foreground ui.Color
}

// categoryTints maps the resource library's category order onto its tints:
// all, movie, series, anime, variety, short.
var categoryTints = [6]CategoryTint{
	{ui.Hex("#F0E1DA"), ui.Hex("#221B33")},
	{ui.Hex("#971E39"), ui.RGBA(255, 255, 255, 0.98)},
	{ui.Hex("#2C4A6E"), ui.RGBA(255, 255, 255, 0.98)},
	{ui.Hex("#563A79"), ui.RGBA(255, 255, 255, 0.98)},
	{ui.Hex("#23624F"), ui.RGBA(255, 255, 255, 0.98)},
	{ui.Hex("#75452F"), ui.RGBA(255, 255, 255, 0.98)},
}

// CategoryTints returns the category pills' colors in display order.
func CategoryTints() [6]CategoryTint { return categoryTints }

// CategoryRing is the ring the selected category tab wears. It is a cool blue
// that stays distinct from every category tint and from the theme accent.
func CategoryRing() ui.Color { return ui.Hex("#5B86E8") }

// QualityBadge is the hero's "4K" chip: it takes the theme's inverse pair, so
// it reads light on the dark hero and dark on the washed light one.
func QualityBadge(t *ui.Theme) (bg, fg ui.Color) { return t.Inverse, t.InverseText }

// BrandBadge is the hero's Dolby-style chip: a violet pill with a lighter
// brand mark and white label, legible and brand-like on both heroes.
func BrandBadge() (bg, mark, fg ui.Color) {
	return ui.Hex("#3E2E9E"), ui.Hex("#8E7BFF"), ui.RGBA(255, 255, 255, 0.96)
}

// PosterBadge is the small quality pill laid over a poster image.
func PosterBadge() (bg, fg ui.Color) {
	return ui.RGBA(10, 8, 14, 0.62), ui.RGBA(255, 255, 255, 0.96)
}

// PosterStatus is the update-state pill over a poster: teal while updating,
// neutral once it is complete.
func PosterStatus(updating bool) (bg, fg ui.Color) {
	if updating {
		return ui.Hex("#2A6E63"), ui.RGBA(255, 255, 255, 0.98)
	}
	return ui.RGBA(10, 8, 14, 0.62), ui.RGBA(255, 255, 255, 0.96)
}

// Surfaces is the glass-and-chip dressing a title page (detail) draws over its
// blurred artwork: quiet translucent chips, outlined tag pills, panel cards and
// the tab rail. Values differ by theme so copy stays legible on both.
type Surfaces struct {
	Chip, ChipBorder, ChipInk        ui.Color
	TagBg, TagBorder, TagInk         ui.Color
	Card, CardBorder                 ui.Color
	TabBg, TabSel, TabInk, TabSelInk ui.Color
}

// DetailSurfaces returns the title page's surface palette for this theme.
func DetailSurfaces(t *ui.Theme) Surfaces {
	if t.Dark {
		return Surfaces{
			Chip: ui.RGBA(18, 15, 24, 0.55), ChipBorder: ui.RGBA(255, 255, 255, 0.10), ChipInk: t.Text,
			TagBg: ui.RGBA(255, 255, 255, 0.06), TagBorder: ui.RGBA(255, 255, 255, 0.22), TagInk: ui.RGBA(255, 255, 255, 0.80),
			Card: ui.RGBA(20, 16, 26, 0.55), CardBorder: ui.RGBA(255, 255, 255, 0.08),
			TabBg: ui.RGBA(255, 255, 255, 0.06), TabSel: ui.RGBA(255, 255, 255, 0.16), TabInk: t.TextMuted, TabSelInk: t.Text,
		}
	}
	return Surfaces{
		Chip: ui.RGBA(255, 255, 255, 0.72), ChipBorder: t.Border, ChipInk: t.Text,
		TagBg: ui.RGBA(0, 0, 0, 0.03), TagBorder: t.Border, TagInk: t.TextMuted,
		Card: ui.RGBA(255, 255, 255, 0.82), CardBorder: t.Border,
		TabBg: ui.RGBA(0, 0, 0, 0.05), TabSel: ui.RGBA(255, 255, 255, 0.95), TabInk: t.TextMuted, TabSelInk: t.Text,
	}
}

// DetailBackdrop is the wash painted over the blurred artwork behind a title
// page: near-black in the dark theme, a light lift in the light one.
func DetailBackdrop(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.RGBA(8, 7, 11, 0.66)
	}
	return ui.RGBA(243, 238, 246, 0.80)
}

// EpisodeChip dresses one episode button in the selection grid: selected wears a
// light outline and brighter ink, the rest sit on a quiet surface.
func EpisodeChip(t *ui.Theme, selected bool) (bg, border, ink ui.Color) {
	if t.Dark {
		if selected {
			return ui.RGBA(255, 255, 255, 0.04), ui.RGBA(255, 255, 255, 0.90), t.Text
		}
		return ui.RGBA(255, 255, 255, 0.06), ui.RGBA(255, 255, 255, 0.10), t.TextMuted
	}
	if selected {
		return ui.RGBA(90, 72, 202, 0.08), ui.Hex("#5A48CA"), t.Text
	}
	return ui.RGBA(0, 0, 0, 0.04), t.Border, t.TextMuted
}

// PlayerStage is the near-black letterbox the video player sits in.
func PlayerStage(t *ui.Theme) ui.Color {
	if t.Dark {
		return ui.Hex("#050407")
	}
	return ui.Hex("#0B0A0F")
}

// PlayerPanel is the now-playing side panel background and its separating edge.
func PlayerPanel(t *ui.Theme) (bg, border ui.Color) {
	if t.Dark {
		return ui.Hex("#1B1420"), ui.RGBA(255, 255, 255, 0.06)
	}
	return ui.Hex("#F6F1F8"), ui.RGBA(0, 0, 0, 0.06)
}

// ControlVeil dresses a control laid over the video: a dark translucent pill so
// the icons read on any frame.
func ControlVeil() ui.Color { return ui.RGBA(0, 0, 0, 0.38) }

// PlayerTrack is the unfilled part of the player's scrub track.
func PlayerTrack() ui.Color { return ui.RGBA(255, 255, 255, 0.28) }
