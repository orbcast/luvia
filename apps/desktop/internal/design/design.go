// Package design centralizes the tokens DESIGN.md defines for the desktop app,
// so the AppShell and every page draw from one palette and one geometry.
package design

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// AppShell geometry in logical pixels; keep synchronized with DESIGN.md.
const (
	ShellInset   = 12
	ShellWidth   = 56
	ShellPadding = 6
	ShellGap     = 6
	ShellRadius  = 22
	// ShellTop is the room the AppShell column, the window controls' capsule
	// and a page's back button keep from the window's top edge; under a title
	// bar holding the system's buttons it is added below that bar.
	ShellTop           = 20
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
	// between the capsule and the window's right edge; the capsule starts on
	// ChromeTop with the AppShell column.
	WindowControlWidth    = 38
	WindowControlHeight   = 32
	WindowControlGap      = 2
	WindowControlRadius   = 999
	WindowControlIconSize = 16
	WindowControlPadding  = 5
	WindowControlInset    = 12

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
	// PageTop is the space a navigation page keeps between the chrome and its
	// first line: the page's top rhythm, on top of ChromeTop. Every navigation
	// page derives its top inset from PageTopInset, so switching destinations
	// never moves the content up or down.
	PageTop = 20
)

// NavigationTint is the grey wash the selected AppShell destination sits on.
func NavigationTint() ui.Color { return ui.RGBA(128, 128, 128, 0.22) }

// PlayIntroBackground is the dark backdrop the play-intro (启播) animation sits
// on. The intro is always dark, whatever the desktop appearance, so its blue
// mark stays legible.
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
// there, so the top corners read as one row. A window whose title bar holds the
// system's controls, as macOS's traffic lights, keeps that bar's room first and
// then ShellTop, so the corner reads as evenly framed rather than tucked under
// the buttons.
func ChromeTop(barBottom float32) float32 {
	if barBottom <= 0 {
		return ShellTop
	}
	return barBottom + ShellTop
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
	// Foot shades the bottom of a piece of artwork, so a caption laid on it keeps
	// its contrast over a bright still as well as a dark one.
	Foot ui.Color
	// InkDim is the quietest ink on artwork: a poster's footnote.
	InkDim ui.Color
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
			Foot:       ui.RGBA(0, 0, 0, 0.78),
			InkDim:     ui.RGBA(255, 255, 255, 0.70),
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
		Foot:       ui.RGBA(0, 0, 0, 0.52),
		InkDim:     ui.RGBA(255, 255, 255, 0.92),
		SideMid:    0.48,
		SideEnd:    0.74,
		FadeStart:  0.66,
		FadePage:   0.90,
	}
}

// MaskOpaque is the opaque stop of a mask that fades artwork out.
func MaskOpaque() ui.Color     { return ui.RGB(0, 0, 0) }
func TrackOnArtwork() ui.Color { return ui.RGBA(255, 255, 255, 0.30) }

// ArtworkBlur is the sigma a page blurs a full-bleed still by when copy and
// surfaces sit on it, so the picture reads as a wash of its own colors. The
// hero backdrop and the detail page share it.
func ArtworkBlur() float32 { return 40 }

// PlayMarkShadow is the shadow under a small play mark laid on artwork: one
// unit of black, enough to lift the white glyph off a pale frame.
func PlayMarkShadow() ui.Color { return ui.RGBA(0, 0, 0, 0.55) }

// PosterShadow is the drop shadow under a detail page's poster: black at half,
// wide and low, so the artwork stands off the page instead of sitting in it.
func PosterShadow() ui.Color { return ui.RGBA(0, 0, 0, 0.48) }

// HeroCover dresses the banner's cover card, which floats beside the copy over
// the blurred still: the shadow that lifts it off that wash, and the hairline
// along its edge.
func HeroCover(t *ui.Theme) (shadow, border ui.Color) {
	if t.Dark {
		return ui.RGBA(0, 0, 0, 0.55), ui.RGBA(255, 255, 255, 0.16)
	}
	return ui.RGBA(0, 0, 0, 0.30), ui.RGBA(255, 255, 255, 0.50)
}

