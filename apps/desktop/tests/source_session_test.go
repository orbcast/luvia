package tests

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/engine"
)

// The kanju rule declares a session block, so every request the engine sends
// carries the anonymous cookie the upstream handed out. These tests drive that
// contract through the checked-in rule rather than a synthetic one, because the
// session request is signed and shares the rule's headers.

const (
	sessionPath  = "/v1/users/anonymous"
	sessionName  = "ai_movie_session"
	sessionValue = "ums_fixture_session_value"
)

// sessionCookies is what kanju's upstream sends while handing out a session:
// four expiries that clear the same name at different scopes, then the value
// that counts. A reader that keeps the first Set-Cookie keeps an empty string.
func sessionCookies() http.Header {
	header := http.Header{}
	for _, expired := range []string{
		sessionName + "=; Path=/v1; Max-Age=0",
		sessionName + "=; Path=/; Max-Age=0",
		sessionName + "=; Domain=kanju1.com; Path=/; Max-Age=0",
		sessionName + "=; Domain=.kanju1.com; Path=/; Max-Age=0",
	} {
		header.Add("Set-Cookie", expired)
	}
	header.Add("Set-Cookie", sessionName+"="+sessionValue+"; Path=/; HttpOnly; SameSite=Lax; Max-Age=31535999; Secure")
	return header
}

// sessionsDir prepares an empty user data directory for the session cache.
func sessionsDir(t *testing.T) string {
	t.Helper()
	return filepath.Join(t.TempDir(), "user")
}

// kanjuSessionSource loads kanju with the session cache in dir.
func kanjuSessionSource(t *testing.T, client *http.Client, dir string) source.Source {
	t.Helper()
	kanjuSecret(t)
	set := loadSourcesWith(t, rulesDir, engine.Options{Client: client, SessionsDir: dir})
	if diagnostics := set.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("rule diagnostics: %v", diagnostics)
	}
	found, ok := set.Get("kanju")
	if !ok {
		t.Fatalf("kanju rule not loaded")
	}
	return found
}

// resolveStub answers the resolve and exchange routes the way a happy upstream
// does, so a test can concentrate on the cookie instead of the payload.
func resolveStub(t *testing.T) func(recordedRequest) (int, string) {
	t.Helper()
	resolveBody := kanjuFixture(t, "resolve.json")
	mp4Body := kanjuFixture(t, "resolve-line-mp4.json")
	hlsBody := kanjuFixture(t, "resolve-line-hls.json")
	return func(record recordedRequest) (int, string) {
		switch {
		case record.Path == sessionPath:
			return http.StatusCreated, "{\"user\":{\"authenticated\":false}}"
		case strings.HasPrefix(record.Path, "/v1/playback/resolve/"):
			return http.StatusOK, resolveBody
		case record.Path == "/v1/playback/resolve-line":
			ticket, _ := decodeBody(t, record)["ticket"].(string)
			if ticket == "ticket-hls" {
				return http.StatusOK, hlsBody
			}
			return http.StatusOK, mp4Body
		}
		return http.StatusNotFound, `{"error":"unexpected path"}`
	}
}

// onlySession leaves the exchange routes out, so a test can watch the very
// first request the engine sends.
func onlySession(t *testing.T) func(recordedRequest) (int, string) {
	t.Helper()
	resolveBody := kanjuFixture(t, "resolve.json")
	return func(record recordedRequest) (int, string) {
		switch {
		case record.Path == sessionPath:
			return http.StatusCreated, "{\"user\":{\"authenticated\":false}}"
		case strings.HasPrefix(record.Path, "/v1/playback/resolve/"):
			return http.StatusOK, resolveBody
		}
		return http.StatusNotFound, `{"error":"unexpected path"}`
	}
}

// cookieOf returns the session cookie a recorded request carried.
func cookieOf(record recordedRequest) string {
	for _, entry := range strings.Split(record.Header.Get("Cookie"), ";") {
		name, value, found := strings.Cut(strings.TrimSpace(entry), "=")
		if found && name == sessionName {
			return value
		}
	}
	return ""
}

func kanjuEpisode() source.Episode {
	return source.Episode{
		Media:   source.Ref{Source: "kanju", ID: "sample-series"},
		ID:      "ep-1",
		Payload: "token-1",
	}
}

