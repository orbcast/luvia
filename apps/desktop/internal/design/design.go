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
	TitleBarGap        = 8
	// NavigationTopGap keeps the column below the title bar instead of
	// hugging it.
	NavigationTopGap = 24

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

// SplashBackground is the dark backdrop the launch animation sits on. The
// opening splash is always dark, whatever the desktop appearance, so its blue
// mark and white wordmark stay legible.
const SplashBackground = "#0B0A0F"

// ShellGroupHeight is the glass panel around count destinations stacked in
// a column: the padding on both ends and one gap between each pair.
func ShellGroupHeight(count int) float32 {
	if count <= 0 {
		return 2 * ShellPadding
	}
	return float32(count)*NavigationHeight + float32(count-1)*ShellGap + 2*ShellPadding
}

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
