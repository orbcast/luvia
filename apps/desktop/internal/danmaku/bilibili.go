package danmaku

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/json"
	"encoding/xml"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"

	"luvia/apps/desktop/internal/source"
)

// bilibili is the site this package reads comments from. Its comments are
// pinned to a moment of the picture and its catalogue is public, which is what
// a source rule's catalogue usually is not.
const (
	searchEndpoint = "https://api.bilibili.com/x/web-interface/wbi/search/type"
	pagesEndpoint  = "https://api.bilibili.com/x/player/pagelist"
	dmEndpoint     = "https://api.bilibili.com/x/v1/dm/list.so"

	// browserUA is what the endpoints answer to. A bare Go user agent is
	// refused, and the desktop string is what a browser sends.
	browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	referer   = "https://www.bilibili.com"

	// maxBody bounds one response so a wrong address cannot fill memory.
	maxBody = 8 << 20
	// maxCandidates is how many search hits are weighed per title.
	maxCandidates = 20
	// maxCachedPages is how many episodes' comments stay in memory.
	maxCachedPages = 24
	// requestTimeout bounds one HTTP round trip.
	requestTimeout = 15 * time.Second
)

// Bilibili finds comments by matching a title against the site's catalogue.
// It is safe for concurrent use and caches what it fetched, since a viewer who
// seeks back and forth asks for the same episode repeatedly.
type Bilibili struct {
	client *http.Client

	mu    sync.Mutex
	cache map[int64][]source.Danmaku
	order []int64
}

// NewBilibili prepares a lookup. A nil client gets one with a bounded timeout.
func NewBilibili(client *http.Client) *Bilibili {
	if client == nil {
		client = &http.Client{Timeout: requestTimeout}
	}
	return &Bilibili{client: client, cache: map[int64][]source.Danmaku{}}
}

// candidate is one search hit, before it is weighed.
type candidate struct {
	BVID     string
	Title    string
	Duration int
	Play     int
}

// page is one part of a video, as the site splits long uploads.
type page struct {
	CID      int64
	Page     int
	Part     string
	Duration int
}

// Lookup matches title against the site and returns the comments of its
// episode. It reports a Reason rather than an error when it finds nothing, so
// the player can say which nothing it found.
func (b *Bilibili) Lookup(ctx context.Context, title string, episode int) (Result, error) {
	query := normalizeTitle(title)
	if query == "" {
		return Result{Reason: ReasonNoMatch}, nil
	}
	found, err := b.search(ctx, query)
	if err != nil {
		return Result{Reason: ReasonFailed}, err
	}
	best, ok := bestCandidate(found, query, episode)
	if !ok {
		return Result{Reason: ReasonNoMatch}, nil
	}
	pages, err := b.pages(ctx, best.BVID)
	if err != nil {
		return Result{Reason: ReasonFailed}, err
	}
	target, ok := pickPage(pages, episode)
	if !ok {
		return Result{Reason: ReasonNoEpisode}, nil
	}
	items, err := b.comments(ctx, target.CID)
	if err != nil {
		return Result{Reason: ReasonFailed}, err
	}
	if len(items) == 0 {
		return Result{Reason: ReasonEmpty}, nil
	}
	return Result{Items: items, Reason: ReasonReady}, nil
}

// search asks the site for videos whose title mentions query.
func (b *Bilibili) search(ctx context.Context, query string) ([]candidate, error) {
	values := url.Values{}
	values.Set("search_type", "video")
	values.Set("keyword", query)
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				BVID     string `json:"bvid"`
				Title    string `json:"title"`
				Duration string `json:"duration"`
				Play     int    `json:"play"`
			} `json:"result"`
		} `json:"data"`
	}
	if err := b.get(ctx, searchEndpoint+"?"+values.Encode(), &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("search answered %d", payload.Code)
	}
	out := make([]candidate, 0, len(payload.Data.Result))
	for _, hit := range payload.Data.Result {
		if hit.BVID == "" {
			continue
		}
		out = append(out, candidate{
			BVID:     hit.BVID,
			Title:    stripTags(hit.Title),
			Duration: clockSeconds(hit.Duration),
			Play:     hit.Play,
		})
		if len(out) >= maxCandidates {
			break
		}
	}
	return out, nil
}

