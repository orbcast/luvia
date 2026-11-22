package danmaku

import (
	"bytes"
	"compress/flate"
	"context"
	"io"
	"net/http"
	"sort"
	"strings"
	"testing"
	"time"
)

// reply is one canned response, keyed by the address it answers.
type reply struct {
	body   []byte
	status int
	header map[string]string
}

// stubTransport answers the endpoints this package calls without a network.
type stubTransport struct {
	answers map[string]reply
	calls   []string
}

func (s *stubTransport) RoundTrip(request *http.Request) (*http.Response, error) {
	address := request.URL.Scheme + "://" + request.URL.Host + request.URL.Path
	s.calls = append(s.calls, request.URL.String())
	// A test that needs to answer two uploads differently keys on the whole
	// address; one that does not keys on the path alone.
	answer, ok := s.answers[request.URL.String()]
	if !ok {
		answer, ok = s.answers[address]
	}
	if !ok {
		return &http.Response{StatusCode: http.StatusNotFound, Body: io.NopCloser(strings.NewReader("")), Header: http.Header{}}, nil
	}
	status := answer.status
	if status == 0 {
		status = http.StatusOK
	}
	header := http.Header{}
	for key, value := range answer.header {
		header.Set(key, value)
	}
	return &http.Response{StatusCode: status, Body: io.NopCloser(bytes.NewReader(answer.body)), Header: header}, nil
}

func stub(answers map[string]reply) (*Bilibili, *stubTransport) {
	transport := &stubTransport{answers: answers}
	return NewBilibili(&http.Client{Transport: transport}), transport
}

const (
	searchPath = "https://api.bilibili.com/x/web-interface/wbi/search/type"
	pagesPath  = "https://api.bilibili.com/x/player/pagelist"
	dmPath     = "https://api.bilibili.com/x/v2/dm/web/seg.so"
)

// deflated wraps body the way the comment endpoint sends it: raw deflate with
// a deflate content encoding.
func deflated(t *testing.T, body string) reply {
	t.Helper()
	var buffer bytes.Buffer
	writer, err := flate.NewWriter(&buffer, flate.DefaultCompression)
	if err != nil {
		t.Fatalf("flate writer: %v", err)
	}
	if _, err := writer.Write([]byte(body)); err != nil {
		t.Fatalf("flate write: %v", err)
	}
	if err := writer.Close(); err != nil {
		t.Fatalf("flate close: %v", err)
	}
	return reply{body: buffer.Bytes(), header: map[string]string{"Content-Encoding": "deflate"}}
}

const searchAnswer = `{"code":0,"data":{"result":[
  {"bvid":"BVsmall","title":"<em class=\"keyword\">全球死考</em> 花絮","duration":"01:00","play":5},
  {"bvid":"BVbig","title":"【1-7季】<em class=\"keyword\">全球死考</em>（最全版）","duration":"18:17:22","play":200495}
]}}`

const pagesAnswer = `{"code":0,"data":[
  {"cid":101,"page":1,"part":"1-4","duration":30564},
  {"cid":102,"page":2,"part":"5","duration":18246}
]}`

// segment is one comment as the segment endpoint sends it, so the tests can
// build an answer without spelling out the protobuf by hand. The numbers come
// as plain fields: the moment, the mode, the size and the colour.
func segment(progress int32, mode int32, color uint32, text string) []byte {
	body := []byte{}
	body = append(body, varintField(2, uint64(progress))...)
	body = append(body, varintField(3, uint64(mode))...)
	body = append(body, varintField(4, 25)...)
	body = append(body, varintField(5, uint64(color))...)
	body = append(body, bytesField(7, []byte(text))...)
	return body
}

// dmAnswer packs the comments the fixtures expect, in one segment; every other
// segment is answered empty, as the endpoint does past the end of an upload.
func dmAnswer() []byte {
	body := []byte{}
	for _, one := range [][]byte{
		segment(12500, 1, 16777215, "late one"),
		segment(3000, 1, 255, "first one"),
		segment(7000, 5, 16711680, "top one"),
		segment(9000, 7, 16777215, "advanced one"),
		segment(11000, 1, 16777215, "   "),
	} {
		body = append(body, bytesField(1, one)...)
	}
	return body
}

