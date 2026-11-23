// Package videocache serves remote video through a local HTTP proxy backed by a
// bounded disk cache. A repeated watch is answered from disk instead of the
// network, and an HLS playlist is rewritten so its variant, segment and key
// URIs also travel through the proxy and are cached the same way.
//
// The player opens the proxy URL; the proxy is transparent at the HTTP level,
// so mp4 and HLS both work without parsing media. Only the bytes a playback asks
// for are fetched, and a partial object is kept as a growing prefix that later
// ranges are cut from. An object larger than the whole cache budget is passed
// straight through, the player's own request and all. Prefetch warms a playlist
// the viewer is likely to open next, out of a share of the budget the playback
// itself never has to give up.
package videocache

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxBytes  int64 = 2 << 30 // 2 GiB on disk
	defaultUserAgent       = "Luvia/1.0 (+videocache)"
)

// prefetchSegments is how many leading entries of a playlist a prefetch warms.
// An episode opens on its first seconds, so a few segments cover the gap between
// the viewer choosing it and the player asking for its first bytes.
const prefetchSegments = 3

// prefetchHops is how deep a prefetch follows playlists: a master playlist
// names variants, and a variant names the segments that are worth warming.
const prefetchHops = 2

// playlistLimit bounds how much of a prefetched object is read for parsing.
const playlistLimit = 1 << 20

// prefetchShare is the fraction of the byte budget the bytes written ahead of
// need may occupy. Warming is a guess about what comes next; it must never take
// the room a playback needs for what the viewer is actually watching.
const prefetchShare = 4

// playlistTag opens every HLS playlist.
var playlistTag = []byte("#EXTM3U")

// errTooLarge reports an object bigger than MaxBytes; it is passed through
// without being cached.
var errTooLarge = errors.New("videocache: object exceeds the cache byte budget")

// errProbe marks an upstream that cannot be asked for a byte range; the player's
// own request is passed through instead.
var errProbe = errors.New("videocache: upstream cannot be probed")

// errNotStored marks a transfer that ended without the bytes a request asked
// for, so the request is answered from upstream instead.
var errNotStored = errors.New("videocache: the transfer did not reach the requested bytes")

// Options tune a Cache. The zero value of each field uses a default.
type Options struct {
	MaxBytes int64
	Client   *http.Client
}

// Cache is a loopback HTTP proxy over a bounded on-disk cache. Point a player at
// ProxyURL and call Close when the app quits.
type Cache struct {
	dir      string
	maxBytes int64
	client   *http.Client

	listener net.Listener
	server   *http.Server
	baseURL  string

	// ctx aborts the transfers a prefetch started when the cache closes.
	ctx    context.Context
	cancel context.CancelFunc

	mu    sync.Mutex
	total int64
	// shown is the transfer a wait reads a speed from, and shownSeq the place
	// of the transfer it came from in the order transfers began.
	shown    Transfer
	shownSeq uint64
	begun    uint64
	// now is the clock transfers are timed by. Tests move it to read a speed
	// without waiting on a network.
	now      func() time.Time
	inflight map[string]*call

	// unwatched holds the keys stored ahead of need and not read since. Those
	// bytes are evicted before anything a playback asked for, so scrubbing back
	// into what was just watched does not have to download it again.
	unwatched map[string]struct{}
}

// call is one in-flight download shared by every request for the same URL.
type call struct {
	done chan struct{}
	err  error
}

// New opens the cache in dir and starts its loopback proxy.
func New(dir string, opts Options) (*Cache, error) {
	if dir == "" {
		return nil, errors.New("videocache: empty cache directory")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, fmt.Errorf("videocache: %w", err)
	}
	c := &Cache{
		dir: dir, maxBytes: opts.MaxBytes,
		client: opts.Client, inflight: map[string]*call{},
		unwatched: map[string]struct{}{}, now: time.Now,
	}
	c.ctx, c.cancel = context.WithCancel(context.Background())
	if c.maxBytes <= 0 {
		c.maxBytes = defaultMaxBytes
	}
	if c.client == nil {
		c.client = &http.Client{Timeout: 60 * time.Second}
	}
	c.total = c.scanSize()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, fmt.Errorf("videocache: listen: %w", err)
	}
	c.listener = listener
	c.baseURL = "http://" + listener.Addr().String()
	mux := http.NewServeMux()
	mux.HandleFunc("/media", c.handle)
	c.server = &http.Server{Handler: mux}
	go func() { _ = c.server.Serve(listener) }()
	return c, nil
}

