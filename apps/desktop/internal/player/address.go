package player

import (
	"context"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
)

// mediaProbeTimeout bounds the head check a line gets before its neighbours
// are tried instead.
const mediaProbeTimeout = 6 * time.Second

// probeClient answers the head checks. The default transport keeps a
// connection apiece to a host, so probing a line costs no new handshake.
var probeClient = &http.Client{Transport: &http.Transport{
	Proxy:               http.ProxyFromEnvironment,
	MaxIdleConnsPerHost: 4,
	IdleConnTimeout:     30 * time.Second,
}}

// playableURL reports whether a resolved address is a media stream the player
// can open: an HLS playlist, a progressive file, or a plainly ranged endpoint.
// A poster image, a blank address and a web page all fail here.
func playableURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil || (parsed.Scheme != "http" && parsed.Scheme != "https") || parsed.Host == "" {
		return false
	}
	full := strings.ToLower(parsed.Path)
	switch {
	case strings.HasSuffix(full, ".m3u8"), strings.Contains(strings.ToLower(parsed.RawQuery), "m3u8"):
		return true
	case strings.HasSuffix(full, ".mp4"), strings.HasSuffix(full, ".m4v"), strings.HasSuffix(full, ".ts"):
		return true
	}
	// Neither an image extension nor a picture host is trusted here: a
	// signed CDN names the picture pipeline (byteimg) and still serves the
	// video, so those cases are left to mediaOffers.
	return true
}

// mediaOffers reports whether an address serves media rather than a picture or
// a page. Some lines answer with a .jpg URL that the CDN serves as an ISO base
// media file, and its Content-Type says image/jpeg all the same, so the
// leading bytes decide and the header only speaks when they say nothing. A
// still that is really a still fails on those bytes.
func mediaOffers(ctx context.Context, raw string) bool {
	if !playableURL(raw) {
		return false
	}
	if snapshot, ok := recognizeHead(fetchHead(ctx, raw)); ok {
		return snapshot
	}
	// A server that ignores the ranged request, or one whose first bytes
	// say nothing (a web page that names itself late), gets one more look
	// with the whole file offered.
	return fetchAndJudge(ctx, raw)
}

// firstPlayable walks the lines the source reported and resolves up to
// lineBudget of them, taking the first that answers with media. A line whose
// address is media on sight needs no check, so a long line list still settles
// at once; a line in passed, one that opened without a picture or failed on
// the engine, is not offered again. The original playback stands when none does.
func firstPlayable(ctx context.Context, src source.Source, episode source.Episode, current source.Playback, passed []string) (source.Playback, error) {
	tried := 0
	for _, line := range current.Lines {
		if line.ID == "" || line.ID == current.Line.ID || passesOver(passed, line.ID) {
			continue
		}
		tried++
		if tried > lineBudget {
			break
		}
		next, err := src.Resolve(ctx, episode, line.ID)
		if err != nil || !mediaOffers(ctx, next.URL) {
			continue
		}
		return next, nil
	}
	if len(passed) > 0 {
		// Nothing else answers: the episode has no line left, and the stage is
		// what says so rather than opening the same stream again.
		return source.Playback{Lines: current.Lines}, nil
	}
	return current, nil
}

// passesOver reports whether id is one of the lines an episode already gave up
// on, which neither the resolve nor the walk over its lines offers again.
func passesOver(passed []string, id string) bool {
	for _, line := range passed {
		if line == id {
			return true
		}
	}
	return false
}

// lineBudget bounds how many neighbouring lines are resolved and checked
// before the original playback stands.
const lineBudget = 8

// headBytes is how many leading bytes tell media from a picture.
const headBytes = 64

// fetchHead reads the first bytes of raw with a ranged request.
func fetchHead(ctx context.Context, raw string) head {
	probeCtx, cancel := context.WithTimeout(ctx, mediaProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, raw, nil)
	if err != nil {
		return head{err: err}
	}
	request.Header.Set("Range", "bytes=0-63")
	return readHead(probeCtx, request)
}

