package media

import (
	"bytes"
	"image"
	"image/color"
	"image/png"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

// artwork returns a tiny valid PNG so the decoder has something real to read.
func artwork(t *testing.T) []byte {
	t.Helper()
	img := image.NewRGBA(image.Rect(0, 0, 4, 4))
	for x := 0; x < 4; x++ {
		img.Set(x, x, color.RGBA{R: 200, G: 160, B: 90, A: 255})
	}
	var buffer bytes.Buffer
	if err := png.Encode(&buffer, img); err != nil {
		t.Fatal(err)
	}
	return buffer.Bytes()
}

// counting serves one body and counts how often it was asked for.
type counting struct {
	mu       sync.Mutex
	requests int
	status   int
	body     []byte
}

func (c *counting) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	c.mu.Lock()
	c.requests++
	status, body := c.status, c.body
	c.mu.Unlock()
	if status == 0 {
		status = http.StatusOK
	}
	w.WriteHeader(status)
	w.Write(body)
}

func (c *counting) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.requests
}

// wait blocks until the loader reports a finished download.
func wait(t *testing.T, changed <-chan struct{}) {
	t.Helper()
	select {
	case <-changed:
	case <-time.After(5 * time.Second):
		t.Fatal("loader never finished")
	}
}

func newLoader(t *testing.T, dir string, client *http.Client) (*Loader, <-chan struct{}) {
	t.Helper()
	loader := New(dir, Options{Client: client})
	loader.SetRunner(func(fn func()) { fn() })
	changed := make(chan struct{}, 8)
	loader.SetChanged(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	return loader, changed
}

// newBounded prepares a loader with explicit budgets and a synchronous runner.
func newBounded(t *testing.T, dir string, client *http.Client, opts Options) (*Loader, <-chan struct{}) {
	t.Helper()
	opts.Client = client
	loader := New(dir, opts)
	loader.SetRunner(func(fn func()) { fn() })
	changed := make(chan struct{}, 8)
	loader.SetChanged(func() {
		select {
		case changed <- struct{}{}:
		default:
		}
	})
	return loader, changed
}

// cacheFiles lists the artwork files a directory holds.
func cacheFiles(t *testing.T, dir string) []string {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	var names []string
	for _, item := range entries {
		if strings.HasSuffix(item.Name(), cacheSuffix) {
			names = append(names, item.Name())
		}
	}
	return names
}

// age backdates a cache file so eviction treats it as older than the rest.
func age(t *testing.T, path string, when time.Time) {
	t.Helper()
	if err := os.Chtimes(path, when, when); err != nil {
		t.Fatal(err)
	}
}

func TestBitmapDecodesAndCachesOnDisk(t *testing.T) {
	server := httptest.NewServer(&counting{body: artwork(t)})
	defer server.Close()
	dir := t.TempDir()
	loader, changed := newLoader(t, dir, server.Client())

	if loader.Bitmap("") != nil {
		t.Fatal("empty URL must not load")
	}
	if loader.Bitmap(server.URL+"/a.png") != nil {
		t.Fatal("first call returns nil while loading")
	}
	wait(t, changed)
	bitmap := loader.Bitmap(server.URL + "/a.png")
	if bitmap == nil {
		t.Fatal("artwork did not load")
	}
	if width, height := bitmap.Size(); width != 4 || height != 4 {
		t.Fatalf("decoded %dx%d", width, height)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("cache files: %d", len(entries))
	}

	reopened, reopenedChanges := newLoader(t, dir, server.Client())
	reopened.Prefetch(server.URL + "/a.png")
	wait(t, reopenedChanges)
	if reopened.Bitmap(server.URL+"/a.png") == nil {
		t.Fatal("cached artwork not reused")
	}
}

func TestCachedFileSkipsNetwork(t *testing.T) {
	counter := &counting{body: artwork(t)}
	endpoint := httptest.NewServer(counter)
	defer endpoint.Close()
	dir := t.TempDir()

	first, changed := newLoader(t, dir, endpoint.Client())
	first.Prefetch(endpoint.URL + "/b.png")
	wait(t, changed)
	if counter.count() != 1 {
		t.Fatalf("requests: %d", counter.count())
	}

	second, secondChanges := newLoader(t, dir, endpoint.Client())
	second.Prefetch(endpoint.URL + "/b.png")
	wait(t, secondChanges)
	if counter.count() != 1 {
		t.Fatalf("cache miss hit the network: %d", counter.count())
	}
}

func TestFailedLoadIsRemembered(t *testing.T) {
	counter := &counting{status: http.StatusNotFound}
	server := httptest.NewServer(counter)
	defer server.Close()
	loader, changed := newLoader(t, t.TempDir(), server.Client())

	loader.Prefetch(server.URL + "/missing.png")
	wait(t, changed)
	if !loader.Failed(server.URL + "/missing.png") {
		t.Fatal("failure not recorded")
	}
	if loader.Bitmap(server.URL+"/missing.png") != nil {
		t.Fatal("failed artwork returned a bitmap")
	}
	if counter.count() != 1 {
		t.Fatalf("failed artwork retried: %d", counter.count())
	}
}

func TestUndecodableBodyFails(t *testing.T) {
	server := httptest.NewServer(&counting{body: []byte("not an image")})
	defer server.Close()
	loader, changed := newLoader(t, "", server.Client())
	loader.Prefetch(server.URL + "/broken.png")
	wait(t, changed)
	if !loader.Failed(server.URL + "/broken.png") {
		t.Fatal("decode failure not recorded")
	}
}

func TestBitmapSurvivesMissingCacheDir(t *testing.T) {
	server := httptest.NewServer(&counting{body: artwork(t)})
	defer server.Close()
	loader, changed := newLoader(t, filepath.Join(t.TempDir(), "missing"), server.Client())
	loader.Prefetch(server.URL + "/c.png")
	wait(t, changed)
	if loader.Bitmap(server.URL+"/c.png") == nil {
		t.Fatal("artwork in a missing cache dir did not load")
	}
}

func TestDiskBudgetDropsTheLeastRecentlyRead(t *testing.T) {
	server := httptest.NewServer(&counting{body: artwork(t)})
	defer server.Close()
	dir := t.TempDir()

	// Two files fill a budget that holds exactly two.
	first, changed := newBounded(t, dir, server.Client(), Options{})
	first.Prefetch(server.URL + "/a.png")
	wait(t, changed)
	first.Prefetch(server.URL + "/b.png")
	wait(t, changed)
	files := cacheFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("cache files: %d", len(files))
	}
	info, err := os.Stat(filepath.Join(dir, files[0]))
	if err != nil {
		t.Fatal(err)
	}
	size := info.Size()

	// b.png is the newest; a.png was written first.
	old := time.Now().Add(-time.Hour)
	for _, name := range files {
		age(t, filepath.Join(dir, name), old)
	}
	newest := filepath.Join(dir, filepath.Base(first.cachePath(server.URL+"/b.png")))
	age(t, newest, time.Now())

	// A third artwork must go in, so the least recently read one goes out.
	bounded, boundedChanges := newBounded(t, dir, server.Client(), Options{MaxBytes: size * 2})
	bounded.Prefetch(server.URL + "/c.png")
	wait(t, boundedChanges)
	files = cacheFiles(t, dir)
	if len(files) != 2 {
		t.Fatalf("cache files after eviction: %d", len(files))
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.Base(first.cachePath(server.URL+"/a.png")))); !os.IsNotExist(err) {
		t.Fatal("the least recently read artwork was not evicted")
	}
	if _, err := os.Stat(filepath.Join(dir, filepath.Base(first.cachePath(server.URL+"/c.png")))); err != nil {
		t.Fatalf("the new artwork was not stored: %v", err)
	}
}

