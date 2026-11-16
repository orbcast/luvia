package rule

import (
	"fmt"
	"strings"
)

// Template is a string with {selector} holes, used wherever a request value or
// title is assembled from more than one field.
type Template struct {
	raw   string
	parts []templatePart
}

type templatePart struct {
	literal  string
	selector Selector
	isHole   bool
}

// ParseTemplate parses text such as "ep {number}" or "{id}".
func ParseTemplate(text string) (Template, error) {
	if text == "" {
		return Template{raw: ""}, nil
	}
	var parts []templatePart
	for pos := 0; pos < len(text); {
		open := strings.IndexByte(text[pos:], '{')
		if open < 0 {
			rest := text[pos:]
			if strings.IndexByte(rest, '}') >= 0 {
				return Template{}, fmt.Errorf("template %q: closing brace without an opening one", text)
			}
			parts = append(parts, templatePart{literal: rest})
			break
		}
		open += pos
		if open > pos {
			parts = append(parts, templatePart{literal: text[pos:open]})
		}
		closeAt := strings.IndexByte(text[open:], '}')
		if closeAt < 0 {
			return Template{}, fmt.Errorf("template %q: unterminated hole", text)
		}
		closeAt += open
		inner := strings.TrimSpace(text[open+1 : closeAt])
		sel, err := ParseSelector(inner)
		if err != nil {
			return Template{}, fmt.Errorf("template %q: %w", text, err)
		}
		parts = append(parts, templatePart{selector: sel, isHole: true})
		pos = closeAt + 1
	}
	return Template{raw: text, parts: parts}, nil
}

// String returns the template text.
func (t Template) String() string { return t.raw }

// IsZero reports whether the template carries no text.
func (t Template) IsZero() bool { return t.raw == "" }

// Selectors returns every hole in the template, in order.
func (t Template) Selectors() []Selector {
	var out []Selector
	for _, part := range t.parts {
		if part.isHole {
			out = append(out, part.selector)
		}
	}
	return out
}

// Render fills the holes through resolve. The bool is false as soon as one hole
// cannot be resolved, so callers can treat the whole value as absent instead of
// sending a half-built request.
func (t Template) Render(resolve func(Selector) (string, bool)) (string, bool) {
	var out strings.Builder
	for _, part := range t.parts {
		if !part.isHole {
			out.WriteString(part.literal)
			continue
		}
		text, ok := resolve(part.selector)
		if !ok {
			return "", false
		}
		out.WriteString(text)
	}
	return out.String(), true
}
