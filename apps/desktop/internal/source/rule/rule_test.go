package rule

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validRule = `{
	"schemaVersion": 1,
	"id": "sample",
	"name": "Sample",
	"baseURL": "https://sample.test",
	"sign": {
		"secret": "s3cret",
		"message": "{method}\n{path}\n{timestamp}\n{nonce}",
		"headers": [{"name": "x-signature", "value": "{signature}"}]
	},
	"home": {
		"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
		"output": {
			"from": "res.sections[*]",
			"fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}
		}
	}
}`

func decodeString(t *testing.T, text string) *Rule {
	t.Helper()
	parsed, err := Decode([]byte(text))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	return parsed
}

func TestDecodeAppliesSignAndLinesDefaults(t *testing.T) {
	parsed := decodeString(t, validRule)
	if parsed.Sign.TimestampUnit != "ms" || parsed.Sign.NonceBytes != 16 {
		t.Fatalf("sign defaults = %+v", parsed.Sign)
	}
	resolve := decodeString(t, `{
		"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
		"resolve": {
			"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}],
			"lines": {"from": "res.lines[*]", "fields": {"id": "id", "url": "url"}}
		}
	}`)
	if resolve.Resolve.Lines.MaxAttempts != DefaultMaxAttempts {
		t.Fatalf("maxAttempts default = %d", resolve.Resolve.Lines.MaxAttempts)
	}
}

func TestDecodeRejectsMalformedJSON(t *testing.T) {
	cases := []string{
		`{"schemaVersion":1}` + ` {"trailing":true}`,
		`{"schemaVersion": "one"}`,
		`{`,
	}
	for _, text := range cases {
		if _, err := Decode([]byte(text)); err == nil {
			t.Fatalf("Decode(%q) must fail", text)
		}
	}
	if _, err := DecodeFile(filepath.Join(t.TempDir(), "missing.json")); err == nil {
		t.Fatal("DecodeFile of a missing file must fail")
	}
	path := filepath.Join(t.TempDir(), "rule.json")
	if err := os.WriteFile(path, []byte(validRule), 0o644); err != nil {
		t.Fatalf("write: %v", err)
	}
	if _, err := DecodeFile(path); err != nil {
		t.Fatalf("DecodeFile: %v", err)
	}
}

func TestValueJSONRoundTrip(t *testing.T) {
	cases := map[string]string{
		`"title"`:                       "selector",
		`{"fn": "trim", "args": ["a"]}`: "function",
		`{"template": "ep {number}"}`:   "template",
		`{"literal": 7}`:                "literal",
	}
	for raw, wantKind := range cases {
		var parsed Value
		if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
			t.Fatalf("unmarshal %s: %v", raw, err)
		}
		if got := parsed.Kind(); got != wantKind {
			t.Fatalf("%s kind = %q", raw, got)
		}
		if parsed.IsZero() {
			t.Fatalf("%s must not be zero", raw)
		}
		encoded, err := json.Marshal(parsed)
		if err != nil {
			t.Fatalf("marshal %s: %v", raw, err)
		}
		var reparsed Value
		if err := json.Unmarshal(encoded, &reparsed); err != nil {
			t.Fatalf("reparse %s: %v", raw, err)
		}
		if reparsed.Kind() != parsed.Kind() {
			t.Fatalf("round trip %s -> %s (%s)", raw, encoded, reparsed.Kind())
		}
	}
	if !(Value{}).IsZero() || (Value{}).Kind() != "unset" {
		t.Fatal("zero value must report unset")
	}
	for _, raw := range []string{`{"fn": ""}`, `{"template": "{unclosed"}`, `{"literal": null, "fn": "trim"}`, `{"unknown": 1}`, `123`} {
		var parsed Value
		if err := json.Unmarshal([]byte(raw), &parsed); err == nil {
			t.Fatalf("Value must reject %s", raw)
		}
	}
}

