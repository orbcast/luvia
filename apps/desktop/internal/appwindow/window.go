// Package appwindow owns the main window's close and reactivation lifecycle.
package appwindow

// Window is the native window behavior needed by the application lifecycle.
// It is implemented by *mygo.Window; tests use a window without native resources.
type Window interface {
	Show()
	Invalidate()
	OnClosed(func()) func()
}

// Main creates one window at a time. All operations run on the application's UI thread.
type Main struct {
	create  func() Window
	current Window
}

func New(create func() Window) *Main { return &Main{create: create} }

func (m *Main) Open() {
	if m.current == nil {
		m.current = m.create()
		m.current.OnClosed(func() { m.current = nil })
	} else {
		m.current.Show()
	}
}

// Refresh recreates a closed window and updates its selected state.
func (m *Main) Refresh() {
	if m.current == nil {
		m.Open()
	}
	m.current.Invalidate()
}
