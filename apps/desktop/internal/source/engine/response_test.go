package engine

import (
	"context"
	"net/http"
	"testing"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// page wraps a JSON payload the way a server-rendered site ships it: the data
// sits in a `_ROUTER_DATA = {...}` assignment inside the document.
func page(payload string) string {
	return `<!doctype html><html><head><script>window.__glue_t = 1;</script>` +
		`<script async data-script-src="modern-inline">_ROUTER_DATA = ` + payload + `;</script>` +
		`</head><body><div id="root"><!--<?- html ?>--></div></body></html>`
}

const routerHomeRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"home": {
		"steps": [{"op": "request", "as": "res", "path": "/", "response": "routerData"}],
		"output": {"from": "res.loaderData.page.sections[*]", "fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
	}
}`

func TestRouterDataReadsEmbeddedPagePayload(t *testing.T) {
	body := page(`{"loaderData":{"page":{"sections":[{"id":"all","title":"hot","cards":[{"id":"s1","title":"one"}]}]}}}`)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, body), nil
	})}
	home, err := buildSource(t, routerHomeRule, client).Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 1 || home.Sections[0].Title != "hot" {
		t.Fatalf("sections = %+v", home.Sections)
	}
	if len(home.Sections[0].Cards) != 1 || home.Sections[0].Cards[0].ID != "s1" {
		t.Fatalf("cards = %+v", home.Sections[0].Cards)
	}
}

func TestRouterDataWithoutMarkerIsInvalidResponse(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `<html><body>nothing here</body></html>`), nil
	})}
	_, err := buildSource(t, routerHomeRule, client).Home(context.Background())
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrInvalidResponse {
		t.Fatalf("kind = %v (%v)", kind, err)
	}
}

func TestRouterDataSkipsOccurrenceThatIsNotJSON(t *testing.T) {
	body := `<!doctype html><script>const _ROUTER_DATA = window.broken || {};</script>` +
		page(`{"loaderData":{"page":{"sections":[{"id":"all","title":"hot","cards":[{"id":"s1","title":"one"}]}]}}}`)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, body), nil
	})}
	home, err := buildSource(t, routerHomeRule, client).Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 1 {
		t.Fatalf("sections = %+v", home.Sections)
	}
}

func TestRouterDataKeepsBracesInsideStrings(t *testing.T) {
	body := page(`{"loaderData":{"page":{"sections":[{"id":"all","title":"a } brace","cards":[{"id":"s1","title":"one"}]}]}}}`)
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, body), nil
	})}
	home, err := buildSource(t, routerHomeRule, client).Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 1 || home.Sections[0].Title != "a } brace" {
		t.Fatalf("sections = %+v", home.Sections)
	}
}

// listRule picks between a keyword search and a browse request, the way a
// source does when one endpoint only accepts a query and the other only pages.
const conditionalListRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"list": {
		"steps": [
			{"op": "request", "as": "res", "path": "/search", "when": {"fn": "not", "args": [{"fn": "empty", "args": ["keyword"]}]},
				"query": [{"key": "q", "value": "{keyword}"}]},
			{"op": "request", "as": "res", "path": "/browse", "when": {"fn": "empty", "args": ["keyword"]},
				"query": [{"key": "page", "value": "{page}"}]}
		],
		"output": {"from": "res.found[*]|res.items[*]", "cards": {"from": "res.found[*]|res.items[*]", "fields": {"id": "id", "title": "title"}}}
	}
}`

func TestWhenSelectsOneOfTwoRequests(t *testing.T) {
	var paths []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		paths = append(paths, request.URL.Path+"?"+request.URL.RawQuery)
		return reply(http.StatusOK, `{"found":[{"id":"a","title":"A"}]}`), nil
	})}
	built := buildSource(t, conditionalListRule, client)
	if _, err := built.List(context.Background(), source.ListQuery{Keyword: "逆袭", Page: 1}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/search?q=%E9%80%86%E8%A2%AD" {
		t.Fatalf("paths = %v", paths)
	}
	paths = nil
	if _, err := built.List(context.Background(), source.ListQuery{Page: 3}); err != nil {
		t.Fatalf("browse: %v", err)
	}
	if len(paths) != 1 || paths[0] != "/browse?page=3" {
		t.Fatalf("paths = %v", paths)
	}
}

func TestWhenSkipsStepWithoutSendingRequest(t *testing.T) {
	sent := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		sent++
		return reply(http.StatusOK, `{"items":[{"id":"a","title":"A"}]}`), nil
	})}
	built := buildSource(t, conditionalListRule, client)
	page, err := built.List(context.Background(), source.ListQuery{Page: 2})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if sent != 1 {
		t.Fatalf("sent = %d, want 1", sent)
	}
	if len(page.Cards) != 1 || page.Cards[0].ID != "a" {
		t.Fatalf("cards = %+v", page.Cards)
	}
}

