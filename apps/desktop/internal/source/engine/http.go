package engine

import (
	"bytes"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"sort"
	"strconv"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// callSite names the block and step a failure happened in, for diagnostics.
type callSite struct {
	op   string
	step string
}

func (s *sourceImpl) fail(site callSite, kind source.ErrorKind, detail string, cause error) *source.Error {
	return &source.Error{
		Kind:   kind,
		Source: s.rule.ID,
		Op:     site.op,
		Step:   site.step,
		Detail: detail,
		Cause:  cause,
	}
}

// fetch performs one request described by the rule and returns decoded JSON.
func (s *sourceImpl) fetch(ctx context.Context, site callSite, req rule.Request, sc scope) (any, error) {
	body, err := s.fetchBody(ctx, site, req, sc)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	return decodeResponse(site, body, req.Response, s.fail)
}

// fetchBody performs one request described by the rule and hands back the body
// as it arrived. A caller that reads a document the engine does not decode
// itself — one that may or may not be compressed, such as a window of an
// upstream's comments — goes through here instead of fetch.
func (s *sourceImpl) fetchBody(ctx context.Context, site callSite, req rule.Request, sc scope) ([]byte, error) {
	path, ok := s.render(req.Path, sc)
	if !ok || path == "" {
		return nil, s.fail(site, source.ErrInvalidRule,
			fmt.Sprintf("path %q did not resolve", req.Path), nil)
	}
	// `path` is the address the upstream knows as its own, and it is what a
	// signature covers even when the request itself goes elsewhere: a
	// catalogue signs its own route, not the file server's absolute URL.
	base := strings.TrimRight(s.rule.BaseURL, "/")
	target := ""
	ownPath := false
	switch {
	case strings.HasPrefix(path, "/"):
		ownPath = true
		target = base + path
	case isAbsoluteURL(path):
		// A step may name an address well outside the catalogue's own host,
		// such as the file server an upstream hands out for its comments. The
		// baseURL is the catalogue's address and does not own those files.
		ownPath = strings.HasPrefix(path, base+"/")
		target = path
	default:
		return nil, s.fail(site, source.ErrInvalidRule,
			fmt.Sprintf("path %q is neither rooted at the baseURL nor absolute", path), nil)
	}
	values := url.Values{}
	for _, pair := range req.Query {
		if value, ok := sc.overrides[pair.Key]; ok {
			// A call may swap a declared value for its own, or drop it, which
			// is how a listing switches between its browse and search intents.
			if value == "" {
				continue
			}
			values.Set(pair.Key, value)
			continue
		}
		if !pair.OmitWhen.IsZero() {
			// A pair may declare itself absent for this call, which is how an
			// exact-match filter stays off until the viewer types something.
			if value, ok := s.eval(pair.OmitWhen, sc); ok && toBool(value) {
				continue
			}
		}
		// Templates render constants as well as variables, so a value is sent
		// exactly as written; only an unresolved or empty hole drops the pair.
		text, resolved := s.render(pair.Value, sc)
		if !resolved || text == "" {
			continue
		}
		values.Add(pair.Key, text)
	}
	pathWithQuery := path
	if encoded := values.Encode(); encoded != "" {
		pathWithQuery += "?" + encoded
		target += "?" + encoded
	}

	// Signing covers the catalogue's own paths. A file server outside it must
	// not receive a signature computed over one of its own paths, so only a
	// path still rooted at the baseURL is signed unless the rule says so.
	wantSign := s.rule.Sign != nil && (req.Sign == nil || *req.Sign) && ownPath

	method := req.Method
	if method == "" {
		method = http.MethodGet
	}
	payload, err := s.encodeBody(req.Body, sc, site)
	if err != nil {
		return nil, err
	}

	headers := map[string]string{}
	for _, header := range s.rule.HTTP.Headers {
		text := expandEnv(header.Value)
		if rendered, ok := s.render(text, sc); ok {
			headers[header.Name] = rendered
		}
	}
	for _, header := range req.Headers {
		text := expandEnv(header.Value)
		if rendered, ok := s.render(text, sc); ok {
			headers[header.Name] = rendered
		}
	}
	if wantSign {
		signed, err := s.signHeaders(site, method, pathWithQuery)
		if err != nil {
			return nil, err
		}
		for name, value := range signed {
			headers[name] = value
		}
	}

	return s.send(ctx, site, method, target, payload, headers)
}

// decodeResponse reads the answer in the shape the request declared. A page
// that renders its payload into a script assignment is read the same way as a
// JSON endpoint, so one rule can browse a category page and search an API.
func decodeResponse(site callSite, body []byte, response string, fail func(callSite, source.ErrorKind, string, error) *source.Error) (any, error) {
	payload := body
	if response == rule.ResponseRouterData {
		extracted, ok := routerData(body)
		if !ok {
			return nil, fail(site, source.ErrInvalidResponse, "response carries no router data", nil)
		}
		payload = extracted
	}
	dec := json.NewDecoder(bytes.NewReader(payload))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, fail(site, source.ErrInvalidResponse, "response is not JSON", err)
	}
	return decoded, nil
}

