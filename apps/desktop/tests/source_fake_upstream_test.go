package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/loader"
)

// fakeUpstreamDir holds a rule for a second, unrelated upstream shape: an
// unsigned JSON API with page numbers, literal fields, direct media addresses
// and no ticket exchange. It exists to prove the rule DSL is not fitted to one
// site; it is deliberately not a complete contract of any real product.
const fakeUpstreamDir = "testdata"

const (
	fakeHome = `{"code":1,"sections":[
		{"type_name":"Movies","videos":[
			{"vod_id":"11","vod_name":"Fake Movie","type_name":"movie","vod_year":"2020","vod_pic":"https://cdn.test/11.jpg","vod_remarks":"HD"}]},
		{"type_name":"Shows","videos":[
			{"vod_id":"22","vod_name":"Fake Show","type_name":"tv","vod_year":"2019","vod_pic":"https://cdn.test/22.jpg"}]}
	]}`

	fakeList = `{"code":1,"page":2,"has_more":true,"list":[
		{"vod_id":"11","vod_name":"Fake Movie","type_name":"documentary","vod_year":"2020","vod_pic":"https://cdn.test/11.jpg"}]}`

	fakeDetail = `{"code":1,"list":[
		{"vod_id":"11","vod_name":"Fake Movie","type_name":"movie","vod_year":"2020","vod_area":"US",
		 "vod_content":"Plot.","vod_pic":"https://cdn.test/11.jpg",
		 "vod_actor":["Actor A","Actor B"],"vod_director":["Director C"]}]}`

	fakeEpisodesPage1 = `{"code":1,"has_more":true,"list":[{"nid":"501","number":1,"name":"Pilot"}]}`
	fakeEpisodesPage2 = `{"code":1,"has_more":false,"list":[{"nid":"502","number":2,"name":"Finale"}]}`

	fakePlayback = `{"code":1,"lines":[
		{"id":"sd","label":"SD","url":"http://cdn.test/1.mp4"},
		{"id":"hd","label":"HD","url":"http://cdn.test/1.m3u8"}]}`
)

// fakeSource returns the loaded fake upstream, failing loudly when the rule did
// not load instead of dereferencing a nil interface.
func fakeSource(t *testing.T, set *loader.Set) source.Source {
	t.Helper()
	fake, ok := set.Get("maccms")
	if !ok {
		t.Fatalf("fake rule not loaded: %v", set.Diagnostics())
	}
	return fake
}

// fakeHandler answers the fake upstream and asserts the rule sent the query the
// block describes.
func fakeHandler(t *testing.T) func(recordedRequest) (int, string) {
	t.Helper()
	return func(record recordedRequest) (int, string) {
		if !strings.HasPrefix(record.URL, "http://upstream.test/api.php/provide/vod/") {
			t.Errorf("unexpected path %s", record.URL)
			return http.StatusNotFound, `{"code":0}`
		}
		switch record.Query.Get("ac") {
		case "list":
			if record.Query.Get("wd") == "" && record.Query.Get("pg") == "1" {
				return http.StatusOK, fakeHome
			}
			return http.StatusOK, fakeList
		case "detail":
			switch record.Query.Get("pg") {
			case "":
				return http.StatusOK, fakeDetail
			case "1":
				return http.StatusOK, fakeEpisodesPage1
			case "2":
				return http.StatusOK, fakeEpisodesPage2
			}
			t.Errorf("unexpected episode page %q", record.Query.Get("pg"))
			return http.StatusNotFound, `{"code":0}`
		case "play":
			return http.StatusOK, fakePlayback
		}
		t.Errorf("unexpected ac=%q", record.Query.Get("ac"))
		return http.StatusNotFound, `{"code":0}`
	}
}

func TestFakeUpstreamHomeUsesLiteralsAndKindMapping(t *testing.T) {
	stub, client := newStubUpstream(t, fakeHandler(t))
	set := loadSources(t, fakeUpstreamDir, client)
	fake := fakeSource(t, set)

	home, err := fake.Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 2 {
		t.Fatalf("sections = %d", len(home.Sections))
	}
	if got := home.Sections[0].ID; got != "latest" {
		t.Fatalf("literal section id = %q", got)
	}
	if got := home.Sections[0].Title; got != "Movies" {
		t.Fatalf("section title = %q", got)
	}
	movie := home.Sections[0].Cards[0]
	if movie.Kind != source.Movie || movie.Year != 2020 || movie.Remarks != "HD" {
		t.Fatalf("card = %+v", movie)
	}
	if home.Sections[1].Cards[0].Kind != source.Series {
		t.Fatalf("tv must fold onto series, got %q", home.Sections[1].Cards[0].Kind)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("requests = %d", len(requests))
	}
	if got := requests[0].URL; !strings.HasSuffix(got, "/api.php/provide/vod/?ac=list&pg=1") {
		t.Fatalf("home request = %s", got)
	}
	if got := requests[0].Header.Get("x-ai-movie-signature"); got != "" {
		t.Fatalf("unsigned rule must not sign: %q", got)
	}
}