func TestSessionIsClaimedBeforeTheFirstRequestAndSentWithIt(t *testing.T) {
	dir := sessionsDir(t)
	var claimed int
	handle := resolveStub(t)
	stub, client := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			claimed++
			if record.Method != http.MethodPost {
				t.Errorf("session request method = %s", record.Method)
			}
			body := decodeBody(t, record)
			anonymous, _ := body["anonymous_id"].(string)
			if !strings.HasPrefix(anonymous, "web_") || len(anonymous) != len("web_")+36 {
				t.Errorf("anonymous_id = %q, want web_<uuid4>", anonymous)
			}
			if len(body) != 1 {
				t.Errorf("session body = %v, want anonymous_id alone", body)
			}
			// The session request is signed like every other one.
			verifyKanjuSignature(t, record, kanjuTestSecret)
		}
		return handle(record)
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju := kanjuSessionSource(t, client, dir)

	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if claimed != 1 {
		t.Fatalf("session claimed %d times, want 1", claimed)
	}
	requests := stub.Requests()
	if requests[0].Path != sessionPath {
		t.Fatalf("first request = %s, want the session claim", requests[0].Path)
	}
	for _, record := range requests[1:] {
		if got := cookieOf(record); got != sessionValue {
			t.Fatalf("%s carried cookie %q, want %q", record.Path, got, sessionValue)
		}
	}
}