// varintField packs one field of a protobuf message as a varint.
func varintField(field int, value uint64) []byte {
	out := appendVarint(nil, uint64(field)<<3)
	return appendVarint(out, value)
}

// bytesField packs one field of a protobuf message as length delimited bytes.
func bytesField(field int, value []byte) []byte {
	out := appendVarint(nil, uint64(field)<<3|2)
	out = appendVarint(out, uint64(len(value)))
	return append(out, value...)
}

func appendVarint(out []byte, value uint64) []byte {
	for value >= 0x80 {
		out = append(out, byte(value)|0x80)
		value >>= 7
	}
	return append(out, byte(value))
}

func happyAnswers(t *testing.T) map[string]reply {
	t.Helper()
	return map[string]reply{
		searchPath: {body: []byte(searchAnswer)},
		pagesPath:  {body: []byte(pagesAnswer)},
		dmPath:     deflated(t, string(dmAnswer())),
	}
}

func TestLookupReturnsTheEpisodesComments(t *testing.T) {
	lookup, _ := stub(happyAnswers(t))
	got, err := lookup.Lookup(context.Background(), "全球死考：刚下刑场你让我去考试", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonReady {
		t.Fatalf("reason = %v, want ready", got.Reason)
	}
	// The fixture answers every segment the same way, so a part long enough to
	// span several brings the same comments back once per segment; what matters
	// is that the right ones survived, sorted by moment, with their colour.
	want := []string{"first one", "top one", "late one"}
	if len(got.Items) == 0 || len(got.Items)%len(want) != 0 {
		t.Fatalf("items = %d, want a multiple of %d: %+v", len(got.Items), len(want), got.Items)
	}
	blocks := len(got.Items) / len(want)
	for i, item := range got.Items {
		if item.Text != want[i/blocks] {
			t.Fatalf("items[%d].Text = %q, want %q", i, item.Text, want[i/blocks])
		}
	}
	if got.Items[0].At != 3*time.Second {
		t.Errorf("first stamp = %v, want 3s", got.Items[0].At)
	}
	if got.Items[len(got.Items)-1].At != 12500*time.Millisecond {
		t.Errorf("last stamp = %v, want 12.5s", got.Items[len(got.Items)-1].At)
	}
	if got.Items[blocks].Color != 0xff0000 {
		t.Errorf("top colour = %#x, want 0xff0000", got.Items[blocks].Color)
	}
}

func TestLookupPrefersTheWatchedUpload(t *testing.T) {
	lookup, transport := stub(happyAnswers(t))
	if _, err := lookup.Lookup(context.Background(), "全球死考", 1); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	// Several hits are opened for their parts, and the most watched is
	// the one whose comments are read: the watched upload leads, because the
	// search's own order is by relevance rather than by what the site holds.
	where := -1
	for i, call := range transport.calls {
		if strings.Contains(call, dmPath) {
			where = i
			break
		}
	}
	if where < 0 {
		t.Fatalf("read no comments at all: %v", transport.calls)
	}
	for i := where - 1; i >= 0; i-- {
		if strings.Contains(transport.calls[i], pagesPath) && strings.Contains(transport.calls[i], "BVbig") {
			return
		}
	}
	t.Fatalf("read %s's comments without weighing the watched upload first: %v", transport.calls[where], transport.calls)
}

// TestLookupTriesTheNextHit covers the catalogue being buried: the first hit
// the search answers mentions the show but holds a single part, so it carries
// no episode nine, and the next hit does.
func TestLookupTriesTheNextHit(t *testing.T) {
	lookup, transport := stub(map[string]reply{
		searchPath: {body: []byte(`{"code":0,"data":{"result":[
  {"bvid":"BVclip","title":"<em class=\"keyword\">全球死考</em>","duration":"01:00","play":5},
  {"bvid":"BVbig","title":"【1-9季】<em class=\"keyword\">全球死考</em>（最全版）","duration":"18:17:22","play":200495}
]}}`)},
		pagesPath + "?bvid=BVclip": {body: []byte(`{"code":0,"data":[{"cid":101,"page":1,"part":"1","duration":600}]}`)},
		pagesPath + "?bvid=BVbig": {body: []byte(`{"code":0,"data":[
  {"cid":101,"page":1,"part":"1-8","duration":30564},
  {"cid":102,"page":2,"part":"9","duration":18246}]}`)},
		dmPath: deflated(t, string(dmAnswer())),
	})
	got, err := lookup.Lookup(context.Background(), "全球死考", 9)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonReady {
		t.Fatalf("reason = %v, want ready", got.Reason)
	}
	asked := false
	for _, call := range transport.calls {
		if strings.Contains(call, pagesPath) && strings.Contains(call, "BVbig") {
			asked = true
		}
	}
	if !asked {
		t.Fatalf("weighed only %v", transport.calls)
	}
}

func TestLookupReportsNoMatch(t *testing.T) {
	lookup, _ := stub(map[string]reply{
		searchPath: {body: []byte(`{"code":0,"data":{"result":[{"bvid":"BVx","title":"something else entirely","duration":"1:00","play":9}]}}`)},
	})
	got, err := lookup.Lookup(context.Background(), "全球死考", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonNoMatch {
		t.Fatalf("reason = %v, want no match", got.Reason)
	}
	if len(got.Items) != 0 {
		t.Fatalf("items = %+v, want none", got.Items)
	}
}

func TestLookupReportsEmptySearch(t *testing.T) {
	lookup, _ := stub(map[string]reply{
		searchPath: {body: []byte(`{"code":0,"data":{"result":[]}}`)},
	})
	got, err := lookup.Lookup(context.Background(), "全球死考", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonNoMatch {
		t.Fatalf("reason = %v, want no match", got.Reason)
	}
}

func TestLookupReportsAnEpisodeTheUploadLacks(t *testing.T) {
	answers := happyAnswers(t)
	answers[pagesPath] = reply{body: []byte(`{"code":0,"data":[{"cid":101,"page":1,"part":"1","duration":60}]}`)}
	lookup, _ := stub(answers)
	got, err := lookup.Lookup(context.Background(), "全球死考", 9)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonNoEpisode {
		t.Fatalf("reason = %v, want no episode", got.Reason)
	}
}

func TestLookupReportsAnEmptyPart(t *testing.T) {
	answers := happyAnswers(t)
	answers[dmPath] = deflated(t, "")
	lookup, _ := stub(answers)
	got, err := lookup.Lookup(context.Background(), "全球死考", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonEmpty {
		t.Fatalf("reason = %v, want empty", got.Reason)
	}
}

func TestLookupReportsAFailedSearch(t *testing.T) {
	lookup, _ := stub(map[string]reply{searchPath: {status: http.StatusInternalServerError}})
	got, err := lookup.Lookup(context.Background(), "全球死考", 1)
	if err == nil {
		t.Fatalf("want an error for a 500")
	}
	if got.Reason != ReasonFailed {
		t.Fatalf("reason = %v, want failed", got.Reason)
	}
}

func TestLookupCachesOnePart(t *testing.T) {
	lookup, transport := stub(happyAnswers(t))
	first := 0
	for i := 0; i < 3; i++ {
		before := len(transport.calls)
		if _, err := lookup.Lookup(context.Background(), "全球死考", 1); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
		if i == 0 {
			first = len(transport.calls) - before
			if first == 0 {
				t.Fatalf("the first lookup asked for nothing")
			}
		}
	}
	// An upload spread over several segments costs one request each, once: the
	// comments stay in memory and later lookups read no segment again.
	after := 0
	for _, call := range transport.calls[first:] {
		if strings.Contains(call, dmPath) {
			after++
		}
	}
	if after != 0 {
		t.Fatalf("later lookups read %d segments, want none", after)
	}
}

func TestLookupWithAnEmptyTitleAsksNothing(t *testing.T) {
	lookup, transport := stub(happyAnswers(t))
	got, err := lookup.Lookup(context.Background(), "  ", 1)
	if err != nil {
		t.Fatalf("lookup: %v", err)
	}
	if got.Reason != ReasonNoMatch {
		t.Fatalf("reason = %v, want no match", got.Reason)
	}
	if len(transport.calls) != 0 {
		t.Fatalf("called %v, want nothing", transport.calls)
	}
}

func TestParseCommentsKeepsTheModesTheStageDraws(t *testing.T) {
	items, err := ParseSegments(dmAnswer())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 5 {
		t.Fatalf("items = %d, want 5: %+v", len(items), items)
	}
	kept := 0
	for _, item := range items {
		if draws(item.mode) && strings.TrimSpace(item.text) != "" {
			kept++
		}
	}
	if kept != 3 {
		t.Fatalf("kept %d, want 3: %+v", kept, items)
	}
}

func TestParseCommentsSortsByMoment(t *testing.T) {
	items, err := ParseSegments(dmAnswer())
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	// The parser keeps the order the endpoint sent; the layer sorts once it has
	// read every segment, so that a moment from the first segment sits beside a
	// moment from the last.
	if items[0].progress != 12500 || items[1].progress != 3000 {
		t.Fatalf("order = %+v, want document order", items)
	}
	sorted := append([]comment(nil), items...)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].progress < sorted[j].progress })
	if sorted[0].text != "first one" || sorted[len(sorted)-1].text != "late one" {
		t.Fatalf("sorted = %+v", sorted)
	}
}

func TestParseCommentsSurvivesShortAttributes(t *testing.T) {
	// A comment with no attributes at all: nothing to place it by.
	items, err := ParseSegments(bytesField(1, bytesField(3, []byte("nowhere"))))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].progress != 0 {
		t.Fatalf("items = %+v, want one at the start", items)
	}
}