// fetchAndJudge asks for the whole address and judges by its opening bytes.
func fetchAndJudge(ctx context.Context, raw string) bool {
	probeCtx, cancel := context.WithTimeout(ctx, mediaProbeTimeout)
	defer cancel()
	request, err := http.NewRequestWithContext(probeCtx, http.MethodGet, raw, nil)
	if err != nil {
		return false
	}
	got := readHead(probeCtx, request)
	// Reading 64 bytes is enough to tell; the rest is not downloaded.
	if got.err != nil && len(got.body) == 0 {
		return false
	}
	answer, ok := recognizeHead(got)
	return ok && answer
}

// head is what a request for the first bytes of an address showed.
type head struct {
	status int
	ctype  string
	body   []byte
	err    error
}

func readHead(ctx context.Context, request *http.Request) head {
	response, err := probeClient.Do(request.WithContext(ctx))
	if err != nil {
		return head{err: err}
	}
	defer response.Body.Close()
	// Each head reads into a buffer of its own. A pool would hand a second line
	// check the bytes the first one is still reading: a retry starts the next
	// resolve while the last one is walking its lines, so two reads overlap.
	body := make([]byte, headBytes)
	n, err := io.ReadAtLeast(response.Body, body, 1)
	if err != nil && err != io.EOF && err != io.ErrUnexpectedEOF {
		return head{err: err}
	}
	return head{
		status: response.StatusCode,
		ctype:  strings.ToLower(response.Header.Get("Content-Type")),
		body:   body[:n],
	}
}

// recognizeHead decides from the opening bytes first, then from the header.
// The second result is false when neither speaks, which asks the caller for a
// plain fetch.
func recognizeHead(got head) (bool, bool) {
	if got.status >= http.StatusBadRequest || got.err != nil {
		return false, true
	}
	prefix := string(got.body)
	switch {
	case strings.Contains(prefix, "#EXTM3U") || strings.Contains(got.ctype, "mpegurl"):
		return true, true
	case isISOMedia(got.body):
		return true, true
	case hasPrefix(got.body, []byte{0x89, 'P', 'N', 'G'}),
		hasPrefix(got.body, []byte{0xFF, 0xD8}),
		hasPrefix(got.body, []byte("GIF8")),
		hasPrefix(got.body, []byte("RIFF")) && strings.Contains(prefix, "WEBP"),
		hasPrefix(got.body, []byte("<svg")),
		hasPrefix(got.body, []byte("<?xml")):
		return false, true
	case looksLikeMarkup(got.body):
		return false, true
	case strings.HasPrefix(got.ctype, "image/"):
		return false, true
	case strings.HasPrefix(got.ctype, "video/"):
		return true, true
	case strings.HasPrefix(got.ctype, "text/html"):
		return false, true
	}
	return false, false
}

// isISOMedia reports the box layout of an mp4 or a fragmented mp4, whose
// first box is ftyp. Some CDNs prefix a small free box, so the opening boxes
// are walked rather than only the first four bytes checked.
func isISOMedia(body []byte) bool {
	for offset := 0; offset+8 <= len(body); {
		size := int(uint32(body[offset])<<24 | uint32(body[offset+1])<<16 | uint32(body[offset+2])<<8 | uint32(body[offset+3]))
		kind := string(body[offset+4 : offset+8])
		if kind == "ftyp" {
			return true
		}
		if kind != "free" && kind != "skip" && kind != "wide" {
			return false
		}
		if size < 8 {
			return false
		}
		offset += size
	}
	return false
}

// looksLikeMarkup reports the opening of a web page or an error document.
func looksLikeMarkup(body []byte) bool {
	trimmed := strings.TrimLeft(string(body), " \t\r\n")
	lower := strings.ToLower(trimmed)
	return strings.HasPrefix(lower, "<!doctype") || strings.HasPrefix(lower, "<html") ||
		strings.HasPrefix(lower, "{")
}

func hasPrefix(body, prefix []byte) bool {
	if len(body) < len(prefix) {
		return false
	}
	for i := range prefix {
		if body[i] != prefix[i] {
			return false
		}
	}
	return true
}
