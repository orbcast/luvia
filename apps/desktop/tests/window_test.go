package tests

import (
	"luvia/apps/desktop/internal/appwindow"
	"testing"
)

type windowStub struct {
	shown, invalidated int
	closed             func()
}

func (w *windowStub) Show()                     { w.shown++ }
func (w *windowStub) Invalidate()               { w.invalidated++ }
func (w *windowStub) OnClosed(fn func()) func() { w.closed = fn; return func() { w.closed = nil } }

func TestMainWindowReactivation(t *testing.T) {
	var windows []*windowStub
	main := appwindow.New(func() appwindow.Window {
		window := &windowStub{}
		windows = append(windows, window)
		return window
	})
	main.Open()
	if len(windows) != 1 || windows[0].closed == nil {
		t.Fatal("first open did not create a managed window")
	}
	main.Open()
	if len(windows) != 1 || windows[0].shown != 1 {
		t.Fatal("activation should reuse existing window")
	}
	main.Refresh()
	if len(windows) != 1 || windows[0].invalidated != 1 || windows[0].shown != 1 {
		t.Fatal("navigation did not refresh existing window")
	}
	windows[0].closed()
	main.Open()
	if len(windows) != 2 || windows[1].closed == nil {
		t.Fatal("activation did not recreate closed window")
	}
	windows[1].closed()
	main.Refresh()
	if len(windows) != 3 || windows[2].invalidated != 1 {
		t.Fatal("menu navigation did not recreate and refresh closed window")
	}
	if windows[0].invalidated != 1 || windows[1].invalidated != 0 {
		t.Fatal("closed windows received further updates")
	}
}