func TestParseCommentsRejectsAMalformedDocument(t *testing.T) {
	if _, err := ParseSegments([]byte{0x0a, 0x40, 0x01}); err == nil {
		t.Fatalf("parse of a truncated comment succeeded")
	}
}

func TestParseSegmentsReadsEveryCommentInTheBody(t *testing.T) {
	body := append(bytesField(1, segment(1000, 1, 0xffffff, "one")), bytesField(1, segment(2000, 4, 0x00ff00, "two"))...)
	items, err := ParseSegments(body)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("items = %d, want 2", len(items))
	}
	if items[1].progress != 2000 || items[1].mode != 4 || items[1].text != "two" {
		t.Fatalf("items[1] = %+v", items[1])
	}
}

func TestParseSegmentsIgnoresWhatItDoesNotRead(t *testing.T) {
	// A comment carries ids and bookkeeping the player has no use for; they do
	// not shift the numbers that matter, which are read by their own tag.
	spice := []byte{}
	spice = append(spice, varintField(1, 11698842455506946)...)
	spice = append(spice, varintField(2, 53335)...)
	spice = append(spice, varintField(3, 1)...)
	spice = append(spice, varintField(8, 1549455946)...)
	spice = append(spice, varintField(9, 8)...)
	spice = append(spice, bytesField(7, []byte("有"))...)
	items, err := ParseSegments(bytesField(1, spice))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 1 || items[0].progress != 53335 || items[0].mode != 1 || items[0].text != "有" {
		t.Fatalf("items = %+v", items)
	}
}

