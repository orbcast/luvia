package engine

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// roundTripFunc lets a test answer HTTP without a server.
type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(request *http.Request) (*http.Response, error) { return f(request) }

func reply(status int, body string) *http.Response {
	return &http.Response{
		StatusCode: status,
		Status:     http.StatusText(status),
		Header:     http.Header{"Content-Type": []string{"application/json"}},
		Body:       io.NopCloser(strings.NewReader(body)),
	}
}

func buildSource(t *testing.T, text string, client *http.Client) source.Source {
	t.Helper()
	parsed, err := rule.Decode([]byte(text))
	if err != nil {
		t.Fatalf("decode rule: %v", err)
	}
	built, err := New(parsed, Options{Client: client})
	if err != nil {
		t.Fatalf("build source: %v", err)
	}
	return built
}

const homeRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"home": {
		"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
		"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
	}
}`

func TestHomeClassifiesUpstreamFailures(t *testing.T) {
	cases := map[string]struct {
		status int
		body   string
		kind   source.ErrorKind
	}{
		"unauthorized": {http.StatusUnauthorized, `{}`, source.ErrUnauthorized},
		"forbidden":    {http.StatusForbidden, `{}`, source.ErrUnauthorized},
		"not found":    {http.StatusNotFound, `{}`, source.ErrNotFound},
		"rate limited": {http.StatusTooManyRequests, `{}`, source.ErrRateLimited},
		"server error": {http.StatusBadGateway, `upstream exploded`, source.ErrNetwork},
		"not json":     {http.StatusOK, `<html>`, source.ErrInvalidResponse},
		"empty body":   {http.StatusOK, ``, source.ErrInvalidResponse},
	}
	for name, testCase := range cases {
		body := testCase.body
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return reply(testCase.status, body), nil
		})}
		built := buildSource(t, homeRule, client)
		_, err := built.Home(context.Background())
		if err == nil {
			t.Fatalf("%s: want an error", name)
		}
		if kind, ok := source.KindOf(err); !ok || kind != testCase.kind {
			t.Fatalf("%s: kind = %v (%v)", name, kind, err)
		}
		var diagnostic *source.Error
		if !errors.As(err, &diagnostic) || diagnostic.Source != "sample" || diagnostic.Op != "home" {
			t.Fatalf("%s: diagnostic = %#v", name, diagnostic)
		}
	}
}

func TestHomeRetriesServerErrors(t *testing.T) {
	var mu sync.Mutex
	attempts := 0
	client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		mu.Lock()
		defer mu.Unlock()
		attempts++
		if attempts < 3 {
			return reply(http.StatusInternalServerError, `{}`), nil
		}
		return reply(http.StatusOK, `{"sections":[]}`), nil
	})}
	built := buildSource(t, homeRule, client)
	home, err := built.Home(context.Background())
	if err != nil {
		t.Fatalf("home after retries: %v", err)
	}
	if len(home.Sections) != 0 {
		t.Fatalf("sections = %+v", home.Sections)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestHomeStopsOnCancelledContext(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		<-request.Context().Done()
		return nil, request.Context().Err()
	})}
	built := buildSource(t, homeRule, client)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	_, err := built.Home(ctx)
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrCanceled {
		t.Fatalf("kind = %v (%v)", kind, err)
	}

	ctx, cancel = context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := built.Home(ctx); err == nil {
		t.Fatal("a deadline must fail the call")
	} else if kind, ok := source.KindOf(err); !ok || kind != source.ErrTimeout {
		t.Fatalf("timeout kind = %v (%v)", kind, err)
	}
}

func TestRequestSendsSignedBodyAndHeaders(t *testing.T) {
	var captured *http.Request
	var capturedBody string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		encoded, _ := io.ReadAll(request.Body)
		captured, capturedBody = request, string(encoded)
		return reply(http.StatusOK, `{"list":[]}`), nil
	})}
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"http": {"headers": [{"name": "x-static", "value": "1"}]},
		"sign": {"secret": "s", "message": "{method} {path} {timestamp} {nonce}",
			"headers": [{"name": "x-signature", "value": "{signature}"}]},
		"list": {
			"steps": [{"op": "request", "as": "res", "method": "POST", "path": "/v1/items",
				"headers": [{"name": "x-step", "value": "{keyword}"}],
				"body": {"page": {"template": "{page}"}, "keyword": {"template": "{keyword}"}}}],
			"output": {"from": "res", "cards": {"from": "list[*]", "fields": {"id": "id", "title": "title"}}}
		}
	}`
	built := buildSource(t, text, client)
	if _, err := built.List(context.Background(), source.ListQuery{Keyword: "hello", Page: 3}); err != nil {
		t.Fatalf("list: %v", err)
	}
	if captured == nil {
		t.Fatal("no request captured")
	}
	if captured.Method != http.MethodPost {
		t.Fatalf("method = %s", captured.Method)
	}
	if got := captured.Header.Get("Content-Type"); got != "application/json" {
		t.Fatalf("content type = %q", got)
	}
	if captured.Header.Get("x-static") != "1" || captured.Header.Get("x-step") != "hello" {
		t.Fatalf("headers = %v", captured.Header)
	}
	if captured.Header.Get("x-signature") == "" {
		t.Fatalf("signed request has no signature header: %v", captured.Header)
	}
	var body map[string]any
	if err := json.Unmarshal([]byte(capturedBody), &body); err != nil {
		t.Fatalf("body %q: %v", capturedBody, err)
	}
	if body["page"] != "3" || body["keyword"] != "hello" {
		t.Fatalf("body = %v", body)
	}
}

