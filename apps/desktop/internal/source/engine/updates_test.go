package engine

import (
	"context"
	"net/http"
	"testing"

	"luvia/apps/desktop/internal/source"
)

// calendarRule answers a whole week at once and picks the requested day with a
// named index, the shape a nested schedule has.
const calendarRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"updates": {
		"kindValues": {"series": "tv"},
		"days": "weekday",
		"steps": [{"op": "request", "as": "res", "path": "/v1/week",
			"query": [{"key": "day", "value": "{weekday}"}, {"key": "kind", "value": "{kind}"}]}],
		"output": {"from": "res.days[*]",
			"cards": {"from": "res.days[by:weekday].items[*]",
				"fields": {"id": "id", "title": "title", "note": "label",
					"kind": {"fn": "kind", "args": ["kind"]}}}}
	}
}`

const weekBody = `{"days": [
	{"weekday": 2, "items": [{"id": "tue", "title": "Tuesday", "label": "ep 2", "kind": "tv"}]},
	{"weekday": 4, "items": [{"id": "thu-a", "title": "Thursday A", "kind": "tv"},
	                         {"id": "thu-b", "title": "Thursday B", "label": "ep 9", "kind": "tv"}]}
]}`

func TestUpdatesReadTheRequestedDay(t *testing.T) {
	var seen []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		seen = append(seen, request.URL.Query().Get("day")+"|"+request.URL.Query().Get("kind"))
		return reply(http.StatusOK, weekBody), nil
	})}
	built := buildSource(t, calendarRule, client)

	updates, err := built.Updates(context.Background(), source.Day{Weekday: 4, Kind: source.Series})
	if err != nil {
		t.Fatalf("updates: %v", err)
	}
	if len(updates) != 2 {
		t.Fatalf("want the two Thursday items, got %d: %+v", len(updates), updates)
	}
	if updates[0].ID != "thu-a" || updates[0].Note != "" || updates[0].Kind != source.Series {
		t.Fatalf("first update = %+v", updates[0])
	}
	if updates[1].ID != "thu-b" || updates[1].Note != "ep 9" {
		t.Fatalf("second update = %+v", updates[1])
	}
	if len(seen) != 1 || seen[0] != "4|tv" {
		t.Fatalf("the request must carry the day and the mapped kind: %v", seen)
	}
}

func TestUpdatesReturnNothingForADayTheWeekOmits(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, weekBody), nil
	})}
	built := buildSource(t, calendarRule, client)

	updates, err := built.Updates(context.Background(), source.Day{Weekday: 6})
	if err != nil {
		t.Fatalf("a missing day is not an error: %v", err)
	}
	if len(updates) != 0 {
		t.Fatalf("day 6 has nothing scheduled, got %+v", updates)
	}
}

func TestUpdatesRejectAWeekdayOutsideTheWeek(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, weekBody), nil
	})}
	built := buildSource(t, calendarRule, client)

	if _, err := built.Updates(context.Background(), source.Day{}); err == nil {
		t.Fatal("weekday 0 must be rejected")
	} else if kind, _ := source.KindOf(err); kind != source.ErrInvalidRule {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
}

func TestUpdatesUnsupportedWithoutTheBlock(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, `{}`), nil
	})}
	built := buildSource(t, homeRule, client)

	if _, err := built.Updates(context.Background(), source.Day{Weekday: 1}); err == nil {
		t.Fatal("a rule without an updates block must not answer")
	} else if kind, _ := source.KindOf(err); kind != source.ErrUnsupported {
		t.Fatalf("error kind = %v (%v)", kind, err)
	}
}

// listRule declares a search intent, so a keyword swaps the browse value out.
const listRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"list": {
		"searchIntent": "search",
		"steps": [{"op": "request", "as": "res", "path": "/v1/list",
			"query": [{"key": "q", "value": "{keyword}"}, {"key": "intent", "value": "latest"}]}],
		"output": {"from": "res", "cards": {"from": "res.cards[*]", "fields": {"id": "id", "title": "title"}}}
	}
}`

func TestListSwapsTheIntentForAKeyword(t *testing.T) {
	var intents []string
	var missing []bool
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		values, present := request.URL.Query()["intent"]
		intents = append(intents, values...)
		missing = append(missing, !present)
		return reply(http.StatusOK, `{"cards":[{"id":"a","title":"A"}]}`), nil
	})}
	built := buildSource(t, listRule, client)

	if _, err := built.List(context.Background(), source.ListQuery{}); err != nil {
		t.Fatalf("browse: %v", err)
	}
	if _, err := built.List(context.Background(), source.ListQuery{Keyword: "dune"}); err != nil {
		t.Fatalf("search: %v", err)
	}
	if len(intents) != 1 || intents[0] != "search" {
		t.Fatalf("only the search call may name an intent, got %v", intents)
	}
	if !missing[0] || missing[1] {
		t.Fatalf("the browse call must drop the intent, got %v", missing)
	}
}
