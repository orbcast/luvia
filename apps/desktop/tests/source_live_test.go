package tests

import (
	"context"
	"net/http"
	"os"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
)

// TestKanjuLiveSmoke talks to the real upstream. It never runs in CI or in
// make check: set LUVIA_LIVE_SMOKE=1 when the network is available and the
// fixtures may be stale.
//
//	LUVIA_LIVE_SMOKE=1 go test ./tests -run TestKanjuLiveSmoke -v
func TestKanjuLiveSmoke(t *testing.T) {
	if os.Getenv("LUVIA_LIVE_SMOKE") != "1" {
		t.Skip("set LUVIA_LIVE_SMOKE=1 to check the rule against the live upstream")
	}
	client := &http.Client{Timeout: 30 * time.Second}
	set := loadSources(t, rulesDir, client)
	if diagnostics := set.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("rule diagnostics: %v", diagnostics)
	}
	kanju, ok := set.Get("kanju")
	if !ok {
		t.Fatal("kanju rule is missing")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
	defer cancel()

	home, err := kanju.Home(ctx)
	if err != nil {
		t.Fatalf("home: %v", err)
	}
	if len(home.Sections) == 0 {
		t.Fatal("home returned no sections")
	}
	for _, section := range home.Sections {
		t.Logf("section %q: %d cards", section.Title, len(section.Cards))
	}

	page, err := kanju.List(ctx, source.ListQuery{Kind: source.Series, Page: 1})
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(page.Cards) == 0 {
		t.Fatal("list returned no cards")
	}
	card := page.Cards[0]
	t.Logf("first series: %s (%s)", card.Title, card.Ref)

	search, err := kanju.List(ctx, source.ListQuery{Keyword: card.Title, Page: 1})
	if err != nil {
		t.Fatalf("search: %v", err)
	}
	t.Logf("search %q returned %d cards", card.Title, len(search.Cards))

	for _, candidate := range page.Cards[:3] {
		count, err := kanju.Episodes(ctx, candidate.Ref)
		if err != nil {
			t.Fatalf("episodes for %s: %v", candidate.Ref, err)
		}
		t.Logf("episodes without a detail call: %s -> %d", candidate.Title, len(count))
	}

	detail, err := kanju.Detail(ctx, card.Ref)
	if err != nil {
		t.Fatalf("detail: %v", err)
	}
	t.Logf("detail: %s, %d episodes", detail.Title, detail.EpisodeCount)

	episodes, err := kanju.Episodes(ctx, card.Ref)
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) == 0 {
		t.Fatal("episodes returned nothing")
	}
	t.Logf("episodes: %d, first payload %q", len(episodes), episodes[0].Payload)

	playback, err := kanju.Resolve(ctx, episodes[0], "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URL == "" {
		t.Fatal("resolve returned an empty address")
	}
	t.Logf("playback: kind %s, line %q, %d offers, expires in %s",
		playback.URLKind, playback.Line.Label, len(playback.Lines), playback.ExpiresIn)
}
