package rule

import (
	"bytes"
	"encoding/json"
	"fmt"
)

// Value is one field mapping. It is either
//
//	"title"                                  selector, relative to the current item
//	{"fn": "coalesce", "args": [...]}        function call
//	{"template": "ep {number}"}              interpolated string
//	{"literal": 7}                           constant
//
// Keeping the four forms closed is what lets validation reject typos instead of
// silently producing empty cards at runtime.
type Value struct {
	kind valueKind
	sel  Selector
	fn   string
	args []Value
	tpl  Template
	lit  any
}

type valueKind int

const (
	valueZero valueKind = iota
	valueSelector
	valueFunc
	valueTemplate
	valueLiteral
)

// Selector returns the mapped selector when the value is a bare selector.
func (v Value) Selector() (Selector, bool) {
	if v.kind != valueSelector {
		return Selector{}, false
	}
	return v.sel, true
}

// Func returns the function name and arguments when the value is a call.
func (v Value) Func() (string, []Value, bool) {
	if v.kind != valueFunc {
		return "", nil, false
	}
	return v.fn, v.args, true
}

// Template returns the interpolation pattern when the value is a template.
func (v Value) Template() (Template, bool) {
	if v.kind != valueTemplate {
		return Template{}, false
	}
	return v.tpl, true
}

// Literal returns the constant when the value is a literal.
func (v Value) Literal() (any, bool) {
	if v.kind != valueLiteral {
		return nil, false
	}
	return v.lit, true
}

// IsZero reports whether the value was omitted from the rule.
func (v Value) IsZero() bool { return v.kind == valueZero }

// Kind names the value form for diagnostics.
func (v Value) Kind() string {
	switch v.kind {
	case valueSelector:
		return "selector"
	case valueFunc:
		return "function"
	case valueTemplate:
		return "template"
	case valueLiteral:
		return "literal"
	default:
		return "unset"
	}
}

// UnmarshalJSON decodes one of the four accepted mapping forms.
func (v *Value) UnmarshalJSON(data []byte) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	var raw any
	if err := dec.Decode(&raw); err != nil {
		return err
	}
	parsed, err := newValue(raw)
	if err != nil {
		return err
	}
	*v = parsed
	return nil
}

// MarshalJSON re-encodes the value in its declarative form.
func (v Value) MarshalJSON() ([]byte, error) {
	switch v.kind {
	case valueSelector:
		return json.Marshal(v.sel.String())
	case valueTemplate:
		return json.Marshal(map[string]string{"template": v.tpl.String()})
	case valueLiteral:
		return json.Marshal(map[string]any{"literal": v.lit})
	case valueFunc:
		return json.Marshal(map[string]any{"fn": v.fn, "args": v.args})
	default:
		return []byte("null"), nil
	}
}

func newValue(raw any) (Value, error) {
	switch item := raw.(type) {
	case string:
		sel, err := ParseSelector(item)
		if err != nil {
			return Value{}, err
		}
		return Value{kind: valueSelector, sel: sel}, nil
	case map[string]any:
		return newObjectValue(item)
	default:
		return Value{}, fmt.Errorf("mapping must be a selector string or an object, got %T", raw)
	}
}

func newObjectValue(item map[string]any) (Value, error) {
	forms := 0
	for key := range item {
		switch key {
		case "fn", "template", "literal":
			forms++
		case "args":
			if _, ok := item["fn"]; !ok {
				return Value{}, fmt.Errorf(`"args" needs "fn"`)
			}
		default:
			return Value{}, fmt.Errorf("mapping object has unknown key %q", key)
		}
	}
	if forms > 1 {
		return Value{}, fmt.Errorf(`mapping object mixes "fn", "template" and "literal"`)
	}
	if name, ok := item["fn"]; ok {
		text, isString := name.(string)
		if !isString || text == "" {
			return Value{}, fmt.Errorf(`"fn" must be a non-empty string`)
		}
		args, err := newArgs(item["args"])
		if err != nil {
			return Value{}, fmt.Errorf("fn %q: %w", text, err)
		}
		return Value{kind: valueFunc, fn: text, args: args}, nil
	}
	if tpl, ok := item["template"]; ok {
		text, isString := tpl.(string)
		if !isString {
			return Value{}, fmt.Errorf(`"template" must be a string`)
		}
		parsed, err := ParseTemplate(text)
		if err != nil {
			return Value{}, err
		}
		return Value{kind: valueTemplate, tpl: parsed}, nil
	}
	if lit, ok := item["literal"]; ok {
		return Value{kind: valueLiteral, lit: lit}, nil
	}
	return Value{}, fmt.Errorf(`mapping object needs "fn", "template" or "literal"`)
}

func newArgs(raw any) ([]Value, error) {
	if raw == nil {
		return nil, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf(`"args" must be an array`)
	}
	args := make([]Value, 0, len(list))
	for _, entry := range list {
		arg, err := newValue(entry)
		if err != nil {
			return nil, err
		}
		args = append(args, arg)
	}
	return args, nil
}
