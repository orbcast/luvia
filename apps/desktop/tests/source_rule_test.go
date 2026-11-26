package tests

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/loader"
	"luvia/apps/desktop/internal/source/rule"
)

func TestSelectorFormsAndResolution(t *testing.T) {
	valid := []string{"id", "res.sections[*]", "res.list[0].title", "res.paging.total",
		"res.days[*].groups.preview[*]", "res.days[weekday].preview[*]", "res.days[by:weekday].preview[*]"}
	for _, text := range valid {
		selector, err := rule.ParseSelector(text)
		if err != nil {
			t.Fatalf("ParseSelector(%q): %v", text, err)
		}
		if selector.String() != text {
			t.Fatalf("round trip %q -> %q", text, selector.String())
		}
	}
	for _, text := range []string{"", "[*]", "res.", "res..id", "res[1abc]", "res[by:]"} {
		if _, err := rule.ParseSelector(text); err == nil {
			t.Fatalf("ParseSelector(%q) must fail", text)
		}
	}

	selector, err := rule.ParseSelector("res.list[0].title")
	if err != nil {
		t.Fatalf("unexpected parse failure: %v", err)
	}
	if got := selector.Root(); got != "res" {
		t.Fatalf("root = %q", got)
	}

	root := map[string]any{
		"res": map[string]any{
			"list": []any{
				map[string]any{"title": "first"},
				map[string]any{"title": "second"},
			},
			"paging": map[string]any{"total": "42"},
		},
	}
	indexed, err := rule.ParseSelector("res.list[1].title")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if value, ok := indexed.Resolve(root); !ok || value != "second" {
		t.Fatalf("indexed resolve = %v (%v)", value, ok)
	}
	outOfRange, err := rule.ParseSelector("res.list[9].title")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if _, ok := outOfRange.Resolve(root); ok {
		t.Fatal("out of range index must not resolve")
	}
	nested, err := rule.ParseSelector("res.list[*]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	items, ok := nested.Resolve(root)
	if !ok {
		t.Fatal("wildcard must resolve")
	}
	if list, isList := items.([]any); !isList || len(list) != 2 {
		t.Fatalf("wildcard value = %v", items)
	}
}

func TestTemplateRender(t *testing.T) {
	template, err := rule.ParseTemplate("https://cdn.test/{id}/play?t={token}")
	if err != nil {
		t.Fatalf("parse template: %v", err)
	}
	if len(template.Selectors()) != 2 {
		t.Fatalf("selectors = %v", template.Selectors())
	}
	values := map[string]string{"id": "42", "token": "abc"}
	rendered, ok := template.Render(func(selector rule.Selector) (string, bool) {
		value, found := values[selector.Root()]
		return value, found
	})
	if !ok || rendered != "https://cdn.test/42/play?t=abc" {
		t.Fatalf("render = %q (%v)", rendered, ok)
	}
	if _, ok := template.Render(func(rule.Selector) (string, bool) { return "", false }); ok {
		t.Fatal("missing placeholder must fail the render")
	}
	for _, text := range []string{"{}", "{id", "id}"} {
		if _, err := rule.ParseTemplate(text); err == nil {
			t.Fatalf("ParseTemplate(%q) must fail", text)
		}
	}
}

func TestRuleDecodeAppliesDefaultsAndRejectsUnknownFields(t *testing.T) {
	parsed, err := rule.Decode([]byte(minimalRule("good", "Good")))
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if parsed.HTTP.TimeoutMs != rule.DefaultTimeoutMs || parsed.HTTP.Concurrency != rule.DefaultConcurrency {
		t.Fatalf("http defaults = %+v", parsed.HTTP)
	}
	if parsed.HTTP.Retries == nil || *parsed.HTTP.Retries != rule.DefaultRetries {
		t.Fatalf("retry default = %v", parsed.HTTP.Retries)
	}

	if _, err := rule.Decode([]byte(`{"schemaVersion":1,"id":"x","name":"X","baseURL":"https://x.test","bogus":1}`)); err == nil {
		t.Fatal("unknown fields must be rejected")
	}
	if _, err := rule.Decode([]byte(`{"schemaVersion":1}`)); err != nil {
		// Decoding is separate from validation; this only proves the shape is read.
		t.Fatalf("decode minimal object: %v", err)
	} else if err := (&rule.Rule{SchemaVersion: 1}).Validate(); err == nil {
		t.Fatal("validation must reject an empty rule")
	}
}

func TestRuleValidationRejectsBrokenRules(t *testing.T) {
	cases := map[string]string{
		"unknown path variable": `{
			"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test",
			"detail": {
				"steps": [{"op": "request", "as": "res", "path": "/v1/{nope}"}],
				"output": {"from": "res", "fields": {"title": "title"}}
			}
		}`,
		"unknown function": `{
			"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test",
			"home": {
				"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
				"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": {"fn": "bogus"}},
					"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
			}
		}`,
		"paginate without items": `{
			"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test",
			"episodes": {
				"steps": [{"op": "paginate", "as": "pages", "mode": "offset", "size": 10, "maxPages": 2,
					"path": "/v1/episodes/{id}"}],
				"output": {"from": "pages[*]", "fields": {"title": "title"}}
			}
		}`,
		"unknown line field": `{
			"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test",
			"resolve": {
				"steps": [{"op": "request", "as": "res", "path": "/v1/playback/{payload}"}],
				"lines": {"from": "res.lines[*]", "fields": {"id": "id", "url": "url", "bogus": "x"}}
			}
		}`,
		"missing line url": `{
			"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test",
			"resolve": {
				"steps": [{"op": "request", "as": "res", "path": "/v1/playback/{payload}"}],
				"lines": {"from": "res.lines[*]", "fields": {"id": "id"}}
			}
		}`,
		"bad base url":   `{"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "ftp://x.test"}`,
		"missing blocks": `{"schemaVersion": 1, "id": "x", "name": "X", "baseURL": "https://x.test"}`,
	}
	for name, text := range cases {
		parsed, err := rule.Decode([]byte(text))
		if err != nil {
			t.Fatalf("%s: decode: %v", name, err)
		}
		if err := parsed.Validate(); err == nil {
			t.Fatalf("%s: validation must fail", name)
		}
	}
}

func TestLoaderIsolatesBrokenRules(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "good", minimalRule("good", "Good"))
	writeRule(t, dir, "badid", minimalRule("other", "Other"))
	writeRule(t, dir, "broken", "{not json")
	writeRule(t, dir, "duplicate", minimalRule("good", "Duplicate"))
	if err := os.MkdirAll(filepath.Join(dir, "empty"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "_ignored"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	writeRule(t, dir, "_ignored", "{not json")

	set, err := loader.Load(dir, engine.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sources := set.Sources()
	if len(sources) != 1 || sources[0].ID() != "good" {
		t.Fatalf("sources = %v", sources)
	}
	diagnostics := set.Diagnostics()
	if len(diagnostics) != 4 {
		t.Fatalf("want 4 diagnostics, got %d: %v", len(diagnostics), diagnostics)
	}
	wantOrder := []string{"badid", "broken", "duplicate", "empty"}
	for index, diagnostic := range diagnostics {
		if !strings.Contains(diagnostic.Path, wantOrder[index]) {
			t.Fatalf("diagnostic %d path = %q, want %q", index, diagnostic.Path, wantOrder[index])
		}
		if diagnostic.Error() == "" {
			t.Fatalf("diagnostic %d has no message", index)
		}
	}
	if !strings.Contains(diagnostics[2].Error(), "duplicate") {
		t.Fatalf("duplicate diagnostic = %v", diagnostics[2])
	}
	if !strings.Contains(diagnostics[3].Error(), "missing rule.json") {
		t.Fatalf("missing rule diagnostic = %v", diagnostics[3])
	}

	if _, ok := set.Get("good"); !ok {
		t.Fatal("Get must find the loaded source")
	}
	if _, ok := set.Get("other"); ok {
		t.Fatal("a skipped rule must not be reachable")
	}

	if err := os.RemoveAll(filepath.Join(dir, "broken")); err != nil {
		t.Fatalf("remove: %v", err)
	}
	reloaded, err := set.Reload()
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if len(reloaded.Diagnostics()) != 3 {
		t.Fatalf("reload diagnostics = %v", reloaded.Diagnostics())
	}
	if len(set.Diagnostics()) != 4 {
		t.Fatal("Reload must not mutate the previous snapshot")
	}
}

func TestLoaderRejectsMissingDirectory(t *testing.T) {
	if _, err := loader.Load(filepath.Join(t.TempDir(), "missing"), engine.Options{}); err == nil {
		t.Fatal("loading a missing directory must fail")
	}
}

func TestLoaderDefaultDirPriority(t *testing.T) {
	userDir := t.TempDir()
	bundledDir := filepath.Join(t.TempDir(), "bundled")

	t.Setenv(loader.EnvDir, "")
	if got := loader.DefaultDir(userDir, bundledDir); got != bundledDir {
		t.Fatalf("empty user dir must fall back to bundled, got %q", got)
	}
	if err := os.Mkdir(filepath.Join(userDir, "kanju"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if err := os.Mkdir(filepath.Join(userDir, "_draft"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if got := loader.DefaultDir(filepath.Join(t.TempDir(), "fresh"), bundledDir); got != bundledDir {
		t.Fatalf("ignored directories must not count as rules, got %q", got)
	}
	if got := loader.DefaultDir(userDir, bundledDir); got != userDir {
		t.Fatalf("user rules must win over bundled, got %q", got)
	}

	override := filepath.Join(t.TempDir(), "override")
	t.Setenv(loader.EnvDir, override)
	if got := loader.DefaultDir(userDir, bundledDir); got != override {
		t.Fatalf("environment override must win, got %q", got)
	}
}

func TestLoaderLoadsCheckedInRules(t *testing.T) {
	set, err := loader.Load(rulesDir, engine.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if diagnostics := set.Diagnostics(); len(diagnostics) != 0 {
		t.Fatalf("checked-in rules must load cleanly: %v", diagnostics)
	}
	sources := set.Sources()
	// Every checked-in rule must load, so the assertion names all of them
	// instead of only counting: a rule that silently stops loading shows up as
	// a shorter list here.
	want := []string{"hongguo", "kanju"}
	if len(sources) != len(want) {
		t.Fatalf("want %d sources, got %d: %v", len(want), len(sources), sources)
	}
	for index, id := range want {
		if sources[index].ID() != id {
			t.Fatalf("source %d = %q, want %q", index, sources[index].ID(), id)
		}
		if sources[index].Name() == "" {
			t.Fatalf("source %q has no name", id)
		}
	}
	if set.Dir() != rulesDir {
		t.Fatalf("dir = %q", set.Dir())
	}
}

// minimalRule is the smallest structurally valid rule: one home block with one
// request and a section mapping.
func minimalRule(id, name string) string {
	return `{
		"schemaVersion": 1,
		"id": "` + id + `",
		"name": "` + name + `",
		"baseURL": "https://` + id + `.test",
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/home"}],
			"output": {
				"from": "res.sections[*]",
				"fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}
			}
		}
	}`
}

// writeRule writes dir/name/rule.json.
func writeRule(t *testing.T, dir, name, content string) {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.MkdirAll(path, 0o755); err != nil {
		t.Fatalf("mkdir %s: %v", path, err)
	}
	if err := os.WriteFile(filepath.Join(path, loader.RuleFile), []byte(content), 0o644); err != nil {
		t.Fatalf("write rule %s: %v", name, err)
	}
}

// TestLoaderListsAccessKeys covers the variables the settings page offers a
// field for: every ${env:NAME} a rule sends, wherever it writes one, sorted
// and without duplicates.
func TestLoaderListsAccessKeys(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "signed", `{
		"schemaVersion": 1, "id": "signed", "name": "Signed", "baseURL": "https://signed.test",
		"http": {"headers": [{"name": "x-extra", "value": "${env:FROM_HTTP}"}]},
		"sign": {
			"secret": "${env:KANJU_SECRET}",
			"message": "{method}\n{path}\n{timestamp}\n{nonce}",
			"headers": [{"name": "x-signature", "value": "{signature}"}]
		},
		"home": {
			"steps": [{
				"op": "request", "as": "res", "path": "/v1/home",
				"query": [{"key": "k", "value": "static"}]
			}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
		},
		"resolve": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/playback/{payload}"}],
			"lines": {
				"from": "res.lines[*]", "fields": {"id": "id", "url": "url"},
				"exchange": {"ticket": "res.ticket", "url": "res.url", "urlKind": {"literal": "mp4"},
					"request": {"method": "POST", "path": "/v1/exchange"}}
			}
		}
	}`)
	writeRule(t, dir, "plain", minimalRule("plain", "Plain"))
	set, err := loader.Load(dir, engine.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	want := []string{"FROM_HTTP", "KANJU_SECRET"}
	got := set.SecretNames()
	if len(got) != len(want) {
		t.Fatalf("names = %v, want %v", got, want)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Fatalf("names = %v, want %v", got, want)
		}
	}
}

// TestVariablesCannotHideInTemplates pins why SecretNames only reads plain
// string fields: a template field rejects ${env:X} at load time, so the
// settings page cannot miss a variable written in one.
func TestVariablesCannotHideInTemplates(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "hidden", `{
		"schemaVersion": 1, "id": "hidden", "name": "Hidden", "baseURL": "https://hidden.test",
		"home": {
			"steps": [{"op": "request", "as": "res", "path": "/v1/home",
				"headers": [{"name": "x-token", "value": "${env:FROM_HEADER}"}]}],
			"output": {"from": "res.sections[*]", "fields": {"id": "id", "title": "title"},
				"cards": {"from": "cards[*]", "fields": {"id": "id", "title": "title"}}}
		}
	}`)
	set, err := loader.Load(dir, engine.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if len(set.Sources()) != 0 || len(set.Diagnostics()) != 1 {
		t.Fatalf("sources = %v, diagnostics = %v", set.Sources(), set.Diagnostics())
	}
	if names := set.SecretNames(); len(names) != 0 {
		t.Fatalf("names = %v, want none", names)
	}
}

// TestLoaderReportsNoAccessKeysWithoutPlaceholders keeps a rule that writes
// its own constants from growing an empty section in the settings page.
func TestLoaderReportsNoAccessKeysWithoutPlaceholders(t *testing.T) {
	dir := t.TempDir()
	writeRule(t, dir, "plain", minimalRule("plain", "Plain"))
	set, err := loader.Load(dir, engine.Options{})
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if names := set.SecretNames(); len(names) != 0 {
		t.Fatalf("names = %v, want none", names)
	}
}
