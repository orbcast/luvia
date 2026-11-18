package tests

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"path/filepath"
	"strings"
	"testing"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/rule"
)

// kanjuSource loads the checked-in kanju rule with the stub transport attached.
func kanjuSource(t *testing.T, client *http.Client) source.Source {
	t.Helper()
	set := loadSources(t, rulesDir, client)
	if diagnostics := set.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("rule diagnostics for %s: %v", rulesDir, diagnostics)
	}
	found, ok := set.Get("kanju")
	if !ok {
		t.Fatalf("kanju rule not loaded; loaded %d sources", len(set.Sources()))
	}
	return found
}

// kanjuFixturePath is where the rule's own fixtures live.
var kanjuFixturePath = []string{rulesDir, "kanju", "fixtures"}

func kanjuFixture(t *testing.T, name string) string {
	t.Helper()
	return readFixture(t, append(append([]string{}, kanjuFixturePath...), name)...)
}

// verifyKanjuSignature recomputes the documented digest from the request the
// engine sent, so the sign block is checked against the algorithm rather than
// against itself.
func verifyKanjuSignature(t *testing.T, record recordedRequest, secret string) {
	t.Helper()
	timestamp := record.Header.Get("x-ai-movie-timestamp")
	nonce := record.Header.Get("x-ai-movie-nonce")
	if timestamp == "" || nonce == "" {
		t.Fatalf("signing headers missing: timestamp=%q nonce=%q", timestamp, nonce)
	}
	if len(nonce) != 32 {
		t.Fatalf("nonce %q is not 16 bytes of hex", nonce)
	}
	message := record.Method + "\n" + record.RequestURI() + "\n" + timestamp + "\n" + nonce
	mac := hmac.New(sha256.New, []byte(secret))
	if _, err := mac.Write([]byte(message)); err != nil {
		t.Fatalf("hash message: %v", err)
	}
	want := hex.EncodeToString(mac.Sum(nil))
	if got := record.Header.Get("x-ai-movie-signature"); got != want {
		t.Fatalf("signature mismatch\n got %s\nwant %s\nmessage %q", got, want, message)
	}
}

func kanjuSecret(t *testing.T) string {
	t.Helper()
	parsed, err := rule.DecodeFile(filepath.Join(rulesDir, "kanju", "rule.json"))
	if err != nil {
		t.Fatalf("decode kanju rule: %v", err)
	}
	return parsed.Sign.Secret
}

func TestKanjuHomeMapsSectionsAndCards(t *testing.T) {
	body := kanjuFixture(t, "home.json")
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, body
	})
	kanju := kanjuSource(t, client)

	home, err := kanju.Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 2 {
		t.Fatalf("want 2 sections, got %d", len(home.Sections))
	}
	featured := home.Sections[0]
	if featured.ID != "featured" || featured.Title != "精选推荐" || featured.Subtitle != "本周热播" {
		t.Fatalf("section header not mapped: %+v", featured)
	}
	if len(featured.Cards) != 2 {
		t.Fatalf("want 2 cards, got %d", len(featured.Cards))
	}
	movie := featured.Cards[0]
	if movie.Ref != (source.Ref{Source: "kanju", ID: "sample-movie"}) {
		t.Fatalf("card ref = %+v", movie.Ref)
	}
	if movie.Kind != source.Movie || movie.Year != 2024 || movie.Area != "内地" {
		t.Fatalf("card fields = %+v", movie)
	}
	if len(movie.Tags) != 2 || movie.Tags[1] != "家庭" {
		t.Fatalf("card tags = %v", movie.Tags)
	}
	if movie.PosterURL != "https://cdn.example/poster-movie.jpg" || movie.Remarks != "HD" {
		t.Fatalf("card media fields = %+v", movie)
	}
	if featured.Cards[1].Kind != source.Series {
		t.Fatalf("tv must fold onto series, got %q", featured.Cards[1].Kind)
	}
	if home.Sections[1].Cards[0].Kind != source.Anime {
		t.Fatalf("anime must stay anime, got %q", home.Sections[1].Cards[0].Kind)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(requests))
	}
	record := requests[0]
	if !strings.HasSuffix(record.URL, "/v1/feed/home?cards=8&mode=preview&scope=public&sections=11") {
		t.Fatalf("home request = %s", record.URL)
	}
	if got := record.Header.Get("x-ai-movie-client-name"); got != "dianyingtiantang-frontend" {
		t.Fatalf("client header = %q", got)
	}
	if got := record.Header.Get("Referer"); got != "https://kanju1.com/" {
		t.Fatalf("referer = %q", got)
	}
	verifyKanjuSignature(t, record, kanjuSecret(t))
}

