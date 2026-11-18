package rule

import (
	"encoding/json"
	"testing"
)

// TestSelectorKeyedLookups covers the two call-dependent steps: a name in
// brackets takes the position the call supplied, and "[by:field]" keeps the
// array entry whose field equals it.
func TestSelectorKeyedLookups(t *testing.T) {
	root := map[string]any{
		"res": map[string]any{"days": []any{
			map[string]any{"weekday": float64(2), "items": []any{map[string]any{"id": "tue"}}},
			map[string]any{"weekday": float64(4), "items": []any{
				map[string]any{"id": "thu-a"}, map[string]any{"id": "thu-b"},
			}},
		}},
	}

	byIndex, err := ParseSelector("res.days[day].items[*]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if names := byIndex.KeyNames(); len(names) != 1 || names[0] != "day" {
		t.Fatalf("key names = %v", names)
	}
	value, ok := byIndex.WithKeys(map[string]int{"day": 1}).Resolve(root)
	if !ok {
		t.Fatal("the keyed selector did not resolve")
	}
	if items, ok := value.([]any); !ok || len(items) != 2 {
		t.Fatalf("day 1 must yield its two items, got %v", value)
	}
	if _, ok := byIndex.Resolve(root); ok {
		t.Fatal("a keyed selector without a position must not resolve")
	}
	if _, ok := byIndex.WithKeys(map[string]int{"day": 9}).Resolve(root); ok {
		t.Fatal("a position past the end must not resolve")
	}

	byField, err := ParseSelector("res.days[by:weekday].items[*]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if fields := byField.ByFields(); len(fields) != 1 || fields[0] != "weekday" {
		t.Fatalf("lookup fields = %v", fields)
	}
	value, ok = byField.WithValues(map[string]int{"weekday": 4}).Resolve(root)
	if !ok {
		t.Fatal("the field lookup did not resolve")
	}
	if items := value.([]any); len(items) != 2 {
		t.Fatalf("weekday 4 must yield its two items, got %v", items)
	}
	numbers := map[string]any{"res": map[string]any{"days": []any{
		map[string]any{"weekday": json.Number("3"), "items": []any{map[string]any{"id": "only"}}},
	}}}
	value, ok = byField.WithValues(map[string]int{"weekday": 3}).Resolve(numbers)
	if !ok {
		t.Fatal("a scheduled day decoded as a JSON number must match too")
	}
	if items := value.([]any); len(items) != 1 {
		t.Fatalf("want the single item of day 3, got %v", items)
	}
	if _, ok := byField.WithValues(map[string]int{"weekday": 6}).Resolve(root); ok {
		t.Fatal("a weekday the schedule omits must not resolve")
	}
	if got := byField.String(); got != "res.days[by:weekday].items[*]" {
		t.Fatalf("round trip = %q", got)
	}
}

// TestSelectorWildcardCollectsOneListPerEntry pins the flattening rule: a
// wildcard in the middle collects one list per entry, so the result stays a
// list of lists until the mapping spreads it.
func TestSelectorWildcardCollectsOneListPerEntry(t *testing.T) {
	root := map[string]any{"a": []any{
		map[string]any{"b": []any{map[string]any{"id": "one"}}},
		map[string]any{"other": true},
		map[string]any{"b": []any{map[string]any{"id": "two"}, map[string]any{"id": "three"}}},
	}}
	selector, err := ParseSelector("a[*].b[*]")
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	value, ok := selector.Resolve(root)
	if !ok {
		t.Fatal("the wildcard selector did not resolve")
	}
	collected, ok := value.([]any)
	if !ok || len(collected) != 2 {
		t.Fatalf("want one list per matching entry, got %v", value)
	}
	// An entry whose shape lacks the field is skipped rather than failing the
	// whole walk, which is why the result holds two lists and not three.
	if _, isList := collected[1].([]any); !isList {
		t.Fatalf("a collected entry must stay a list, got %T", collected[1])
	}
}

// TestSelectorRejectsMalformedSteps keeps the grammar closed.
func TestSelectorRejectsMalformedSteps(t *testing.T) {
	for _, text := range []string{
		"res[by:]", "res[by:a.b]", "res[]", "res[ ]", "[*].a", "a[*]b", "a..b", "a[1x]",
	} {
		if _, err := ParseSelector(text); err == nil {
			t.Fatalf("ParseSelector(%q) must fail", text)
		}
	}
	for _, text := range []string{
		"a[by:weekday]", "a[day]", "a[*].b[*]", "a[0].b", "a.b",
	} {
		if _, err := ParseSelector(text); err != nil {
			t.Fatalf("ParseSelector(%q): %v", text, err)
		}
	}
}

// TestSelectorScansData covers "from" selectors the rule itself writes, like a
// season summary and a line list, both plain paths and wildcards.
func TestSelectorScansData(t *testing.T) {
	root := map[string]any{"res": map[string]any{
		"sections": []any{map[string]any{"id": "a"}, map[string]any{"id": "b"}},
		"paging":   map[string]any{"total": "12"},
	}}
	value, ok := MustParseSelector(t, "res.sections[*]").Resolve(root)
	if !ok {
		t.Fatal("wildcard did not resolve")
	}
	if items := value.([]any); len(items) != 2 {
		t.Fatalf("want two sections, got %v", items)
	}
	value, ok = MustParseSelector(t, "res.sections[1].id").Resolve(root)
	if !ok || value != "b" {
		t.Fatalf("index did not resolve: %v %v", value, ok)
	}
	if _, ok := MustParseSelector(t, "res.sections[0]").Resolve(map[string]any{"res": map[string]any{}}); ok {
		t.Fatal("a missing field must not resolve")
	}
}

// MustParseSelector parses text in a test or fails it.
func MustParseSelector(t *testing.T, text string) Selector {
	t.Helper()
	selector, err := ParseSelector(text)
	if err != nil {
		t.Fatalf("ParseSelector(%q): %v", text, err)
	}
	return selector
}
