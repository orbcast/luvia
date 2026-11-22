package danmaku

import (
	"bytes"
	"compress/flate"
	"compress/zlib"
	"context"
	"encoding/json"
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
	searchEndpoint   = "https://api.bilibili.com/x/web-interface/wbi/search/type"
	pagesEndpoint    = "https://api.bilibili.com/x/player/pagelist"
	segmentsEndpoint = "https://api.bilibili.com/x/v2/dm/web/seg.so"

	// browserUA is what the endpoints answer to. A bare Go user agent is
	// refused, and the desktop string is what a browser sends.
	browserUA = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/131.0.0.0 Safari/537.36"
	referer   = "https://www.bilibili.com"

	// maxBody bounds one response so a wrong address cannot fill memory.
	maxBody = 8 << 20
	// maxCandidates is how many search hits are weighed per title.
	maxCandidates = 40
	// searchPageSize is how many hits one page of a search asks for.
	searchPageSize = 20
	// maxCachedPages is how many episodes' comments stay in memory.
	maxCachedPages = 24
	// maxSegments caps the segments one episode's comments are read from.
	maxSegments = 24
	// segmentSpan is how much of the picture one segment's comments cover; a
	// part longer than this spreads its comments over more than one segment.
	segmentSpan = 6 * time.Minute
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
	// Uploaded is when the video was posted, in Unix seconds, as the site's
	// search reports it. A freshly posted show buries its own series under
	// whatever mentions it this week, so the age of a hit decides between a
	// review of one episode and the upload that carries all of them.
	Uploaded int64
	// Parts is how many parts the site lists for the video, which says whether
	// the upload holds a series at all.
	Parts int
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
	// The show's name is searched for three ways: alone, with the episode, and
	// with the range a series upload is named by. A name alone answers with
	// whatever mentions it, which for a popular show is a wall of reaction
	// videos and analysis breakdowns, and the site's own upload of the series
	// ranks below them; a name and an episode answers with that one clip; a
	// name and a range answers with the upload that holds every episode as a
	// part. Any one of them misses what the others find, so all are collected.
	queries := []string{query}
	if episode > 0 {
		queries = append(queries, query+" 第"+strconv.Itoa(episode)+"集")
		queries = append(queries, query+"（1-"+strconv.Itoa(seriesRange)+"）")
	}
	var found []candidate
	for _, one := range queries {
		hits, err := b.search(ctx, one)
		if err != nil {
			return Result{Reason: ReasonFailed}, err
		}
		found = mergeHits(found, hits)
	}
	// A site's catalogue is a search engine's, and a title answers with
	// whatever mentions it: for a popular show that is a wall of reaction
	// videos, analysis breakdowns and wallpapers, and the show's own upload
	// sits somewhere among them. So the hits are tried in order of how likely
	// each is to be that upload, until one carries a part for this episode with
	// comments in it, rather than the first hit alone deciding the layer.
	ordered := candidates(found, query, episode)
	if len(ordered) == 0 {
		return Result{Reason: ReasonNoMatch}, nil
	}
	// The show's own upload is near the top of the hits and not necessarily the
	// first. Its part count is not in the search answer either, only the upload
	// itself lists it, so the first few are opened for their parts before any is
	// judged on its comments. The order the hits were ranked in is kept: what
	// makes an upload the show's own is its name, and a radio play of the same
	// show is split into more parts than the show is.
	width := min(len(ordered), maxProbes)
	var probes []probe
	for i := 0; i < width; i++ {
		pages, err := b.pages(ctx, ordered[i].BVID)
		if err != nil {
			continue
		}
		probes = append(probes, probe{hit: ordered[i], pages: pages})
	}
	for _, hit := range ordered[width:] {
		probes = append(probes, probe{hit: hit})
	}
	why := ReasonNoMatch
	for _, one := range probes[:min(len(probes), maxTries)] {
		if why == ReasonNoMatch {
			why = ReasonNoEpisode
		}
		if one.pages == nil {
			pages, err := b.pages(ctx, one.hit.BVID)
			if err != nil {
				return Result{Reason: ReasonFailed}, err
			}
			one.pages = pages
		}
		target, ok := pickPage(one.pages, episode)
		if !ok {
			continue
		}
		items, err := b.comments(ctx, target.CID, time.Duration(target.Duration)*time.Second)
		if err != nil {
			continue
		}
		if len(items) == 0 {
			why = ReasonEmpty
			continue
		}
		return Result{Items: items, Reason: ReasonReady}, nil
	}
	return Result{Reason: why}, nil
}

