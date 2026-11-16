package engine

import (
	"context"
	"fmt"
	"net/http"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// Options configures an engine instance. The zero value is usable.
type Options struct {
	// Client overrides the HTTP client. Tests inject one that reaches a local
	// fixture server; production keeps the default.
	Client *http.Client
}

type sourceImpl struct {
	rule    *rule.Rule
	client  *http.Client
	sem     chan struct{}
	retries int
	timeout time.Duration
}

// New validates the rule and builds a Source. A rule that fails validation is
// rejected here, so a running source is always structurally sound.
func New(parsed *rule.Rule, opts Options) (source.Source, error) {
	if err := parsed.Validate(); err != nil {
		return nil, fmt.Errorf("rule %q: %w", parsed.ID, err)
	}
	client := opts.Client
	if client == nil {
		client = &http.Client{}
	}
	retries := rule.DefaultRetries
	if parsed.HTTP.Retries != nil {
		retries = *parsed.HTTP.Retries
	}
	concurrency := parsed.HTTP.Concurrency
	if concurrency < 1 {
		concurrency = 1
	}
	return &sourceImpl{
		rule:    parsed,
		client:  client,
		sem:     make(chan struct{}, concurrency),
		retries: retries,
		timeout: time.Duration(parsed.HTTP.TimeoutMs) * time.Millisecond,
	}, nil
}

func (s *sourceImpl) ID() string { return s.rule.ID }

func (s *sourceImpl) Name() string { return s.rule.Name }

func (s *sourceImpl) unsupported(op string) error {
	return s.fail(callSite{op: op}, source.ErrUnsupported, "rule has no "+op+" block", nil)
}

func (s *sourceImpl) Home(ctx context.Context) (source.Home, error) {
	block := s.rule.Home
	if block == nil {
		return source.Home{}, s.unsupported("home")
	}
	vars, err := s.runSteps(ctx, "home", block.Steps, nil)
	if err != nil {
		return source.Home{}, err
	}
	items, err := s.collect("home", block.Output.From, vars)
	if err != nil {
		return source.Home{}, err
	}
	sections := make([]source.Section, 0, len(items))
	for _, item := range items {
		sc := scope{item: item, vars: vars}
		section := source.Section{
			ID:       s.evalString(field(block.Output.Fields, "id"), sc),
			Title:    s.evalString(field(block.Output.Fields, "title"), sc),
			Subtitle: s.evalString(field(block.Output.Fields, "subtitle"), sc),
		}
		if block.Output.Cards != nil {
			section.Cards = s.mapCards(block.Output.Cards, item, vars)
		}
		sections = append(sections, section)
	}
	return source.Home{Sections: sections}, nil
}

func (s *sourceImpl) List(ctx context.Context, query source.ListQuery) (source.Page, error) {
	block := s.rule.List
	if block == nil {
		return source.Page{}, s.unsupported("list")
	}
	page := query.Page
	if page < 1 {
		page = 1
	}
	kindValue := ""
	if query.Kind != "" {
		kindValue = block.KindValues[string(query.Kind)]
	}
	vars, err := s.runSteps(ctx, "list", block.Steps, map[string]any{
		"page":    page,
		"keyword": strings.TrimSpace(query.Keyword),
		"kind":    kindValue,
	})
	if err != nil {
		return source.Page{}, err
	}
	out := block.Output
	if out.Cards == nil {
		return source.Page{}, s.fail(callSite{op: "list", step: "list.output"},
			source.ErrInvalidRule, "list output has no cards mapping", nil)
	}
	result := source.Page{Cards: s.mapCards(out.Cards, vars, vars), Page: page}
	if !out.Page.IsZero() {
		if value, ok := s.eval(out.Page, scope{vars: vars}); ok {
			if number, ok := toInt(value); ok {
				result.Page = number.(int)
			}
		}
	}
	if !out.HasMore.IsZero() {
		if value, ok := s.eval(out.HasMore, scope{vars: vars}); ok {
			result.HasMore = toBool(value)
		}
	}
	return result, nil
}

func (s *sourceImpl) Detail(ctx context.Context, ref source.Ref) (source.Detail, error) {
	block := s.rule.Detail
	if block == nil {
		return source.Detail{}, s.unsupported("detail")
	}
	vars, err := s.runSteps(ctx, "detail", block.Steps, map[string]any{"id": ref.ID})
	if err != nil {
		return source.Detail{}, err
	}
	object, err := s.collectOne("detail", block.Output.From, vars)
	if err != nil {
		return source.Detail{}, err
	}
	sc := scope{item: object, vars: vars}
	fields := block.Output.Fields
	detail := source.Detail{
		Ref:          source.Ref{Source: s.rule.ID, ID: firstNonEmpty(s.evalString(field(fields, "id"), sc), ref.ID)},
		Title:        s.evalString(field(fields, "title"), sc),
		Kind:         kindOf(s.evalString(field(fields, "kind"), sc)),
		Area:         s.evalString(field(fields, "area"), sc),
		Tags:         s.evalList(field(fields, "tags"), sc),
		Overview:     s.evalString(field(fields, "overview"), sc),
		PosterURL:    s.evalString(field(fields, "posterUrl"), sc),
		BackdropURL:  s.evalString(field(fields, "backdropUrl"), sc),
		Remarks:      s.evalString(field(fields, "remarks"), sc),
		Actors:       s.evalList(field(fields, "actors"), sc),
		Directors:    s.evalList(field(fields, "directors"), sc),
		EpisodeCount: 0,
	}
	if value, ok := s.eval(field(fields, "year"), sc); ok {
		if number, ok := toInt(value); ok {
			detail.Year = number.(int)
		}
	}
	if value, ok := s.eval(field(fields, "episodeCount"), sc); ok {
		if number, ok := toInt(value); ok {
			detail.EpisodeCount = number.(int)
		}
	}
	return detail, nil
}

func (s *sourceImpl) Episodes(ctx context.Context, ref source.Ref) ([]source.Episode, error) {
	block := s.rule.Episodes
	if block == nil {
		return nil, s.unsupported("episodes")
	}
	vars, err := s.runSteps(ctx, "episodes", block.Steps, map[string]any{"id": ref.ID})
	if err != nil {
		return nil, err
	}
	items, err := s.collect("episodes", block.Output.From, vars)
	if err != nil {
		return nil, err
	}
	media := source.Ref{Source: s.rule.ID, ID: ref.ID}
	episodes := make([]source.Episode, 0, len(items))
	for _, item := range items {
		sc := scope{item: item, vars: vars}
		episode := source.Episode{
			Media:   media,
			ID:      s.evalString(field(block.Output.Fields, "id"), sc),
			Title:   s.evalString(field(block.Output.Fields, "title"), sc),
			Payload: s.evalString(field(block.Output.Fields, "payload"), sc),
		}
		if value, ok := s.eval(field(block.Output.Fields, "number"), sc); ok {
			if number, ok := toInt(value); ok {
				episode.Number = number.(int)
			}
		}
		if episode.ID == "" {
			continue
		}
		episodes = append(episodes, episode)
	}
	return episodes, nil
}

func (s *sourceImpl) runSteps(ctx context.Context, op string, steps []rule.Step, vars map[string]any) (map[string]any, error) {
	if vars == nil {
		vars = map[string]any{}
	}
	for index, step := range steps {
		site := callSite{op: op, step: fmt.Sprintf("%s.steps[%d]", op, index)}
		switch step.Op {
		case rule.OpRequest:
			result, err := s.fetch(ctx, site, step.Request, scope{vars: vars})
			if err != nil {
				return nil, err
			}
			vars[step.As] = result
		case rule.OpPaginate:
			items, err := s.runPaginate(ctx, site, step, vars)
			if err != nil {
				return nil, err
			}
			vars[step.As] = items
		default:
			return nil, s.fail(site, source.ErrInvalidRule, "unknown op "+step.Op, nil)
		}
	}
	return vars, nil
}

func (s *sourceImpl) runPaginate(ctx context.Context, site callSite, step rule.Step, vars map[string]any) ([]any, error) {
	var all []any
	start := step.Start
	if start < 1 {
		start = 1
	}
	for index := 0; index < step.MaxPages; index++ {
		local := make(map[string]any, len(vars)+2)
		for key, value := range vars {
			local[key] = value
		}
		if step.Mode == "page" {
			local["page"] = start + index
			local["offset"] = index * step.Size
		} else {
			local["offset"] = index * step.Size
			local["page"] = start + index
		}
		response, err := s.fetch(ctx, site, step.Request, scope{vars: local})
		if err != nil {
			return nil, err
		}
		items := s.resolveItems(step.Items, response)
		if len(items) == 0 {
			break
		}
		all = append(all, items...)
		if step.HasMore == "" {
			continue
		}
		selector, err := rule.ParseSelector(step.HasMore)
		if err != nil {
			return nil, s.fail(site, source.ErrInvalidRule, "bad hasMore selector", err)
		}
		value, ok := selector.Resolve(response)
		if !ok || !toBool(value) {
			break
		}
	}
	return all, nil
}

func (s *sourceImpl) collect(op, from string, vars map[string]any) ([]any, error) {
	selector, err := rule.ParseSelector(from)
	if err != nil {
		return nil, s.fail(callSite{op: op, step: op + ".output.from"}, source.ErrInvalidRule, "bad selector", err)
	}
	value, ok := selector.Resolve(vars)
	if !ok {
		return nil, s.fail(callSite{op: op, step: op + ".output.from"},
			source.ErrInvalidResponse, fmt.Sprintf("%q is missing from the response", from), nil)
	}
	return asItems(value), nil
}

func (s *sourceImpl) collectOne(op, from string, vars map[string]any) (any, error) {
	items, err := s.collect(op, from, vars)
	if err != nil {
		return nil, err
	}
	if len(items) == 0 {
		return nil, s.fail(callSite{op: op, step: op + ".output.from"},
			source.ErrInvalidResponse, fmt.Sprintf("%q resolved to nothing", from), nil)
	}
	return items[0], nil
}

// asItems accepts either an array or a single object; rules should not need a
// wrapper type to say which one the upstream sent.
func asItems(value any) []any {
	if list, ok := value.([]any); ok {
		return list
	}
	if value == nil {
		return nil
	}
	return []any{value}
}

func (s *sourceImpl) resolveItems(selectorText string, root any) []any {
	selector, err := rule.ParseSelector(selectorText)
	if err != nil {
		return nil
	}
	value, ok := selector.Resolve(root)
	if !ok {
		return nil
	}
	return asItems(value)
}

func (s *sourceImpl) mapCards(shape *rule.Shape, root any, vars map[string]any) []source.Card {
	items := s.resolveItems(shape.From, root)
	cards := make([]source.Card, 0, len(items))
	for _, item := range items {
		sc := scope{item: item, vars: vars}
		card := source.Card{
			Ref:         source.Ref{Source: s.rule.ID, ID: s.evalString(field(shape.Fields, "id"), sc)},
			Title:       s.evalString(field(shape.Fields, "title"), sc),
			Kind:        kindOf(s.evalString(field(shape.Fields, "kind"), sc)),
			Area:        s.evalString(field(shape.Fields, "area"), sc),
			Tags:        s.evalList(field(shape.Fields, "tags"), sc),
			Remarks:     s.evalString(field(shape.Fields, "remarks"), sc),
			PosterURL:   s.evalString(field(shape.Fields, "posterUrl"), sc),
			BackdropURL: s.evalString(field(shape.Fields, "backdropUrl"), sc),
		}
		if value, ok := s.eval(field(shape.Fields, "year"), sc); ok {
			if number, ok := toInt(value); ok {
				card.Year = number.(int)
			}
		}
		if card.ID == "" {
			continue
		}
		cards = append(cards, card)
	}
	return cards
}

func (s *sourceImpl) evalList(value rule.Value, sc scope) []string {
	resolved, ok := s.eval(value, sc)
	if !ok {
		return nil
	}
	return toStrings(resolved)
}

func field(fields map[string]rule.Value, key string) rule.Value {
	return fields[key]
}

func kindOf(raw string) source.Kind {
	text := strings.TrimSpace(raw)
	if text == "" {
		return ""
	}
	return source.Kind(mapKind(text))
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}
	return ""
}
