package engine

import (
	"encoding/json"
	"testing"

	"luvia/apps/desktop/internal/source/rule"
)

func value(t *testing.T, raw string) rule.Value {
	t.Helper()
	var parsed rule.Value
	if err := json.Unmarshal([]byte(raw), &parsed); err != nil {
		t.Fatalf("decode value %s: %v", raw, err)
	}
	return parsed
}

func TestCallFunctionTable(t *testing.T) {
	all := func(values ...any) ([]any, []bool) {
		found := make([]bool, len(values))
		for index := range found {
			found[index] = true
		}
		return values, found
	}
	cases := []struct {
		name  string
		args  []any
		found []bool
		want  any
		ok    bool
	}{
		{"coalesce", []any{"", nil, "picked"}, []bool{true, true, true}, "picked", true},
		{"coalesce", []any{"", nil}, []bool{true, true}, nil, false},
		{"coalesce", []any{"missing", nil}, []bool{false, true}, nil, false},
		{"trimPrefix", []any{"resolve://ticket", "resolve://"}, []bool{true, true}, "ticket", true},
		{"trimPrefix", []any{"x"}, []bool{true}, nil, false},
		{"trimSuffix", []any{"ep.m3u8", ".m3u8"}, []bool{true, true}, "ep", true},
		{"trimSuffix", []any{"x"}, []bool{true}, nil, false},
		{"trim", []any{"  padded  "}, []bool{true}, "padded", true},
		{"trim", []any{"x"}, []bool{false}, nil, false},
		{"lower", []any{"ABC"}, []bool{true}, "abc", true},
		{"upper", []any{"abc"}, []bool{true}, "ABC", true},
		{"int", []any{json.Number("7")}, []bool{true}, 7, true},
		{"int", []any{"nope"}, []bool{true}, nil, false},
		{"int", nil, nil, nil, false},
		{"kind", []any{"documentary"}, []bool{true}, "movie", true},
		{"kind", []any{"x"}, []bool{false}, nil, false},
		{"urlKind", []any{"https://cdn.test/a.m3u8"}, []bool{true}, "hls", true},
		{"urlKind", nil, nil, nil, false},
		{"list", []any{[]any{"a", "", "b"}}, []bool{true}, []string{"a", "b"}, true},
		{"list", []any{"solo"}, []bool{true}, []string{"solo"}, true},
		{"list", []any{"x"}, []bool{false}, nil, false},
		{"first", []any{[]any{"a", "b"}}, []bool{true}, "a", true},
		{"first", []any{[]any{}}, []bool{true}, nil, false},
		{"last", []any{[]any{"a", "b"}}, []bool{true}, "b", true},
		{"last", nil, nil, nil, false},
		{"unknown", []any{"a"}, []bool{true}, nil, false},
	}
	for _, testCase := range cases {
		args, found := testCase.args, testCase.found
		if args == nil {
			args, found = all()
		}
		got, ok := callFunction(testCase.name, args, found)
		if ok != testCase.ok {
			t.Fatalf("%s%v: ok = %v, want %v", testCase.name, testCase.args, ok, testCase.ok)
		}
		if !ok {
			continue
		}
		if stringOf(got) != stringOf(testCase.want) || kindOfValue(got) != kindOfValue(testCase.want) {
			t.Fatalf("%s%v: got %#v, want %#v", testCase.name, testCase.args, got, testCase.want)
		}
	}
}

func stringOf(value any) string {
	switch item := value.(type) {
	case []string:
		return "[]" + joinStrings(item)
	case nil:
		return "nil"
	default:
		return toString(item)
	}
}

func joinStrings(items []string) string {
	out := ""
	for index, item := range items {
		if index > 0 {
			out += ","
		}
		out += item
	}
	return out
}

func kindOfValue(value any) string {
	switch value.(type) {
	case nil:
		return "nil"
	case int:
		return "int"
	case string:
		return "string"
	case []string:
		return "list"
	default:
		return "other"
	}
}