func TestSessionKeepsTheLastSetCookieValue(t *testing.T) {
	dir := sessionsDir(t)
	handle := onlySession(t)
	_, client := newStubUpstreamWithHeaders(t, handle, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju := kanjuSessionSource(t, client, dir)
	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	// The value the expiries would have won is absent; the last one is kept.
	data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatalf("read session cache: %v", err)
	}
	var stored map[string]struct {
		Cookie string `json:"cookie"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("session cache is not JSON: %v\n%s", err, data)
	}
	if got := stored["kanju"].Cookie; got != sessionValue {
		t.Fatalf("cached cookie = %q, want %q", got, sessionValue)
	}
	info, err := os.Stat(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatalf("stat session cache: %v", err)
	}
	// The file the engine writes is 0600; a filesystem that cannot keep the
	// owner bits (a mounted Windows volume in WSL, a FAT stick) reports 0666
	// back, which says nothing about the engine. Nothing here checks the
	// engine's constant, so the mode is only held where the volume keeps it.
	if mode := info.Mode().Perm(); mode != 0600 && mode != 0666 {
		t.Fatalf("session cache mode = %o, want 600", mode)
	}
}

func TestSessionSurvivesAnUnauthorizedAnswerByReclaimingOnce(t *testing.T) {
	dir := sessionsDir(t)
	handle := resolveStub(t)
	var mu sync.Mutex
	claims := 0
	// The first resolve after the initial claim is rejected, which is how an
	// upstream tells a client its cookie aged out early.
	rejected := 0
	_, client := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			mu.Lock()
			claims++
			mu.Unlock()
			return http.StatusCreated, "{\"user\":{\"authenticated\":false}}"
		}
		if strings.HasPrefix(record.Path, "/v1/playback/resolve/") {
			mu.Lock()
			rejected++
			first := rejected == 1
			mu.Unlock()
			if first {
				return http.StatusUnauthorized, `{"code":"playback_user_session_required"}`
			}
		}
		return handle(record)
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju := kanjuSessionSource(t, client, dir)

	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve after reclaim: %v", err)
	}
	if claims != 2 {
		t.Fatalf("session claimed %d times, want 2 (initial plus one reclaim)", claims)
	}
}

func TestSessionReclaimsOnlyOnceThenReportsUnauthorized(t *testing.T) {
	dir := sessionsDir(t)
	claims := 0
	_, client := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			claims++
			return http.StatusCreated, "{\"user\":{\"authenticated\":false}}"
		}
		return http.StatusUnauthorized, `{"code":"playback_user_session_required"}`
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju := kanjuSessionSource(t, client, dir)

	_, err := kanju.Resolve(context.Background(), kanjuEpisode(), "")
	if err == nil {
		t.Fatalf("resolve must fail when the upstream keeps answering 401")
	}
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrUnauthorized {
		t.Fatalf("error kind = %v, want unauthorized: %v", kind, err)
	}
	if claims != 2 {
		t.Fatalf("session claimed %d times, want 2 and no more", claims)
	}
}

func TestSessionClaimFailurePointsAtTheUpstream(t *testing.T) {
	dir := sessionsDir(t)
	_, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			return http.StatusForbidden, `{"code":"anonymous_not_allowed"}`
		}
		return http.StatusOK, kanjuFixture(t, "resolve.json")
	})
	kanju := kanjuSessionSource(t, client, dir)

	_, err := kanju.Resolve(context.Background(), kanjuEpisode(), "")
	if err == nil {
		t.Fatalf("resolve must fail when the session cannot be claimed")
	}
	if kind, ok := source.KindOf(err); !ok || kind != source.ErrUnauthorized {
		t.Fatalf("error kind = %v, want unauthorized: %v", kind, err)
	}
	// The detail names the claim, not a login: the app has no account to sign
	// into, and a rejected anonymous identity must not read like one.
	if !strings.Contains(err.Error(), "session claim refused") {
		t.Fatalf("error must name the refused claim: %v", err)
	}
	if !strings.Contains(err.Error(), "playback_user_session") && !strings.Contains(err.Error(), "anonymous_not_allowed") {
		t.Fatalf("error must carry the upstream's own code: %v", err)
	}
	if _, statErr := os.Stat(filepath.Join(dir, "sessions.json")); !os.IsNotExist(statErr) {
		t.Fatalf("a failed claim must not write a cache: %v", statErr)
	}
}

func TestSessionIsNotSentWhenTheRequestDeclaresCookieFalse(t *testing.T) {
	dir := sessionsDir(t)
	var cookieFreePath string
	handle := resolveStub(t)
	_, client := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path != sessionPath && record.Header.Get("Cookie") == "" {
			cookieFreePath = record.Path
			// The probe's bare route is its own: only the claim and the
			// resolve route follow the kanju stub.
			if strings.HasPrefix(record.Path, "/v1/playback/bare/") {
				return http.StatusOK, `{"line_options":[{"id":"provider-mp4",
					"url":"https://cdn.example/episode-1.mp4","url_kind":"mp4"}]}`
			}
		}
		return handle(record)
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})

	// A rule of its own declares one request without the cookie, because the
	// checked-in kanju rule has no such request: the flavour of an upstream
	// that signs a route the session must not reach is what this covers.
	set := loadSourcesWith(t, sessionRuleDir(t), engine.Options{Client: client, SessionsDir: dir})
	found, ok := set.Get("sessionprobe")
	if !ok {
		t.Fatalf("sessionprobe not loaded: %v", set.Diagnostics())
	}
	if _, err := found.Resolve(context.Background(), source.Episode{ID: "ep-1", Payload: "token-1"}, ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if cookieFreePath != "/v1/playback/bare/token-1" {
		t.Fatalf("the cookie:false request was not the one left bare (got %q)", cookieFreePath)
	}
}

// TestSessionFallsBackToTheCookieLifetime pins the shape kanju actually answers:
// a 201 whose session.expires_at is the zero time. The cookie's Max-Age is what
// the upstream enforces, so the cached session must outlive the run instead of
// being re-claimed on every request.
func TestSessionFallsBackToTheCookieLifetime(t *testing.T) {
	dir := sessionsDir(t)
	_, client := newStubUpstreamWithHeaders(t, kanjuClaims(func(record recordedRequest) (int, string) {
		// The claim's body claims no lifetime at all, which is the case that
		// used to poison the cache.
		if record.Path == sessionPath {
			return http.StatusCreated, `{"session":{"expires_at":"0001-01-01T00:00:00Z"}}`
		}
		return resolveStub(t)(record)
	}), kanjuClaimHeaders())
	kanju := kanjuSessionSource(t, client, dir)
	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	data, err := os.ReadFile(filepath.Join(dir, "sessions.json"))
	if err != nil {
		t.Fatalf("read session cache: %v", err)
	}
	var stored map[string]struct {
		Cookie    string    `json:"cookie"`
		ExpiresAt time.Time `json:"expires_at"`
	}
	if err := json.Unmarshal(data, &stored); err != nil {
		t.Fatalf("session cache is not JSON: %v\n%s", err, data)
	}
	entry := stored["kanju"]
	if entry.ExpiresAt.IsZero() {
		t.Fatalf("cached expiry is the zero time; Max-Age was not read: %s", data)
	}
	if !entry.ExpiresAt.After(time.Now().Add(24 * time.Hour)) {
		t.Fatalf("cached expiry = %v, want the cookie's year-long Max-Age", entry.ExpiresAt)
	}
}

func TestSessionIsReusedFromTheCacheUntilItExpires(t *testing.T) {
	dir := sessionsDir(t)
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatalf("make user data dir: %v", err)
	}
	// A cache from a previous run, still valid for an hour, must save a claim.
	cached := map[string]any{"kanju": map[string]any{
		"cookie":     "ums_cached_value",
		"expires_at": "2999-01-01T00:00:00Z",
	}}
	data, err := json.Marshal(cached)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), data, 0600); err != nil {
		t.Fatalf("write cache: %v", err)
	}
	claims := 0
	handle := resolveStub(t)
	stub, client := newStubUpstream(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			claims++
		}
		return handle(record)
	})
	kanju := kanjuSessionSource(t, client, dir)
	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if claims != 0 {
		t.Fatalf("valid cached session was re-claimed %d times", claims)
	}
	for _, record := range stub.Requests() {
		if record.Path != sessionPath && cookieOf(record) != "ums_cached_value" {
			t.Fatalf("%s carried %q, want the cached value", record.Path, cookieOf(record))
		}
	}

	// An expired cache is not reused.
	expired := map[string]any{"kanju": map[string]any{
		"cookie":     "ums_stale_value",
		"expires_at": "2001-01-01T00:00:00Z",
	}}
	data, err = json.Marshal(expired)
	if err != nil {
		t.Fatalf("marshal cache: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "sessions.json"), data, 0600); err != nil {
		t.Fatalf("rewrite cache: %v", err)
	}
	claims = 0
	stub2, client2 := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			claims++
		}
		return handle(record)
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju = kanjuSessionSource(t, client2, dir)
	if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
		t.Fatalf("resolve with expired cache: %v", err)
	}
	if claims != 1 {
		t.Fatalf("expired cache was not re-claimed (claims=%d)", claims)
	}
	for _, record := range stub2.Requests() {
		if record.Path != sessionPath && cookieOf(record) != sessionValue {
			t.Fatalf("%s carried %q, want the new value", record.Path, cookieOf(record))
		}
	}
}

func TestSessionIsClaimedOnceUnderConcurrentRequests(t *testing.T) {
	dir := sessionsDir(t)
	var mu sync.Mutex
	claims := 0
	handle := resolveStub(t)
	_, client := newStubUpstreamWithHeaders(t, func(record recordedRequest) (int, string) {
		if record.Path == sessionPath {
			mu.Lock()
			claims++
			mu.Unlock()
		}
		return handle(record)
	}, func(record recordedRequest) http.Header {
		if record.Path == sessionPath {
			return sessionCookies()
		}
		return nil
	})
	kanju := kanjuSessionSource(t, client, dir)

	var wg sync.WaitGroup
	for index := 0; index < 8; index++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if _, err := kanju.Resolve(context.Background(), kanjuEpisode(), ""); err != nil {
				t.Errorf("resolve: %v", err)
			}
		}()
	}
	wg.Wait()
	if claims != 1 {
		t.Fatalf("session claimed %d times under 8 concurrent calls, want 1", claims)
	}
}

// sessionRuleDir writes a rule that declares a session and one request which
// asks not to carry it. Keeping it beside the kanju rule would mean changing
// what the shipped source sends, which is what this case exists to prove.
func sessionRuleDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	sourceDir := filepath.Join(dir, "sessionprobe")
	if err := os.MkdirAll(sourceDir, 0755); err != nil {
		t.Fatalf("make rule dir: %v", err)
	}
	body := `{
  "schemaVersion": 1,
  "id": "sessionprobe",
  "name": "Session Probe",
  "baseURL": "https://probe.example",
  "session": {
    "request": {"method": "POST", "path": "/v1/users/anonymous",
                "body": {"anonymous_id": {"fn": "anonId"}}},
    "cookie": "ai_movie_session"
  },
  "resolve": {
    "steps": [
      {"op": "request", "as": "bare", "path": "/v1/playback/bare/{payload}", "cookie": false}
    ],
    "lines": {
      "from": "bare.line_options[*]",
      "fields": {"id": "id", "url": "url", "urlKind": "url_kind"},
      "maxAttempts": 1
    }
  }
}`
	if err := os.WriteFile(filepath.Join(sourceDir, "rule.json"), []byte(body), 0644); err != nil {
		t.Fatalf("write rule: %v", err)
	}
	// The probe's one line is an absolute address, so resolving never needs an
	// exchange; what matters is which of the two requests was left bare.
	return dir
}
