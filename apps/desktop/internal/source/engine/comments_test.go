package engine

import (
	"context"
	"net/http"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
)

// commentsRule reads comments from an upstream that answers a whole title's
// thread and names the episode each comment was sent on.
const commentsRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"comments": {
		"steps": [{"op": "request", "as": "res", "path": "/v1/comments",
			"query": [{"key": "id", "value": "{id}"}]}],
		"output": {"from": "res.results[*]",
			"fields": {"at": {"fn": "int", "args": ["context.position_seconds"]},
				"text": "content", "author": "author.nickname",
				"episode": "context.episode_id"}}
	}
}`

const commentsBody = `{"results": [
	{"content": "later", "context": {"position_seconds": 90, "episode_id": "ep-1"}, "author": {"nickname": "Ada"}},
	{"content": "opening", "context": {"position_seconds": 3, "episode_id": "ep-1"}, "author": {"nickname": "Bo"}},
	{"content": "other episode", "context": {"position_seconds": 10, "episode_id": "ep-2"}}
]}`

func TestCommentsKeepTheEpisodeAndSortByTime(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return reply(http.StatusOK, commentsBody), nil
	})}
	built := buildSource(t, commentsRule, client)
	commenter, ok := built.(source.Commenter)
	if !ok {
		t.Fatal("a rule with a comments block must offer comments")
	}

	got, err := commenter.Comments(context.Background(), source.Episode{
		Media: source.Ref{Source: "sample", ID: "title-1"}, ID: "ep-1",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Fatalf("kept %d comments, want the two of this episode", len(got))
	}
	if got[0].Text != "opening" || got[1].Text != "later" {
		t.Fatalf("comments are not oldest first: %+v", got)
	}
	if got[0].At != 3*time.Second || got[1].At != 90*time.Second {
		t.Fatalf("comments lost their moments: %+v", got)
	}
	if got[0].Author != "Bo" {
		t.Fatalf("author: %q", got[0].Author)
	}
}

// TestCommentsWithoutRuleBlockAreUnsupported keeps a source without the block
// from claiming an empty comment layer.
func TestCommentsWithoutRuleBlockAreUnsupported(t *testing.T) {
	built := buildSource(t, calendarRule, nil)
	commenter, ok := built.(source.Commenter)
	if !ok {
		t.Fatal("every rule source must answer the capability check")
	}
	if _, err := commenter.Comments(context.Background(), source.Episode{}, 0); err == nil {
		t.Fatal("a rule without a comments block answered with comments")
	}
}