// maxTries bounds how many hits have their parts opened and their comments
// fetched before the layer is called empty or unmatched: each costs two
// requests, and the show is either near the top or not on the site.
const maxTries = 5

// seriesMark pulls the range a series upload is named by, as in "（1-100）" or
// "(1-100)": the first episode and the last.
var seriesMark = regexp.MustCompile(`[（(]\s*(\d+)\s*[-–~]\s*(\d+)\s*[）)]`)

// namesTheSeries reports whether a title is the show's name and a range of
// episodes, as the site's own catalogue names a series: "凡人修仙传（1-100）".
// Anything else wrapped round the range is somebody's own cut of the show, a
// reaction series or a radio play of it.
func namesTheSeries(title, query string) bool {
	if !seriesMark.MatchString(title) {
		return false
	}
	return normalizeTitle(seriesMark.ReplaceAllString(title, "")) == query
}

// coversEpisode reports whether a title names a range of episodes that holds
// this one, which is what the site's own catalogue of a series does.
func coversEpisode(title string, episode int) bool {
	for _, match := range seriesMark.FindAllStringSubmatch(title, -1) {
		low, errLow := strconv.Atoi(match[1])
		high, errHigh := strconv.Atoi(match[2])
		if errLow == nil && errHigh == nil && low <= episode && episode <= high {
			return true
		}
	}
	return false
}

// maxProbes bounds how many hits are opened just for their part count, which
// is what ranks a series above a clip of one episode.
const maxProbes = 5

// seriesRange is the upper bound a series upload is named by: the site's own
// catalogue lists the show as "凡人修仙传（1-100）", every episode a part.
const seriesRange = 100

// probe is one candidate and the parts the site listed for it, when it was
// opened.
type probe struct {
	hit   candidate
	pages []page
}

// maxSearchPages is how many pages of one search are read. The site answers
// twenty hits a page and orders them by relevance to the words searched for, so
// a show airing right now buries its own first episodes under the current ones:
// the upload that carries episode thirty is not on the first page of a search
// for it.
const maxSearchPages = 3

// mergeHits appends the hits one search answered that the earlier ones did not
// carry, so a video found by both searches is weighed once.
func mergeHits(into, more []candidate) []candidate {
	seen := make(map[string]bool, len(into)+len(more))
	for _, hit := range into {
		seen[hit.BVID] = true
	}
	for _, hit := range more {
		if seen[hit.BVID] {
			continue
		}
		seen[hit.BVID] = true
		into = append(into, hit)
	}
	return into
}

// search asks the site for videos whose title mentions query, and for the next
// pages of that answer until it has enough hits to weigh.
func (b *Bilibili) search(ctx context.Context, query string) ([]candidate, error) {
	var out []candidate
	for page := 1; page <= maxSearchPages; page++ {
		hits, err := b.searchPage(ctx, query, page)
		if err != nil {
			if page == 1 {
				return nil, err
			}
			break
		}
		if len(hits) == 0 {
			break
		}
		out = append(out, hits...)
		if len(out) >= maxCandidates {
			break
		}
	}
	return out, nil
}