// routerData pulls the JSON object out of a `_ROUTER_DATA = {...}` assignment.
// The literal appears several times on a page, so the first balanced object that
// also parses as JSON wins.
func routerData(body []byte) ([]byte, bool) {
	const marker = "_ROUTER_DATA"
	rest := body
	for {
		at := bytes.Index(rest, []byte(marker))
		if at < 0 {
			return nil, false
		}
		rest = rest[at+len(marker):]
		// The assignment form is `_ROUTER_DATA = {...}`. Anything else that
		// mentions the name, such as a guard that reads the global, is skipped
		// instead of being mistaken for the payload.
		after := bytes.TrimLeft(rest, " \t\r\n")
		if len(after) == 0 || after[0] != '=' {
			continue
		}
		after = bytes.TrimLeft(after[1:], " \t\r\n")
		if len(after) == 0 || after[0] != '{' {
			continue
		}
		if object, ok := scanObject(after); ok && json.Valid(object) {
			return object, true
		}
	}
}

// scanObject returns the object that starts at the first byte of body. It
// counts braces while stepping over string literals, so a brace inside a title
// does not end the object early.
func scanObject(body []byte) ([]byte, bool) {
	if len(body) == 0 || body[0] != '{' {
		return nil, false
	}
	depth := 0
	inString := false
	escaped := false
	for index := 0; index < len(body); index++ {
		char := body[index]
		if inString {
			switch {
			case escaped:
				escaped = false
			case char == '\\':
				escaped = true
			case char == '"':
				inString = false
			}
			continue
		}
		switch char {
		case '"':
			inString = true
		case '{':
			depth++
		case '}':
			depth--
			if depth == 0 {
				return body[:index+1], true
			}
		}
	}
	return nil, false
}

