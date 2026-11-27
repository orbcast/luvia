package engine

import (
	"context"
	"crypto/rand"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// sessionFile is where claimed sessions are kept in the user data directory,
// beside the values a reader supplies for ${env:NAME}. A session is a credential
// the upstream issued, so the file is readable only by its owner and replaced
// whole rather than edited in place.
const sessionFile = "sessions.json"

const sessionMode os.FileMode = 0600

// session is one claimed identity: the cookie to send and when it stops being
// worth sending. A zero expiry means the upstream stated none, so the session
// lives as long as the process and is never reused across runs.
type session struct {
	Cookie    string    `json:"cookie"`
	ExpiresAt time.Time `json:"expires_at,omitempty"`
}

// alive reports whether the session may still be sent.
func (s session) alive(now time.Time) bool {
	if s.Cookie == "" {
		return false
	}
	return s.ExpiresAt.IsZero() || now.Before(s.ExpiresAt)
}

// sessionStore holds one source's session in memory, claims it on first use and
// writes it through to disk so the next launch does not ask again.
//
// A rule that declares no session gets no store at all; every method tolerates a
// nil store so the request path stays free of conditionals.
type sessionStore struct {
	spec *rule.Session
	src  *sourceImpl
	// dir is the user data directory sessions.json lives in, empty when the
	// session must not outlive the process.
	dir string
	// anonID is the anonymous_id this source sends when it claims a session. It
	// is generated once and kept, so reclaiming after a 401 presents the same
	// identity the upstream already knows instead of a new stranger.
	anonID string
	once   sync.Once

	mu         sync.Mutex
	current    session
	loaded     bool
	reclaiming bool
}

// newSessionStore prepares the store for a source that declares a session. too.
func newSessionStore(spec *rule.Session, src *sourceImpl, dir string) *sessionStore {
	if spec == nil {
		return nil
	}
	return &sessionStore{spec: spec, src: src, dir: dir}
}

// anonymousID returns the identity this source claims sessions under, generating
// it on first use. The `anonId` function hands it back to the claim's body, which
// is evaluated while the store's own mutex is held, so it keeps its identity in
// a once instead of that mutex.
func (s *sessionStore) anonymousID() string {
	s.once.Do(func() {
		s.anonID = newAnonymousID()
	})
	return s.anonID
}

// newAnonymousID builds the `web_<uuid4>` value the upstream expects. UUIDv4 is
// a layout over 16 random bytes, so no third-party dependency is needed.
func newAnonymousID() string {
	var raw [16]byte
	if _, err := rand.Read(raw[:]); err != nil {
		// A failure here would fingerprint every user the same way, which is
		// worse than not having a session at all; fall back to the clock.
		binary.BigEndian.PutUint64(raw[:8], uint64(time.Now().UnixNano()))
		binary.BigEndian.PutUint64(raw[8:], uint64(os.Getpid()))
	}
	raw[6] = (raw[6] & 0x0f) | 0x40
	raw[8] = (raw[8] & 0x3f) | 0x80
	text := fmt.Sprintf("%x", raw)
	return "web_" + text[0:8] + "-" + text[8:12] + "-" + text[12:16] + "-" + text[16:20] + "-" + text[20:32]
}

// cookie returns the cookie to send with a request, claiming the session first
// when nothing usable is on hand. It returns "" when there is nothing to send,
// which a rule that declares no session always does.
func (s *sessionStore) cookie(ctx context.Context, site callSite) (string, error) {
	if s == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if !s.loaded {
		s.loaded = true
		s.current = s.readCache()
	}
	if s.current.alive(time.Now()) {
		return s.current.Cookie, nil
	}
	return s.claimLocked(ctx, site)
}

// reclaim drops whatever session is on hand and claims a new one. The engine
// calls it once, after a 401, so an upstream that invalidated the cookie early
// costs one extra request rather than a failed playback.
func (s *sessionStore) reclaim(ctx context.Context, site callSite) (string, error) {
	if s == nil {
		return "", nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.reclaiming && s.current.alive(time.Now()) {
		// Another request already replaced the session this one is about to
		// find stale; sending the fresh value is the whole point.
		return s.current.Cookie, nil
	}
	s.current = session{}
	s.loaded = true
	s.removeCache()
	return s.claimLocked(ctx, site)
}

// claimLocked performs the claim. The caller holds the mutex, so concurrent
// requests queue here instead of each spending a request on an identity only
// one of them can use.
func (s *sessionStore) claimLocked(ctx context.Context, site callSite) (string, error) {
	s.reclaiming = true
	session, err := s.claim(ctx, site)
	s.reclaiming = false
	if err != nil {
		return "", err
	}
	s.current = session
	s.writeCache(session)
	return session.Cookie, nil
}

// claim sends the declared request and reads the identity out of the answer.
func (s *sessionStore) claim(ctx context.Context, site callSite) (session, error) {
	spec := s.spec
	sc := scope{vars: map[string]any{}}
	path, ok := s.src.render(spec.Request.Path, sc)
	if !ok || path == "" {
		return session{}, s.src.fail(site, source.ErrInvalidRule,
			fmt.Sprintf("session.request.path %q did not resolve", spec.Request.Path), nil)
	}
	target := strings.TrimRight(s.src.rule.BaseURL, "/") + path
	payload, err := s.src.encodeBody(spec.Request.Body, sc, site)
	if err != nil {
		return session{}, err
	}
	headers := map[string]string{}
	for _, header := range s.src.rule.HTTP.Headers {
		if rendered, ok := s.src.render(expandEnv(header.Value), sc); ok {
			headers[header.Name] = rendered
		}
	}
	for _, header := range spec.Request.Headers {
		if rendered, ok := s.src.render(expandEnv(header.Value), sc); ok {
			headers[header.Name] = rendered
		}
	}
	method := spec.Request.Method
	if method == "" {
		method = http.MethodPost
	}
	if s.src.rule.Sign != nil && (spec.Request.Sign == nil || *spec.Request.Sign) {
		signed, err := s.src.signHeaders(site, method, path)
		if err != nil {
			return session{}, err
		}
		for name, value := range signed {
			headers[name] = value
		}
	}
	answer, wire, err := s.src.send(ctx, site, method, target, payload, headers)
	if err != nil {
		if source.IsKind(err, source.ErrUnauthorized) || source.IsKind(err, source.ErrNotFound) {
			return session{}, s.refused(site, err)
		}
		return session{}, err
	}
	cookie := s.sessionCookie(wire)
	if cookie == "" {
		return session{}, s.refused(site, nil)
	}
	expiry := s.expiry(answer, wire)
	return session{Cookie: cookie, ExpiresAt: expiry}, nil
}

// refused reports a claim the upstream would not honour. The kind is the same
// one a rejected credential gets, because that is what happened; the detail
// says which request refused so a rule author can tell it from a bad signature.
// Nothing here is user-facing text: the screens localize the kind, and the
// message the player shows for a missing access key would be wrong here.
func (s *sessionStore) refused(site callSite, cause error) *source.Error {
	return s.src.fail(site, source.ErrUnauthorized, "session claim refused", cause)
}

// expiry decides when the claimed session stops being usable. The declared ttl
// field wins because the upstream states it in the claim's own answer, which is
// the token's expiry rather than the cookie's; the cookie's Max-Age or Expires is
// the fallback, and a claim that states neither lasts for the process only.
//
// The claim's own `expires_at` is a claim on paper — kanju answers 201 with
// `session.expires_at: "0001-01-01T00:00:00Z"` — so a zero or past instant falls
// through to the cookie rather than teaching the cache that the session is
// already dead. The cookie's Max-Age is the number the upstream actually
// enforces.
func (s *sessionStore) expiry(answer []byte, wire wireResponse) time.Time {
	now := time.Now()
	if name := strings.TrimSpace(s.spec.TTL); name != "" {
		var object map[string]any
		if err := json.Unmarshal(answer, &object); err == nil {
			if raw, ok := lookupField(object, name); ok {
				if instant, err := time.Parse(time.RFC3339, strings.TrimSpace(toString(raw))); err == nil && instant.After(now) {
					return instant
				}
			}
		}
	}
	cookieExpiry := time.Time{}
	for _, header := range wire.setCookie {
		if instant, ok := parseSetCookieExpiry(header, s.spec.Cookie, now); ok && instant.After(cookieExpiry) {
			cookieExpiry = instant
		}
	}
	return cookieExpiry
}

// parseSetCookieExpiry reads the lifetime a Set-Cookie header states, either as
// Max-Age seconds or as an Expires date. A header that clears the cookie states
// neither a future Max-Age nor a future date, so it yields nothing.
func parseSetCookieExpiry(header, name string, now time.Time) (time.Time, bool) {
	if _, ok := parseSetCookie(header, name); !ok {
		return time.Time{}, false
	}
	for _, attribute := range strings.Split(header, ";")[1:] {
		key, value, found := strings.Cut(strings.TrimSpace(attribute), "=")
		if !found {
			continue
		}
		switch strings.ToLower(strings.TrimSpace(key)) {
		case "max-age":
			seconds, err := strconv.Atoi(strings.TrimSpace(value))
			if err != nil || seconds <= 0 {
				continue
			}
			return now.Add(time.Duration(seconds) * time.Second), true
		case "expires":
			// RFC 1123 with GMT is what an upstream writes; the other layouts
			// are the ones older servers still emit.
			for _, layout := range []string{http.TimeFormat, time.RFC1123, time.RFC850, time.ANSIC} {
				if instant, err := time.Parse(layout, strings.TrimSpace(value)); err == nil {
					if instant.After(now) {
						return instant, true
					}
					return time.Time{}, false
				}
			}
		}
	}
	return time.Time{}, false
}

// lookupField reads a dotted field path out of a decoded answer.
func lookupField(object map[string]any, path string) (any, bool) {
	var current any = object
	for _, part := range strings.Split(path, ".") {
		fields, ok := current.(map[string]any)
		if !ok {
			return nil, false
		}
		current, ok = fields[part]
		if !ok {
			return nil, false
		}
	}
	return current, true
}

// sessionCookie reads the value to keep out of an answer's Set-Cookie headers.
//
// An upstream that rotates a session clears the same name first — once per Path
// and Domain it may have set it under — and only the last non-empty value is the
// identity it just issued. Taking the first header, or the first one with a
// value, hands back one of the clearing directives and every later request is
// answered as anonymous.
func (s *sessionStore) sessionCookie(wire wireResponse) string {
	value := ""
	for _, header := range wire.setCookie {
		if kept, ok := parseSetCookie(header, s.spec.Cookie); ok {
			// `ok` already means "carries this name and a value", so the last
			// such header is the issued one: earlier ones only clear it.
			value = kept
		}
	}
	return value
}

// parseSetCookie reports the value of one Set-Cookie header when it carries the
// session name with a value, and false for a header that only clears it.
func parseSetCookie(header, name string) (string, bool) {
	pair, _, _ := strings.Cut(header, ";")
	key, value, found := strings.Cut(strings.TrimSpace(pair), "=")
	if !found || strings.TrimSpace(key) != name || strings.TrimSpace(value) == "" {
		return "", false
	}
	return value, true
}

// readCache loads the stored session for this source. A missing, unreadable or
// expired entry is simply no session: the claim below replaces it.
func (s *sessionStore) readCache() session {
	if s.dir == "" {
		return session{}
	}
	data, err := os.ReadFile(filepath.Join(s.dir, sessionFile))
	if err != nil {
		return session{}
	}
	var stored map[string]session
	if err := json.Unmarshal(data, &stored); err != nil {
		return session{}
	}
	entry, ok := stored[s.src.rule.ID]
	if !ok || !entry.alive(time.Now()) {
		return session{}
	}
	return entry
}

// writeCache stores the session without disturbing what other sources kept.
// Failing to write is not a failed claim: the session works for this run.
func (s *sessionStore) writeCache(entry session) {
	if s.dir == "" {
		return
	}
	if err := os.MkdirAll(s.dir, 0755); err != nil {
		return
	}
	path := filepath.Join(s.dir, sessionFile)
	stored := map[string]session{}
	if data, err := os.ReadFile(path); err == nil {
		if err := json.Unmarshal(data, &stored); err != nil {
			stored = map[string]session{}
		}
	}
	stored[s.src.rule.ID] = entry
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	temporary, err := os.CreateTemp(s.dir, ".sessions-*")
	if err != nil {
		return
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(append(data, '\n')); err != nil {
		temporary.Close()
		return
	}
	if err := temporary.Close(); err != nil {
		return
	}
	if err := os.Chmod(name, sessionMode); err != nil {
		return
	}
	os.Rename(name, path)
}

// removeCache forgets this source's stored session, so a stale cookie is not
// read back by the next launch either.
func (s *sessionStore) removeCache() {
	if s.dir == "" {
		return
	}
	path := filepath.Join(s.dir, sessionFile)
	data, err := os.ReadFile(path)
	if err != nil {
		return
	}
	stored := map[string]session{}
	if err := json.Unmarshal(data, &stored); err != nil {
		return
	}
	if _, ok := stored[s.src.rule.ID]; !ok {
		return
	}
	delete(stored, s.src.rule.ID)
	if len(stored) == 0 {
		os.Remove(path)
		return
	}
	encoded, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return
	}
	temporary, err := os.CreateTemp(s.dir, ".sessions-*")
	if err != nil {
		return
	}
	name := temporary.Name()
	defer os.Remove(name)
	if _, err := temporary.Write(append(encoded, '\n')); err != nil {
		temporary.Close()
		return
	}
	if err := temporary.Close(); err != nil {
		return
	}
	if err := os.Chmod(name, sessionMode); err != nil {
		return
	}
	os.Rename(name, path)
}
