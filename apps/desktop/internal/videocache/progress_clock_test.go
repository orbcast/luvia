package videocache

import "time"

// moveClock sets the clock transfers are timed by, so a test reads a speed
// without waiting on one.
func (c *Cache) moveClock(to time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	at := to
	c.now = func() time.Time { return at }
}
