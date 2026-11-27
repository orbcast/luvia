package engine

import (
	"context"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"time"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/rule"
)

// defaultCommentLimit caps how many comments one episode hands the player,
// enough to fill the picture without carrying a whole thread into memory.
const defaultCommentLimit = 400

// Options configures an engine instance. The zero value is usable.
type Options struct {
	// Client overrides the HTTP client. Tests inject one that reaches a local
	// fixture server; production keeps the default.
	Client *http.Client
	// SessionsDir is where a claimed upstream session is kept so the next
	// launch reuses it. Empty keeps sessions in memory only, which is what a
	// test that must not touch the user's files wants.
	SessionsDir string
}

type sourceImpl struct {
	rule    *rule.Rule
	client  *http.Client
	sem     chan struct{}
	retries int
	timeout time.Duration
	// retryWhen is the rule's own list of answers worth another try, copied
	// out of the rule so a request does not walk a slice it cannot own.
	retryWhen []rule.RetryWhen
	// session holds the claimed identity a rule that declares one sends with
	// every request. It is nil when the rule declares no session.
	session *sessionStore
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
	impl := &sourceImpl{
		rule:      parsed,
		client:    client,
		sem:       make(chan struct{}, concurrency),
		retries:   retries,
		timeout:   time.Duration(parsed.HTTP.TimeoutMs) * time.Millisecond,
		retryWhen: parsed.HTTP.RetryWhen,
	}
	impl.session = newSessionStore(parsed.Session, impl, opts.SessionsDir)
	return impl, nil
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
	for index, item := range items {
		sc := itemScope(item, index+1, vars)
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
	keyword := strings.TrimSpace(query.Keyword)
	vars, err := s.runStepsWith(ctx, "list", block.Steps, map[string]any{
		"page":    page,
		"keyword": keyword,
		"kind":    kindValue,
	}, intentOverride(block.SearchIntent, keyword))
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

// dayItems resolves the cards of one weekday. Cards carry no day of their own,
// so a rule that maps a schedule the upstream sent in full indexes the day it
// wants: "res.days[weekday].groups.series.preview[*]", where the bracket takes
// its position from the call's weekday. A rule that asks for one day per
// request names no index and gets the answer as it is.
func (s *sourceImpl) dayItems(selector rule.Selector, vars map[string]any, weekday int) []any {
	for _, field := range selector.ByFields() {
		// The schedule is looked up by the weekday it carries rather than by
		// its position, so a payload that leaves out a day with nothing
		// scheduled still maps every remaining day to the right one.
		values := make(map[string]int, len(selector.ByFields()))
		values[field] = weekday
		items := s.resolveItems(selector.WithValues(values), vars)
		if len(items) > 0 {
			return items
		}
	}
	if len(selector.KeyNames()) > 0 {
		keys := make(map[string]int, len(selector.KeyNames()))
		for index, name := range selector.KeyNames() {
			if index == 0 {
				keys[name] = weekday - 1
				continue
			}
			keys[name] = 0
		}
		return s.resolveItems(selector.WithKeys(keys), vars)
	}
	return s.resolveItems(selector, vars)
}

func (s *sourceImpl) Updates(ctx context.Context, day source.Day) ([]source.Update, error) {
	block := s.rule.Updates
	if block == nil {
		return nil, s.unsupported("updates")
	}
	if day.Weekday < 1 || day.Weekday > 7 {
		return nil, s.fail(callSite{op: "updates"}, source.ErrInvalidRule,
			fmt.Sprintf("weekday %d is not 1 through 7", day.Weekday), nil)
	}
	kindValue := ""
	if day.Kind != "" {
		kindValue = block.KindValues[string(day.Kind)]
	}
	vars, err := s.runSteps(ctx, "updates", block.Steps, map[string]any{
		"weekday": day.Weekday,
		"kind":    kindValue,
	})
	if err != nil {
		return nil, err
	}
	if block.Output.Cards == nil {
		return nil, s.fail(callSite{op: "updates", step: "updates.output"},
			source.ErrInvalidRule, "updates output has no cards mapping", nil)
	}

	selector, err := rule.ParseSelector(block.Output.Cards.From)
	if err != nil {
		return nil, s.fail(callSite{op: "updates", step: "updates.output"},
			source.ErrInvalidRule, "bad cards selector", err)
	}
	// A day with nothing scheduled resolves to nothing, which is an empty
	// answer rather than a broken rule: the player shows the day as empty.
	items := s.dayItems(selector, vars, day.Weekday)
	updates := make([]source.Update, 0, len(items))
	for index, item := range items {
		sc := itemScope(item, index+1, vars)
		shape := block.Output.Cards
		update := source.Update{Note: s.evalString(field(shape.Fields, "note"), sc)}
		update.Card = cardOf(s.rule.ID, shape.Fields, sc, s)
		if update.ID == "" {
			continue
		}
		updates = append(updates, update)
	}
	return updates, nil
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
	sc := itemScope(object, 1, vars)
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
		Cast:         s.evalPeople(field(fields, "cast"), sc),
		Crew:         s.evalPeople(field(fields, "crew"), sc),
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
	for index, item := range items {
		sc := itemScope(item, index+1, vars)
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

// Comments returns the comments an episode carries, oldest first, capped at
// limit. A rule without a comments block does not offer them at all, which
// leaves the danmaku switch off instead of claiming an empty layer.
func (s *sourceImpl) Comments(ctx context.Context, episode source.Episode, limit int) ([]source.Danmaku, error) {
	block := s.rule.Comments
	if block == nil {
		return nil, s.unsupported("comments")
	}
	if limit <= 0 {
		limit = defaultCommentLimit
	}
	vars, err := s.runSteps(ctx, "comments", block.Steps, map[string]any{
		"id": episode.Media.ID, "payload": episode.ID,
	})
	if err != nil {
		return nil, err
	}
	items, err := s.collect("comments", block.Output.From, vars)
	if err != nil {
		return nil, err
	}
	comments := make([]source.Danmaku, 0, len(items))
	for index, item := range items {
		if len(comments) >= limit {
			break
		}
		sc := itemScope(item, index+1, vars)
		text := strings.TrimSpace(s.evalString(field(block.Output.Fields, "text"), sc))
		if text == "" {
			continue
		}
		// The upstream answers a whole title's comments, not one episode's,
		// so a mapped episode keeps only the comments sent on it; a rule that
		// maps none keeps everything it got.
		if want := s.evalString(field(block.Output.Fields, "episode"), sc); want != "" && want != episode.ID {
			continue
		}
		comment := source.Danmaku{
			Text:   text,
			Author: s.evalString(field(block.Output.Fields, "author"), sc),
		}
		if value, ok := s.eval(field(block.Output.Fields, "at"), sc); ok {
			if seconds, ok := toInt(value); ok {
				comment.At = time.Duration(seconds.(int)) * time.Second
			}
		}
		if value, ok := s.eval(field(block.Output.Fields, "color"), sc); ok {
			if color, ok := toInt(value); ok {
				comment.Color = uint32(color.(int)) & 0xFFFFFF
			}
		}
		comments = append(comments, comment)
	}
	sort.SliceStable(comments, func(i, j int) bool { return comments[i].At < comments[j].At })
	return comments, nil
}

func (s *sourceImpl) runSteps(ctx context.Context, op string, steps []rule.Step, vars map[string]any) (map[string]any, error) {
	return s.runStepsWith(ctx, op, steps, vars, nil)
}

// runStepsWith runs a step pipeline with query values this call overrides,
// such as the listing intent a keyword turns from browse to search.
func (s *sourceImpl) runStepsWith(ctx context.Context, op string, steps []rule.Step, vars map[string]any, overrides map[string]string) (map[string]any, error) {
	if vars == nil {
		vars = map[string]any{}
	}
	for index, step := range steps {
		site := callSite{op: op, step: fmt.Sprintf("%s.steps[%d]", op, index)}
		if !step.When.IsZero() {
			value, ok := s.eval(step.When, scope{vars: vars, overrides: overrides})
			if !ok || !toBool(value) {
				// A step that does not apply binds nothing, so a later step
				// that names the variable sees it as absent and says so.
				continue
			}
		}
		switch step.Op {
		case rule.OpRequest:
			result, err := s.fetch(ctx, site, step.Request, scope{vars: vars, overrides: overrides})
			if err != nil {
				return nil, err
			}
			vars[step.As] = result
		case rule.OpPaginate:
			items, err := s.runPaginate(ctx, site, step, vars, overrides)
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

func (s *sourceImpl) runPaginate(ctx context.Context, site callSite, step rule.Step, vars map[string]any, overrides map[string]string) ([]any, error) {
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
		response, err := s.fetch(ctx, site, step.Request, scope{vars: local, overrides: overrides})
		if err != nil {
			return nil, err
		}
		items := s.resolveText(step.Items, response)
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

// collect resolves the output selector. A `from` may name several candidates
// separated by `|`, which is how a block reads whichever branch of a choice
// actually ran; the first candidate that resolves wins.
func (s *sourceImpl) collect(op, from string, vars map[string]any) ([]any, error) {
	value, ok := resolveCandidates(from, vars)
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
// wrapper type to say which one the upstream sent. An array of arrays is
// flattened one level, which is what a wildcard that collected one list per
// branch yields.
func asItems(value any) []any {
	if list, ok := value.([]any); ok {
		nested := false
		for _, entry := range list {
			if _, isList := entry.([]any); !isList {
				nested = false
				break
			}
			nested = true
		}
		if !nested {
			return list
		}
		flat := make([]any, 0, len(list))
		for _, entry := range list {
			flat = append(flat, entry.([]any)...)
		}
		return flat
	}
	if value == nil {
		return nil
	}
	return []any{value}
}

// resolveText resolves selector text. Rules that carry no call-time index, such
// as the lines of a resolve block, name their list plainly and go through here.
func (s *sourceImpl) resolveText(selectorText string, root any) []any {
	value, ok := resolveCandidates(selectorText, root)
	if !ok {
		return nil
	}
	return asItems(value)
}

// resolveCandidates reads a `from` that may list several selectors separated by
// `|`, returning the first one that resolves. A source uses it when the same
// block reads two response shapes, such as a keyword endpoint and a category
// page that carry their cards under different keys.
func resolveCandidates(selectorText string, root any) (any, bool) {
	for _, candidate := range strings.Split(selectorText, "|") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			continue
		}
		selector, err := rule.ParseSelector(candidate)
		if err != nil {
			continue
		}
		if value, ok := selector.Resolve(root); ok {
			return value, true
		}
	}
	return nil, false
}

// resolveItems resolves a selector into the list its mapping walks: an array
// survives as itself, and a nested result (when the selector ends on a wildcard
// that collected a list per branch) is flattened one level into that list.
func (s *sourceImpl) resolveItems(selector rule.Selector, root any) []any {
	value, ok := selector.Resolve(root)
	if !ok {
		return nil
	}
	return asItems(value)
}

func (s *sourceImpl) mapCards(shape *rule.Shape, root any, vars map[string]any) []source.Card {
	items := s.resolveText(shape.From, root)
	cards := make([]source.Card, 0, len(items))
	for index, item := range items {
		card := cardOf(s.rule.ID, shape.Fields, itemScope(item, index+1, vars), s)
		if card.ID == "" {
			continue
		}
		cards = append(cards, card)
	}
	return cards
}

// cardOf maps one upstream item onto a card. Listings, home sections and day
// schedules share the mapping, so a field means the same thing everywhere.
func cardOf(id string, fields map[string]rule.Value, sc scope, s *sourceImpl) source.Card {
	card := source.Card{
		Ref:         source.Ref{Source: id, ID: s.evalString(field(fields, "id"), sc)},
		Title:       s.evalString(field(fields, "title"), sc),
		Kind:        kindOf(s.evalString(field(fields, "kind"), sc)),
		Area:        s.evalString(field(fields, "area"), sc),
		Tags:        s.evalList(field(fields, "tags"), sc),
		Remarks:     s.evalString(field(fields, "remarks"), sc),
		Overview:    s.evalString(field(fields, "overview"), sc),
		PosterURL:   s.evalString(field(fields, "posterUrl"), sc),
		BackdropURL: s.evalString(field(fields, "backdropUrl"), sc),
	}
	if value, ok := s.eval(field(fields, "year"), sc); ok {
		if number, ok := toInt(value); ok {
			card.Year = number.(int)
		}
	}
	return card
}

// evalPeople reads a roster field: each entry of the list keeps the name and
// the role the upstream named, so the page can say "饰 苏小满" beside an actor
// instead of repeating a bare name. Entry keys are the ones the rule maps:
// name, role and image. An entry that names no one is dropped, and an empty
// role stays empty rather than becoming an empty caption.
func (s *sourceImpl) evalPeople(value rule.Value, sc scope) []source.CastMember {
	resolved, ok := s.eval(value, sc)
	if !ok {
		return nil
	}
	entries, ok := resolved.([]any)
	if !ok {
		return nil
	}
	out := make([]source.CastMember, 0, len(entries))
	for _, entry := range entries {
		fields, ok := entry.(map[string]any)
		if !ok {
			// A roster of plain names still reads: the list carries who, not
			// what they did.
			if name := toString(entry); name != "" {
				out = append(out, source.CastMember{Name: name})
			}
			continue
		}
		member := source.CastMember{
			Name:  toString(fields["name"]),
			Role:  toString(fields["role"]),
			Image: toString(fields["image"]),
		}
		if member.Name == "" {
			continue
		}
		out = append(out, member)
	}
	if len(out) == 0 {
		return nil
	}
	return out
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

// intentOverride decides the intent a listing sends. A rule that declares no
// search intent keeps whatever its query says, which is how an upstream with a
// single listing mode stays untouched. With one declared, a keyword swaps the
// browse intent for the search intent and a browse drops the parameter, since
// the upstream only matches keywords under its search intent and rejects both
// values at once.
func intentOverride(searchIntent, keyword string) map[string]string {
	if searchIntent == "" {
		return nil
	}
	if keyword == "" {
		return map[string]string{"intent": ""}
	}
	return map[string]string{"intent": searchIntent}
}