// searchPage asks for one page of the site's answer.
func (b *Bilibili) searchPage(ctx context.Context, query string, page int) ([]candidate, error) {
	values := url.Values{}
	values.Set("search_type", "video")
	values.Set("keyword", query)
	values.Set("page", strconv.Itoa(page))
	values.Set("page_size", strconv.Itoa(searchPageSize))
	var payload struct {
		Code int `json:"code"`
		Data struct {
			Result []struct {
				BVID     string `json:"bvid"`
				Title    string `json:"title"`
				Duration string `json:"duration"`
				Play     int    `json:"play"`
				Pubdate  int64  `json:"pubdate"`
				// Videos is the part count the site puts on a search hit, the
				// same number Page carries for one of an upload's parts.
				Videos int `json:"videos"`
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
			Uploaded: hit.Pubdate,
			Parts:    hit.Videos,
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
//
// The endpoint the site used to hand out the whole list as XML now answers
// 412, so the comments are read segment by segment, in the protobuf the
// segment endpoint replies with.
func (b *Bilibili) comments(ctx context.Context, cid int64, span time.Duration) ([]source.Danmaku, error) {
	if items, ok := b.cached(cid); ok {
		return items, nil
	}
	// A part's length says how many segments to read: an upload that folds ten
	// episodes into one file spreads its comments over ten times as many, and
	// one read would only bring the first slice's.
	reach := min(maxSegments, max(1, 1+int(span/segmentSpan)))
	if span == 0 {
		reach = 1
	}
	var items []source.Danmaku
	for index := 1; index <= reach; index++ {
		values := url.Values{}
		values.Set("type", "1")
		values.Set("oid", strconv.FormatInt(cid, 10))
		values.Set("pid", strconv.FormatInt(cid, 10))
		values.Set("segment_index", strconv.Itoa(index))
		body, err := b.body(ctx, segmentsEndpoint+"?"+values.Encode())
		if err != nil {
			if index > 1 {
				// A segment past the end answers nothing useful; what was read
				// so far is the whole of it.
				break
			}
			return nil, err
		}
		some, err := ParseSegments(body)
		if err != nil {
			return nil, err
		}
		for _, one := range some {
			if !draws(one.mode) || strings.TrimSpace(one.text) == "" {
				continue
			}
			items = append(items, source.Danmaku{
				At:    time.Duration(one.progress) * time.Millisecond,
				Text:  one.text,
				Color: one.color,
			})
		}
		if len(some) == 0 {
			// The episode has no more segments than the ones already read: a
			// segment past the end holds no comment at all.
			break
		}
	}
	sort.SliceStable(items, func(i, j int) bool { return items[i].At < items[j].At })
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

// draws says whether the stage can draw a comment of this mode: the ones that
// cross the picture and the ones pinned to its top or bottom.
func draws(mode int32) bool {
	switch mode {
	case 1, 4, 5:
		return true
	}
	return false
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
// candidates orders the hits by how likely each one is to be the upload that
// carries the show itself, leaving out the ones that do not name it at all.
func candidates(hits []candidate, query string, episode int) []candidate {
	type ranked struct {
		hit   candidate
		score int
	}
	var kept []ranked
	for _, hit := range hits {
		strength := titleMatch(normalizeTitle(hit.Title), query)
		if strength == 0 {
			continue
		}
		// A title that is the show's name and nothing else is the upload
		// that carries the show; one that wraps the name in a pitch is about
		// it. The name alone is not enough, though: copies of a popular video
		// are titled its name too, so the bonus waits for the parts the site
		// lists to confirm the upload holds a series.
		score := strength << 30
		// A title that is the show's name and nothing else is the upload that
		// carries the show, whatever it holds: the site's own catalogue names
		// the series that way, and a copy of one episode names its episode.
		if namesOnlyTheShow(hit.Title, query) {
			score += exactNameBonus
		}
		if namesTheSeries(hit.Title, query) {
			// The site's catalogue names a series by the show and its range;
			// that is the upload the layer should read, ahead of any other cut
			// of the same show.
			score += exactNameBonus << 2
		}
		// Volume, not the site's play count, is what tells the upload that
		// carries the show from the one that talks about it: a hundred-part
		// series is a long upload and ranks high here, while a widely watched
		// review of one episode is short and does not.
		// A number of parts says the upload holds a series, but nothing about
		// which one: a radio play of the show is split into parts too. It is
		// weighed enough to lift a series over a reaction to one episode, and
		// the catalogue's own naming is what lifts the show's own upload.
		score += min(hit.Duration/60, 9999) << 12
		score += min(hit.Parts, 200) << 14
		score += min(hit.Play/1000, 999) << 4
		// Age, as a guess at a series: an upload that keeps growing more
		// parts was posted before the episodes it now holds, so an older hit
		// that mentions the show is likelier to be the series than this week's
		// reaction to it.
		score += ageBonus(hit.Uploaded)
		if episode > 0 {
			switch {
			case namesTheSeries(hit.Title, query) && coversEpisode(hit.Title, episode):
				// The show's name and the range holding this episode is the
				// catalogue copy of the series, and it covers the episode by
				// construction rather than by saying its number.
				score += exactNameBonus << 1
			case namesTheEpisode(hit.Title, episode):
				score += episodeBonus << 2
			case namesAnotherEpisode(hit.Title, episode):
				score -= episodeBonus
			}
		}
		kept = append(kept, ranked{hit: hit, score: score})
	}
	sort.SliceStable(kept, func(i, j int) bool { return kept[i].score > kept[j].score })
	out := make([]candidate, len(kept))
	for i, entry := range kept {
		out[i] = entry.hit
	}
	return out
}

// bestCandidate is the most likely hit, or none when nothing named the show.
func bestCandidate(hits []candidate, query string, episode int) (candidate, bool) {
	ranked := candidates(hits, query, episode)
	if len(ranked) == 0 {
		return candidate{}, false
	}
	return ranked[0], true
}

// episodeBonus outweighs a long upload's volume, so a hit that names this very
// episode wins over a compilation that merely runs long.
const episodeBonus = 1 << 16

// exactNameBonus outweighs every kind of pitch a hit can wrap a show's name in:
// a title that is the name itself is the show's own upload.
const exactNameBonus = 1 << 24

// ageBonus turns a hit's age into a weight, saturating at roughly six years:
// past that, one old upload is as likely as another to be the series.
func ageBonus(uploaded int64) int {
	if uploaded <= 0 {
		return 0
	}
	years := time.Since(time.Unix(uploaded, 0)).Hours() / (24 * 365)
	if years <= 0 {
		return 0
	}
	if years > 6 {
		years = 6
	}
	return int(years * float64(1<<10))
}

// namesOnlyTheShow reports whether a title is the show's name and nothing else,
// once the marks a title wraps itself in are gone: the upload that carries a
// show is titled its name, while everything about it adds a pitch.
func namesOnlyTheShow(title, query string) bool {
	return normalizeTitle(title) == query
}

// partMark pulls the episode numbers a title spells out, as in "第12集" or
// "12集", so a title can be read for which episode it is about.
var partMark = regexp.MustCompile(`第?\s*(\d+)\s*[集话話期]`)

// namesTheEpisode reports whether a title says it is this episode. A hit that
// names no episode at all does not count either way, since a series' name is
// usually written without one.
func namesTheEpisode(title string, episode int) bool {
	for _, match := range partMark.FindAllStringSubmatch(title, -1) {
		if value, err := strconv.Atoi(match[1]); err == nil && value == episode {
			return true
		}
	}
	return false
}

// namesAnotherEpisode reports whether a title says it is some other episode:
// the 201st episode of a show is not the first, whatever else it names.
func namesAnotherEpisode(title string, episode int) bool {
	named := false
	for _, match := range partMark.FindAllStringSubmatch(title, -1) {
		value, err := strconv.Atoi(match[1])
		if err != nil {
			continue
		}
		named = true
		if value == episode {
			return false
		}
	}
	return named
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
