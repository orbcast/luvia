package engine

import (
	"bytes"
	"compress/gzip"
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
)

// windowsRule reads an upstream's timeline comments: one request names the
// manifest, then each window is a file of its own under a file server that is
// not the catalogue's own host.
const windowsRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"manifest": {
		"steps": [{"op": "request", "as": "source", "path": "/v1/episodes/{payload}/source"}],
		"output": {"from": "source", "fields": {"source": "source.manifestUrl"}}
	},
	"segments": {
		"steps": [],
		"output": {"from": "document.items[*]",
			"fields": {"at": {"fn": "seconds", "args": ["0"]},
				"text": "4", "color": "2", "author": "5"}},
		"list": {"from": "document.segments[*]", "url": "url", "count": "count", "maxSegments": 8}
	}
}`

const windowsSourceBody = `{"protocol": "aimovie.danmu-source.v1", "status": "ready",
	"manifestUrl": "https://files.test/dmu_x/manifests/19.json"}`

// windowsManifestBody keeps its windows relative to itself, which is the shape
// that forces the engine to resolve them against the manifest rather than the
// catalogue.
const windowsManifestBody = `{"schema_version": "danmuku.manifest.v1", "comment_count": 3,
	"segments": [
		{"from_ms": 0, "to_ms": 300000, "count": 2, "url": "../segments/0-300000.a.json.gz"},
		{"from_ms": 300000, "to_ms": 600000, "count": 1, "url": "../segments/300000-600000.b.json.gz"}
	]}`

const firstWindowBody = `{"schema_version": "danmuku.segment.v1", "timebase": "milliseconds",
	"items": [[0, 1, 14463824, 25, "左上角笑死我了!!", "dmc_1"],
	          [17000, 1, 16777215, 25, "这是在练习台词吗", "dmc_2"]]}`

const secondWindowBody = `{"schema_version": "danmuku.segment.v1", "timebase": "milliseconds",
	"items": [[305000, 1, 255, 25, "第二段", "dmc_3"]]}`

// windowsTransport answers the three addresses a windowed read touches, and
// records which ones were asked for.
func windowsTransport(t *testing.T, firstBody []byte) (*http.Client, *[]string) {
	return windowsTransportWith(t, firstBody, []byte(secondWindowBody))
}

// windowsTransportWith lets a test answer both windows, which is what a body
// that arrives packed needs: packing one window says nothing about the other.
func windowsTransportWith(t *testing.T, firstBody, secondBody []byte) (*http.Client, *[]string) {
	t.Helper()
	var seen []string
	client := &http.Client{Transport: roundTripFunc(func(req *http.Request) (*http.Response, error) {
		seen = append(seen, req.URL.String())
		switch {
		case strings.HasSuffix(req.URL.Path, "/source"):
			return reply(http.StatusOK, windowsSourceBody), nil
		case strings.HasSuffix(req.URL.Path, "/19.json"):
			return reply(http.StatusOK, windowsManifestBody), nil
		case strings.HasSuffix(req.URL.Path, "0-300000.a.json.gz"):
			return rawReply(firstBody), nil
		case strings.HasSuffix(req.URL.Path, "300000-600000.b.json.gz"):
			return rawReply(secondBody), nil
		default:
			return reply(http.StatusNotFound, `{}`), nil
		}
	})}
	return client, &seen
}

// rawReply answers with a body and no Content-Encoding at all, which is what a
// file server storing a file named `.json.gz` actually sends.
func rawReply(body []byte) *http.Response {
	return &http.Response{
		StatusCode: http.StatusOK,
		Status:     http.StatusText(http.StatusOK),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(bytes.NewReader(body)),
	}
}

func TestManifestReadsEveryWindowInOrder(t *testing.T) {
	client, seen := windowsTransport(t, []byte(firstWindowBody))
	publisher, ok := buildSource(t, windowsRule, client).(source.Manifest)
	if !ok {
		t.Fatal("a rule with manifest and segments blocks must offer them")
	}

	got, err := publisher.Manifest(context.Background(), source.Episode{
		Media: source.Ref{Source: "sample", ID: "title-1"}, ID: "ep-1",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d comments, want every window's entries", len(got))
	}
	want := []struct {
		at   time.Duration
		text string
	}{{0, "左上角笑死我了!!"}, {17 * time.Second, "这是在练习台词吗"}, {305 * time.Second, "第二段"}}
	for index, expected := range want {
		if got[index].At != expected.at || got[index].Text != expected.text {
			t.Fatalf("comment %d is %q at %v, want %q at %v",
				index, got[index].Text, got[index].At, expected.text, expected.at)
		}
	}
	if got[0].Color != 14463824 {
		t.Fatalf("colour is %#x, want the packed value read from the array", got[0].Color)
	}
	if got[0].Author != "dmc_1" {
		t.Fatalf("author is %q, want the id packed after the text", got[0].Author)
	}
	// The window addresses are relative to the manifest, not to the catalogue.
	last := (*seen)[len(*seen)-1]
	if last != "https://files.test/dmu_x/segments/300000-600000.b.json.gz" {
		t.Fatalf("last request went to %s, want the address resolved against the manifest", last)
	}
}

func TestManifestUnpacksACompressedWindow(t *testing.T) {
	var packed bytes.Buffer
	writer := gzip.NewWriter(&packed)
	if _, err := writer.Write([]byte(firstWindowBody)); err != nil {
		t.Fatal(err)
	}
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}

	client, _ := windowsTransportWith(t, packed.Bytes(), []byte(secondWindowBody))
	publisher := buildSource(t, windowsRule, client).(source.Manifest)
	got, err := publisher.Manifest(context.Background(), source.Episode{
		Media: source.Ref{Source: "sample", ID: "title-1"}, ID: "ep-1",
	}, 0)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 3 {
		t.Fatalf("read %d comments from a gzipped window, want 3", len(got))
	}
}

func TestManifestStopsAtTheLimit(t *testing.T) {
	client, seen := windowsTransport(t, []byte(firstWindowBody))
	publisher := buildSource(t, windowsRule, client).(source.Manifest)

	got, err := publisher.Manifest(context.Background(), source.Episode{
		Media: source.Ref{Source: "sample", ID: "title-1"}, ID: "ep-1",
	}, 1)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 {
		t.Fatalf("kept %d comments, want the single one asked for", len(got))
	}
	for _, address := range *seen {
		if strings.Contains(address, "300000-600000") {
			t.Fatal("read the second window although the limit was already met")
		}
	}
}

func TestManifestIsUnsupportedWithoutItsBlocks(t *testing.T) {
	built := buildSource(t, commentsRule, &http.Client{Transport: roundTripFunc(
		func(*http.Request) (*http.Response, error) { return reply(http.StatusOK, `{}`), nil })})
	publisher, ok := built.(source.Manifest)
	if !ok {
		return
	}
	if _, err := publisher.Manifest(context.Background(), source.Episode{ID: "ep-1"}, 0); err == nil {
		t.Fatal("a rule without manifest blocks must not answer with comments")
	}
}