// ProxyURL returns the local proxy URL for an http(s) address, or the address
// unchanged when it is not a remote URL the proxy can fetch.
func (c *Cache) ProxyURL(raw string) string {
	if c == nil || c.baseURL == "" {
		return raw
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return raw
	}
	return c.baseURL + "/media?u=" + url.QueryEscape(raw)
}

// Close stops the proxy and abandons the prefetches still running.
func (c *Cache) Close() error {
	if c == nil || c.server == nil {
		return nil
	}
	c.cancel()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.server.Shutdown(ctx)
}

// Prefetch warms what a viewer is likely to open next, in the background and
// without an answer to wait for. Only an HLS playlist is warmed: it names the
// bytes an episode opens on, while a single media address does not, so fetching
// one would download a whole film the viewer may never watch.
func (c *Cache) Prefetch(raw string) {
	if c == nil || c.ctx == nil || !isPlaylistAddress(raw) {
		return
	}
	if parsed, err := url.Parse(raw); err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return
	}
	go c.prefetchPlaylist(raw, prefetchHops)
}

// isPlaylistAddress is a cheap guess from the address alone, used only to decide
// whether warming is worth a request. The playlist tag in the body is the real
// test; a wrong guess here costs one fetch and warms nothing.
func isPlaylistAddress(raw string) bool {
	return strings.Contains(strings.ToLower(raw), "m3u8")
}

// prefetchPlaylist fetches raw when it is not held yet and, while hops remain,
// warms the leading segments the playlist names.
func (c *Cache) prefetchPlaylist(raw string, hops int) {
	if c.ctx.Err() != nil {
		return
	}
	body, ok := c.prefetchObject(raw)
	if !ok || !bytes.HasPrefix(body, playlistTag) {
		return
	}
	if hops <= 0 {
		return
	}
	base, err := url.Parse(raw)
	if err != nil {
		return
	}
	refs := playlistRefs(string(body))
	for _, ref := range refs[:min(prefetchSegments, len(refs))] {
		// A stored playlist has been rewritten to point at this proxy, so its
		// references are proxy addresses; the transfer has to be opened
		// upstream, or the prefetch would fetch itself and store a second copy.
		target, err := url.Parse(c.originOf(ref))
		if err != nil {
			continue
		}
		if !target.IsAbs() {
			target = base.ResolveReference(target)
		}
		if target.Scheme != "http" && target.Scheme != "https" {
			continue
		}
		if c.ctx.Err() != nil {
			return
		}
		if body, ok := c.prefetchObject(target.String()); ok && bytes.HasPrefix(body, playlistTag) {
			c.prefetchPlaylist(target.String(), hops-1)
		}
	}
}

// prefetchObject stores raw when the cache does not hold it yet, and returns the
// bytes it holds either way so a playlist can be read without a second fetch.
func (c *Cache) prefetchObject(raw string) ([]byte, bool) {
	key := keyOf(raw)
	path, part := c.pathOf(key), c.partOf(key)
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return readHead(path), true
	}
	c.mu.Lock()
	if _, running := c.inflight[key]; running {
		// A playback is already fetching this object; its own transfer is the
		// warm-up, and a second one would only duplicate it.
		c.mu.Unlock()
		return nil, false
	}
	c.mu.Unlock()
	if !c.prefetchRoom() {
		return nil, false
	}

	c.mu.Lock()
	if _, running := c.inflight[key]; running {
		c.mu.Unlock()
		return nil, false
	}
	c.inflight[key] = &call{done: make(chan struct{})}
	// The key joins the warm set before the first byte lands, not after: the
	// transfer itself calls addSize, and that can evict. An object half written
	// ahead of need that is not yet marked as one would be dropped as though a
	// playback had read it, taking the room the warm-up was meant to fill.
	c.unwatched[key] = struct{}{}
	c.mu.Unlock()

	parsed, _ := url.Parse(raw)
	// A warm-up is fetched for an episode nobody is watching yet, so its
	// transfer is not published: the speed a wait shows belongs to the
	// playback that wait is on.
	err := c.fetchRange(c.ctx, raw, parsed, path, part, 0, -1, false)

	c.mu.Lock()
	delete(c.inflight, key)
	if err != nil {
		// Nothing was stored, so nothing is warm.
		delete(c.unwatched, key)
	}
	c.mu.Unlock()
	if err != nil {
		return nil, false
	}
	if info, err := os.Stat(path); err == nil && info.Size() > 0 {
		return readHead(path), true
	}
	return readHead(part), true
}

