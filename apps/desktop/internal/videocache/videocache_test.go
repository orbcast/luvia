package videocache

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// mediaServer serves fixed bodies, counts hits and honours HEAD with a length.
type mediaServer struct {
	routes map[string][]byte
	mu     sync.Mutex
	hits   map[string]int
}

func newMediaServer(t *testing.T, routes map[string][]byte) (*httptest.Server, *mediaServer) {
	t.Helper()
	m := &mediaServer{routes: routes, hits: map[string]int{}}
	server := httptest.NewServer(m)
	t.Cleanup(server.Close)
	return server, m
}

func (m *mediaServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.mu.Lock()
	body, ok := m.routes[r.URL.Path]
	// The cache asks how large an object is before fetching it; that probe is
	// not a fetch of the object.
	if r.Method == http.MethodGet && !isSizeProbe(r.Header.Get("Range")) {
		m.hits[r.URL.Path]++
	}
	m.mu.Unlock()
	if !ok {
		http.NotFound(w, r)
		return
	}
	w.Header().Set("Content-Length", strconv.Itoa(len(body)))
	if r.Method == http.MethodHead {
		return
	}
	http.ServeContent(w, r, "", time.Time{}, bytes.NewReader(body))
}

func isSizeProbe(rangeHeader string) bool {
	return rangeHeader == "bytes=0-0"
}

func (m *mediaServer) count(path string) int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.hits[path]
}

func newCache(t *testing.T, dir string, opts Options) *Cache {
	t.Helper()
	cache, err := New(dir, opts)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = cache.Close() })
	return cache
}

func get(t *testing.T, rawURL string) (int, http.Header, []byte) {
	t.Helper()
	resp, err := http.Get(rawURL)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, body
}

func TestProxyCachesFileAndReuses(t *testing.T) {
	server, origin := newMediaServer(t, map[string][]byte{"/movie.mp4": []byte("0123456789abc")})
	dir := t.TempDir()
	cache := newCache(t, dir, Options{Client: server.Client()})
	proxy := cache.ProxyURL(server.URL + "/movie.mp4")

	status, _, body := get(t, proxy)
	if status != http.StatusOK || string(body) != "0123456789abc" {
		t.Fatalf("first read: %d %q", status, body)
	}
	status, _, body = get(t, proxy)
	if status != http.StatusOK || string(body) != "0123456789abc" {
		t.Fatalf("second read: %d %q", status, body)
	}
	if origin.count("/movie.mp4") != 1 {
		t.Fatalf("cache miss hit upstream %d times", origin.count("/movie.mp4"))
	}

	// The object survives a restart: a new cache over the same dir refetches
	// nothing.
	if err := cache.Close(); err != nil {
		t.Fatal(err)
	}
	reopened := newCache(t, dir, Options{Client: server.Client()})
	status, _, body = get(t, reopened.ProxyURL(server.URL+"/movie.mp4"))
	if status != http.StatusOK || string(body) != "0123456789abc" {
		t.Fatalf("reopened read: %d %q", status, body)
	}
	if origin.count("/movie.mp4") != 1 {
		t.Fatalf("reopened cache hit upstream: %d", origin.count("/movie.mp4"))
	}
}

func TestProxyServesRangeFromCache(t *testing.T) {
	server, _ := newMediaServer(t, map[string][]byte{"/clip.mp4": []byte("0123456789abc")})
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	proxy := cache.ProxyURL(server.URL + "/clip.mp4")
	get(t, proxy)

	request, _ := http.NewRequest(http.MethodGet, proxy, nil)
	request.Header.Set("Range", "bytes=2-5")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || string(body) != "2345" {
		t.Fatalf("range read: %d %q", resp.StatusCode, body)
	}
}

func TestProxyRewritesPlaylistKeysAndSegments(t *testing.T) {
	server, origin := newMediaServer(t, map[string][]byte{
		"/master.m3u8": []byte("#EXTM3U\n#EXT-X-STREAM-INF:BANDWIDTH=800000\nv/index.m3u8\n"),
		"/v/index.m3u8": []byte("#EXTM3U\n#EXT-X-VERSION:3\n#EXT-X-TARGETDURATION:4\n" +
			"#EXT-X-KEY:METHOD=AES-128,URI=\"../key.bin\"\n#EXTINF:4,\nseg0.ts\n#EXTINF:4,\nseg1.ts\n#EXT-X-ENDLIST\n"),
		"/v/seg0.ts": []byte("SEG0"),
		"/v/seg1.ts": []byte("SEG1"),
		"/key.bin":   []byte("KEYKEYKEYKEYKEYK"),
	})
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})

	status, header, body := get(t, cache.ProxyURL(server.URL+"/master.m3u8"))
	if status != http.StatusOK {
		t.Fatalf("master: %d", status)
	}
	if ct := header.Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("master content-type: %q", ct)
	}
	variantLine := firstURILine(string(body))
	wantVariant := cache.ProxyURL(server.URL + "/v/index.m3u8")
	if variantLine != wantVariant {
		t.Fatalf("variant line = %q, want %q", variantLine, wantVariant)
	}

	status, _, body = get(t, variantLine)
	if status != http.StatusOK {
		t.Fatalf("variant: %d", status)
	}
	text := string(body)
	if !strings.Contains(text, cache.ProxyURL(server.URL+"/v/seg0.ts")) {
		t.Fatalf("segment not rewritten: %q", text)
	}
	if !strings.Contains(text, `URI="`+cache.ProxyURL(server.URL+"/key.bin")+`"`) {
		t.Fatalf("key not rewritten: %q", text)
	}

	// Fetching a rewritten segment caches it and returns the real bytes.
	status, _, seg := get(t, cache.ProxyURL(server.URL+"/v/seg0.ts"))
	if status != http.StatusOK || string(seg) != "SEG0" {
		t.Fatalf("segment read: %d %q", status, seg)
	}
	if origin.count("/v/seg0.ts") != 1 {
		t.Fatalf("segment fetched %d times", origin.count("/v/seg0.ts"))
	}
}

