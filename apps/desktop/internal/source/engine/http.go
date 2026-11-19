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
	path, ok := s.render(req.Path, sc)
	if !ok || path == "" {
		return nil, s.fail(site, source.ErrInvalidRule,
			fmt.Sprintf("path %q did not resolve", req.Path), nil)
	}
	if !strings.HasPrefix(path, "/") {
		return nil, s.fail(site, source.ErrInvalidRule,
			fmt.Sprintf("path %q must start with /", path), nil)
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
	target := strings.TrimRight(s.rule.BaseURL, "/") + path
	pathWithQuery := path
	if encoded := values.Encode(); encoded != "" {
		pathWithQuery += "?" + encoded
		target += "?" + encoded
	}

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
	wantSign := s.rule.Sign != nil && (req.Sign == nil || *req.Sign)
	if wantSign {
		signed, err := s.signHeaders(site, method, pathWithQuery)
		if err != nil {
			return nil, err
		}
		for name, value := range signed {
			headers[name] = value
		}
	}

	body, err := s.send(ctx, site, method, target, payload, headers)
	if err != nil {
		return nil, err
	}
	if len(bytes.TrimSpace(body)) == 0 {
		return nil, nil
	}
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	var decoded any
	if err := dec.Decode(&decoded); err != nil {
		return nil, s.fail(site, source.ErrInvalidResponse, "response is not JSON", err)
	}
	return decoded, nil
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