// originOf maps a reference back to its upstream address, undoing the proxy
// rewrite a stored playlist carries.
func (c *Cache) originOf(ref string) string {
	prefix := c.baseURL + "/media?u="
	if !strings.HasPrefix(ref, prefix) {
		return ref
	}
	unescaped, err := url.QueryUnescape(strings.TrimPrefix(ref, prefix))
	if err != nil {
		return ref
	}
	return unescaped
}

// readHead returns as much of a stored object as reading a playlist needs.
func readHead(path string) []byte {
	file, err := os.Open(path)
	if err != nil {
		return nil
	}
	defer file.Close()
	body, err := io.ReadAll(io.LimitReader(file, playlistLimit))
	if err != nil {
		return nil
	}
	return body
}

// playlistRefs lists the media a playlist names, in order: a master names its
// variants, a variant names its segments, and the first entries of either are
// what an episode opens on. Tags are skipped; their URI attributes are not
// media a prefetch should fetch.
func playlistRefs(playlist string) []string {
	var refs []string
	for _, line := range strings.Split(playlist, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		refs = append(refs, trimmed)
	}
	return refs
}

func (c *Cache) handle(w http.ResponseWriter, r *http.Request) {
	raw := r.URL.Query().Get("u")
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		http.Error(w, "videocache: bad media url", http.StatusBadRequest)
		return
	}
	key := keyOf(raw)
	path := c.pathOf(key)
	part := c.partOf(key)

	if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
		c.touch(path)
		c.serveFile(w, r, path, raw)
		return
	}

	c.mu.Lock()
	if existing, ok := c.inflight[key]; ok {
		c.mu.Unlock()
		<-existing.done
		if existing.err != nil {
			c.fail(w, existing.err)
			return
		}
		if _, statErr := os.Stat(path); statErr == nil {
			c.touch(path)
			c.serveFile(w, r, path, raw)
			return
		}
		c.serveFromPart(w, r, part, raw)
		return
	}
	// The player decides how much it needs; fetching more than that wastes
	// the network without shortening its wait.
	start, end, ranged, hadRange := requestedRange(r.Header.Get("Range"))
	if hadRange && !ranged {
		c.passthrough(w, r, raw)
		return
	}
	job := &call{done: make(chan struct{})}
	c.inflight[key] = job
	c.mu.Unlock()

	job.err = c.fetchRange(r.Context(), raw, parsed, path, part, start, end, true)

	// Let the next request in before this one is answered: a player may want
	// a second range from the object this request is writing.
	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	close(job.done)

	if job.err != nil {
		if errors.Is(job.err, errNotStored) {
			c.passthrough(w, r, raw)
			return
		}
		if errors.Is(job.err, errTooLarge) || errors.Is(job.err, errProbe) {
			c.passthrough(w, r, raw)
			return
		}
		c.fail(w, job.err)
		return
	}
	if !c.serveFromPart(w, r, part, raw) {
		if info, statErr := os.Stat(path); statErr == nil && info.Size() > 0 {
			c.touch(path)
			c.serveFile(w, r, path, raw)
			return
		}
		// The transfer moved on to bytes this request did not ask for; the
		// player gets its own request upstream instead of an empty answer.
		c.passthrough(w, r, raw)
	}
}

