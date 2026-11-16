package engine

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"luvia/apps/desktop/internal/source/rule"
)

// scope is what a mapping or template sees: the current item when one is being
// mapped, plus every variable bound by the block's steps.
type scope struct {
	item any
	vars map[string]any
}

func (s *sourceImpl) resolveSelector(sel rule.Selector, sc scope) (any, bool) {
	if _, ok := sc.item.(map[string]any); ok {
		if value, found := sel.Resolve(sc.item); found {
			return value, true
		}
	}
	return sel.Resolve(sc.vars)
}

func (s *sourceImpl) render(text string, sc scope) (string, bool) {
	parsed, err := rule.ParseTemplate(text)
	if err != nil {
		return "", false
	}
	return parsed.Render(func(sel rule.Selector) (string, bool) {
		value, ok := s.resolveSelector(sel, sc)
		if !ok {
			return "", false
		}
		return toString(value), true
	})
}

// eval resolves a field mapping. The bool is false when the mapping refers to a
// missing value; callers then leave the field empty instead of failing the call.
func (s *sourceImpl) eval(value rule.Value, sc scope) (any, bool) {
	if sel, ok := value.Selector(); ok {
		return s.resolveSelector(sel, sc)
	}
	if tpl, ok := value.Template(); ok {
		rendered, ok := tpl.Render(func(sel rule.Selector) (string, bool) {
			found, exists := s.resolveSelector(sel, sc)
			if !exists {
				return "", false
			}
			return toString(found), true
		})
		if !ok {
			return nil, false
		}
		return rendered, true
	}
	if name, args, ok := value.Func(); ok {
		values := make([]any, len(args))
		found := make([]bool, len(args))
		for index, arg := range args {
			values[index], found[index] = s.eval(arg, sc)
		}
		return callFunction(name, values, found)
	}
	if literal, ok := value.Literal(); ok {
		return literal, true
	}
	return nil, false
}

// evalString evaluates a mapping that feeds a text field such as a title.
func (s *sourceImpl) evalString(value rule.Value, sc scope) string {
	result, _ := s.eval(value, sc)
	return toString(result)
}

func callFunction(name string, args []any, found []bool) (any, bool) {
	switch name {
	case "coalesce":
		for index, arg := range args {
			if !found[index] {
				continue
			}
			if text := toString(arg); text != "" {
				return text, true
			}
		}
		return "", false
	case "trimPrefix":
		value, ok := stringArgs(args, found, 0)
		if !ok {
			return nil, false
		}
		prefix, ok := stringArgs(args, found, 1)
		if !ok {
			return nil, false
		}
		return strings.TrimPrefix(value, prefix), true
	case "trimSuffix":
		value, ok := stringArgs(args, found, 0)
		if !ok {
			return nil, false
		}
		suffix, ok := stringArgs(args, found, 1)
		if !ok {
			return nil, false
		}
		return strings.TrimSuffix(value, suffix), true
	case "trim":
		value, ok := stringArgs(args, found, 0)
		if !ok {
			return nil, false
		}
		return strings.TrimSpace(value), true
	case "lower":
		value, ok := stringArgs(args, found, 0)
		if !ok {
			return nil, false
		}
		return strings.ToLower(value), true
	case "upper":
		value, ok := stringArgs(args, found, 0)
		if !ok {
			return nil, false
		}
		return strings.ToUpper(value), true
	case "int":
		if len(args) == 0 || !found[0] {
			return nil, false
		}
		return toInt(args[0])
	case "kind":
		if len(args) == 0 || !found[0] {
			return nil, false
		}
		return string(mapKind(toString(args[0]))), true
	case "urlKind":
		if len(args) == 0 || !found[0] {
			return nil, false
		}
		return inferURLKind(toString(args[0])), true
	case "list":
		if len(args) == 0 || !found[0] {
			return nil, false
		}
		return toStrings(args[0]), true
	case "first":
		list := asList(args, found)
		if len(list) == 0 {
			return nil, false
		}
		return list[0], true
	case "last":
		list := asList(args, found)
		if len(list) == 0 {
			return nil, false
		}
		return list[len(list)-1], true
	default:
		return nil, false
	}
}

func stringArgs(args []any, found []bool, index int) (string, bool) {
	if index >= len(args) || !found[index] {
		return "", false
	}
	return toString(args[index]), true
}

func asList(args []any, found []bool) []any {
	if len(args) == 0 || !found[0] {
		return nil
	}
	if list, ok := args[0].([]any); ok {
		return list
	}
	return []any{args[0]}
}

// toString renders a decoded JSON value as text. Numbers keep their literal
// form because JSON is decoded with UseNumber.
func toString(value any) string {
	switch item := value.(type) {
	case nil:
		return ""
	case string:
		return item
	case bool:
		return strconv.FormatBool(item)
	case json.Number:
		return item.String()
	case float64:
		return strconv.FormatFloat(item, 'f', -1, 64)
	case int:
		return strconv.Itoa(item)
	case int64:
		return strconv.FormatInt(item, 10)
	default:
		return fmt.Sprint(item)
	}
}

func toInt(value any) (any, bool) {
	switch item := value.(type) {
	case json.Number:
		parsed, err := item.Int64()
		if err != nil {
			return nil, false
		}
		return int(parsed), true
	case float64:
		return int(item), true
	case int:
		return item, true
	case string:
		parsed, err := strconv.Atoi(strings.TrimSpace(item))
		if err != nil {
			return nil, false
		}
		return parsed, true
	default:
		return nil, false
	}
}

func toBool(value any) bool {
	switch item := value.(type) {
	case bool:
		return item
	case string:
		switch strings.ToLower(strings.TrimSpace(item)) {
		case "true", "1", "yes":
			return true
		}
		return false
	case json.Number:
		parsed, err := item.Int64()
		return err == nil && parsed != 0
	case float64:
		return item != 0
	case int:
		return item != 0
	default:
		return false
	}
}

func toStrings(value any) []string {
	switch item := value.(type) {
	case []any:
		out := make([]string, 0, len(item))
		for _, entry := range item {
			if text := toString(entry); text != "" {
				out = append(out, text)
			}
		}
		return out
	case []string:
		return item
	case nil:
		return nil
	default:
		if text := toString(item); text != "" {
			return []string{text}
		}
		return nil
	}
}

// mapKind folds an upstream category onto the player's closed kind set.
func mapKind(raw string) string {
	switch strings.ToLower(strings.TrimSpace(raw)) {
	case "movie", "film", "documentary", "doc":
		return "movie"
	case "anime", "animation", "donghua", "cartoon":
		return "anime"
	case "variety", "sports", "reality", "show":
		return "variety"
	case "series", "tv", "drama", "short_drama", "shortdrama":
		return "series"
	default:
		return "series"
	}
}

// inferURLKind decides mp4 versus hls from a declared kind or an address.
func inferURLKind(raw string) string {
	text := strings.ToLower(strings.TrimSpace(raw))
	switch {
	case text == "":
		return "mp4"
	case strings.Contains(text, "hls"), strings.Contains(text, "m3u8"):
		return "hls"
	case strings.Contains(text, "mp4"):
		return "mp4"
	}
	trimmed := text
	if cut := strings.IndexAny(trimmed, "?#"); cut >= 0 {
		trimmed = trimmed[:cut]
	}
	if strings.HasSuffix(trimmed, ".m3u8") {
		return "hls"
	}
	return "mp4"
}