func TestSignedRequestWithoutSecretIsUnauthorized(t *testing.T) {
	t.Setenv("LUVIA_TEST_SIGN_SECRET", "")
	os.Unsetenv("LUVIA_TEST_SIGN_SECRET")
	sent := false
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		sent = true
		return reply(http.StatusOK, `{"sections":[]}`), nil
	})}
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"sign": {"secret": "${env:LUVIA_TEST_SIGN_SECRET}", "message": "{method} {path}",
			"headers": [{"name": "x-signature", "value": "{signature}"}]},
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
		}
	}`
	built := buildSource(t, text, client)
	_, err := built.Home(context.Background())
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrUnauthorized {
		t.Fatalf("missing secret kind = %v (%v)", kind, err)
	}
	if !strings.Contains(err.Error(), "LUVIA_TEST_SIGN_SECRET") {
		t.Fatalf("error must name the missing variable: %v", err)
	}
	if sent {
		t.Fatal("a request must not leave with an unresolved secret")
	}
}

func TestSignedRequestExpandsSecretFromEnvironment(t *testing.T) {
	t.Setenv("LUVIA_TEST_SIGN_SECRET", "hunter2")
	var captured *http.Request
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		captured = request
		return reply(http.StatusOK, `{"sections":[]}`), nil
	})}
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"sign": {"secret": "${env:LUVIA_TEST_SIGN_SECRET}", "message": "{method} {path}",
			"headers": [{"name": "x-signature", "value": "{signature}"}]},
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
		}
	}`
	built := buildSource(t, text, client)
	if _, err := built.Home(context.Background()); err != nil {
		t.Fatalf("home: %v", err)
	}
	if captured == nil {
		t.Fatal("no request captured")
	}
	// The default message joins the four parts with newlines and the rule
	// below overrides it with a single space, so the digest covers that form.
	// `path` is the catalogue's own route, query included, not the absolute
	// address the request travels to.
	message := captured.Method + " " + captured.URL.RequestURI()
	mac := hmac.New(sha256.New, []byte("hunter2"))
	mac.Write([]byte(message))
	if want := hex.EncodeToString(mac.Sum(nil)); captured.Header.Get("x-signature") != want {
		t.Fatalf("signature = %q, want %q", captured.Header.Get("x-signature"), want)
	}
	if strings.Contains(captured.Header.Get("x-signature"), "env:") {
		t.Fatal("placeholder leaked into the signature")
	}
}

func TestPaginateOffsetStopsOnEndOfPages(t *testing.T) {
	var offsets []string
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		offset := request.URL.Query().Get("offset")
		offsets = append(offsets, offset)
		return reply(http.StatusOK, `{"episodes":[{"id":"e-`+offset+`"}]}`), nil
	})}
	text := `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"episodes": {
			"steps": [{"op": "paginate", "as": "pages", "mode": "offset", "size": 100, "maxPages": 2,
				"path": "/v1/episodes/{id}", "query": [{"key": "offset", "value": "{offset}"}],
				"items": "episodes[*]"}],
			"output": {"from": "pages[*]", "fields": {"id": "id", "title": "id"}}
		}
	}`
	built := buildSource(t, text, client)
	episodes, err := built.Episodes(context.Background(), source.Ref{Source: "sample", ID: "s1"})
	if err != nil {
		t.Fatalf("episodes: %v", err)
	}
	if len(episodes) != 2 || len(offsets) != 2 || offsets[1] != "100" {
		t.Fatalf("episodes = %+v offsets = %v", episodes, offsets)
	}
}