func TestReadingAnArtworkDatesItAgain(t *testing.T) {
	counter := &counting{body: artwork(t)}
	server := httptest.NewServer(counter)
	defer server.Close()
	dir := t.TempDir()

	first, changed := newBounded(t, dir, server.Client(), Options{})
	first.Prefetch(server.URL + "/a.png")
	wait(t, changed)
	path := filepath.Join(dir, filepath.Base(first.cachePath(server.URL+"/a.png")))
	age(t, path, time.Now().Add(-time.Hour))

	// A second loader reads it from disk, which dates the file now.
	second, secondChanges := newBounded(t, dir, server.Client(), Options{})
	second.Prefetch(server.URL + "/a.png")
	wait(t, secondChanges)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if time.Since(info.ModTime()) > time.Minute {
		t.Fatalf("reading did not date the cache file: %v", info.ModTime())
	}
	if counter.count() != 1 {
		t.Fatalf("the cached artwork was fetched again: %d", counter.count())
	}
}

func TestMemoryBudgetDropsTheLeastRecentlyUsed(t *testing.T) {
	server := httptest.NewServer(&counting{body: artwork(t)})
	defer server.Close()
	loader, changed := newBounded(t, t.TempDir(), server.Client(), Options{MemoryBytes: 64})

	loader.Prefetch(server.URL + "/a.png")
	wait(t, changed)
	loader.Prefetch(server.URL + "/b.png")
	wait(t, changed)

	loader.mu.Lock()
	held := len(loader.entries)
	total := loader.held
	loader.mu.Unlock()
	if held != 1 {
		t.Fatalf("decoded bitmaps kept: %d", held)
	}
	if total > 64 {
		t.Fatalf("memory budget exceeded: %d", total)
	}
	if loader.Failed(server.URL + "/a.png") {
		t.Fatal("an evicted artwork was remembered as failed")
	}
}

func TestFailedLoadsStayOutsideTheMemoryBudget(t *testing.T) {
	counter := &counting{status: http.StatusNotFound}
	server := httptest.NewServer(counter)
	defer server.Close()
	loader, changed := newBounded(t, t.TempDir(), server.Client(), Options{MemoryBytes: 64})

	loader.Prefetch(server.URL + "/missing.png")
	wait(t, changed)
	if !loader.Failed(server.URL + "/missing.png") {
		t.Fatal("a failure was dropped by the memory budget")
	}
	if counter.count() != 1 {
		t.Fatalf("failed artwork retried: %d", counter.count())
	}
}

func TestNegativeDiskBudgetKeepsNothing(t *testing.T) {
	server := httptest.NewServer(&counting{body: artwork(t)})
	defer server.Close()
	dir := t.TempDir()
	loader, changed := newBounded(t, dir, server.Client(), Options{MaxBytes: -1})

	loader.Prefetch(server.URL + "/a.png")
	wait(t, changed)
	if loader.Bitmap(server.URL+"/a.png") == nil {
		t.Fatal("artwork did not load")
	}
	if files := cacheFiles(t, dir); len(files) != 0 {
		t.Fatalf("cache files written with no budget: %d", len(files))
	}
}
