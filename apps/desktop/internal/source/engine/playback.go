package engine

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// lineOption is one mapped playback route before it is turned into an address.
type lineOption struct {
	id      string
	label   string
	urlKind string
	lineURL string
	ticket  string
	def     bool
	item    any
}

func (s *sourceImpl) Resolve(ctx context.Context, episode source.Episode, lineID string) (source.Playback, error) {
	block := s.rule.Resolve
	if block == nil {
		return source.Playback{}, s.unsupported("resolve")
	}
	vars, err := s.runSteps(ctx, "resolve", block.Steps, map[string]any{
		"id":      episode.ID,
		"payload": episode.Payload,
		"media":   episode.Media.ID,
		"lineId":  lineID,
	})
	if err != nil {
		return source.Playback{}, err
	}
	site := callSite{op: "resolve", step: "resolve.lines"}
	options := s.mapLines(block.Lines, vars)
	candidates, err := orderLines(options, lineID, site, s)
	if err != nil {
		return source.Playback{}, err
	}

	prefer := block.Lines.PreferURLKind
	reasons := make([]string, 0, 2)
	var fallback *source.Playback
	for index, option := range candidates {
		if index >= block.Lines.MaxAttempts {
			break
		}
		address, urlKind, err := s.lineAddress(ctx, site, block, option, vars)
		if err != nil {
			reasons = append(reasons, option.id+": "+reason(err))
			continue
		}
		if address == "" {
			reasons = append(reasons, option.id+": no playable address")
			continue
		}
		lines := make([]source.Line, 0, len(options))
		for _, entry := range options {
			label := entry.label
			if label == "" {
				label = entry.id
			}
			lines = append(lines, source.Line{
				ID:      entry.id,
				Label:   label,
				URLKind: inferURLKind(firstNonEmpty(entry.urlKind, entry.lineURL)),
				Default: entry.def,
			})
		}
		chosen := source.Line{
			ID:      option.id,
			Label:   firstNonEmpty(option.label, option.id),
			URLKind: urlKind,
			Default: option.def,
		}
		playback := source.Playback{
			URL:       address,
			URLKind:   urlKind,
			Line:      chosen,
			Lines:     lines,
			ExpiresIn: s.expires(block.Lines, vars),
		}
		if prefer == "" || urlKind == prefer {
			return playback, nil
		}
		if fallback == nil {
			saved := playback
			fallback = &saved
		}
	}
	if fallback != nil {
		return *fallback, nil
	}
	return source.Playback{}, s.fail(site, source.ErrNoPlayableLine,
		"no line produced a playable address: "+strings.Join(reasons, "; "), nil)
}

// mapLines maps the upstream line array onto options, keeping declaration order
// and dropping duplicates by id.
func (s *sourceImpl) mapLines(lines rule.Lines, vars map[string]any) []lineOption {
	items := s.resolveItems(lines.From, vars)
	seen := make(map[string]bool, len(items))
	options := make([]lineOption, 0, len(items))
	for _, item := range items {
		sc := scope{item: item, vars: vars}
		option := lineOption{
			id:      s.evalString(field(lines.Fields, "id"), sc),
			label:   s.evalString(field(lines.Fields, "label"), sc),
			urlKind: s.evalString(field(lines.Fields, "urlKind"), sc),
			lineURL: s.evalString(field(lines.Fields, "url"), sc),
			ticket:  s.evalString(field(lines.Fields, "ticket"), sc),
			item:    item,
		}
		if value, ok := s.eval(field(lines.Fields, "default"), sc); ok {
			option.def = toBool(value)
		}
		if option.id == "" || seen[option.id] {
			continue
		}
		seen[option.id] = true
		options = append(options, option)
	}
	return options
}

// orderLines puts the requested line first, then the upstream default, then
// declaration order.
func orderLines(options []lineOption, lineID string, site callSite, s *sourceImpl) ([]lineOption, error) {
	if lineID != "" {
		for _, option := range options {
			if option.id == lineID {
				return []lineOption{option}, nil
			}
		}
		return nil, s.fail(site, source.ErrNotFound, fmt.Sprintf("line %q is not offered", lineID), nil)
	}
	ordered := make([]lineOption, 0, len(options))
	for _, option := range options {
		if option.def {
			ordered = append(ordered, option)
		}
	}
	for _, option := range options {
		if !option.def {
			ordered = append(ordered, option)
		}
	}
	return ordered, nil
}

// lineAddress returns a playable address for one line, exchanging a ticket when
// the upstream does not expose the address directly.
func (s *sourceImpl) lineAddress(ctx context.Context, site callSite, block *rule.ResolveBlock, option lineOption, vars map[string]any) (string, string, error) {
	lines := block.Lines
	if isAbsoluteURL(option.lineURL) {
		return option.lineURL, inferURLKind(firstNonEmpty(option.urlKind, option.lineURL)), nil
	}
	ticket := option.ticket
	if ticket == "" {
		ticket = stripResolvePrefix(option.lineURL)
	}
	if ticket == "" && lines.Exchange != nil {
		if value, ok := s.eval(lines.Exchange.Ticket, scope{item: option.item, vars: vars}); ok {
			ticket = toString(value)
		}
	}
	if lines.Exchange == nil || ticket == "" {
		return "", "", nil
	}
	local := make(map[string]any, len(vars)+1)
	for key, value := range vars {
		local[key] = value
	}
	local["ticket"] = ticket
	response, err := s.fetch(ctx, site, lines.Exchange.Request, scope{vars: local})
	if err != nil {
		return "", "", err
	}
	selector, err := rule.ParseSelector(lines.Exchange.URL)
	if err != nil {
		return "", "", s.fail(site, source.ErrInvalidRule, "bad exchange url selector", err)
	}
	value, ok := selector.Resolve(response)
	if !ok {
		return "", "", s.fail(site, source.ErrInvalidResponse, "exchange response has no address", nil)
	}
	address := toString(value)
	if !isAbsoluteURL(address) {
		return "", "", nil
	}
	declaredKind := option.urlKind
	if !lines.Exchange.URLKind.IsZero() {
		if kindValue, ok := s.eval(lines.Exchange.URLKind, scope{item: option.item, vars: vars}); ok {
			declaredKind = toString(kindValue)
		}
	}
	return address, inferURLKind(firstNonEmpty(declaredKind, address)), nil
}

func (s *sourceImpl) expires(lines rule.Lines, vars map[string]any) time.Duration {
	if lines.ExpiresIn.IsZero() {
		return 0
	}
	value, ok := s.eval(lines.ExpiresIn, scope{vars: vars})
	if !ok {
		return 0
	}
	number, ok := toInt(value)
	if !ok {
		return 0
	}
	seconds := number.(int)
	if seconds <= 0 {
		return 0
	}
	return time.Duration(seconds) * time.Second
}

func isAbsoluteURL(raw string) bool {
	if raw == "" {
		return false
	}
	parsed, err := url.Parse(raw)
	if err != nil {
		return false
	}
	return (parsed.Scheme == "http" || parsed.Scheme == "https") && parsed.Host != ""
}

func stripResolvePrefix(raw string) string {
	const prefix = "resolve://"
	if len(raw) < len(prefix) || !strings.EqualFold(raw[:len(prefix)], prefix) {
		return ""
	}
	return raw[len(prefix):]
}

// reason keeps the join-detail short: classification plus the upstream excerpt.
func reason(err error) string {
	var target *source.Error
	if errors.As(err, &target) {
		if target.Detail != "" {
			return string(target.Kind) + " (" + target.Detail + ")"
		}
		return string(target.Kind)
	}
	return "failed"
}