func TestNamesTheSeriesReadsTheCataloguesRange(t *testing.T) {
	cases := []struct {
		title string
		query string
		want  bool
	}{
		{"凡人修仙传（1-100）", "凡人修仙传", true},
		{"凡人修仙传(1-100)", "凡人修仙传", true},
		{"凡人修仙传（1001-1100）", "凡人修仙传", true},
		{"多人有声剧 |《凡人修仙传》忘语著 完结", "凡人修仙传", false},
		{"凡人修仙传 第2集", "凡人修仙传", false},
	}
	for _, one := range cases {
		if got := namesTheSeries(one.title, one.query); got != one.want {
			t.Errorf("namesTheSeries(%q) = %v, want %v", one.title, got, one.want)
		}
	}
}

func TestCoversEpisodeReadsTheRange(t *testing.T) {
	if !coversEpisode("凡人修仙传（1-100）", 30) {
		t.Fatalf("the range 1-100 does not cover 30")
	}
	if coversEpisode("凡人修仙传（1001-1100）", 30) {
		t.Fatalf("the range 1001-1100 covers 30")
	}
}

func TestCandidatesPutTheCatalogueOfTheSeriesFirst(t *testing.T) {
	hits := []candidate{
		// Longer, watched more, and split into more parts than the show is: on
		// every measure but its name a radio play of the show outranks it.
		{BVID: "radio", Title: "多人有声剧 |《凡人修仙传》忘语著 完结", Duration: 108 * 3600, Play: 9000000, Parts: 181},
		{BVID: "cut", Title: "【完结 108小时】《凡人修仙传》一口气看完", Duration: 108 * 3600, Play: 9000000},
		// A first episode that is named the same way, but does not hold this
		// one: it is the show's catalogue of an earlier range only if the range
		// covers the episode asked for.
		{BVID: "series", Title: "凡人修仙传（1-100）", Duration: 2137 * 60, Play: 766294, Parts: 100},
		{BVID: "clip", Title: "凡人修仙传 第2集", Duration: 1900, Play: 3000},
	}
	got := candidates(hits, "凡人修仙传", 2)
	if len(got) == 0 || got[0].BVID != "series" {
		t.Fatalf("ranked = %+v, want the catalogue of the series first", got)
	}
}

