package videocache

import (
	"sync"
	"time"
)

// clock is the time transfers are timed by. Tests move it to read a rate
// without waiting on a network. The function itself is swapped by tests while
// transfers are running, so it is read under the lock like the rest of the
// cache's state.
func (c *Cache) clock() time.Time {
	c.mu.Lock()
	now := c.now
	c.mu.Unlock()
	return now()
}

// progressQuiet is how long after its last byte a transfer stops being
// reported. The player reads a stored object from disk, so its bytes are on
// their way the moment they land; a transfer that went quiet has finished or
// stalled, and its speed would only mislead a viewer still on another wait.
const progressQuiet = 2 * time.Second

// progressTick is how often a transfer shows what it moved. A viewer reading the
// speed wants a frame a second, not one per chunk.
const progressTick = 500 * time.Millisecond

// Transfer is what one download has moved so far.
type Transfer struct {
	// Done is the bytes received.
	Done int64
	// Rate is the speed since the transfer opened, in bytes per second.
	Rate float64
	// Started is when the transfer opened and Moved when it last received a
	// byte; Cache.Transfer uses both to tell a live transfer from a stale one.
	Started time.Time
	Moved   time.Time
}

// readAt is the transfer as it stands at now: the rate averaged over its whole
// length, because a single slow chunk belongs to the network rather than to the
// download a viewer is waiting on.
func (t Transfer) readAt(now time.Time) Transfer {
	if span := now.Sub(t.Started); span > 0 {
		t.Rate = float64(t.Done) / span.Seconds()
	}
	return t
}

// mover is the download a Cache publishes while it runs. A transfer with no
// cache to publish to, as in a test of a download alone, gets a nil mover,
// whose methods do nothing. A warm-up for an episode nobody watches yet gets a
// mover too, but an unpublished one: it still counts what it moved, and shows
// nothing, because the speed a wait reads belongs to the playback it waits on.
type mover struct {
	cache *Cache
	watch bool
	// seq is the transfer's place in the order of transfers the cache has
	// begun, which decides whether it still owns what the cache shows.
	seq    uint64
	mu     sync.Mutex
	done   int64
	start  time.Time
	lastUp time.Time
	// shown is whether a frame of this transfer was published: the first byte
	// is shown at once, and the ones after it at the pace of the tick.
	shown bool
}

// begin starts a transfer that already stored done bytes. watch says whether a
// playback is waiting on it, and so whether it is shown.
func (c *Cache) begin(done int64, watch bool) *mover {
	if c == nil {
		return nil
	}
	now := c.clock()
	c.mu.Lock()
	c.begun++
	seq := c.begun
	c.mu.Unlock()
	return &mover{cache: c, watch: watch, seq: seq, done: done, start: now}
}

// Write counts the bytes of a transfer, which the download copies through it.
func (m *mover) Write(p []byte) (int, error) {
	if m == nil {
		return len(p), nil
	}
	m.mu.Lock()
	m.done += int64(len(p))
	m.mu.Unlock()
	m.show()
	return len(p), nil
}

// show publishes the transfer, if a playback waits on it and enough time passed
// since the last look.
func (m *mover) show() {
	if m == nil || !m.watch {
		return
	}
	now := m.cache.clock()
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.shown && now.Sub(m.lastUp) < progressTick {
		return
	}
	m.shown, m.lastUp = true, now
	c := m.cache
	c.mu.Lock()
	// A newer transfer is the one a viewer is waiting on, so an older one
	// never takes its place.
	if c.shownSeq <= m.seq {
		c.shown = Transfer{Done: m.done, Started: m.start, Moved: now}
		c.shownSeq = m.seq
	}
	c.mu.Unlock()
}

// settle stops the transfer and forgets it, so a viewer on the wait that follows
// the transfer does not read back the speed of the one before it.
func (m *mover) settle() {
	if m == nil {
		return
	}
	c := m.cache
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.shownSeq == m.seq {
		c.shown, c.shownSeq = Transfer{}, 0
	}
}

// Transfer reports the download the proxy is moving right now, or the most
// recent one while it is recent enough to still be the wait a viewer is in. It
// is read on the main thread.
func (c *Cache) Transfer() Transfer {
	if c == nil {
		return Transfer{}
	}
	c.mu.Lock()
	current, seq := c.shown, c.shownSeq
	c.mu.Unlock()
	if seq == 0 {
		return Transfer{}
	}
	now := c.clock()
	if now.Sub(current.Moved) > progressQuiet {
		return Transfer{}
	}
	return current.readAt(now)
}
