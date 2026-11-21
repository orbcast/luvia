package rule

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"
)

// Selector is a restricted path through decoded JSON: dot-separated field
// names, numeric indexes and wildcards, which flatten the array they meet and
// keep walking. There is deliberately no expression language; anything more
// belongs in a value function.
type Selector struct {
	segments []segment
	// keys holds the call variables a "[name]" step reads its position from.
	keys map[string]int
	// values holds the call variables a "[by:field]" step matches a field
	// against, such as the weekday a schedule should show.
	values map[string]int
}

// WithKeys returns the selector resolved against call values: every "[name]"
// step takes its position from keys[name], so one rule can read the entry a
// call asked for instead of a fixed one.
func (s Selector) WithKeys(keys map[string]int) Selector {
	copied := s
	copied.keys = keys
	return copied
}

// WithValues returns the selector matched against call values: every
// "[by:field]" step keeps the array entry whose field equals the value bound
// to that field's name.
func (s Selector) WithValues(values map[string]int) Selector {
	copied := s
	copied.values = values
	return copied
}

// ByFields lists the fields a "[by:field]" step matches on.
func (s Selector) ByFields() []string {
	var fields []string
	for _, item := range s.segments {
		if item.kind == segBy {
			fields = append(fields, item.name)
		}
	}
	return fields
}

// KeyNames lists the call variables the selector reads for its index steps.
func (s Selector) KeyNames() []string {
	var names []string
	for _, item := range s.segments {
		if item.kind == segKey {
			names = append(names, item.name)
		}
	}
	return names
}

type segmentKind int

const (
	segField segmentKind = iota
	segIndex
	segAll
	segKey
	segBy
	// segQuoted steps to a field whose name a dot cannot carry, such as a
	// server-rendered route key like "player_(series_id)/(vid)/page".
	segQuoted
)

type segment struct {
	kind  segmentKind
	name  string
	index int
}

// ParseSelector parses a selector such as "res.cards[*]" or "pagination.page".
func ParseSelector(text string) (Selector, error) {
	trimmed := strings.TrimSpace(text)
	if trimmed == "" {
		return Selector{}, fmt.Errorf("selector is empty")
	}
	var segments []segment
	for pos := 0; pos < len(trimmed); {
		start := pos
		for pos < len(trimmed) && isIdentifierByte(trimmed[pos]) {
			pos++
		}
		name := trimmed[start:pos]
		if name == "" {
			return Selector{}, fmt.Errorf("selector %q: expected a field name at offset %d", text, start)
		}
		segments = append(segments, segment{kind: segField, name: name})
		if pos < len(trimmed) && trimmed[pos] == '[' {
			end := strings.IndexByte(trimmed[pos:], ']')
			if end < 0 {
				return Selector{}, fmt.Errorf("selector %q: unterminated index", text)
			}
			inner := trimmed[pos+1 : pos+end]
			pos += end + 1
			switch {
			case inner == "*":
				segments = append(segments, segment{kind: segAll})
			case len(inner) > 1 && (inner[0] == '"' || inner[0] == '\'') && inner[len(inner)-1] == inner[0]:
				// A quoted name reads a field a dot cannot spell, which is how
				// a page keeps its data under a route key with brackets in it.
				field, err := strconv.Unquote(inner)
				if err != nil || field == "" {
					return Selector{}, fmt.Errorf("selector %q: bad quoted field %q", text, inner)
				}
				segments = append(segments, segment{kind: segQuoted, name: field})
			case strings.HasPrefix(inner, "by:"):
				// A "by" step looks an array up by one field instead of by
				// position: "days[by:weekday]" is the day whose weekday equals
				// the call's weekday, which keeps a schedule that omits the
				// days with nothing scheduled readable.
				field := strings.TrimSpace(strings.TrimPrefix(inner, "by:"))
				if !isName(field) {
					return Selector{}, fmt.Errorf("selector %q: bad lookup %q", text, inner)
				}
				segments = append(segments, segment{kind: segBy, name: field})
			case inner != "" && isIdentifierByte(inner[0]) && !isNumber(inner):
				// A name steps to a field of its own, as in
				// "days[weekdayIndex]": the engine fills it per call and the
				// mapping then reads one day of a schedule.
				if !isName(inner) {
					return Selector{}, fmt.Errorf("selector %q: bad index %q", text, inner)
				}
				segments = append(segments, segment{kind: segKey, name: inner})
			default:
				index, err := strconv.Atoi(inner)
				if err != nil || index < 0 {
					return Selector{}, fmt.Errorf("selector %q: bad index %q", text, inner)
				}
				segments = append(segments, segment{kind: segIndex, index: index})
			}
		}
		if pos == len(trimmed) {
			break
		}
		if trimmed[pos] != '.' {
			return Selector{}, fmt.Errorf("selector %q: unexpected %q at offset %d", text, trimmed[pos], pos)
		}
		pos++
		if pos == len(trimmed) {
			return Selector{}, fmt.Errorf("selector %q: trailing dot", text)
		}
	}
	if segments[0].kind == segAll {
		return Selector{}, fmt.Errorf("selector %q: a wildcard cannot name the root", text)
	}
	return Selector{segments: segments}, nil
}

