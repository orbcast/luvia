package rule

import (
	"fmt"
	"strconv"
	"strings"
)

// Selector is a restricted path through decoded JSON: dot-separated field
// names, numeric indexes and a trailing wildcard. There is deliberately no
// expression language; anything more belongs in a value function.
type Selector struct {
	segments []segment
}

type segmentKind int

const (
	segField segmentKind = iota
	segIndex
	segAll
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
			if inner == "*" {
				segments = append(segments, segment{kind: segAll})
			} else {
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
	for i, item := range segments {
		if item.kind == segAll && i != len(segments)-1 {
			return Selector{}, fmt.Errorf("selector %q: wildcard must be the last segment", text)
		}
	}
	return Selector{segments: segments}, nil
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
		case segAll:
			out.WriteString("[*]")
		}
	}
	return out.String()
}

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
	for _, item := range s.segments {
		switch item.kind {
		case segField:
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
		case segAll:
			list, ok := current.([]any)
			if !ok {
				return nil, false
			}
			return list, true
		}
	}
	return current, true
}