func TestScalarConversion(t *testing.T) {
	for _, testCase := range []struct {
		in   any
		want string
	}{
		{json.Number("42"), "42"},
		{float64(1.5), "1.5"},
		{int(3), "3"},
		{int64(4), "4"},
		{true, "true"},
		{"text", "text"},
		{nil, ""},
	} {
		if got := toString(testCase.in); got != testCase.want {
			t.Fatalf("toString(%#v) = %q", testCase.in, got)
		}
	}

	for _, testCase := range []struct {
		in     any
		want   int
		wanted bool
	}{
		{json.Number("12"), 12, true},
		{json.Number("1.5"), 0, false},
		{float64(2.9), 2, true},
		{3, 3, true},
		{" 5 ", 5, true},
		{"x", 0, false},
		{true, 0, false},
	} {
		got, ok := toInt(testCase.in)
		if ok != testCase.wanted {
			t.Fatalf("toInt(%#v) ok = %v", testCase.in, ok)
		}
		if ok && got != testCase.want {
			t.Fatalf("toInt(%#v) = %v", testCase.in, got)
		}
	}

	for _, testCase := range []struct {
		in   any
		want bool
	}{
		{true, true},
		{"YES", true},
		{"no", false},
		{json.Number("1"), true},
		{json.Number("0"), false},
		{json.Number("x"), false},
		{2.5, true},
		{0, false},
		{nil, false},
	} {
		if got := toBool(testCase.in); got != testCase.want {
			t.Fatalf("toBool(%#v) = %v", testCase.in, got)
		}
	}

	if got := toStrings([]any{"a", "", 2}); len(got) != 2 || got[1] != "2" {
		t.Fatalf("toStrings list = %v", got)
	}
	if got := toStrings([]string{"a"}); len(got) != 1 || got[0] != "a" {
		t.Fatalf("toStrings fixed = %v", got)
	}
	if got := toStrings(nil); got != nil {
		t.Fatalf("toStrings nil = %v", got)
	}
	if got := toStrings(""); got != nil {
		t.Fatalf("toStrings empty = %v", got)
	}
	if got := toStrings(9); len(got) != 1 || got[0] != "9" {
		t.Fatalf("toStrings scalar = %v", got)
	}
}

func TestKindAndURLKindMapping(t *testing.T) {
	kinds := map[string]string{
		"movie": "movie", "FILM": "movie", "documentary": "movie", "doc": "movie",
		"anime": "anime", "animation": "anime", "donghua": "anime", "cartoon": "anime",
		"variety": "variety", "sports": "variety", "reality": "variety", "show": "variety",
		"series": "series", "tv": "series", "drama": "series", "short_drama": "series",
		"shortdrama": "series", "unknown": "series", "  ": "series",
	}
	for raw, want := range kinds {
		if got := mapKind(raw); got != want {
			t.Fatalf("mapKind(%q) = %q, want %q", raw, got, want)
		}
	}
	urlKinds := map[string]string{
		"": "mp4", "HLS": "hls", "m3u8": "hls", "https://cdn.test/a.m3u8": "hls",
		"https://cdn.test/a.mp4": "mp4", "https://cdn.test/a": "mp4",
	}
	for raw, want := range urlKinds {
		if got := inferURLKind(raw); got != want {
			t.Fatalf("inferURLKind(%q) = %q, want %q", raw, got, want)
		}
	}
}

func TestEvalPrefersItemThenVars(t *testing.T) {
	impl := &sourceImpl{}
	sc := scope{
		item: map[string]any{"title": "from item"},
		vars: map[string]any{"res": map[string]any{"title": "from vars"}, "shared": "vars"},
	}
	if got, ok := impl.eval(value(t, `"title"`), sc); !ok || got != "from item" {
		t.Fatalf("item selector = %#v (%v)", got, ok)
	}
	if got, ok := impl.eval(value(t, `"res.title"`), sc); !ok || got != "from vars" {
		t.Fatalf("vars selector = %#v (%v)", got, ok)
	}
	if got, ok := impl.eval(value(t, `{"literal": 5}`), sc); !ok || toString(got) != "5" {
		t.Fatalf("literal = %#v (%v)", got, ok)
	}
	if got, ok := impl.eval(value(t, `{"template": "{shared}-{res.title}"}`), sc); !ok || got != "vars-from vars" {
		t.Fatalf("template = %#v (%v)", got, ok)
	}
	if _, ok := impl.eval(value(t, `{"template": "{missing}"}`), sc); ok {
		t.Fatal("template with a missing hole must not resolve")
	}
	if got, ok := impl.eval(value(t, `{"fn": "upper", "args": ["shared"]}`), sc); !ok || got != "VARS" {
		t.Fatalf("function = %#v (%v)", got, ok)
	}
	if _, ok := impl.eval(rule.Value{}, sc); ok {
		t.Fatal("an unset value must not resolve")
	}
	if got := impl.evalString(value(t, `{"fn": "int", "args": ["res.missing"]}`), sc); got != "" {
		t.Fatalf("evalString of a failed call = %q", got)
	}
	if got, ok := impl.render("{shared}", sc); !ok || got != "vars" {
		t.Fatalf("render = %q (%v)", got, ok)
	}
	if _, ok := impl.render("{unclosed", sc); ok {
		t.Fatal("render of a broken template must fail")
	}
}