// PosterVeil dresses the follow switch laid on a poster's artwork: a wash dark
// enough that the glyph reads on a white poster as on a black one, and darker
// still once the title is followed, so the accent has something to sit on.
func PosterVeil() ui.Color   { return ui.RGBA(10, 8, 14, 0.55) }
func PosterVeilOn() ui.Color { return ui.RGBA(10, 8, 14, 0.72) }

// PosterVeilInk is the glyph of a poster's unfollowed switch: white, because the
// artwork under it can be any colour.
func PosterVeilInk() ui.Color { return ui.RGBA(255, 255, 255, 0.90) }

// PosterPlayDisc is the play mark that appears on a poster while the pointer is
// on it: a white disc with a dark glyph, the one shape on the artwork that is not
// the artwork's own colour.
func PosterPlayDisc() ui.Color { return ui.RGBA(255, 255, 255, 0.92) }
func PosterPlayInk() ui.Color  { return ui.RGBA(0, 0, 0, 0.72) }

// CategoryTint is the pill color of one resource-library category. The tints
// are dyed paper, not badges: the same hue in both themes, quiet enough to sit
// on the page's canvas beside five more, and the selected one leans on the
// theme's accent instead of a colour of its own.
type CategoryTint struct {
	Background ui.Color
	Foreground ui.Color
}

// categoryTints maps the resource library's category order onto its tints:
// all, movie, series, anime, variety, short. Each is one hue at a low alpha
// over the surface, and its ink stays the theme's text, so a row of pills reads
// as one family rather than six brands.
var categoryTints = [6]CategoryTint{
	{ui.RGBA(230, 170, 90, 0.14), ui.RGBA(255, 255, 255, 0.92)},
	{ui.RGBA(232, 128, 122, 0.14), ui.RGBA(255, 255, 255, 0.92)},
	{ui.RGBA(122, 158, 230, 0.16), ui.RGBA(255, 255, 255, 0.92)},
	{ui.RGBA(168, 136, 224, 0.16), ui.RGBA(255, 255, 255, 0.92)},
	{ui.RGBA(112, 196, 160, 0.14), ui.RGBA(255, 255, 255, 0.92)},
	{ui.RGBA(206, 160, 120, 0.14), ui.RGBA(255, 255, 255, 0.92)},
}

// lightCategoryTints is the same six hues over the light canvas: darker ink and
// a touch more of the hue, or a pill on a pale page would wash out.
var lightCategoryTints = [6]CategoryTint{
	{ui.RGBA(150, 104, 40, 0.16), ui.Hex("#221B33")},
	{ui.RGBA(150, 48, 44, 0.13), ui.Hex("#221B33")},
	{ui.RGBA(44, 86, 158, 0.13), ui.Hex("#221B33")},
	{ui.RGBA(88, 58, 150, 0.13), ui.Hex("#221B33")},
	{ui.RGBA(26, 106, 78, 0.13), ui.Hex("#221B33")},
	{ui.RGBA(126, 80, 44, 0.13), ui.Hex("#221B33")},
}

// CategoryTints returns the category pills' colors in display order.
func CategoryTints(t *ui.Theme) [6]CategoryTint {
	if t.Dark {
		return categoryTints
	}
	return lightCategoryTints
}

// CategoryWash is the faint accent a selected control is filled with: the ring
// carries the state, the wash only keeps the control from reading as an empty
// outline. The week strip's selected day and the library's selected pill share
// it, so "picked" means one thing across the app.
func CategoryWash(t *ui.Theme) ui.Color {
	return ui.RGBA(t.Accent.R, t.Accent.G, t.Accent.B, 0.14)
}

