package tests

import (
	"context"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
)

// hongguoSource loads the checked-in hongguo rule over the stub transport.
func hongguoSource(t *testing.T, client *http.Client) source.Source {
	t.Helper()
	set := loadSources(t, rulesDir, client)
	if diagnostics := set.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("hongguo rule must load cleanly: %v", diagnostics)
	}
	hongguo, ok := set.Get("hongguo")
	if !ok {
		t.Fatal("hongguo rule is not in the loaded set")
	}
	return hongguo
}

// hongguoFixture reads one checked-in response sample.
func hongguoFixture(t *testing.T, name string) string {
	t.Helper()
	return readFixture(t, rulesDir, "hongguo", "fixtures", name)
}

// TestHongguoHomeReadsSectionsFromThePage checks that the home page is read out
// of the server-rendered HTML rather than a JSON endpoint, and that each tab
// becomes a section whose cards keep the poster, the category names and the
// episode line the catalog filters read.
func TestHongguoHomeReadsSectionsFromThePage(t *testing.T) {
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, hongguoFixture(t, "home.page.txt")
	})
	hongguo := hongguoSource(t, client)

	home, err := hongguo.Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 4 {
		t.Fatalf("want 4 tabs, got %d: %+v", len(home.Sections), home.Sections)
	}
	wantIDs := []string{"all", "human", "comic", "ai"}
	for index, section := range home.Sections {
		if section.ID != wantIDs[index] {
			t.Fatalf("section %d id = %q, want %q", index, section.ID, wantIDs[index])
		}
		if section.Title == "" {
			t.Fatalf("section %d has no title", index)
		}
		if len(section.Cards) == 0 {
			t.Fatalf("section %d has no cards", index)
		}
	}
	first := home.Sections[0].Cards[0]
	if first.Ref != (source.Ref{Source: "hongguo", ID: "7698351941423729689"}) {
		t.Fatalf("card ref = %+v", first.Ref)
	}
	if first.Title != "莞城往事：八十五块闯出亿万身家" {
		t.Fatalf("card title = %q", first.Title)
	}
	// short_drama folds onto series, which is the only kind the player knows
	// for a short drama.
	if first.Kind != source.Series {
		t.Fatalf("card kind = %q", first.Kind)
	}
	if len(first.Tags) != 2 || first.Tags[0] != "逆袭" || first.Tags[1] != "剧情" {
		t.Fatalf("card tags = %v", first.Tags)
	}
	if !strings.Contains(first.Remarks, "全150集") {
		t.Fatalf("card remarks = %q", first.Remarks)
	}
	if first.PosterURL != "https://p3-novel.byteimg.com/fixture/home-1.jpg" {
		t.Fatalf("card poster = %q", first.PosterURL)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(requests))
	}
	if requests[0].Path != "/" {
		t.Fatalf("home path = %q", requests[0].Path)
	}
	// The upstream answers the search endpoint differently without a browser
	// user agent, so the rule has to send one everywhere.
	if got := requests[0].Header.Get("User-Agent"); !strings.Contains(got, "Chrome/") {
		t.Fatalf("user agent = %q", got)
	}
}

// TestHongguoBrowsingReadsTheCategoryPage checks the browsing half of the list
// block: an empty keyword takes the category page, and the section name there
// is series_name rather than series_title.
func TestHongguoBrowsingReadsTheCategoryPage(t *testing.T) {
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, hongguoFixture(t, "category.page.txt")
	})
	hongguo := hongguoSource(t, client)

	page, err := hongguo.List(context.Background(), source.ListQuery{Page: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Cards) != 2 {
		t.Fatalf("want 2 cards, got %d", len(page.Cards))
	}
	if page.Cards[0].Title != "二婚嫁对人，前夫悔破防" {
		t.Fatalf("card title = %q", page.Cards[0].Title)
	}
	if page.Page != 2 {
		t.Fatalf("page = %d, want the page the upstream reported", page.Page)
	}
	if len(page.Cards[0].Tags) != 4 || page.Cards[0].Tags[3] != "逆风翻盘" {
		t.Fatalf("card tags = %v", page.Cards[0].Tags)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("a browse must send one request, got %d", len(requests))
	}
	record := requests[0]
	if record.Path != "/category" {
		t.Fatalf("browse path = %q", record.Path)
	}
	if got := record.Query.Get("page"); got != "2" {
		t.Fatalf("browse page = %q", got)
	}
	if record.Query.Has("query") {
		t.Fatalf("browse must not carry a keyword: %v", record.Query)
	}
}