// serveFromPart answers from what the transfer has stored so far, when it can;
// a playback that wants bytes the transfer does not hold yet is passed through.
func (c *Cache) serveFromPart(w http.ResponseWriter, r *http.Request, part, raw string) bool {
	start, end, ranged, present := requestedRange(r.Header.Get("Range"))
	if !ranged {
		return false
	}
	have := partialSize(part)
	if !present {
		start, end = 0, have-1
	}
	if start >= have {
		return false
	}
	if end < 0 || end >= have {
		end = have - 1
	}
	source, err := os.Open(part)
	if err != nil {
		return false
	}
	defer source.Close()
	w.Header().Set("Content-Type", contentTypeOf(part, raw))
	w.Header().Set("Accept-Ranges", "bytes")
	w.Header().Set("Content-Range", fmt.Sprintf("bytes %d-%d/%d", start, end, have))
	w.Header().Set("Content-Length", strconv.FormatInt(end-start+1, 10))
	w.WriteHeader(http.StatusPartialContent)
	_, _ = io.Copy(w, io.NewSectionReader(source, start, end-start+1))
	// A part is read the same way a complete object is, so it ages the same
	// way: the prefix a playback keeps asking for must not look stale. It also
	// stops being a guess: a playback asked for these bytes.
	c.touch(part)
	c.watched(keyOf(raw))
	return true
}

func (c *Cache) fail(w http.ResponseWriter, err error) {
	http.Error(w, "videocache: "+err.Error(), http.StatusBadGateway)
}

// fetchRange asks upstream for the bytes a playback needs. A complete object is
// cached in file, with a playlist rewritten. A partial one is appended to the
// transfer's part file, starting from its own beginning, so every range a player
// asks for later comes out of the same growing prefix.
func (c *Cache) fetchRange(ctx context.Context, raw string, parsed *url.URL, file, part string, start, end int64, watch bool) error {
	total, err := c.remoteSize(ctx, raw)
	if err != nil {
		return err
	}
	if total > 0 && total > c.maxBytes {
		return errTooLarge
	}
	partial := start > 0 || (end >= 0 && total > 0 && end+1 < total)
	if !partial {
		return c.fetchWhole(ctx, raw, parsed, file, c.begin(0, watch))
	}
	// A later range is cut from the prefix the transfer stores; when the
	// prefix does not reach it yet, the request is answered from upstream.
	if end >= 0 && partialSize(part) < end+1 {
		return errNotStored
	}
	stored := partialSize(part)
	return c.fetchPart(ctx, raw, parsed, part, c.begin(stored, watch))
}

// fetchWhole stores the entire object in file.
func (c *Cache) fetchWhole(ctx context.Context, raw string, parsed *url.URL, file string, moved *mover) error {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Accept", "*/*")
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusOK && response.StatusCode != http.StatusPartialContent {
		response.Body.Close()
		return fmt.Errorf("upstream %s: %d", parsed.Host, response.StatusCode)
	}
	return c.store(response, parsed, raw, file, 0, moved)
}

// fetchPart continues the transfer stored in part. The first request stores the
// object from its start, because a player that is handed a prefix asks for the
// bytes after it next, not for the offset it asked first.
func (c *Cache) fetchPart(ctx context.Context, raw string, parsed *url.URL, part string, moved *mover) error {
	have := partialSize(part)
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return err
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Accept", "*/*")
	if have > 0 {
		request.Header.Set("Range", fmt.Sprintf("bytes=%d-", have))
	}
	response, err := c.client.Do(request)
	if err != nil {
		return err
	}
	if response.StatusCode != http.StatusPartialContent && response.StatusCode != http.StatusOK {
		response.Body.Close()
		return fmt.Errorf("upstream %s: %d", parsed.Host, response.StatusCode)
	}
	return c.store(response, parsed, raw, part, have, moved)
}