// CategoryPill is the surface of one category pill: its fill, the ink on it, and
// the hairline it wears when selected. An unselected pill carries no border; the
// selected one takes the theme's accent as a ring and the faintest accent wash
// as its fill, so the row keeps one highlight language with the rest of the app
// and never competes with the posters under it.
func CategoryPill(t *ui.Theme, index int, selected bool) (bg, ink, border ui.Color) {
	tint := CategoryTints(t)[index]
	if !selected {
		return tint.Background, tint.Foreground, ui.Transparent
	}
	return CategoryWash(t), t.Text, ui.RGBA(t.Accent.R, t.Accent.G, t.Accent.B, 0.55)
}

// ControlSurface is the ground of a form control — a dropdown's trigger, a
// text field, the filter bar's buttons — so one page's controls share one body:
// the theme's surface inside a hairline, rather than the glass material some
// rows used and the flat surface others did.
func ControlSurface(t *ui.Theme) (bg, border ui.Color) { return t.Surface, t.Border }

// QualityBadge is the resolution mark a poster and a hero carry in the corner:
// the theme's inverse pair, because it states a fact about the artwork the way
// the page's main action is drawn, and must read over any frame.
func QualityBadge(t *ui.Theme) (bg, ink ui.Color) { return t.Inverse, t.InverseText }

// BrandBadge marks a brand the source itself named in the title's remarks — a
// Dolby print, a studio's own line — with the violet the desk uses for press
// marks, so it reads as the source's claim and not as the page's own accent.
func BrandBadge() (bg, edge, ink ui.Color) {
	return ui.Hex("#3E2E9E"), ui.Hex("#8E7BFF"), ui.RGBA(255, 255, 255, 0.96)
}

// PosterBadge is the plate a poster's factual corner mark sits on: nearly black
// and translucent, so it holds white type over any frame without becoming a
// block of its own.
func PosterBadge() (bg, ink ui.Color) {
	return ui.RGBA(10, 8, 14, 0.62), ui.RGBA(255, 255, 255, 0.96)
}

// PosterStatus is the top-right mark a poster carries: a show still updating is
// named in the app's teal, because it is news rather than a quality, and a
// finished one keeps the plain translucent plate.
func PosterStatus(updating bool) (bg, ink ui.Color) {
	if updating {
		return ui.Hex("#2A6E63"), ui.RGBA(255, 255, 255, 0.98)
	}
	return ui.RGBA(10, 8, 14, 0.62), ui.RGBA(255, 255, 255, 0.96)
}

// PageTopInset is where a navigation page's content starts on the vertical, so
// Discover, Library, Following, Search and Settings begin on one line. It is the
// chrome's own height plus the top of the page's rhythm (PageTop), derived from
// the title bar rather than picked per page: a page that starts at 24, 28 or 56
// makes the whole window jump when the viewer switches destinations.
func PageTopInset(barBottom float32) float32 { return ChromeTop(barBottom) + PageTop }

// SettingCard is one group of settings on a page: its rows on the theme's own
// surface inside a hairline. Settings read as a list of decisions, so the page
// groups them by what they concern and draws each group as one body, instead of
// scattering rows down the canvas.
func SettingCard(t *ui.Theme) (bg, border ui.Color) { return t.Surface, t.Border }

// SettingGroup is the box a group of rows sits in: the card's fill, its hairline
// and its corner, named once so every card on the page is the same box.
type SettingGroup struct {
	Fill   ui.Color
	Border ui.Color
	Radius float32
}

// SettingCardSpec is SettingGroup filled from the theme.
func SettingCardSpec(t *ui.Theme) SettingGroup {
	return SettingGroup{Fill: t.Surface, Border: t.Border, Radius: 18}
}

// SettingRowHeight is one row of a settings card: the control's own 38 units
// and the air above and below it, so a card with one row and a card with four
// share their rhythm.
const SettingRowHeight = 62

// SegmentedPill dresses the two-way switch the filters use (all titles or the
// viewer's own, grid or list): a quiet track, and the chosen segment lifted onto
// the theme's surface with the accent as its ink. It matches the category pills'
// radius and height so a filter bar reads as one row of controls.
type Segmented struct {
	Track, Selected   ui.Color
	Ink, SelInk       ui.Color
	Border, SelBorder ui.Color
}