func firstURILine(playlist string) string {
	for _, line := range strings.Split(playlist, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed != "" && !strings.HasPrefix(trimmed, "#") {
			return trimmed
		}
	}
	return ""
}

func TestProxyPassesTooLargeThroughWithoutCaching(t *testing.T) {
	big := []byte(strings.Repeat("x", 64))
	server, _ := newMediaServer(t, map[string][]byte{"/big.mp4": big})
	dir := t.TempDir()
	cache := newCache(t, dir, Options{Client: server.Client(), MaxBytes: 16})

	status, _, body := get(t, cache.ProxyURL(server.URL+"/big.mp4"))
	if status != http.StatusOK || len(body) != len(big) {
		t.Fatalf("passthrough: %d len=%d", status, len(body))
	}
	if cached := countCacheFiles(t, dir); cached != 0 {
		t.Fatalf("object over the budget was cached: %d files", cached)
	}
}

// An oversized object must keep serving the player's byte ranges: a player that
// asks for the first bytes and is handed the whole file instead stalls before
// the first frame.
func TestProxyKeepsRangeOnObjectOverBudget(t *testing.T) {
	big := []byte(strings.Repeat("x", 64))
	server, _ := newMediaServer(t, map[string][]byte{"/big.mp4": big})
	dir := t.TempDir()
	cache := newCache(t, dir, Options{Client: server.Client(), MaxBytes: 16})
	proxy := cache.ProxyURL(server.URL + "/big.mp4")
	get(t, proxy) // warms the size check the way a player's first request does

	request, _ := http.NewRequest(http.MethodGet, proxy, nil)
	request.Header.Set("Range", "bytes=0-7")
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || len(body) != 8 {
		t.Fatalf("oversized range read: %d len=%d", resp.StatusCode, len(body))
	}
	if closeErr := cache.Close(); closeErr != nil {
		t.Fatal(closeErr)
	}
}

func TestProxyEvictsLeastRecentlyUsed(t *testing.T) {
	server, _ := newMediaServer(t, map[string][]byte{
		"/a.mp4": []byte("AAAAAA"),
		"/b.mp4": []byte("BBBBBB"),
	})
	dir := t.TempDir()
	cache := newCache(t, dir, Options{Client: server.Client(), MaxBytes: 10})

	get(t, cache.ProxyURL(server.URL+"/a.mp4"))
	time.Sleep(10 * time.Millisecond)
	get(t, cache.ProxyURL(server.URL+"/b.mp4"))

	if _, err := os.Stat(cache.pathOf(keyOf(server.URL + "/a.mp4"))); !os.IsNotExist(err) {
		t.Fatalf("oldest object was not evicted: %v", err)
	}
	if _, err := os.Stat(cache.pathOf(keyOf(server.URL + "/b.mp4"))); err != nil {
		t.Fatalf("newest object missing: %v", err)
	}
}

func TestProxyURLPassesThroughLocalAddresses(t *testing.T) {
	cache := newCache(t, t.TempDir(), Options{})
	for _, raw := range []string{"", "file:///tmp/x.mp4", "mem://clip", "/relative/path"} {
		if got := cache.ProxyURL(raw); got != raw {
			t.Fatalf("ProxyURL(%q) = %q, want unchanged", raw, got)
		}
	}
	proxied := cache.ProxyURL("https://cdn.test/a.m3u8")
	if !strings.HasPrefix(proxied, cache.baseURL+"/media?u=") {
		t.Fatalf("http url not proxied: %q", proxied)
	}
}

func countCacheFiles(t *testing.T, dir string) int {
	t.Helper()
	descriptors, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, descriptor := range descriptors {
		if !descriptor.IsDir() && strings.HasSuffix(descriptor.Name(), ".cache") {
			count++
		}
	}
	return count
}

func TestNewRequiresDirectory(t *testing.T) {
	if _, err := New("", Options{}); err == nil {
		t.Fatal("empty dir must fail")
	}
}

