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
	// keys names the key each argument fills, for the functions that build a
	// record out of several fields: {"fn": "record", "args": [{"name": "x"}]}
	// makes the entry {"name": ...}. An argument without a key carries none.
	keys []string
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

// ArgumentKeys returns the key each argument of a call fills, for the
// functions that build a record from several fields. It is empty when the call
// names no keys.
func (v Value) ArgumentKeys() []string {
	return v.keys
}

// Each binds one entry of a list to the value of `item`, so a mapping written
// for a single entry can walk the whole list: a roster maps each of its
// people with the same function call instead of one field path per attribute.
// The entry a mapping reads is the current item, as it is inside a walk.
func (v Value) Each(selector Selector) Value {
	return Value{kind: valueFunc, fn: "each", args: []Value{v, {kind: valueSelector, sel: selector}}}
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
		if len(v.keys) > 0 {
			return json.Marshal(map[string]any{"fn": v.fn, "args": v.args, "keys": v.keys})
		}
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
		case "args", "keys":
			if _, ok := item["fn"]; !ok {
				return Value{}, fmt.Errorf(`%q needs "fn"`, key)
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
		keys, err := newKeys(item["keys"], len(args))
		if err != nil {
			return Value{}, fmt.Errorf("fn %q: %w", text, err)
		}
		return Value{kind: valueFunc, fn: text, args: args, keys: keys}, nil
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

// newKeys reads the per-argument key names a record-building call declares.
func newKeys(raw any, count int) ([]string, error) {
	if raw == nil {
		// A call that names no keys still fills the member's name from its
		// arguments positionally, which is what a roster of bare names needs.
		keys := make([]string, count)
		for index := range keys {
			keys[index] = "name"
		}
		return keys, nil
	}
	list, ok := raw.([]any)
	if !ok {
		return nil, fmt.Errorf(`"keys" must be an array`)
	}
	if count > 0 && len(list) != count {
		return nil, fmt.Errorf(`"keys" must name every argument: %d keys for %d args`, len(list), count)
	}
	keys := make([]string, 0, len(list))
	for _, entry := range list {
		key, ok := entry.(string)
		if !ok || key == "" {
			return nil, fmt.Errorf(`"keys" must hold non-empty strings`)
		}
		keys = append(keys, key)
	}
	return keys, nil
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