// store copies response into file at offset, adding what it wrote to the cache
// budget and reporting the transfer while it runs, so a wait on these bytes has
// a progress and a speed to show.
func (c *Cache) store(response *http.Response, parsed *url.URL, raw, file string, offset int64, moved *mover) error {
	defer response.Body.Close()
	flags := os.O_CREATE | os.O_WRONLY
	if offset == 0 {
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	// The transfer ends with the copy, whether it finished or broke off: a wait
	// that follows must not read back the speed of this one.
	defer moved.settle()
	target, err := os.OpenFile(file, flags, 0o644)
	if err != nil {
		return err
	}
	limit := c.maxBytes + 1
	body := io.TeeReader(io.LimitReader(response.Body, limit), moved)
	written, copyErr := io.Copy(target, body)
	closeErr := target.Close()
	if copyErr != nil {
		return copyErr
	}
	if closeErr != nil {
		return closeErr
	}
	if offset == 0 && isPlaylist(raw, response.Header.Get("Content-Type"), file) {
		body, readErr := os.ReadFile(file)
		if readErr != nil {
			return readErr
		}
		rewritten := c.rewritePlaylist(body, parsed)
		if writeErr := os.WriteFile(file, rewritten, 0o644); writeErr != nil {
			return writeErr
		}
		// The file holds the rewritten bytes, not the bytes that arrived.
		written = int64(len(rewritten))
	}
	c.addSize(file, written)
	return nil
}

// remoteSize asks upstream how large an object is. A server that will not say
// gives zero, and the cache then decides from what it is handed.
func (c *Cache) remoteSize(ctx context.Context, raw string) (int64, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, raw, nil)
	if err != nil {
		return 0, err
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Range", "bytes=0-0")
	response, err := c.client.Do(request)
	if err != nil {
		return 0, err
	}
	response.Body.Close()
	if response.StatusCode == http.StatusPartialContent {
		return totalFromRange(response.Header.Get("Content-Range")), nil
	}
	if response.StatusCode == http.StatusOK {
		return response.ContentLength, nil
	}
	return 0, fmt.Errorf("upstream %s: %d", hostOf(raw), response.StatusCode)
}

// requestedRange reads the byte range a playback asked for. end is -1 for an
// open range, and ranged reports whether the header could be understood at all.
func requestedRange(header string) (start, end int64, ranged, present bool) {
	header = strings.TrimSpace(header)
	if header == "" {
		return 0, -1, true, false
	}
	spec, found := strings.CutPrefix(header, "bytes=")
	if !found || strings.Contains(spec, ",") {
		return 0, -1, false, true
	}
	from, to, found := strings.Cut(spec, "-")
	if !found {
		return 0, -1, false, true
	}
	if from == "" {
		// A suffix range asks for the last bytes; it is passed through.
		return 0, -1, false, true
	}
	start, err := strconv.ParseInt(strings.TrimSpace(from), 10, 64)
	if err != nil || start < 0 {
		return 0, -1, false, true
	}
	if strings.TrimSpace(to) == "" {
		return start, -1, true, true
	}
	end, err = strconv.ParseInt(strings.TrimSpace(to), 10, 64)
	if err != nil || end < start {
		return 0, -1, false, true
	}
	return start, end, true, true
}

// totalFromRange reads the object size out of Content-Range.
func totalFromRange(header string) int64 {
	value, found := strings.CutPrefix(strings.TrimSpace(header), "bytes ")
	if !found {
		return 0
	}
	_, total, found := strings.Cut(value, "/")
	if !found {
		return 0
	}
	size, err := strconv.ParseInt(strings.TrimSpace(total), 10, 64)
	if err != nil {
		return 0
	}
	return size
}

// partialSize reports how many bytes a path holds, or zero.
func partialSize(path string) int64 {
	info, err := os.Stat(path)
	if err != nil {
		return 0
	}
	return info.Size()
}

// fileSize is partialSize with the error kept, for span validation.
func fileSize(path string) (int64, error) {
	info, err := os.Stat(path)
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// hostOf names an upstream host without parsing a URL again.
func hostOf(raw string) string {
	parsed, err := url.Parse(raw)
	if err != nil {
		return raw
	}
	return parsed.Host
}

// headSize asks upstream for the object size, returning -1 when it cannot tell.
func (c *Cache) headSize(ctx context.Context, raw string) int64 {
	request, err := http.NewRequestWithContext(ctx, http.MethodHead, raw, nil)
	if err != nil {
		return -1
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	response, err := c.client.Do(request)
	if err != nil {
		return -1
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return -1
	}
	return response.ContentLength
}

// passthrough streams the original request upstream without caching, preserving
// Range so big files still seek.
func (c *Cache) passthrough(w http.ResponseWriter, r *http.Request, raw string) {
	request, err := http.NewRequestWithContext(r.Context(), http.MethodGet, raw, nil)
	if err != nil {
		c.fail(w, err)
		return
	}
	request.Header.Set("User-Agent", defaultUserAgent)
	request.Header.Set("Accept", "*/*")
	for _, name := range []string{"Range", "If-Range"} {
		if value := r.Header.Get(name); value != "" {
			request.Header.Set(name, value)
		}
	}
	response, err := c.client.Do(request)
	if err != nil {
		c.fail(w, err)
		return
	}
	defer response.Body.Close()
	for name, values := range response.Header {
		for _, value := range values {
			w.Header().Add(name, value)
		}
	}
	w.WriteHeader(response.StatusCode)
	_, _ = io.Copy(w, response.Body)
}

// serveFile serves a cached object, honouring Range for seeking.
func (c *Cache) serveFile(w http.ResponseWriter, r *http.Request, path, raw string) {
	file, err := os.Open(path)
	if err != nil {
		c.fail(w, err)
		return
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		c.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", contentTypeOf(path, raw))
	c.watched(keyOf(raw))
	http.ServeContent(w, r, "", info.ModTime(), file)
}

var uriAttr = regexp.MustCompile(`URI="([^"]*)"`)

// rewritePlaylist points every URI in an HLS playlist at the proxy, resolving
// relative references against the playlist's own address.
func (c *Cache) rewritePlaylist(body []byte, base *url.URL) []byte {
	lines := strings.Split(string(body), "\n")
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#EXTM3U") {
			continue
		}
		if strings.HasPrefix(trimmed, "#") {
			lines[i] = uriAttr.ReplaceAllStringFunc(line, func(match string) string {
				parts := uriAttr.FindStringSubmatch(match)
				if len(parts) < 2 {
					return match
				}
				return `URI="` + c.proxyRef(base, parts[1]) + `"`
			})
			continue
		}
		lines[i] = c.proxyRef(base, trimmed)
	}
	return []byte(strings.Join(lines, "\n"))
}

func (c *Cache) proxyRef(base *url.URL, ref string) string {
	parsed, err := url.Parse(ref)
	if err != nil {
		return ref
	}
	absolute := parsed
	if !parsed.IsAbs() {
		absolute = base.ResolveReference(parsed)
	}
	return c.ProxyURL(absolute.String())
}

// isPlaylist reports whether the downloaded bytes are an HLS playlist.
func isPlaylist(raw, contentType, path string) bool {
	lower := strings.ToLower(contentType)
	if strings.Contains(lower, "mpegurl") {
		return true
	}
	if strings.Contains(strings.ToLower(raw), "m3u8") {
		return true
	}
	file, err := os.Open(path)
	if err != nil {
		return false
	}
	defer file.Close()
	head := make([]byte, 7)
	n, _ := io.ReadFull(file, head)
	return bytes.HasPrefix(head[:n], []byte("#EXTM3U"))
}

// contentTypeOf names a cached object from its bytes.
func contentTypeOf(path, raw string) string {
	file, err := os.Open(path)
	if err != nil {
		return "application/octet-stream"
	}
	defer file.Close()
	head := make([]byte, 512)
	n, _ := file.Read(head)
	head = head[:n]
	if bytes.HasPrefix(head, []byte("#EXTM3U")) {
		return "application/vnd.apple.mpegurl"
	}
	return http.DetectContentType(head)
}

func keyOf(raw string) string {
	sum := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(sum[:])
}

func (c *Cache) pathOf(key string) string {
	return filepath.Join(c.dir, key+".cache")
}

// partOf is where an object still being transferred grows. Its name ends in
// .part so the cache never mistakes it for a complete object on startup, but it
// is a cache file all the same: a range-read object never completes, so its
// bytes have to be counted and evicted like any other.
func (c *Cache) partOf(key string) string {
	return filepath.Join(c.dir, key+".cache.part")
}

// touch marks an object as recently used for the LRU order.
func (c *Cache) touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// addSize records bytes written to path and evicts the least recently used
// objects when the cache grows past its byte budget.
func (c *Cache) addSize(path string, size int64) {
	c.mu.Lock()
	c.total += size
	over := c.total > c.maxBytes
	c.mu.Unlock()
	if over {
		c.evict(path)
	}
}

// prefetchRoom reports whether a transfer written ahead of need still fits. The
// budget keeps a quarter of itself for warming, so a guess about what comes next
// cannot push out what is being watched; a viewer who scrubs through more than
// that simply stops being warmed.
func (c *Cache) prefetchRoom() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	if len(c.unwatched) == 0 {
		// The playlist is the first object warmed and the one that names the
		// rest; refusing it would leave nothing to warm at all.
		return true
	}
	return c.warmBytes() < c.maxBytes/prefetchShare
}

// warmBytes sums the stored bytes nothing has read yet. The caller holds mu.
func (c *Cache) warmBytes() int64 {
	var total int64
	for key := range c.unwatched {
		total += partialSize(c.pathOf(key)) + partialSize(c.partOf(key))
	}
	return total
}

// watched drops key from the warm set: a playback read it, so its bytes are
// wanted data now rather than a guess, and eviction may drop them last.
func (c *Cache) watched(key string) {
	c.mu.Lock()
	delete(c.unwatched, key)
	c.mu.Unlock()
}

// isCacheFile reports whether name is an object the cache owns bytes in. A part
// file is a transfer still growing, and it counts: an object a player reads by
// range never becomes complete, so leaving parts out would let them fill the
// disk while eviction found nothing to remove.
func isCacheFile(name string) bool {
	return strings.HasSuffix(name, ".cache") || strings.HasSuffix(name, ".cache.part")
}

// evict removes the oldest objects until the cache is back within budget,
// never removing the object just stored.
//
// Bytes written ahead of need go first, oldest first, because nothing has asked
// for them yet; only then does the cache fall back to the least recently read
// object. Without that order a warm-up could take the room a playback needs and
// make scrubbing back into what was just watched download it again.
func (c *Cache) evict(keep string) {
	type entry struct {
		path string
		size int64
		mod  time.Time
	}
	descriptors, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	entries := make([]entry, 0, len(descriptors))
	var total int64
	for _, descriptor := range descriptors {
		if descriptor.IsDir() || !isCacheFile(descriptor.Name()) {
			continue
		}
		full := filepath.Join(c.dir, descriptor.Name())
		info, infoErr := descriptor.Info()
		if infoErr != nil {
			continue
		}
		entries = append(entries, entry{path: full, size: info.Size(), mod: info.ModTime()})
		total += info.Size()
	}
	sort.Slice(entries, func(i, j int) bool {
		warmI, warmJ := c.warm(entries[i].path), c.warm(entries[j].path)
		if warmI != warmJ {
			return warmI
		}
		return entries[i].mod.Before(entries[j].mod)
	})
	for _, item := range entries {
		if total <= c.maxBytes {
			break
		}
		if item.path == keep || c.transferring(item.path) {
			continue
		}
		if removeErr := os.Remove(item.path); removeErr == nil {
			total -= item.size
			c.forget(item.path)
		}
	}
	c.mu.Lock()
	c.total = total
	c.mu.Unlock()
}

// warm reports whether path holds bytes stored ahead of need.
func (c *Cache) warm(path string) bool {
	key := keyOfPath(path)
	c.mu.Lock()
	defer c.mu.Unlock()
	_, warm := c.unwatched[key]
	return warm
}

// forget drops an evicted file from the warm set.
func (c *Cache) forget(path string) {
	key := keyOfPath(path)
	c.mu.Lock()
	delete(c.unwatched, key)
	c.mu.Unlock()
}

// keyOfPath recovers the cache key a file name carries.
func keyOfPath(path string) string {
	name := filepath.Base(path)
	if trimmed, found := strings.CutSuffix(name, ".cache.part"); found {
		return trimmed
	}
	return strings.TrimSuffix(name, ".cache")
}

// transferring reports whether path is the part file of a transfer still in
// flight. Removing one would leave the transfer writing to an unlinked file and
// the playback asking for bytes it can no longer read.
func (c *Cache) transferring(path string) bool {
	name, found := strings.CutSuffix(filepath.Base(path), ".cache.part")
	if !found {
		return false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	_, running := c.inflight[name]
	return running
}

// scanSize sums the cache directory at startup.
func (c *Cache) scanSize() int64 {
	descriptors, err := os.ReadDir(c.dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, descriptor := range descriptors {
		if descriptor.IsDir() || !isCacheFile(descriptor.Name()) {
			continue
		}
		if info, infoErr := descriptor.Info(); infoErr == nil {
			total += info.Size()
		}
	}
	return total
}