func TestPickPageReadsRanges(t *testing.T) {
	pages := []page{
		{CID: 1, Page: 1, Part: "1-4"},
		{CID: 2, Page: 2, Part: "5"},
		{CID: 3, Page: 3, Part: "6"},
	}
	cases := []struct {
		episode int
		want    int64
		ok      bool
	}{
		{episode: 1, want: 1, ok: true},
		{episode: 4, want: 1, ok: true},
		{episode: 5, want: 2, ok: true},
		{episode: 6, want: 3, ok: true},
		{episode: 7, want: 0, ok: false},
	}
	for _, c := range cases {
		got, ok := pickPage(pages, c.episode)
		if ok != c.ok || (ok && got.CID != c.want) {
			t.Errorf("pickPage(%d) = %d/%v, want %d/%v", c.episode, got.CID, ok, c.want, c.ok)
		}
	}
	if _, ok := pickPage(nil, 1); ok {
		t.Fatalf("picked a part from no pages")
	}
}

func TestPickPageFallsBackToPosition(t *testing.T) {
	pages := []page{{CID: 1, Page: 1, Part: "opening"}, {CID: 2, Page: 2, Part: "ending"}}
	got, ok := pickPage(pages, 2)
	if !ok || got.CID != 2 {
		t.Fatalf("pickPage = %d/%v, want the second part", got.CID, ok)
	}
	if _, ok := pickPage(pages, 5); ok {
		t.Fatalf("picked a part past the end")
	}
}

func TestNormalizeTitleDropsTheMarkup(t *testing.T) {
	got := normalizeTitle("【1-7季】<em class=\"keyword\">全球死考</em>（最全版）")
	if !strings.Contains(got, "全球死考") {
		t.Fatalf("normalised to %q, want the title kept", got)
	}
	if strings.ContainsAny(got, "【】（）<>") {
		t.Fatalf("normalised to %q, want the wrappers gone", got)
	}
}

func TestClockSeconds(t *testing.T) {
	cases := map[string]int{"1:00": 60, "18:17:22": 65842, "59": 59, "": 0, "1:x": 0}
	for input, want := range cases {
		if got := clockSeconds(input); got != want {
			t.Errorf("clockSeconds(%q) = %d, want %d", input, got, want)
		}
	}
}

func TestBestCandidateNeedsTheWholeTitle(t *testing.T) {
	hits := []candidate{
		{BVID: "a", Title: "全球死考 第1集", Play: 1},
		{BVID: "b", Title: "别的剧", Play: 1000},
	}
	got, ok := bestCandidate(hits, normalizeTitle("全球死考"), 1)
	if !ok || got.BVID != "a" {
		t.Fatalf("best = %q/%v, want the matching hit", got.BVID, ok)
	}
}
