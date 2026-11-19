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
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

const (
	maxDownload = 12 << 20
	parallelism = 4

	// defaultDiskBytes bounds the artwork kept on disk. Posters average well
	// under a hundred kilobytes, so this holds a few thousand of them.
	defaultDiskBytes = 128 << 20
	// defaultMemoryBytes bounds the decoded bitmaps, counted as their pixels.
	// The mip levels made for drawing them smaller and the copies a GPU
	// renderer uploads are not counted, so the real footprint is a multiple of
	// this: a poster decodes to a few megabytes and this holds dozens of them.
	defaultMemoryBytes = 96 << 20

	// cacheSuffix names the artwork files. Only these take part in the disk
	// budget: the temporary file a write goes through is renamed away.
	cacheSuffix = ".img"
)

type entry struct {
	bitmap  *ui.Bitmap
	err     error
	loading bool
	// used is when a page last asked for this artwork, for the memory budget.
	used time.Time
	// size is what the decoded bitmap costs, zero until it decodes.
	size int64
}

// Options configures a Loader.
type Options struct {
	// Client fetches artwork. Nil means http.DefaultClient.
	Client *http.Client
	// MaxBytes bounds the artwork kept on disk. Zero takes the default and a
	// negative value keeps nothing.
	MaxBytes int64
	// MemoryBytes bounds the decoded bitmaps kept in memory. Zero takes the
	// default and a negative value keeps none.
	MemoryBytes int64
}

// Loader downloads artwork, decodes it on the main thread and remembers both
// outcomes so a broken or slow URL is never fetched twice.
type Loader struct {
	client      *http.Client
	dir         string
	run         func(func())
	mu          sync.Mutex
	entries     map[string]*entry
	sem         chan struct{}
	changed     func()
	maxBytes    int64
	memoryBytes int64
	// disk counts the cache files, held counts the decoded bitmaps.
	disk int64
	held int64
}

// New prepares a loader that keeps downloaded bytes in dir.
func New(dir string, opts Options) *Loader {
	client := opts.Client
	if client == nil {
		client = http.DefaultClient
	}
	maxBytes := opts.MaxBytes
	if maxBytes == 0 {
		maxBytes = defaultDiskBytes
	}
	memoryBytes := opts.MemoryBytes
	if memoryBytes == 0 {
		memoryBytes = defaultMemoryBytes
	}
	l := &Loader{
		client: client, dir: dir, run: mygo.RunOnMain,
		entries: map[string]*entry{}, sem: make(chan struct{}, parallelism),
		maxBytes: maxBytes, memoryBytes: memoryBytes,
	}
	if dir != "" && maxBytes > 0 {
		l.mu.Lock()
		l.evict("")
		l.mu.Unlock()
	}
	return l
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
		e.used = time.Now()
		l.mu.Unlock()
		return bitmap
	}
	e := &entry{loading: true, used: time.Now()}
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
	if data := l.read(path); len(data) > 0 {
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

// read returns a cache file and dates it now, so eviction sees the order pages
// asked for artwork in, not the order it was downloaded in.
func (l *Loader) read(path string) []byte {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil || len(data) == 0 {
		return nil
	}
	now := time.Now()
	os.Chtimes(path, now, now)
	return data
}

// remember writes the cache file through a temporary name, so a killed process
// never leaves a truncated image behind, and drops the ones nobody read for
// longest when the disk budget is spent. A failing cache only costs a refetch.
func (l *Loader) remember(path string, data []byte) {
	if path == "" {
		return
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.maxBytes <= 0 || int64(len(data)) > l.maxBytes {
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
		return
	}
	l.disk += int64(len(data))
	l.evict(path)
}

// evict counts the cache files and removes the ones nobody read for longest
// until the disk budget holds again. keep is the file just written.
func (l *Loader) evict(keep string) {
	if l.dir == "" || l.maxBytes <= 0 {
		return
	}
	type cached struct {
		path string
		used time.Time
		size int64
	}
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		l.disk = 0
		return
	}
	var total int64
	var files []cached
	for _, item := range entries {
		if item.IsDir() || !strings.HasSuffix(item.Name(), cacheSuffix) {
			continue
		}
		info, err := item.Info()
		if err != nil {
			continue
		}
		total += info.Size()
		files = append(files, cached{
			path: filepath.Join(l.dir, item.Name()),
			used: info.ModTime(),
			size: info.Size(),
		})
	}
	l.disk = total
	if total <= l.maxBytes {
		return
	}
	sort.Slice(files, func(i, j int) bool { return files[i].used.Before(files[j].used) })
	for _, item := range files {
		if l.disk <= l.maxBytes {
			return
		}
		if item.path == keep {
			continue
		}
		if err := os.Remove(item.path); err != nil {
			continue
		}
		l.disk -= item.size
	}
}

func (l *Loader) finish(e *entry, bitmap *ui.Bitmap, err error) {
	l.mu.Lock()
	e.bitmap, e.err, e.loading = bitmap, err, false
	e.used = time.Now()
	if bitmap != nil {
		width, height := bitmap.Size()
		e.size = int64(width) * int64(height) * 4
		l.held += e.size
		l.trim(e)
	}
	changed := l.changed
	l.mu.Unlock()
	if changed != nil {
		changed()
	}
}

// trim drops the decoded bitmaps nobody looked at for longest until the memory
// budget holds. Failed loads stay: they cost nothing and remember that the URL
// is broken. A page that asks for a dropped artwork again reads it from the
// disk cache and decodes it once more.
func (l *Loader) trim(keep *entry) {
	if l.memoryBytes <= 0 || l.held <= l.memoryBytes {
		return
	}
	type bitmap struct {
		url string
		e   *entry
	}
	var bitmaps []bitmap
	for url, e := range l.entries {
		if e.bitmap != nil && e != keep {
			bitmaps = append(bitmaps, bitmap{url: url, e: e})
		}
	}
	sort.Slice(bitmaps, func(i, j int) bool { return bitmaps[i].e.used.Before(bitmaps[j].e.used) })
	for _, item := range bitmaps {
		if l.held <= l.memoryBytes {
			return
		}
		l.held -= item.e.size
		delete(l.entries, item.url)
	}
}

func (l *Loader) cachePath(url string) string {
	if l.dir == "" {
		return ""
	}
	sum := sha256.Sum256([]byte(url))
	return filepath.Join(l.dir, hex.EncodeToString(sum[:])+cacheSuffix)
}