func TestFakeUpstreamListCarriesPageAndKeyword(t *testing.T) {
	stub, client := newStubUpstream(t, fakeHandler(t))
	set := loadSources(t, fakeUpstreamDir, client)
	fake := fakeSource(t, set)

	page, err := fake.List(context.Background(), source.ListQuery{Page: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Page != 2 || !page.HasMore || len(page.Cards) != 1 {
		t.Fatalf("page = %+v", page)
	}
	if page.Cards[0].Kind != source.Movie {
		t.Fatalf("documentary must fold onto movie, got %q", page.Cards[0].Kind)
	}

	if _, err := fake.List(context.Background(), source.ListQuery{Keyword: "fake movie"}); err != nil {
		t.Fatalf("search: %v", err)
	}

	requests := stub.Requests()
	if len(requests) != 2 {
		t.Fatalf("requests = %d", len(requests))
	}
	if got := requests[0].Query.Get("pg"); got != "2" {
		t.Fatalf("page query = %q", got)
	}
	if got := requests[1].Query.Get("wd"); got != "fake movie" {
		t.Fatalf("keyword query = %q", got)
	}
	if got := requests[1].Query.Get("pg"); got != "1" {
		t.Fatalf("search must fall back to page 1, got %q", got)
	}
}

func TestFakeUpstreamDetailReadsFirstArrayItem(t *testing.T) {
	_, client := newStubUpstream(t, fakeHandler(t))
	set := loadSources(t, fakeUpstreamDir, client)
	fake := fakeSource(t, set)

	detail, err := fake.Detail(context.Background(), source.Ref{Source: "maccms", ID: "11"})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Title != "Fake Movie" || detail.Kind != source.Movie || detail.Year != 2020 {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Area != "US" || detail.Overview != "Plot." {
		t.Fatalf("detail text = %+v", detail)
	}
	if len(detail.Actors) != 2 || len(detail.Directors) != 1 {
		t.Fatalf("detail lists = %+v", detail)
	}
}

func TestFakeUpstreamEpisodesPaginateByPageNumber(t *testing.T) {
	stub, client := newStubUpstream(t, fakeHandler(t))
	set := loadSources(t, fakeUpstreamDir, client)
	fake := fakeSource(t, set)

	episodes, err := fake.Episodes(context.Background(), source.Ref{Source: "maccms", ID: "11"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 2 {
		t.Fatalf("episodes = %d", len(episodes))
	}
	if episodes[0].ID != "501" || episodes[1].ID != "502" {
		t.Fatalf("episode ids = %q %q", episodes[0].ID, episodes[1].ID)
	}
	if episodes[1].Number != 2 || episodes[1].Title != "Finale" || episodes[1].Payload != "502" {
		t.Fatalf("episode = %+v", episodes[1])
	}
	requests := stub.Requests()
	if len(requests) != 2 {
		t.Fatalf("want two pages, got %d", len(requests))
	}
	if got := requests[0].Query.Get("pg"); got != "1" {
		t.Fatalf("first page = %q", got)
	}
	if got := requests[1].Query.Get("pg"); got != "2" {
		t.Fatalf("second page = %q", got)
	}
}

func TestFakeUpstreamResolveUsesDirectAddresses(t *testing.T) {
	stub, client := newStubUpstream(t, fakeHandler(t))
	set := loadSources(t, fakeUpstreamDir, client)
	fake := fakeSource(t, set)

	playback, err := fake.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "maccms", ID: "11"},
		ID:      "501",
		Payload: "501",
	}, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URL != "http://cdn.test/1.m3u8" || playback.URLKind != "hls" {
		t.Fatalf("playback = %+v", playback)
	}
	if playback.Line.ID != "hd" || playback.Line.Label != "HD" {
		t.Fatalf("line = %+v", playback.Line)
	}
	if len(playback.Lines) != 2 {
		t.Fatalf("line menu = %+v", playback.Lines)
	}
	if playback.Lines[0].URLKind != "mp4" || playback.Lines[1].URLKind != "hls" {
		t.Fatalf("line kinds = %+v", playback.Lines)
	}
	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("direct addresses need no exchange, got %d requests", len(requests))
	}
	if got := requests[0].Query.Get("nid"); got != "501" {
		t.Fatalf("payload query = %q", got)
	}
}

// TestFakeUpstreamRuleUsesOnlySchemaKeys proves the alternate rule carries no
// site-specific extension: every top-level key belongs to the shared schema.
func TestFakeUpstreamRuleUsesOnlySchemaKeys(t *testing.T) {
	allowed := map[string]bool{
		"schemaVersion": true, "id": true, "name": true, "baseURL": true, "http": true,
		"sign": true, "home": true, "list": true, "detail": true, "episodes": true, "resolve": true,
	}
	var decoded map[string]any
	if err := json.Unmarshal([]byte(readFixture(t, fakeUpstreamDir, "maccms", "rule.json")), &decoded); err != nil {
		t.Fatalf("decode fake rule: %v", err)
	}
	for key := range decoded {
		if !allowed[key] {
			t.Fatalf("unexpected top-level key %q", key)
		}
	}
}