// TestHongguoSearchReadsTheJSONEndpoint checks the other half: a keyword takes
// the JSON search endpoint, which is the only place the site still answers with
// JSON, and its results are wrapped one level deeper than the category page.
func TestHongguoSearchReadsTheJSONEndpoint(t *testing.T) {
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, hongguoFixture(t, "search.json")
	})
	hongguo := hongguoSource(t, client)

	page, err := hongguo.List(context.Background(), source.ListQuery{Keyword: "逆袭", Page: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(page.Cards) != 2 {
		t.Fatalf("want 2 cards, got %d", len(page.Cards))
	}
	if page.Cards[0].Title != "莞城往事：八十五块闯出亿万身家" {
		t.Fatalf("card title = %q", page.Cards[0].Title)
	}
	if page.Cards[0].Ref.ID != "7698351941423729689" {
		t.Fatalf("card id = %q", page.Cards[0].Ref.ID)
	}
	if page.Page != 1 {
		t.Fatalf("page = %d", page.Page)
	}
	// The search endpoint reports categories under category_list and leaves
	// tags null, while the browse page reports tags and no category_list. The
	// rule reads the one key both shapes can honour, so search results carry no
	// tags instead of inventing them.
	if len(page.Cards[0].Tags) != 0 {
		t.Fatalf("card tags = %v, want none from this endpoint", page.Cards[0].Tags)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("a search must send one request, got %d", len(requests))
	}
	record := requests[0]
	if record.Path != "/api/search/result" {
		t.Fatalf("search path = %q", record.Path)
	}
	if got := record.Query.Get("query"); got != "逆袭" {
		t.Fatalf("search query = %q", got)
	}
	if got := record.Query.Get("count"); got != "10" {
		t.Fatalf("search count = %q, want the largest page the endpoint takes", got)
	}
}

// TestHongguoDetailReadsTheSeriesRecord checks the detail mapping: the title
// field differs from the card's, and the cast list is a nested object array.
func TestHongguoDetailReadsTheSeriesRecord(t *testing.T) {
	_, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		if record.Path != "/detail" {
			return http.StatusNotFound, `{"error":"unexpected path"}`
		}
		return http.StatusOK, hongguoFixture(t, "detail.page.txt")
	})
	hongguo := hongguoSource(t, client)

	detail, err := hongguo.Detail(context.Background(), source.Ref{Source: "hongguo", ID: "7706512650816982040"})
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	if detail.Title != "少夫人来自东北3" {
		t.Fatalf("title = %q", detail.Title)
	}
	if detail.EpisodeCount != 3 {
		t.Fatalf("episode count = %d", detail.EpisodeCount)
	}
	if detail.Kind != source.Series {
		t.Fatalf("kind = %q", detail.Kind)
	}
	if len(detail.Tags) != 6 || detail.Tags[0] != "家庭" {
		t.Fatalf("tags = %v", detail.Tags)
	}
	if len(detail.Actors) != 2 || detail.Actors[0] != "梁雯晶" {
		t.Fatalf("actors = %v", detail.Actors)
	}
	if !strings.Contains(detail.Remarks, "全3集") {
		t.Fatalf("remarks = %q", detail.Remarks)
	}
}

// TestHongguoEpisodesNumberTheVidList checks that the episode list, which the
// site only ships as bare ids inside the detail page, is numbered from its
// position because the upstream carries no episode index of its own.
func TestHongguoEpisodesNumberTheVidList(t *testing.T) {
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, hongguoFixture(t, "detail.page.txt")
	})
	hongguo := hongguoSource(t, client)

	episodes, err := hongguo.Episodes(context.Background(), source.Ref{Source: "hongguo", ID: "7706512650816982040"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("want 3 episodes, got %d", len(episodes))
	}
	wantVids := []string{"7706514645208534041", "7706514815799282713", "7706514815799282714"}
	for index, episode := range episodes {
		if episode.Number != index+1 {
			t.Fatalf("episode %d number = %d, want %d", index, episode.Number, index+1)
		}
		if episode.ID != wantVids[index] || episode.Payload != wantVids[index] {
			t.Fatalf("episode %d = %+v", index, episode)
		}
		if episode.Media != (source.Ref{Source: "hongguo", ID: "7706512650816982040"}) {
			t.Fatalf("episode %d media = %+v", index, episode.Media)
		}
	}
	if len(stub.Requests()) != 1 {
		t.Fatalf("episodes must reuse one detail request, got %d", len(stub.Requests()))
	}
}