func TestOutputFromFallsBackToLaterCandidate(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"items":[{"id":"b","title":"B"}]}`), nil
	})}
	page, err := buildSource(t, conditionalListRule, client).List(context.Background(), source.ListQuery{Page: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Cards) != 1 || page.Cards[0].ID != "b" {
		t.Fatalf("cards = %+v", page.Cards)
	}
}

func TestOutputFromReportsWhenNoCandidateResolves(t *testing.T) {
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"episodes": {
			"steps": [{"op": "request", "as": "res", "path": "/detail/{id}"}],
			"output": {"from": "res.first[*]|res.second[*]",
				"fields": {"id": "item", "payload": "item"}}
		}
	}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"detail":{"other":[]}}`), nil
	})}
	_, err := buildSource(t, text, client).Episodes(context.Background(), source.Ref{ID: "42"})
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrInvalidResponse {
		t.Fatalf("kind = %v (%v)", kind, err)
	}
}

// scalarRule maps a list of plain strings, which is all some detail pages give
// for their episodes.
const scalarRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"episodes": {
		"steps": [{"op": "request", "as": "res", "path": "/detail/{id}"}],
		"output": {"from": "res.detail.vid_list[*]",
			"fields": {"id": "item", "payload": "item", "number": "index"}}
	}
}`

func TestScalarItemsExposeValueAndPosition(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"detail":{"vid_list":["v1","v2","v3"]}}`), nil
	})}
	episodes, err := buildSource(t, scalarRule, client).Episodes(context.Background(), source.Ref{ID: "42"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 3 {
		t.Fatalf("episodes = %+v", episodes)
	}
	if episodes[0].ID != "v1" || episodes[0].Payload != "v1" || episodes[0].Number != 1 {
		t.Fatalf("first = %+v", episodes[0])
	}
	if episodes[2].ID != "v3" || episodes[2].Number != 3 {
		t.Fatalf("last = %+v", episodes[2])
	}
}

const objectRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"episodes": {
		"steps": [{"op": "request", "as": "res", "path": "/detail/{id}"}],
		"output": {"from": "res.detail.items[*]",
			"fields": {"id": "vid", "payload": "vid", "number": "sort", "title": "item"}}
	}
}`

func TestObjectItemsStillReadTheirOwnFields(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"detail":{"items":[{"vid":"v1","sort":7,"name":"first"}]}}`), nil
	})}
	episodes, err := buildSource(t, objectRule, client).Episodes(context.Background(), source.Ref{ID: "42"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 1 || episodes[0].ID != "v1" || episodes[0].Number != 7 {
		t.Fatalf("episodes = %+v", episodes)
	}
}

func TestEmptyAndNotDriveACondition(t *testing.T) {
	cases := []struct {
		keyword string
		want    string
	}{
		{keyword: "", want: "/browse"},
		{keyword: "x", want: "/search"},
	}
	for _, testCase := range cases {
		got := ""
		client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
			got = request.URL.Path
			return reply(http.StatusOK, `{"items":[]}`), nil
		})}
		if _, err := buildSource(t, conditionalListRule, client).List(context.Background(), source.ListQuery{Keyword: testCase.keyword, Page: 1}); err != nil {
			t.Fatalf("list %q: %v", testCase.keyword, err)
		}
		if got != testCase.want {
			t.Fatalf("keyword %q hit %q, want %q", testCase.keyword, got, testCase.want)
		}
	}
}

func TestConditionalStepsMayReuseAVariableName(t *testing.T) {
	parsed, err := rule.Decode([]byte(conditionalListRule))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := parsed.Validate(); err != nil {
		t.Fatalf("validate: %v", err)
	}
}

func TestUnconditionalRebindingIsStillRejected(t *testing.T) {
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/a"}, {"op": "request", "as": "res", "path": "/b"}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
		}
	}`
	parsed, err := rule.Decode([]byte(text))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if err := parsed.Validate(); err == nil {
		t.Fatal("rebinding without a condition must be rejected")
	}
}

func TestCardsFromAlsoAcceptsCandidates(t *testing.T) {
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/"}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "search_list[*]|video_list[*]", "fields": {"id": "series_id", "title": "series_title"}}}
		}
	}`
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{"sections":[{"id":"all","title":"hot","video_list":[{"series_id":"s1","series_title":"one"}]}]}`), nil
	})}
	home, err := buildSource(t, text, client).Home(context.Background())
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) != 1 || len(home.Sections[0].Cards) != 1 || home.Sections[0].Cards[0].ID != "s1" {
		t.Fatalf("sections = %+v", home.Sections)
	}
}
