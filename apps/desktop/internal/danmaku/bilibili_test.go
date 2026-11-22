package danmaku

import (
	"bytes"
	"compress/flate"
	"context"
	"io"
	"net/http"
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
	answer, ok := s.answers[address]
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
	dmPath     = "https://api.bilibili.com/x/v1/dm/list.so"
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

const dmAnswer = `<?xml version="1.0" encoding="UTF-8"?><i><chatid>101</chatid><maxlimit>8000</maxlimit>
<d p="12.50000,1,25,16777215,1794849136,0,abc,111,10">late one</d>
<d p="3.00000,1,25,255,1794849136,0,abc,112,10">first one</d>
<d p="7.00000,5,25,16711680,1794849136,0,abc,113,10">top one</d>
<d p="9.00000,7,25,16777215,1794849136,0,abc,114,10">advanced one</d>
<d p="   ,1,25,16777215,1794849136,0,abc,115,10">broken stamp</d>
<d p="11.00000,1,25,16777215,1794849136,0,abc,116,10">   </d>
</i>`

func happyAnswers(t *testing.T) map[string]reply {
	t.Helper()
	return map[string]reply{
		searchPath: {body: []byte(searchAnswer)},
		pagesPath:  {body: []byte(pagesAnswer)},
		dmPath:     deflated(t, dmAnswer),
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
	want := []string{"first one", "top one", "late one"}
	if len(got.Items) != len(want) {
		t.Fatalf("items = %d, want %d: %+v", len(got.Items), len(want), got.Items)
	}
	for i, text := range want {
		if got.Items[i].Text != text {
			t.Errorf("items[%d].Text = %q, want %q", i, got.Items[i].Text, text)
		}
	}
	if got.Items[0].At != 3*time.Second {
		t.Errorf("first stamp = %v, want 3s", got.Items[0].At)
	}
	if got.Items[1].Color != 0xff0000 {
		t.Errorf("top colour = %#x, want 0xff0000", got.Items[1].Color)
	}
}

func TestLookupPrefersTheWatchedUpload(t *testing.T) {
	lookup, transport := stub(happyAnswers(t))
	if _, err := lookup.Lookup(context.Background(), "全球死考", 1); err != nil {
		t.Fatalf("lookup: %v", err)
	}
	for _, call := range transport.calls {
		if strings.Contains(call, pagesPath) && !strings.Contains(call, "BVbig") {
			t.Fatalf("weighed %s, want the most watched hit", call)
		}
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
	answers[dmPath] = deflated(t, `<?xml version="1.0"?><i><chatid>101</chatid></i>`)
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
	for i := 0; i < 3; i++ {
		if _, err := lookup.Lookup(context.Background(), "全球死考", 1); err != nil {
			t.Fatalf("lookup %d: %v", i, err)
		}
	}
	comments := 0
	for _, call := range transport.calls {
		if strings.Contains(call, dmPath) {
			comments++
		}
	}
	if comments != 1 {
		t.Fatalf("fetched comments %d times, want once", comments)
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
	items, err := ParseComments(strings.NewReader(dmAnswer))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("items = %d, want 3: %+v", len(items), items)
	}
	for _, item := range items {
		if item.Text == "advanced one" {
			t.Fatalf("kept a mode the stage cannot draw")
		}
	}
}

func TestParseCommentsSortsByMoment(t *testing.T) {
	items, err := ParseComments(strings.NewReader(dmAnswer))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	for i := 1; i < len(items); i++ {
		if items[i].At < items[i-1].At {
			t.Fatalf("items are out of order: %+v", items)
		}
	}
}

func TestParseCommentsSurvivesShortAttributes(t *testing.T) {
	items, err := ParseComments(strings.NewReader(`<i><d p="1">no attributes</d><d p="2,1">two</d></i>`))
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if len(items) != 0 {
		t.Fatalf("items = %+v, want none", items)
	}
}

func TestParseCommentsRejectsAMalformedDocument(t *testing.T) {
	if _, err := ParseComments(strings.NewReader("not xml at all")); err != nil {
		t.Fatalf("parse: %v", err)
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