func (s *sourceImpl) encodeBody(fields map[string]rule.Value, sc scope, site callSite) ([]byte, error) {
	if len(fields) == 0 {
		return nil, nil
	}
	keys := make([]string, 0, len(fields))
	for key := range fields {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	object := make(map[string]any, len(fields))
	for _, key := range keys {
		value, ok := s.eval(fields[key], sc)
		if !ok {
			continue
		}
		object[key] = value
	}
	encoded, err := json.Marshal(object)
	if err != nil {
		return nil, s.fail(site, source.ErrInvalidRule, "request body is not encodable", err)
	}
	return encoded, nil
}

// signHeaders computes the HMAC-SHA256 signature and the headers that carry it.
func (s *sourceImpl) signHeaders(site callSite, method, pathWithQuery string) (map[string]string, error) {
	sign := s.rule.Sign
	now := time.Now()
	timestamp := now.UnixMilli()
	if sign.TimestampUnit == "s" {
		timestamp = now.Unix()
	}
	nonceBytes := make([]byte, sign.NonceBytes)
	if _, err := rand.Read(nonceBytes); err != nil {
		return nil, s.fail(site, source.ErrInvalidRule, "cannot generate nonce", err)
	}
	stamp := strconv.FormatInt(timestamp, 10)
	nonce := hex.EncodeToString(nonceBytes)
	vars := map[string]any{
		"method":    method,
		"path":      pathWithQuery,
		"timestamp": stamp,
		"nonce":     nonce,
	}
	message, ok := s.render(sign.Message, scope{vars: vars})
	if !ok {
		return nil, s.fail(site, source.ErrInvalidRule, "sign.message did not resolve", nil)
	}
	secret, ok := expandEnvStrict(sign.Secret)
	if !ok {
		// Signing with the unexpanded placeholder would reach upstream as a
		// plain signature failure and hide the real cause, so the shipped rule
		// (whose secret lives in ${env:KANJU_SECRET}) stops here instead.
		return nil, s.fail(site, source.ErrUnauthorized,
			fmt.Sprintf("sign.secret %q is not set in the environment", sign.Secret), nil)
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write([]byte(message))
	signature := hex.EncodeToString(mac.Sum(nil))

	headers := make(map[string]string, len(sign.Headers))
	for _, header := range sign.Headers {
		text := expandEnv(header.Value)
		rendered, ok := s.render(text, scope{vars: map[string]any{
			"signature": signature,
			"method":    method,
			"path":      pathWithQuery,
			"timestamp": stamp,
			"nonce":     nonce,
		}})
		if !ok {
			return nil, s.fail(site, source.ErrInvalidRule,
				fmt.Sprintf("sign header %q did not resolve", header.Name), nil)
		}
		headers[header.Name] = rendered
	}
	return headers, nil
}

// send executes the HTTP call with the source's timeout, concurrency limit and
// retry policy, and maps transport and status failures onto error kinds.
func (s *sourceImpl) send(ctx context.Context, site callSite, method, target string, payload []byte, headers map[string]string) ([]byte, error) {
	select {
	case s.sem <- struct{}{}:
		defer func() { <-s.sem }()
	case <-ctx.Done():
		return nil, s.ctxError(site, ctx.Err())
	}

	retryable := method == http.MethodGet || method == http.MethodHead
	attempts := 1
	if retryable {
		attempts += s.retries
	}
	var lastErr error
	for attempt := 0; attempt < attempts; attempt++ {
		if attempt > 0 {
			backoff := time.Duration(200*(1<<uint(attempt-1))) * time.Millisecond
			select {
			case <-time.After(backoff):
			case <-ctx.Done():
				return nil, s.ctxError(site, ctx.Err())
			}
		}
		body, retry, err := s.attempt(ctx, site, method, target, payload, headers)
		if err == nil {
			return body, nil
		}
		lastErr = err
		if !retry {
			return nil, err
		}
	}
	if lastErr == nil {
		lastErr = s.fail(site, source.ErrNetwork, "request failed", nil)
	}
	return nil, lastErr
}

func (s *sourceImpl) attempt(ctx context.Context, site callSite, method, target string, payload []byte, headers map[string]string) ([]byte, bool, error) {
	attemptCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var reader io.Reader
	if len(payload) > 0 {
		reader = bytes.NewReader(payload)
	}
	req, err := http.NewRequestWithContext(attemptCtx, method, target, reader)
	if err != nil {
		return nil, false, s.fail(site, source.ErrInvalidRule, "cannot build request", err)
	}
	if len(payload) > 0 {
		req.Header.Set("Content-Type", "application/json")
	}
	for name, value := range headers {
		req.Header.Set(name, value)
	}
	resp, err := s.client.Do(req)
	if err != nil {
		if ctxErr := attemptCtx.Err(); ctxErr != nil {
			return nil, false, s.ctxError(site, ctxErr)
		}
		return nil, true, s.fail(site, source.ErrNetwork, "request failed", err)
	}
	defer resp.Body.Close()
	body, readErr := io.ReadAll(io.LimitReader(resp.Body, maxResponseBytes))
	if readErr != nil {
		return nil, true, s.fail(site, source.ErrNetwork, "response read failed", readErr)
	}
	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return body, false, nil
	}
	detail := fmt.Sprintf("%s %s returned %d: %s", method, site.step, resp.StatusCode, excerpt(body))
	switch {
	case resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden:
		return nil, false, s.fail(site, source.ErrUnauthorized, detail, nil)
	case resp.StatusCode == http.StatusNotFound:
		return nil, false, s.fail(site, source.ErrNotFound, detail, nil)
	case resp.StatusCode == http.StatusTooManyRequests:
		return nil, false, s.fail(site, source.ErrRateLimited, detail, nil)
	case resp.StatusCode >= 500:
		return nil, true, s.fail(site, source.ErrNetwork, detail, nil)
	default:
		return nil, false, s.fail(site, source.ErrInvalidResponse, detail, nil)
	}
}

const maxResponseBytes = 8 << 20

func (s *sourceImpl) ctxError(site callSite, err error) error {
	if errors.Is(err, context.DeadlineExceeded) {
		return s.fail(site, source.ErrTimeout, "request timed out", err)
	}
	return s.fail(site, source.ErrCanceled, "request canceled", err)
}

func excerpt(body []byte) string {
	text := strings.Join(strings.Fields(string(body)), " ")
	if len(text) > 240 {
		text = text[:240] + "..."
	}
	if text == "" {
		return "(empty body)"
	}
	return text
}

// expandEnv substitutes ${env:NAME} so rules can keep secrets out of the file
// while still declaring where they come from. Missing variables stay literal.
func expandEnv(text string) string {
	expanded, _ := expandEnvStrict(text)
	return expanded
}

// expandEnvStrict is expandEnv for values that must not be sent unresolved. It
// reports false when any ${env:NAME} hole is left open, so a caller can refuse
// the request instead of signing or sending a placeholder.
func expandEnvStrict(text string) (string, bool) {
	const marker = "${env:"
	resolved := true
	for {
		start := strings.Index(text, marker)
		if start < 0 {
			return text, resolved
		}
		end := strings.IndexByte(text[start:], '}')
		if end < 0 {
			return text, resolved
		}
		end += start
		name := text[start+len(marker) : end]
		replacement, ok := os.LookupEnv(name)
		if !ok || name == "" {
			resolved = false
			text = text[:start] + text[end+1:]
			continue
		}
		text = text[:start] + replacement + text[end+1:]
	}
}