// SegmentedPill returns the segmented switch's colors for this theme.
func SegmentedPill(t *ui.Theme) Segmented {
	if t.Dark {
		return Segmented{
			Track: ui.RGBA(255, 255, 255, 0.05), Selected: ui.RGBA(255, 255, 255, 0.12),
			Ink: t.TextMuted, SelInk: t.Text,
			Border: ui.Transparent, SelBorder: ui.RGBA(255, 255, 255, 0.16),
		}
	}
	return Segmented{
		Track: ui.RGBA(0, 0, 0, 0.04), Selected: t.Surface,
		Ink: t.TextMuted, SelInk: t.Text,
		Border: ui.Transparent, SelBorder: t.Border,
	}
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

// PlayerPanel is the now-playing side panel background and the line where it
// meets the stage. The dark theme keeps the panel almost black over a black
// stage, so a faint lift is enough; the light theme puts a pale list against a
// black stage, and there a sixth of black draws the border the seam needs — the
// cutting line reads as an edge instead of two colors meeting by accident.
func PlayerPanel(t *ui.Theme) (bg, border ui.Color) {
	if t.Dark {
		return ui.Hex("#1B1420"), ui.RGBA(255, 255, 255, 0.08)
	}
	return ui.Hex("#F6F1F8"), ui.RGBA(0, 0, 0, 0.20)
}

// PlayerPanelGlass is the floating now-playing panel's tint and hairline. It
// keeps one deep tint at either theme, as a card laid over a frame does.
func PlayerPanelGlass() (fill, border ui.Color) {
	return PlayerPanelGlassFill(), PlayerPanelGlassBorder()
}

// PlayerPanelGlassFill and PlayerPanelGlassBorder are the panel's tint and
// hairline on their own, for a popover that dresses itself in the same glass.
func PlayerPanelGlassFill() ui.Color   { return ui.RGBA(20, 13, 23, 0.80) }
func PlayerPanelGlassBorder() ui.Color { return ui.RGBA(255, 255, 255, 0.10) }

// PlayerChip is the quiet surface of a control inside the player panel: the tab
// rail, the sort chip and an episode number. Selected takes the brighter wash
// and ink, which the picture's own controls use when the pointer is on them.
func PlayerChip(selected bool) (bg, ink ui.Color) {
	if selected {
		return ui.RGBA(255, 255, 255, 0.20), ui.RGBA(255, 255, 255, 1)
	}
	return ui.RGBA(255, 255, 255, 0.07), ui.RGBA(255, 255, 255, 0.78)
}

// PlayerShade is the stage's one gradient: a strip of black under the top line —
// the way out, the episode marker and the pill of controls — that walks to
// nothing before the picture's mass, so white text and the capsules read on any
// frame while the film keeps its own colors.
//
// There is deliberately no veil under the bottom bar. The picture fills the whole
// stage, so a gradient there would be painted over the frame's own last rows,
// which in a portrait episode are the picture (and where subtitles sit). The
// bottom bar earns its contrast from its capsule, its shadowed glyphs and the
// margins a fitted picture leaves instead.
type PlayerShade struct {
	Top       ui.Color
	TopHeight float32
	// Mid is where the strip is darkest. It is not at the top edge: the way out
	// and the pill are capsules hung off it, and a hard band across the frame's
	// first rows would read as a bar rather than as a shade.
	Mid float32
}

// PlayerShadeSpec returns the stage's top strip: nothing at the top edge,
// half-black some way down, gone by 132 units, which is one control row plus
// the room its shadow needs.
func PlayerShadeSpec() PlayerShade {
	return PlayerShade{Top: ui.RGBA(0, 0, 0, 0.5), TopHeight: 132, Mid: 0.42}
}

// PlayerAutoHide is how long the stage's bars stay after the last input: long
// enough to read the time or pick an episode, short enough that a viewer
// watching a scene is not left with controls over it.
const PlayerAutoHide = 2600 * time.Millisecond

// The stage's own ink. A playing picture is the one place the theme's page
// colors do not apply: the stage is black in both themes, and everything laid
// on it is white or a share of white, because a frame can be any color.
func PlayerInk() ui.Color          { return ui.RGBA(255, 255, 255, 0.94) }
func PlayerMuted() ui.Color        { return ui.RGBA(255, 255, 255, 0.62) }
func PlayerInkDim() ui.Color       { return ui.RGBA(255, 255, 255, 0.70) }
func PlayerInkActive() ui.Color    { return ui.RGBA(255, 255, 255, 1) }
func PlayerOnArtworkInk() ui.Color { return ui.RGBA(20, 20, 22, 1) }
func PlayerHover() ui.Color        { return ui.RGBA(255, 255, 255, 0.12) }
func PlayerChipOff() ui.Color      { return ui.RGBA(255, 255, 255, 0.06) }
func PlayerChipOn() ui.Color       { return ui.RGBA(255, 255, 255, 0.18) }
func PlayerChipHover() ui.Color    { return ui.RGBA(255, 255, 255, 0.12) }

// PlayerWaitingVeil is the plate behind a wait's spinner and its label.
func PlayerWaitingVeil() ui.Color { return ui.RGBA(0, 0, 0, 0.5) }

// PlayerWaitingRing is the spinner's lit arc, given how far the wait has turned.
func PlayerWaitingRing(turn float32) ui.Color {
	return ui.RGBA(255, 255, 255, 0.35+0.5*turn)
}

// PlayerMiniatureVeil is the dark frame a danmaku area preset draws its band on.
func PlayerMiniatureVeil() ui.Color { return ui.RGBA(0, 0, 0, 0.45) }

// PlayerTrack is the unfilled part of the player's scrub track.
func PlayerTrack() ui.Color { return ui.RGBA(255, 255, 255, 0.28) }

// DanmakuLaneHeight is one lane of comments: the line itself plus the room a
// second line of the same lane needs to be told apart from it.
const DanmakuLaneHeight = 30

// DanmakuBand is where the layer of comments may start and end over the
// picture, as shares of its height: the band the viewer picks ("Screen area"),
// clear of the stage's own top line and of the control bar at the bottom.
const (
	DanmakuBandTop    float32 = 0.12
	DanmakuBandBottom float32 = 0.20
)

// danmakuBands are the screen-area presets, as the span of the picture the layer
// may use: full, the upper half, the middle third and the lower half.
var danmakuBands = [4][2]float32{
	{0, 1},
	{0, 0.5},
	{0.28, 0.72},
	{0.5, 1},
}

// DanmakuBand returns the top and bottom of the band preset area, before the
// stage's own clearance at either end is taken off.
func DanmakuBand(area int) (top, bottom float32) {
	if area < 0 || area >= len(danmakuBands) {
		area = 0
	}
	return danmakuBands[area][0], danmakuBands[area][1]
}

// DanmakuInk is a flying comment's text at the opacity the viewer picked, and
// its soft shadow: white-on-shadow reads on a bright frame as well as a dark
// one, and no theme can darken it, because the frame under it is the theme.
func DanmakuInk(opacity float32) ui.Color {
	if opacity < 0 {
		opacity = 0
	}
	if opacity > 1 {
		opacity = 1
	}
	return ui.RGBA(255, 255, 255, opacity)
}

// DanmakuShadow is the depth under a flying comment, fixed whatever its opacity:
// the shadow is what keeps a faint comment legible, so it does not fade with it.
func DanmakuShadow() ui.Color { return ui.RGBA(0, 0, 0, 0.75) }

// DanmakuPinnedVeil is the wash behind a comment pinned to the top or the
// bottom of its band, which may land over a bright face or a white wall.
func DanmakuPinnedVeil() ui.Color { return ui.RGBA(0, 0, 0, 0.42) }
