package appshell

import "github.com/egoist/mygo/ui"

// desktopTheme centralizes DESIGN.md tokens. Desktop accessibility preferences
// are applied by the caller after choosing the palette.
func desktopTheme(base *ui.Theme) ui.Theme {
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