func TestProxyRejectsNonRemoteURL(t *testing.T) {
	cache := newCache(t, t.TempDir(), Options{})
	for _, query := range []string{"", "?u=" + "file%3A%2F%2F%2Ftmp%2Fx"} {
		resp, err := http.Get(cache.baseURL + "/media" + query)
		if err != nil {
			t.Fatal(err)
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusBadRequest {
			t.Fatalf("query %q: status %d", query, resp.StatusCode)
		}
	}
	var absent *Cache
	if got := absent.ProxyURL("http://cdn.test/a.m3u8"); got != "http://cdn.test/a.m3u8" {
		t.Fatalf("nil cache proxied: %q", got)
	}
}

func TestProxyReturnsBadGatewayWhenUpstreamFails(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	resp, err := http.Get(cache.ProxyURL(server.URL + "/missing.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusBadGateway {
		t.Fatalf("status = %d, want 502", resp.StatusCode)
	}
}

func TestProxyDetectsPlaylistByContentType(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/list":
			if r.Method == http.MethodHead {
				return
			}
			w.Header().Set("Content-Type", "application/vnd.apple.mpegurl")
			w.Write([]byte("#EXTM3U\n#EXTINF:4,\nseg.ts\n"))
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	status, header, body := get(t, cache.ProxyURL(server.URL+"/list"))
	if status != http.StatusOK {
		t.Fatalf("status %d", status)
	}
	if ct := header.Get("Content-Type"); !strings.Contains(ct, "mpegurl") {
		t.Fatalf("content-type %q", ct)
	}
	if want := cache.ProxyURL(server.URL + "/seg.ts"); !strings.Contains(string(body), want) {
		t.Fatalf("segment not rewritten by content-type: %q", body)
	}
}

func TestProxySharesOneDownloadAcrossRequests(t *testing.T) {
	var mu sync.Mutex
	gets := 0
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodHead {
			w.Header().Set("Content-Length", "5")
			return
		}
		mu.Lock()
		gets++
		mu.Unlock()
		time.Sleep(100 * time.Millisecond)
		w.Write([]byte("HELLO"))
	}))
	defer server.Close()
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	proxy := cache.ProxyURL(server.URL + "/x.mp4")

	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			resp, err := http.Get(proxy)
			if err != nil {
				errs <- err
				return
			}
			defer resp.Body.Close()
			body, _ := io.ReadAll(resp.Body)
			if string(body) != "HELLO" {
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	mu.Lock()
	defer mu.Unlock()
	// Two requests open the object: the size probe and the transfer itself.
	if gets != 2 {
		t.Fatalf("upstream opened %d times, want 2", gets)
	}
}

// A player asking for the first bytes of a large film must not wait for the
// whole download: the response carries the prefix as it arrives, and the bytes
// it was handed come from the start of the object, not from an offset upstream
// answered at random.
func TestProxyAnswersRangeFromTheGrowingPrefix(t *testing.T) {
	big := []byte(strings.Repeat("y", 1<<16))
	server, _ := newMediaServer(t, map[string][]byte{"/film.mp4": big})
	dir := t.TempDir()
	cache := newCache(t, dir, Options{Client: server.Client()})
	proxy := cache.ProxyURL(server.URL + "/film.mp4")

	request, _ := http.NewRequest(http.MethodGet, proxy, nil)
	request.Header.Set("Range", "bytes=0-63")
	start := time.Now()
	resp, err := http.DefaultClient.Do(request)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	head, _ := io.ReadAll(io.LimitReader(resp.Body, 64))
	if resp.StatusCode != http.StatusPartialContent {
		t.Fatalf("status %d", resp.StatusCode)
	}
	if rangeHeader := resp.Header.Get("Content-Range"); !strings.HasPrefix(rangeHeader, "bytes 0-") {
		t.Fatalf("range answered from %q", rangeHeader)
	}
	if len(head) != 64 || head[0] != 'y' {
		t.Fatalf("first bytes: %d %q", len(head), head[:min(4, len(head))])
	}
	if elapsed := time.Since(start); elapsed > 5*time.Second {
		t.Fatalf("the player waited %s for its first bytes", elapsed)
	}
}

// A player that asks for a different part of the same film must be served from
// the transfer already open, not from nowhere: the response carries the bytes it
// asked for as soon as the prefix has reached them.
func TestProxyServesLaterRangeWhileTheTransferGrows(t *testing.T) {
	big := []byte(strings.Repeat("z", 1<<16))
	server, _ := newMediaServer(t, map[string][]byte{"/film.mp4": big})
	cache := newCache(t, t.TempDir(), Options{Client: server.Client()})
	proxy := cache.ProxyURL(server.URL + "/film.mp4")

	first, _ := http.NewRequest(http.MethodGet, proxy, nil)
	first.Header.Set("Range", "bytes=0-1023")
	resp, err := http.DefaultClient.Do(first)
	if err != nil {
		t.Fatal(err)
	}
	io.Copy(io.Discard, resp.Body)
	resp.Body.Close()

	second, _ := http.NewRequest(http.MethodGet, proxy, nil)
	second.Header.Set("Range", "bytes=0-511")
	resp, err = http.DefaultClient.Do(second)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusPartialContent || len(body) != 512 {
		t.Fatalf("second read: %d len=%d", resp.StatusCode, len(body))
	}
}