// pages lists the parts a video is split into.
func (b *Bilibili) pages(ctx context.Context, bvid string) ([]page, error) {
	values := url.Values{}
	values.Set("bvid", bvid)
	var payload struct {
		Code int `json:"code"`
		Data []struct {
			CID      int64  `json:"cid"`
			Page     int    `json:"page"`
			Part     string `json:"part"`
			Duration int    `json:"duration"`
		} `json:"data"`
	}
	if err := b.get(ctx, pagesEndpoint+"?"+values.Encode(), &payload); err != nil {
		return nil, err
	}
	if payload.Code != 0 {
		return nil, fmt.Errorf("pagelist answered %d", payload.Code)
	}
	out := make([]page, 0, len(payload.Data))
	for _, p := range payload.Data {
		out = append(out, page{CID: p.CID, Page: p.Page, Part: p.Part, Duration: p.Duration})
	}
	return out, nil
}

// comments fetches and caches one part's comments.
func (b *Bilibili) comments(ctx context.Context, cid int64) ([]source.Danmaku, error) {
	if items, ok := b.cached(cid); ok {
		return items, nil
	}
	values := url.Values{}
	values.Set("oid", strconv.FormatInt(cid, 10))
	body, err := b.body(ctx, dmEndpoint+"?"+values.Encode())
	if err != nil {
		return nil, err
	}
	items, err := ParseComments(bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	b.remember(cid, items)
	return items, nil
}

func (b *Bilibili) cached(cid int64) ([]source.Danmaku, bool) {
	b.mu.Lock()
	defer b.mu.Unlock()
	items, ok := b.cache[cid]
	return items, ok
}

func (b *Bilibili) remember(cid int64, items []source.Danmaku) {
	b.mu.Lock()
	defer b.mu.Unlock()
	if _, ok := b.cache[cid]; ok {
		return
	}
	b.cache[cid] = items
	b.order = append(b.order, cid)
	for len(b.order) > maxCachedPages {
		delete(b.cache, b.order[0])
		b.order = b.order[1:]
	}
}

// get fetches a JSON endpoint into out.
func (b *Bilibili) get(ctx context.Context, address string, out any) error {
	body, err := b.body(ctx, address)
	if err != nil {
		return err
	}
	if err := json.Unmarshal(body, out); err != nil {
		return fmt.Errorf("%s: %w", address, err)
	}
	return nil
}

// body fetches an endpoint and returns its decoded bytes. The comment endpoint
// answers raw deflate without saying so in a way net/http unwraps, so the
// stream is decoded here.
func (b *Bilibili) body(ctx context.Context, address string) ([]byte, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, address, nil)
	if err != nil {
		return nil, err
	}
	request.Header.Set("User-Agent", browserUA)
	request.Header.Set("Referer", referer)
	response, err := b.client.Do(request)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", address, err)
	}
	defer response.Body.Close()
	if response.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("%s: upstream answered %d", address, response.StatusCode)
	}
	reader, err := decompress(response)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", address, err)
	}
	body, err := io.ReadAll(io.LimitReader(reader, maxBody+1))
	if err != nil {
		return nil, fmt.Errorf("%s: %w", address, err)
	}
	if len(body) > maxBody {
		return nil, fmt.Errorf("%s: larger than %d bytes", address, maxBody)
	}
	return body, nil
}