const resolveRule = `{
	"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
	"resolve": {
		"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}],
		"lines": {
			"from": "res.lines[*]",
			"fields": {"id": "id", "label": "label", "url": "url", "ticket": "ticket"},
			"exchange": {"ticket": "ticket", "request": {"method": "POST", "path": "/line",
				"body": {"ticket": {"template": "{ticket}"}}}, "url": "line.url"},
			"expiresIn": "res.expires_in"
		}
	}
}`

func TestResolveEdgeCases(t *testing.T) {
	cases := []struct {
		name   string
		body   string
		status int
		url    string
		kind   string
		err    source.ErrorKind
	}{
		{"absolute url", `{"lines":[{"id":"a","label":"A","url":"https://cdn.test/a.m3u8"}]}`, http.StatusOK,
			"https://cdn.test/a.m3u8", "hls", ""},
		{"no lines", `{"lines":[]}`, http.StatusOK, "", "", source.ErrNoPlayableLine},
		{"unusable line", `{"lines":[{"id":"a"}]}`, http.StatusOK, "", "", source.ErrNoPlayableLine},
	}
	for _, testCase := range cases {
		body := testCase.body
		client := &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
			return reply(testCase.status, body), nil
		})}
		built := buildSource(t, resolveRule, client)
		playback, err := built.Resolve(context.Background(), source.Episode{
			Media: source.Ref{Source: "sample", ID: "s1"}, ID: "e1", Payload: "p1",
		}, "")
		if testCase.err != "" {
			if kind, ok := source.KindOf(err); !ok || kind != testCase.err {
				t.Fatalf("%s: kind = %v (%v)", testCase.name, kind, err)
			}
			continue
		}
		if err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
		if playback.URL != testCase.url || playback.URLKind != testCase.kind {
			t.Fatalf("%s: playback = %+v", testCase.name, playback)
		}
	}
}

func TestResolveMarksExpiryAndExchanges(t *testing.T) {
	client := &http.Client{Transport: roundTripFunc(func(request *http.Request) (*http.Response, error) {
		if strings.HasSuffix(request.URL.Path, "/line") {
			return reply(http.StatusOK, `{"line":{"url":"https://cdn.test/final.mp4","url_kind":"mp4"}}`), nil
		}
		return reply(http.StatusOK, `{"expires_in":120,"lines":[{"id":"a","url":"resolve://t1","ticket":"t1"}]}`), nil
	})}
	built := buildSource(t, resolveRule, client)
	playback, err := built.Resolve(context.Background(), source.Episode{
		Media: source.Ref{Source: "sample", ID: "s1"}, ID: "e1", Payload: "p1",
	}, "")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if playback.URL != "https://cdn.test/final.mp4" || playback.URLKind != "mp4" {
		t.Fatalf("playback = %+v", playback)
	}
	if playback.ExpiresIn != 120*time.Second {
		t.Fatalf("expiry = %v", playback.ExpiresIn)
	}
	if playback.Line.Label != "a" {
		t.Fatalf("label falls back to id, got %q", playback.Line.Label)
	}
}

func TestMissingClientUsesDefaultTransport(t *testing.T) {
	parsed, err := rule.Decode([]byte(homeRule))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if _, err := New(parsed, Options{}); err != nil {
		t.Fatalf("New without options: %v", err)
	}
	if _, err := New(parsed, Options{Client: &http.Client{Transport: roundTripFunc(func(*http.Request) (*http.Response, error) {
		return nil, &url.Error{Op: "Get", URL: "https://sample.test", Err: errors.New("boom")}
	})}}); err != nil {
		t.Fatalf("New with client: %v", err)
	}
	if _, err := New(&rule.Rule{SchemaVersion: 1}, Options{}); err == nil {
		t.Fatal("New must reject an invalid rule")
	}
}
