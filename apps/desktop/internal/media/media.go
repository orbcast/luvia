// Package media loads remote artwork once and caches it: the bytes on disk and
// the decoded bitmap in memory. Pages ask for a bitmap by URL and redraw when
// the loader reports a new one.
package media

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"sync"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	maxDownload = 12 << 20
	parallelism = 4
)

type entry struct {
	bitmap  *ui.Bitmap
	err     error
	loading bool
}

// Loader downloads artwork, decodes it on the main thread and remembers both
// outcomes so a broken or slow URL is never fetched twice.
type Loader struct {
	client  *http.Client
	dir     string
	run     func(func())
	mu      sync.Mutex
	entries map[string]*entry
	sem     chan struct{}
	changed func()
}

// New prepares a loader that keeps downloaded bytes in dir.
func New(dir string, client *http.Client) *Loader {
	if client == nil {
		client = http.DefaultClient
	}
	return &Loader{
		client: client, dir: dir, run: mygo.RunOnMain,
		entries: map[string]*entry{}, sem: make(chan struct{}, parallelism),
	}
}

// SetRunner chooses where decoded bitmaps are handed back. Tests pass a
// synchronous runner; the app keeps the default, the main thread.
func (l *Loader) SetRunner(run func(func())) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.run = run
}

// SetChanged registers the redraw that runs after a load finishes.
func (l *Loader) SetChanged(fn func()) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.changed = fn
}

// Bitmap returns the artwork at url once it is decoded, and starts loading it
// otherwise. It returns nil for an empty URL or one that failed to load.
func (l *Loader) Bitmap(url string) *ui.Bitmap {
	if url == "" {
		return nil
	}
	l.mu.Lock()
	if e, ok := l.entries[url]; ok {
		bitmap := e.bitmap
		l.mu.Unlock()
		return bitmap
	}
	e := &entry{loading: true}
	l.entries[url] = e
	l.mu.Unlock()
	go l.load(url, e)
	return nil
}

// Prefetch starts loading url without asking for its bitmap.
func (l *Loader) Prefetch(url string) {
	l.Bitmap(url)
}

// Failed reports whether url could not be loaded.
func (l *Loader) Failed(url string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()
	e, ok := l.entries[url]
	return ok && e.err != nil
}

func (l *Loader) load(url string, e *entry) {
	l.sem <- struct{}{}
	data, err := l.bytes(url)
	<-l.sem
	if err != nil {
		l.finish(e, nil, err)
		return
	}
	l.mu.Lock()
	run := l.run
	l.mu.Unlock()
	decode := func() {
		bitmap, err := ui.DecodeBitmap(data)
		l.finish(e, bitmap, err)
	}
	if run == nil {
		decode()
		return
	}
	run(decode)
}

func (l *Loader) bytes(url string) ([]byte, error) {
	path := l.cachePath(url)
	if data, err := os.ReadFile(path); err == nil && len(data) > 0 {
		return data, nil
	}
	request, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("artwork %s: %w", url, err)
	}
	response, err := l.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("artwork %s: %w", url, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("artwork %s: upstream returned %d", url, response.StatusCode)
	}
	data, err := io.ReadAll(io.LimitReader(response.Body, maxDownload+1))
	if err != nil {
		return nil, fmt.Errorf("artwork %s: %w", url, err)
	}
	if len(data) > maxDownload {
		return nil, fmt.Errorf("artwork %s: larger than %d bytes", url, maxDownload)
	}
	if len(data) == 0 {
		return nil, fmt.Errorf("artwork %s: empty response", url)
	}
	l.remember(path, data)
	return data, nil
}

// remember writes the cache file through a temporary name, so a killed process
// never leaves a truncated image behind. A failing cache only costs a refetch.
func (l *Loader) remember(path string, data []byte) {
	if l.dir == "" {
		return
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return
	}
	temp := path + ".tmp"
	if err := os.WriteFile(temp, data, 0o644); err != nil {
		return
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
	}
}

func (l *Loader) finish(e *entry, bitmap *ui.Bitmap, err error) {
	l.mu.Lock()
	e.bitmap, e.err, e.loading = bitmap, err, false
	changed := l.changed
	l.mu.Unlock()
	if changed != nil {
		changed()
	}
}

func (l *Loader) cachePath(url string) string {
	if l.dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(l.dir, hex.EncodeToString(sum[:])+".img")
}