// decompress unwraps a deflate stream. A zlib header is used when present and
// a raw deflate stream otherwise, which is what the comment endpoint sends.
func decompress(response *http.Response) (io.Reader, error) {
	if !strings.Contains(response.Header.Get("Content-Encoding"), "deflate") {
		return response.Body, nil
	}
	head := make([]byte, 2)
	n, err := io.ReadFull(response.Body, head)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) && !errors.Is(err, io.EOF) {
		return nil, err
	}
	stream := io.MultiReader(bytes.NewReader(head[:n]), response.Body)
	// A zlib stream starts with 0x78 and a checksum flag that divides by 31.
	if n == 2 && head[0]&0x0f == 8 && (uint16(head[0])<<8|uint16(head[1]))%31 == 0 {
		return zlib.NewReader(stream)
	}
	return flate.NewReader(stream), nil
}

// ParseComments reads the comment document into the player's shape. It keeps
// the modes that cross the picture or sit at its top or bottom, and drops the
// ones the player cannot draw.
func ParseComments(r io.Reader) ([]source.Danmaku, error) {
	var document struct {
		Items []struct {
			P    string `xml:"p,attr"`
			Text string `xml:",chardata"`
		} `xml:"d"`
	}
	decoder := xml.NewDecoder(r)
	decoder.Strict = false
	decoder.CharsetReader = func(_ string, input io.Reader) (io.Reader, error) { return input, nil }
	// A document with no comment in it is not a failure: an episode may have
	// none, and the endpoint answers an empty one the same way.
	if err := decoder.Decode(&document); err != nil && !errors.Is(err, io.EOF) {
		return nil, fmt.Errorf("comments: %w", err)
	}
	out := make([]source.Danmaku, 0, len(document.Items))
	for _, item := range document.Items {
		text := strings.TrimSpace(item.Text)
		if text == "" {
			continue
		}
		at, mode, color, ok := parseP(item.P)
		if !ok {
			continue
		}
		switch mode {
		case 1, 4, 5:
		default:
			continue
		}
		out = append(out, source.Danmaku{At: at, Text: text, Color: color})
	}
	sort.SliceStable(out, func(i, j int) bool { return out[i].At < out[j].At })
	return out, nil
}

// parseP reads the comma separated attributes a comment carries: the moment it
// was sent at, its mode, its size, its color, and then bookkeeping the player
// has no use for.
func parseP(p string) (time.Duration, int, uint32, bool) {
	fields := strings.Split(p, ",")
	if len(fields) < 4 {
		return 0, 0, 0, false
	}
	seconds, err := strconv.ParseFloat(strings.TrimSpace(fields[0]), 64)
	if err != nil || seconds < 0 {
		return 0, 0, 0, false
	}
	mode, err := strconv.Atoi(strings.TrimSpace(fields[1]))
	if err != nil {
		return 0, 0, 0, false
	}
	color, err := strconv.ParseUint(strings.TrimSpace(fields[3]), 10, 32)
	if err != nil {
		return 0, 0, 0, false
	}
	return time.Duration(seconds * float64(time.Second)), mode, uint32(color), true
}

// highlight matches the emphasis tag the search puts around the words that hit.
var highlight = regexp.MustCompile(`<[^>]*>`)

// stripTags removes the markup a search hit carries around its matching words.
func stripTags(s string) string {
	s = highlight.ReplaceAllString(s, "")
	return strings.TrimSpace(htmlUnescape(s))
}

// htmlUnescape resolves the entities a search hit may carry.
func htmlUnescape(s string) string {
	replacer := strings.NewReplacer(
		"&amp;", "&", "&lt;", "<", "&gt;", ">", "&quot;", `"`, "&#39;", "'", "&apos;", "'", "&nbsp;", " ",
	)
	return replacer.Replace(s)
}

// clockSeconds reads the "mm:ss" or "hh:mm:ss" duration a search hit reports.
func clockSeconds(s string) int {
	total := 0
	for _, part := range strings.Split(strings.TrimSpace(s), ":") {
		value, err := strconv.Atoi(strings.TrimSpace(part))
		if err != nil {
			return 0
		}
		total = total*60 + value
	}
	return total
}