func TestValidateRejectsEachBrokenArea(t *testing.T) {
	base := func(body string) string {
		return `{"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test", ` + body + `}`
	}
	cases := map[string]string{
		"schema version": `{"schemaVersion": 2, "id": "sample", "name": "Sample", "baseURL": "https://sample.test"}`,
		"id":             `{"schemaVersion": 1, "id": "not a name", "name": "Sample", "baseURL": "https://sample.test"}`,
		"name":           `{"schemaVersion": 1, "id": "sample", "name": " ", "baseURL": "https://sample.test"}`,
		"baseURL":        base(`"home": null`),
	}
	delete(cases, "baseURL")
	baseURLs := []string{"", "https://", "https://x.test/?a=1", "https://x.test/#frag", "ftp://x.test"}
	for _, raw := range baseURLs {
		text := `{"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "` + raw + `", "home": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}}}`
		if err := decodeString(t, text).Validate(); err == nil {
			t.Fatalf("baseURL %q must be rejected", raw)
		}
	}

	steps := `"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
		"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}`
	cases = map[string]string{
		"schema version":   `{"schemaVersion": 2, "id": "sample", "name": "Sample", "baseURL": "https://sample.test"}`,
		"id":               `{"schemaVersion": 1, "id": "not a name", "name": "Sample", "baseURL": "https://sample.test"}`,
		"name":             `{"schemaVersion": 1, "id": "sample", "name": " ", "baseURL": "https://sample.test"}`,
		"no blocks":        base(`"schemaVersion": 1`)[:0] + `{"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test"}`,
		"sign message":     base(`"sign": {"secret": "s", "message": "{bogus}", "headers": [{"name": "a", "value": "b"}]}, "home": {` + steps + `}`),
		"sign secret":      base(`"sign": {"message": "{method}{path}{timestamp}{nonce}", "headers": [{"name": "a", "value": "b"}]}, "home": {` + steps + `}`),
		"sign header":      base(`"sign": {"secret": "s", "message": "{method}{path}{timestamp}{nonce}", "headers": [{"name": "a", "value": "{bogus}"}]}, "home": {` + steps + `}`),
		"sign headers":     base(`"sign": {"secret": "s", "message": "{method}{path}{timestamp}{nonce}"}, "home": {` + steps + `}`),
		"sign unit":        base(`"sign": {"secret": "s", "message": "{method}{path}{timestamp}{nonce}", "timestampUnit": "min", "headers": [{"name": "a", "value": "b"}]}, "home": {` + steps + `}`),
		"sign nonce":       base(`"sign": {"secret": "s", "message": "{method}{path}{timestamp}{nonce}", "nonceBytes": 65, "headers": [{"name": "a", "value": "b"}]}, "home": {` + steps + `}`),
		"kind value":       base(`"list": {"kindValues": {"cartoon": "x"}, ` + steps + `}`),
		"list fields":      base(`"list": {"steps": [{"op": "request", "as": "res", "path": "/v1/list"}], "output": {"from": "res", "fields": {"id": "id"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"list no cards":    base(`"list": {"steps": [{"op": "request", "as": "res", "path": "/v1/list"}], "output": {"from": "res"}}`),
		"detail cards":     base(`"detail": {"steps": [{"op": "request", "as": "res", "path": "/v1/d/{id}"}], "output": {"from": "res", "fields": {"title": "title"}, "cards": {"from": "a[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"detail title":     base(`"detail": {"steps": [{"op": "request", "as": "res", "path": "/v1/d/{id}"}], "output": {"from": "res", "fields": {"id": "id"}}}`),
		"output from":      base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "nope.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"field name":       base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"bogus": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"section title":    base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"section page":     base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "page": "res.page", "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"shape fields":     base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id"}}}}`),
		"step op":          base(`"home": {"steps": [{"op": "jump", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"step as":          base(`"home": {"steps": [{"op": "request", "as": "not name", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"rebound":          base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}, {"op": "request", "as": "res", "path": "/v1/more"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"no steps":         base(`"home": {"steps": [], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"method":           base(`"home": {"steps": [{"op": "request", "as": "res", "method": "TRACE", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"path":             base(`"home": {"steps": [{"op": "request", "as": "res", "path": "v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"query key":        base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "query": [{"key": "", "value": "x"}]}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"query value":      base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "query": [{"key": "q", "value": ""}]}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"query var":        base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "query": [{"key": "q", "value": "{bogus}"}]}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"header var":       base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "headers": [{"name": "x", "value": "{bogus}"}]}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"body var":         base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "body": {"q": "bogus"}}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"paginate mode":    base(`"episodes": {"steps": [{"op": "paginate", "as": "pages", "mode": "cursor", "size": 1, "maxPages": 1, "items": "list[*]", "path": "/v1/e/{id}"}], "output": {"from": "pages[*]", "fields": {"title": "title"}}}`),
		"paginate items":   base(`"episodes": {"steps": [{"op": "paginate", "as": "pages", "mode": "page", "size": 1, "maxPages": 1, "path": "/v1/e/{id}"}], "output": {"from": "pages[*]", "fields": {"title": "title"}}}`),
		"episode field":    base(`"episodes": {"steps": [{"op": "paginate", "as": "pages", "mode": "page", "size": 1, "maxPages": 1, "items": "list[*]", "path": "/v1/e/{id}"}], "output": {"from": "pages[*]", "fields": {"url": "url"}}}`),
		"resolve no url":   base(`"resolve": {"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}], "lines": {"from": "res.lines[*]", "fields": {"id": "id"}}}`),
		"resolve id":       base(`"resolve": {"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}], "lines": {"from": "res.lines[*]", "fields": {"url": "url"}}}`),
		"resolve prefer":   base(`"resolve": {"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}], "lines": {"from": "res.lines[*]", "fields": {"id": "id", "url": "url"}, "preferUrlKind": "flv"}}`),
		"resolve attempts": base(`"resolve": {"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}], "lines": {"from": "res.lines[*]", "fields": {"id": "id", "url": "url"}, "maxAttempts": -1}}`),
		"exchange url":     base(`"resolve": {"steps": [{"op": "request", "as": "res", "path": "/play/{payload}"}], "lines": {"from": "res.lines[*]", "fields": {"id": "id", "url": "url"}, "exchange": {"ticket": "ticket", "request": {"path": "/line", "body": {"ticket": "ticket"}}}}}`),
		"response":         base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "response": "xml"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"when var":         base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home", "when": {"template": "{bogus}"}}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"from candidate":   base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]|", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]", "fields": {"id": "id", "title": "t"}}}}`),
		"cards candidate":  base(`"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}], "output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"}, "cards": {"from": "cards[*]|", "fields": {"id": "id", "title": "t"}}}}`),
	}
	for name, text := range cases {
		parsed, err := Decode([]byte(text))
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if err := parsed.Validate(); err == nil {
			t.Fatalf("%s: validation must fail", name)
		} else if !strings.Contains(err.Error(), "") {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if err := decodeString(t, validRule).Validate(); err != nil {
		t.Fatalf("valid rule: %v", err)
	}
}

// TestValidateChecksTheWindowBlocks covers the pair a per-window source needs.
// A segments block alone has no manifest to read windows from, and a manifest
// alone is a request whose answer nobody reads, so both are rule errors rather
// than a source that quietly has no comments.
func TestValidateChecksTheWindowBlocks(t *testing.T) {
	const (
		manifest = `"manifest": {"steps": [{"op": "request", "as": "source", "path": "/v1/c/{id}"}],
			"output": {"from": "source", "fields": {"source": "source.url"}}}`
		windows = `"output": {"from": "document.items[*]", "fields": {"at": "0", "text": "4"}},
			"list": {"from": "document.segments[*]", "url": "url"}`
	)
	cases := map[string]string{
		"segments alone":  `"segments": {` + windows + `}`,
		"manifest alone":  manifest,
		"manifest fields": strings.Replace(manifest, `"fields": {"source": "source.url"}`, `"fields": {"url": "source.url"}`, 1),
		"manifest var":    strings.Replace(manifest, `"source.url"`, `"{bogus}"`, 1),
		"list negative":   `"segments": {"steps": [], ` + strings.Replace(windows, `"url": "url"`, `"url": "url", "maxSegments": -1`, 1) + `}, ` + manifest,
		"list selector":   `"segments": {"steps": [], ` + strings.Replace(windows, `"document.segments[*]"`, `"not a selector!"`, 1) + `}, ` + manifest,
		"list url":        `"segments": {"steps": [], ` + strings.Replace(windows, `"url": "url"`, `"url": ""`, 1) + `}, ` + manifest,
		"episodes list":   `"episodes": {"list": {"from": "a[*]", "url": "url"}, "steps": [{"op": "paginate", "as": "pages", "mode": "page", "size": 1, "maxPages": 1, "items": "list[*]", "path": "/v1/e/{id}"}], "output": {"from": "pages[*]", "fields": {"title": "title"}}}`,
	}
	for name, body := range cases {
		t.Run(name, func(t *testing.T) {
			text := `{"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test", ` + body + `}`
			// Decoding reads the selectors a block holds, so a broken one is
			// refused there; the rest is caught by validation. Either way the
			// rule must not load.
			parsed, err := Decode([]byte(text))
			if err == nil {
				err = parsed.Validate()
			}
			if err == nil {
				t.Fatal("the rule must not load")
			}
		})
	}
	t.Run("a complete pair", func(t *testing.T) {
		text := `{"schemaVersion": 1, "id": "sample", "name": "Sample", "baseURL": "https://sample.test",
			` + manifest + `, "segments": {"steps": [], ` + windows + `}}`
		parsed, err := Decode([]byte(text))
		if err != nil {
			t.Fatalf("decode: %v", err)
		}
		if err := parsed.Validate(); err != nil {
			t.Fatalf("a manifest and a segments block must validate: %v", err)
		}
	})
}

func TestValidateChecksRetryWhen(t *testing.T) {
	home := `"home": {"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
		"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
			"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}}`
	cases := map[string]struct {
		retryWhen string
		ok        bool
	}{
		"a marker":            {`[{"status": 503, "contains": "project_auth_unavailable"}]`, true},
		"an empty body":       {`[{"status": 200, "empty": true}]`, true},
		"any status":          {`[{"contains": "try later"}]`, true},
		"nothing to match":    {`[{}]`, false},
		"status out of range": {`[{"status": 700, "contains": "x"}]`, false},
		"status zero is fine": {`[{"status": 0, "empty": true}]`, true},
	}
	for name, testCase := range cases {
		text := `{"schemaVersion": 1, "id": "sample", "name": "Sample", ` +
			`"baseURL": "https://sample.test", "http": {"retryWhen": ` + testCase.retryWhen + `}, ` + home + `}`
		parsed, err := Decode([]byte(text))
		if err == nil {
			err = parsed.Validate()
		}
		if testCase.ok && err != nil {
			t.Fatalf("%s: want it to load, got %v", name, err)
		}
		if !testCase.ok && err == nil {
			t.Fatalf("%s: want it rejected", name)
		}
	}
}