func TestKanjuListSendsBrowseAndSearchQueries(t *testing.T) {
	body := kanjuFixture(t, "list.json")
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, body
	})
	kanju := kanjuSource(t, client)

	page, err := kanju.List(context.Background(), source.ListQuery{Kind: source.Movie})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if page.Page != 1 || !page.HasMore || len(page.Cards) != 2 {
		t.Fatalf("page = %+v", page)
	}
	if page.Cards[1].Kind != source.Movie {
		t.Fatalf("documentary must fold onto movie, got %q", page.Cards[1].Kind)
	}
	if page.Cards[0].Year != 2024 {
		t.Fatalf("card year = %d", page.Cards[0].Year)
	}

	if _, err := kanju.List(context.Background(), source.ListQuery{Keyword: "星际世界", Page: 2}); err != nil {
		t.Fatalf("search: %v", err)
	}

	requests := stub.Requests()
	if len(requests) != 2 {
		t.Fatalf("want 2 requests, got %d", len(requests))
	}
	browse := requests[0].Query
	if browse.Get("kind") != "movie" || browse.Get("page") != "1" || browse.Get("limit") != "24" {
		t.Fatalf("browse query = %v", browse)
	}
	if browse.Has("q") {
		t.Fatalf("browse query must not carry a keyword: %v", browse)
	}
	search := requests[1].Query
	if search.Get("q") != "星际世界" || search.Get("page") != "2" || search.Has("kind") {
		t.Fatalf("search query = %v", search)
	}
	// Browsing carries no intent at all: the upstream's default listing is
	// what the catalog page wants, and the search intent is only added once a
	// keyword arrives.
	if browse.Has("intent") {
		t.Fatalf("browse must not send an intent: %v", browse["intent"])
	}
	if intents := search["intent"]; len(intents) != 1 || intents[0] != "search" {
		t.Fatalf("search intent = %v", intents)
	}
}

func TestKanjuUpdatesMapDayGroupsAndSendWeekday(t *testing.T) {
	body := kanjuFixture(t, "updates.json")
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, body
	})
	kanju := kanjuSource(t, client)

	updates, err := kanju.Updates(context.Background(), source.Day{Weekday: 3})
	if err != nil {
		t.Fatalf("updates: %v", err)
	}
	// The calendar answers the whole week; the rule indexes the requested
	// day, so only that day's titles come back.
	if len(updates) != 6 {
		t.Fatalf("want the six series of day 3, got %d", len(updates))
	}
	first := updates[0]
	if first.Ref.Source != "kanju" || first.ID != "av_TtKIVe2dt577SiDSbLVOX4u2rMhPMc4VtGHBtiesmc6HXtExyHPunOjNU7OZ7XA9yPkmCWXDcx4uPXg" {
		t.Fatalf("update = %+v", first)
	}
	if first.Title != "走马上任" || first.Note != "更新至第35集" || first.Kind != source.Series {
		t.Fatalf("update fields = %+v", first)
	}
	if !strings.Contains(first.PosterURL, "77db6c8f") {
		t.Fatalf("poster = %q", first.PosterURL)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(requests))
	}
	query := requests[0].Query
	if query.Get("weekday") != "3" || query.Get("week") != "current" || query.Get("kind") != "all" {
		t.Fatalf("calendar query = %v", query)
	}
	if requests[0].Path != "/v1/watch-calendar" {
		t.Fatalf("calendar path = %s", requests[0].Path)
	}
	verifyKanjuSignature(t, requests[0], kanjuSecret(t))
}

func TestKanjuUpdatesKeepAnotherDaySeparate(t *testing.T) {
	body := kanjuFixture(t, "updates.json")
	_, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, body
	})
	kanju := kanjuSource(t, client)

	tuesday, err := kanju.Updates(context.Background(), source.Day{Weekday: 2})
	if err != nil {
		t.Fatalf("updates: %v", err)
	}
	if len(tuesday) != 6 || tuesday[0].ID != "av_bHsT4Byd4DtKoC2SuzrKZAdGpJTrIz3o513nByE_1oxzUk01xUP7wJzuxEaMy2lb2k3fWVr7okk72Zk" {
		t.Fatalf("day 2 must read its own entries, got %d: %+v", len(tuesday), tuesday)
	}
}

