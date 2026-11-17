// Package videocache serves remote video through a local HTTP proxy backed by a
// bounded disk cache. A repeated watch is answered from disk instead of the
// network, and an HLS playlist is rewritten so its variant, segment and key
// URIs also travel through the proxy and are cached the same way.
//
// The player opens the proxy URL; the proxy is transparent at the HTTP level,
// so mp4 and HLS both work without parsing media. An object is fetched whole
// and stored before it is served; an object larger than MaxObject is streamed
// straight through without being cached, keeping seeks on big files cheap.
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
	"strings"
	"sync"
	"time"
)

const (
	defaultMaxBytes  int64 = 2 << 30   // 2 GiB on disk
	defaultMaxObject int64 = 512 << 20 // largest single object fetched whole
	defaultUserAgent       = "Luvia/1.0 (+videocache)"
)

// errTooLarge reports an object bigger than MaxObject; the caller streams it
// through instead of caching it.
var errTooLarge = errors.New("videocache: object exceeds the cache object limit")

// Options tune a Cache. The zero value of each field uses a default.
type Options struct {
	MaxBytes  int64
	MaxObject int64
	Client    *http.Client
}

// Cache is a loopback HTTP proxy over a bounded on-disk cache. Point a player at
// ProxyURL and call Close when the app quits.
type Cache struct {
	dir       string
	maxBytes  int64
	maxObject int64
	client    *http.Client

	listener net.Listener
	server   *http.Server
	baseURL  string

	mu       sync.Mutex
	total    int64
	inflight map[string]*call
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
		dir: dir, maxBytes: opts.MaxBytes, maxObject: opts.MaxObject,
		client: opts.Client, inflight: map[string]*call{},
	}
	if c.maxBytes <= 0 {
		c.maxBytes = defaultMaxBytes
	}
	if c.maxObject <= 0 {
		c.maxObject = defaultMaxObject
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

// Close stops the proxy.
func (c *Cache) Close() error {
	if c == nil || c.server == nil {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return c.server.Shutdown(ctx)
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
		http.Error(w, http.StatusText(http.StatusBadGateway), http.StatusBadGateway)
		return
	}
	job := &call{done: make(chan struct{})}
	c.inflight[key] = job
	c.mu.Unlock()

	job.err = c.download(r.Context(), raw, parsed, path)

	c.mu.Lock()
	delete(c.inflight, key)
	c.mu.Unlock()
	close(job.done)

	if job.err != nil {
		if errors.Is(job.err, errTooLarge) {
			c.passthrough(w, r, raw)
			return
		}
		c.fail(w, job.err)
		return
	}
	c.touch(path)
	c.serveFile(w, r, path, raw)
}

func (c *Cache) fail(w http.ResponseWriter, err error) {
	http.Error(w, "videocache: "+err.Error(), http.StatusBadGateway)
}

// download fetches raw whole into path, rewriting a playlist, then records its
// size. It returns errTooLarge before reading the body when the object is too
// big to cache.
func (c *Cache) download(ctx context.Context, raw string, parsed *url.URL, path string) error {
	if size := c.headSize(ctx, raw); size > c.maxObject {
		return errTooLarge
	}
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
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return fmt.Errorf("upstream %s: %d", parsed.Host, response.StatusCode)
	}

	temp := path + ".tmp"
	file, err := os.Create(temp)
	if err != nil {
		return err
	}
	written, copyErr := io.Copy(file, io.LimitReader(response.Body, c.maxObject+1))
	closeErr := file.Close()
	if copyErr != nil {
		os.Remove(temp)
		return copyErr
	}
	if closeErr != nil {
		os.Remove(temp)
		return closeErr
	}
	if written > c.maxObject {
		os.Remove(temp)
		return errTooLarge
	}

	if isPlaylist(raw, response.Header.Get("Content-Type"), temp) {
		body, readErr := os.ReadFile(temp)
		if readErr != nil {
			os.Remove(temp)
			return readErr
		}
		rewritten := c.rewritePlaylist(body, parsed)
		if writeErr := os.WriteFile(temp, rewritten, 0o644); writeErr != nil {
			os.Remove(temp)
			return writeErr
		}
		written = int64(len(rewritten))
	}
	if err := os.Rename(temp, path); err != nil {
		os.Remove(temp)
		return err
	}
	c.addSize(path, written)
	return nil
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

// touch marks an object as recently used for the LRU order.
func (c *Cache) touch(path string) {
	now := time.Now()
	_ = os.Chtimes(path, now, now)
}

// addSize records a stored object and evicts the least recently used objects
// when the cache grows past its byte budget.
func (c *Cache) addSize(path string, size int64) {
	c.mu.Lock()
	c.total += size
	over := c.total > c.maxBytes
	c.mu.Unlock()
	if over {
		c.evict(path)
	}
}

// evict removes the oldest objects until the cache is back within budget,
// never removing the object just stored.
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
		if descriptor.IsDir() || !strings.HasSuffix(descriptor.Name(), ".cache") {
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
	sort.Slice(entries, func(i, j int) bool { return entries[i].mod.Before(entries[j].mod) })
	for _, item := range entries {
		if total <= c.maxBytes {
			break
		}
		if item.path == keep {
			continue
		}
		if removeErr := os.Remove(item.path); removeErr == nil {
			total -= item.size
		}
	}
	c.mu.Lock()
	c.total = total
	c.mu.Unlock()
}

// scanSize sums the cache directory at startup.
func (c *Cache) scanSize() int64 {
	descriptors, err := os.ReadDir(c.dir)
	if err != nil {
		return 0
	}
	var total int64
	for _, descriptor := range descriptors {
		if descriptor.IsDir() || !strings.HasSuffix(descriptor.Name(), ".cache") {
			continue
		}
		if info, infoErr := descriptor.Info(); infoErr == nil {
			total += info.Size()
		}
	}
	return total
}