// TestHongguoResolvePostsTheVid checks that playback asks the fallback endpoint
// for the episode's vid and takes the plain mp4 address it answers with.
func TestHongguoResolvePostsTheVid(t *testing.T) {
	stub, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusOK, hongguoFixture(t, "play.json")
	})
	hongguo := hongguoSource(t, client)

	playback, err := hongguo.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "hongguo", ID: "7706512650816982040"},
		ID:      "7706514645208534041",
		Payload: "7706514645208534041",
		Number:  1,
	}, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URL != "https://v11-hgweb.qznovelvod.com/fixture/video" {
		t.Fatalf("url = %q", playback.URL)
	}
	if playback.URLKind != "mp4" {
		t.Fatalf("url kind = %q", playback.URLKind)
	}
	if playback.Line.ID != "public" {
		t.Fatalf("line = %+v", playback.Line)
	}
	// The address is a plain mp4 that expires on its own clock, so the rule
	// declares no lifetime and the caller re-resolves when the player fails.
	if playback.ExpiresIn != 0 {
		t.Fatalf("expiry = %v, want none declared", playback.ExpiresIn)
	}

	requests := stub.Requests()
	if len(requests) != 1 {
		t.Fatalf("want 1 request, got %d", len(requests))
	}
	record := requests[0]
	if record.Method != http.MethodPost {
		t.Fatalf("method = %s", record.Method)
	}
	if record.Path != "/api/video/protected/fallback-play-info" {
		t.Fatalf("path = %q", record.Path)
	}
	if got, _ := decodeBody(t, record)["item_id"].(string); got != "7706514645208534041" {
		t.Fatalf("item_id = %q", got)
	}
}

// TestHongguoResolveReportsLockedEpisodes checks that the paywall stays visible:
// episodes past the free window answer 403 and the error keeps that kind, so the
// player can say the episode is unavailable instead of pretending it loaded.
func TestHongguoResolveReportsLockedEpisodes(t *testing.T) {
	_, client := newStubUpstream(t, func(recordedRequest) (int, string) {
		return http.StatusForbidden, hongguoFixture(t, "play-locked.json")
	})
	hongguo := hongguoSource(t, client)

	_, err := hongguo.Resolve(context.Background(), source.Episode{
		Media:   source.Ref{Source: "hongguo", ID: "7706512650816982040"},
		ID:      "7706514815799282714",
		Payload: "7706514815799282714",
		Number:  3,
	}, "")
	if err == nil {
		t.Fatal("want an error for a locked episode")
	}
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrUnauthorized {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
}

// TestHongguoLiveSmoke talks to the real site. It never runs in CI or in make
// check: set LUVIA_LIVE_SMOKE=1 when the network is available and the fixtures
// may be stale. The paywall is expected: only the first few episodes resolve,
// so the test stops at the first episode that answers and reports the count it
// could not reach instead of failing.
//
//	LUVIA_LIVE_SMOKE=1 go test ./tests -run TestHongguoLiveSmoke -v
func TestHongguoLiveSmoke(t *testing.T) {
	if os.Getenv("LUVIA_LIVE_SMOKE") != "1" {
		t.Skip("set LUVIA_LIVE_SMOKE=1 to check the rule against the live upstream")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	hongguo := hongguoSource(t, client)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()

	home, err := hongguo.Home(ctx)
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	for _, section := range home.Sections {
		t.Logf("section %q (%s): %d cards", section.Title, section.ID, len(section.Cards))
	}

	page, err := hongguo.List(ctx, source.ListQuery{Page: 2})
	if err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(page.Cards) == 0 {
		t.Fatal("browse returned no cards")
	}
	t.Logf("browse page %d: %d cards, first %q", page.Page, len(page.Cards), page.Cards[0].Title)

	card := page.Cards[0]
	search, err := hongguo.List(ctx, source.ListQuery{Keyword: card.Title, Page: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("search %q returned %d cards", card.Title, len(search.Cards))

	detail, err := hongguo.Detail(ctx, card.Ref)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	t.Logf("detail: %s, %d episodes, %d actors", detail.Title, detail.EpisodeCount, len(detail.Actors))

	episodes, err := hongguo.Episodes(ctx, card.Ref)
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) == 0 {
		t.Fatal("episodes returned nothing")
	}
	t.Logf("episodes: %d, first %+v", len(episodes), episodes[0])

	playable := 0
	for _, episode := range episodes {
		playback, err := hongguo.Resolve(ctx, episode, "")
		if err != nil {
			t.Logf("episode %d is not playable: %v", episode.Number, err)
			break
		}
		if playback.URL == "" {
			t.Fatalf("episode %d resolved to an empty address", episode.Number)
		}
		playable++
		if playable == 1 {
			t.Logf("playback: kind %s, line %q, url %s", playback.URLKind, playback.Line.Label, playback.URL)
		}
	}
	t.Logf("playable episodes in the free window: %d of %d", playable, len(episodes))
}