func TestKanjuUpdatesRejectAnImpossibleWeekday(t *testing.T) {
	_, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, `{"days":[]}`
	})
	kanju := kanjuSource(t, client)

	_, err := kanju.Updates(context.Background(), source.Day{})
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrInvalidRule {
		t.Fatalf("weekday 0 must be a rule error, got %v (%v)", kind, err)
	}
}

func TestKanjuDetailMapsFields(t *testing.T) {
	body := kanjuFixture(t, "detail.json")
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, body
	})
	kanju := kanjuSource(t, client)

	detail, err := kanju.Detail(context.Background(), source.Ref{Source: "kanju", ID: "sample-series"})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Title != "示例剧集" || detail.Kind != source.Series {
		t.Fatalf("detail = %+v", detail)
	}
	if detail.Year != 2023 || detail.Area != "内地" {
		t.Fatalf("detail fields = %+v", detail)
	}
	if len(detail.Tags) != 2 || len(detail.Actors) != 2 || len(detail.Directors) != 1 {
		t.Fatalf("detail lists = %+v", detail)
	}
	if detail.Overview != "一段用于验证规则映射的简介。" {
		t.Fatalf("overview = %q", detail.Overview)
	}
	if detail.EpisodeCount != 3 {
		t.Fatalf("episode count = %d", detail.EpisodeCount)
	}
	if !strings.HasSuffix(stub.Requests()[0].URL, "/v1/catalog/sample-series") {
		t.Fatalf("detail request = %s", stub.Requests()[0].URL)
	}
}

func TestKanjuEpisodesFollowOffsetPagination(t *testing.T) {
	stub, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		switch record.Query.Get("offset") {
		case "0":
			return http.StatusOK, kanjuFixture(t, "episodes-page-1.json")
		case "200":
			return http.StatusOK, kanjuFixture(t, "episodes-page-2.json")
		}
		return http.StatusNotFound, `{"error":"unexpected offset"}`
	})
	kanju := kanjuSource(t, client)

	episodes, err := kanju.Episodes(context.Background(), source.Ref{Source: "kanju", ID: "sample-series"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("want 3 episodes across two pages, got %d", len(episodes))
	}
	for index, episode := range episodes {
		if episode.Number != index+1 {
			t.Fatalf("episode %d number = %d", index, episode.Number)
		}
		if episode.Media != (source.Ref{Source: "kanju", ID: "sample-series"}) {
			t.Fatalf("episode %d media = %+v", index, episode.Media)
		}
	}
	if episodes[0].Payload != "token-1" || episodes[2].Payload != "token-3" {
		t.Fatalf("episode payloads = %q %q", episodes[0].Payload, episodes[2].Payload)
	}
	if episodes[1].Title != "" {
		t.Fatalf("missing title must stay empty, got %q", episodes[1].Title)
	}

	requests := stub.Requests()
	if len(requests) != 2 {
		t.Fatalf("want 2 page requests, got %d", len(requests))
	}
	for index, record := range requests {
		if record.Query.Get("limit") != "200" {
			t.Fatalf("page %d limit = %q", index, record.Query.Get("limit"))
		}
		if !strings.HasSuffix(record.URL, "/v1/catalog/sample-series/episodes?limit=200&offset="+[]string{"0", "200"}[index]) {
			t.Fatalf("page %d request = %s", index, record.URL)
		}
	}
}