// normalizeTitle reduces a title to the words that identify the show, so the
// catalogue's title and the site's title can be compared. Every mark a title
// wraps itself in — brackets of either width, quotes, punctuation, spacing —
// goes, and what is left is the letters and digits of the name itself. That is
// done by keeping what identifies rather than by listing what to strip: the
// wrappers differ by script and by uploader, and a list of them never ends.
func normalizeTitle(title string) string {
	words := strings.Map(func(r rune) rune {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return unicode.ToLower(r)
		}
		return -1
	}, stripTags(title))
	return words
}

// bestCandidate picks the hit that best answers the title. A catalogue names a
// show one way and an upload names it another, so a hit counts when its name
// shares enough of the title to be the same show; among those the most watched
// one wins, because a show's popular upload is the one whose comments were
// written by people watching the same cut.
func bestCandidate(hits []candidate, query string, episode int) (candidate, bool) {
	var best candidate
	bestScore := -1
	for _, hit := range hits {
		strength := titleMatch(normalizeTitle(hit.Title), query)
		if strength == 0 {
			continue
		}
		score := strength<<30 + hit.Play
		if strings.Contains(normalizeTitle(hit.Title), strconv.Itoa(episode)) {
			score += 1 << 20
		}
		if score > bestScore {
			best, bestScore = hit, score
		}
	}
	return best, bestScore >= 0
}

// titleMatch says how well a candidate's name answers the query, and zero when
// the two do not name the same show. A catalogue often carries the whole name
// while an upload carries a shortened one, so the words the two share matter
// more than either name being contained in the other.
func titleMatch(name, query string) int {
	if name == "" || query == "" {
		return 0
	}
	if name == query {
		return 3
	}
	short, long := name, query
	if len([]rune(short)) > len([]rune(long)) {
		short, long = long, short
	}
	if len([]rune(short)) >= minSharedRunes && strings.Contains(long, short) {
		return 2
	}
	shared := longestSharedRun([]rune(name), []rune(query))
	if shared >= minSharedRunes && shared*2 >= len([]rune(short)) {
		return 1
	}
	return 0
}

// minSharedRunes is how much of a title two names have to share to be taken for
// the same show. Four characters is short enough for a Chinese title and long
// enough that unrelated shows do not collide.
const minSharedRunes = 4

// longestSharedRun returns the length of the longest stretch of runes the two
// names have in common.
func longestSharedRun(a, b []rune) int {
	best := 0
	previous := make([]int, len(b)+1)
	current := make([]int, len(b)+1)
	for i := 1; i <= len(a); i++ {
		for j := 1; j <= len(b); j++ {
			if a[i-1] == b[j-1] {
				current[j] = previous[j-1] + 1
				if current[j] > best {
					best = current[j]
				}
			} else {
				current[j] = 0
			}
		}
		previous, current = current, previous
	}
	return best
}

// partNumbers pulls the episode numbers a part label names, so "1-4" covers
// four episodes and "全球死考-3" covers the third.
var partNumbers = regexp.MustCompile(`\d+`)

// pickPage chooses the part that holds episode. A part that names the episode
// wins, and a part labelled with a range covers the episodes it spans. A show
// uploaded one part per episode carries no labels at all, and then the part's
// own position is the episode.
func pickPage(pages []page, episode int) (page, bool) {
	if len(pages) == 0 || episode < 1 {
		return page{}, false
	}
	labelled := false
	for _, p := range pages {
		numbers := partNumbers.FindAllString(p.Part, -1)
		if len(numbers) == 0 {
			continue
		}
		labelled = true
		if len(numbers) == 1 {
			if value, err := strconv.Atoi(numbers[0]); err == nil && value == episode {
				return p, true
			}
			continue
		}
		low, errLow := strconv.Atoi(numbers[0])
		high, errHigh := strconv.Atoi(numbers[1])
		if errLow == nil && errHigh == nil && low <= episode && episode <= high {
			return p, true
		}
	}
	if labelled {
		return page{}, false
	}
	if episode <= len(pages) {
		return pages[episode-1], true
	}
	return page{}, false
}
