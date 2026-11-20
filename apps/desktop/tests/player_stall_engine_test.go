package tests

import (
	"io"
	"sync"
	"testing"

	"github.com/egoist/mygo/ui"
)

// A stream that opened and drew nothing is a stall the player has to let the
// viewer see: the system's engines report it as neither ready, buffering, nor
// failed. These decoders stand in for such streams on the player every platform
// builds, so no system media framework is involved. The scripts built on them
// are in player_stall_test.go.

// heldStream is a stream that opened and gave no picture: the player opens it
// off the test's goroutine, because opening waits on the first frame. The test
// ends the stall when it is done, for the player to release.
func heldStream(t *testing.T) <-chan *ui.VideoPlayer {
	t.Helper()
	held := &heldDecoder{stall: make(chan struct{})}
	t.Cleanup(held.release)
	built := make(chan *ui.VideoPlayer, 1)
	go func() {
		pl, err := ui.NewVideoPlayer(held)
		if err != nil {
			// The player is built before the framework is asked for its first
			// frame, so a refused stream leaves the test with no player at all.
			close(built)
			return
		}
		built <- pl
	}()
	return built
}

// heldDecoder stalls on its first frame, as a stream that opened and drew no
// picture does, until release.
type heldDecoder struct {
	stall chan struct{}
	once  sync.Once
}

func (d *heldDecoder) Next() (ui.VideoFrame, error) {
	<-d.stall
	return ui.VideoFrame{}, io.ErrUnexpectedEOF
}

func (d *heldDecoder) Rewind() error { return nil }

func (d *heldDecoder) release() { d.once.Do(func() { close(d.stall) }) }

// newFailedPlayer returns a player that fails to open with err, as a decoder
// that rejects the stream does.
func newFailedPlayer(t *testing.T, err error) (*ui.VideoPlayer, error) {
	t.Helper()
	return ui.NewVideoPlayer(failingDecoder{err: err})
}

// failingDecoder fails on its first frame: the stream a player cannot show.
type failingDecoder struct{ err error }

func (d failingDecoder) Next() (ui.VideoFrame, error) { return ui.VideoFrame{}, d.err }
func (d failingDecoder) Rewind() error                { return nil }