// keyIndex reads a call variable as an array position.
func (s Selector) keyIndex(name string) (int, bool) {
	if s.keys == nil {
		return 0, false
	}
	position, ok := s.keys[name]
	return position, ok
}

// keyValue reads the call value a "[by:field]" step matches.
func (s Selector) keyValue(field string) (int, bool) {
	if s.values == nil {
		return 0, false
	}
	value, ok := s.values[field]
	return value, ok
}

// numberValue reads a decoded JSON number as an int.
func numberValue(value any) (int, bool) {
	switch item := value.(type) {
	case float64:
		return int(item), true
	case int:
		return item, true
	case json.Number:
		parsed, err := item.Int64()
		if err != nil {
			return 0, false
		}
		return int(parsed), true
	}
	return 0, false
}

func isNumber(text string) bool {
	for _, r := range text {
		if r < '0' || r > '9' {
			return false
		}
	}
	return text != ""
}

func isIdentifierByte(b byte) bool {
	switch {
	case b >= 'a' && b <= 'z':
		return true
	case b >= 'A' && b <= 'Z':
		return true
	case b >= '0' && b <= '9':
		return true
	case b == '_':
		return true
	default:
		return false
	}
}

// String returns the canonical selector text.
func (s Selector) String() string {
	if len(s.segments) == 0 {
		return ""
	}
	var out strings.Builder
	for i, item := range s.segments {
		switch item.kind {
		case segField:
			if i > 0 {
				out.WriteByte('.')
			}
			out.WriteString(item.name)
		case segIndex:
			out.WriteString("[" + strconv.Itoa(item.index) + "]")
		case segKey:
			out.WriteString("[" + item.name + "]")
		case segBy:
			out.WriteString("[by:" + item.name + "]")
		case segQuoted:
			out.WriteString("[" + strconv.Quote(item.name) + "]")
		case segAll:
			out.WriteString("[*]")
		}
	}
	return out.String()
}

// HasIndices reports whether the selector steps through an array anywhere but
// its last segment. Rules that read a nested shape use it to tell a mapping
// that walks a list apart from one that only names fields.
// Root names the first field of the selector, which is the variable it reads.
func (s Selector) Root() string {
	if len(s.segments) == 0 {
		return ""
	}
	return s.segments[0].name
}

// Resolve walks root with the selector. The bool is false when any step is
// missing or has the wrong shape; callers treat that as "value absent".
func (s Selector) Resolve(root any) (any, bool) {
	current := root
	for index, item := range s.segments {
		isLast := index == len(s.segments)-1
		switch item.kind {
		case segField, segQuoted:
			object, ok := current.(map[string]any)
			if !ok {
				return nil, false
			}
			next, exists := object[item.name]
			if !exists || next == nil {
				return nil, false
			}
			current = next
		case segIndex:
			list, ok := current.([]any)
			if !ok || item.index >= len(list) {
				return nil, false
			}
			current = list[item.index]
		case segKey:
			// The index comes from the call's variables, which is how a rule
			// reads one entry of a list the upstream sent in full.
			position, ok := s.keyIndex(item.name)
			if !ok {
				return nil, false
			}
			list, ok := current.([]any)
			if !ok || position < 0 || position >= len(list) {
				return nil, false
			}
			current = list[position]
		case segBy:
			list, ok := current.([]any)
			if !ok {
				return nil, false
			}
			wanted, ok := s.keyValue(item.name)
			if !ok {
				return nil, false
			}
			matched := any(nil)
			for _, entry := range list {
				object, ok := entry.(map[string]any)
				if !ok {
					continue
				}
				if actual, ok := numberValue(object[item.name]); ok && actual == wanted {
					matched = entry
					break
				}
			}
			if matched == nil {
				return nil, false
			}
			current = matched

		case segAll:
			list, ok := current.([]any)
			if !ok {
				return nil, false
			}
			if isLast {
				return list, true
			}
			// A wildcard in the middle descends one level: it walks the rest
			// of the path on every element and returns what each branch
			// yields, in order and one level deep, which is how
			// "days[*].groups.series.preview[*]" unrolls a nested list
			// without an expression language. A branch whose shape lacks the
			// field is skipped, like a join over the array.
			rest := Selector{segments: s.segments[index+1:], keys: s.keys, values: s.values}
			out := make([]any, 0, len(list))
			for _, entry := range list {
				value, found := rest.Resolve(entry)
				if !found {
					continue
				}
				out = append(out, value)
			}
			if len(out) == 0 {
				return nil, false
			}
			return out, true
		}
	}
	return current, true
}