func TestKanjuResolvePrefersMP4AndExchangesTickets(t *testing.T) {
	resolveBody := kanjuFixture(t, "resolve.json")
	hlsBody := kanjuFixture(t, "resolve-line-hls.json")
	mp4Body := kanjuFixture(t, "resolve-line-mp4.json")
	stub, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		switch {
		case strings.HasSuffix(record.URL, "/v1/playback/resolve/token-1"):
			return http.StatusOK, resolveBody
		case strings.HasSuffix(record.URL, "/v1/playback/resolve-line?view=compact"):
			ticket, _ := decodeBody(t, record)["ticket"].(string)
			if ticket == "ticket-hls" {
				return http.StatusOK, hlsBody
			}
			if ticket == "ticket-mp4" {
				return http.StatusOK, mp4Body
			}
			return http.StatusBadRequest, `{"error":"unknown ticket"}`
		}
		return http.StatusNotFound, `{"error":"unexpected path"}`
	})
	kanju := kanjuSource(t, client)

	playback, err := kanju.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "kanju", ID: "sample-series"},
		ID:      "ep-1",
		Payload: "token-1",
	}, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URL != "https://cdn.example/ep-1.mp4?sign=mp4" || playback.URLKind != "mp4" {
		t.Fatalf("playback must prefer mp4: %+v", playback)
	}
	if playback.Line.ID != "line-mp4" || playback.Line.Label != "线路二" {
		t.Fatalf("chosen line = %+v", playback.Line)
	}
	if len(playback.Lines) != 2 || playback.Lines[0].ID != "line-hls" || playback.Lines[1].Label != "线路二" {
		t.Fatalf("line menu = %+v", playback.Lines)
	}
	if playback.Lines[1].URLKind != "mp4" || !playback.Lines[0].Default {
		t.Fatalf("line kinds = %+v", playback.Lines)
	}

	requests := stub.Requests()
	if len(requests) != 3 {
		t.Fatalf("want resolve plus two exchanges, got %d", len(requests))
	}
	if !strings.HasSuffix(requests[1].URL, "/v1/playback/resolve-line?view=compact") {
		t.Fatalf("exchange request = %s", requests[1].URL)
	}
	if requests[1].Method != http.MethodPost {
		t.Fatalf("exchange method = %s", requests[1].Method)
	}
	if ticket, _ := decodeBody(t, requests[1])["ticket"].(string); ticket != "ticket-hls" {
		t.Fatalf("first exchange ticket = %q", ticket)
	}
	if ticket, _ := decodeBody(t, requests[2])["ticket"].(string); ticket != "ticket-mp4" {
		t.Fatalf("second exchange ticket = %q", ticket)
	}
}

func TestKanjuResolveHonoursRequestedLine(t *testing.T) {
	resolveBody := kanjuFixture(t, "resolve.json")
	hlsBody := kanjuFixture(t, "resolve-line-hls.json")
	stub, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		if strings.HasSuffix(record.URL, "/v1/playback/resolve/token-1") {
			return http.StatusOK, resolveBody
		}
		return http.StatusOK, hlsBody
	})
	kanju := kanjuSource(t, client)

	playback, err := kanju.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "kanju", ID: "sample-series"},
		ID:      "ep-1",
		Payload: "token-1",
	}, "line-hls")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URLKind != "hls" || playback.Line.ID != "line-hls" {
		t.Fatalf("requested line not used: %+v", playback)
	}
	if len(stub.Requests()) != 2 {
		t.Fatalf("want resolve plus one exchange, got %d", len(stub.Requests()))
	}
}

func TestKanjuResolveRejectsUnknownLine(t *testing.T) {
	resolveBody := kanjuFixture(t, "resolve.json")
	_, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, resolveBody
	})
	kanju := kanjuSource(t, client)

	_, err := kanju.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "kanju", ID: "sample-series"},
		ID:      "ep-1",
		Payload: "token-1",
	}, "line-missing")
	if err == nil {
		t.Fatal("want an error for an unknown line")
	}
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrNotFound {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
}

func TestKanjuResolveReportsWhenEveryLineFails(t *testing.T) {
	resolveBody := kanjuFixture(t, "resolve.json")
	stub, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		if strings.HasSuffix(record.URL, "/v1/playback/resolve/token-1") {
			return http.StatusOK, resolveBody
		}
		return http.StatusInternalServerError, `{"error":"upstream down"}`
	})
	kanju := kanjuSource(t, client)

	_, err := kanju.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "kanju", ID: "sample-series"},
		ID:      "ep-1",
		Payload: "token-1",
	}, "")
	if err == nil {
		t.Fatal("want an error when no line resolves")
	}
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrNoPlayableLine {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
	if !strings.Contains(err.Error(), "line-hls") {
		t.Fatalf("error must name the failed lines: %v", err)
	}
	if len(stub.Requests()) != 3 {
		t.Fatalf("each line must be tried once, got %d requests", len(stub.Requests()))
	}
}

func TestKanjuUnsupportedOperationWhenBlockMissing(t *testing.T) {
	parsed, err := rule.DecodeFile(filepath.Join(rulesDir, "kanju", "rule.json"))
	if err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	parsed.Resolve = nil
	built, err := engine.New(parsed, engine.Options{})
	if err != nil {
		t.Fatalf("build source: %v", err)
	}
	_, err = built.Resolve(context.Background(), source.Episode{}, "")
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrUnsupported {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
}
