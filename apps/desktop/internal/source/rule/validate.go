package rule

import (
	"fmt"
	"net/url"
	"strings"
)

type functionSpec struct {
	minArgs int
	maxArgs int
}

// KnownFunctions is the closed function table available to rules. Engine owns
// the implementations; this package owns the names and arities so validation
// rejects unknown calls before anything runs.
var KnownFunctions = map[string]functionSpec{
	"coalesce":   {minArgs: 1, maxArgs: 0},
	"trimPrefix": {minArgs: 2, maxArgs: 2},
	"trimSuffix": {minArgs: 2, maxArgs: 2},
	"trim":       {minArgs: 1, maxArgs: 1},
	"lower":      {minArgs: 1, maxArgs: 1},
	"upper":      {minArgs: 1, maxArgs: 1},
	"int":        {minArgs: 1, maxArgs: 1},
	"seconds":    {minArgs: 1, maxArgs: 1},
	"kind":       {minArgs: 1, maxArgs: 1},
	"urlKind":    {minArgs: 1, maxArgs: 1},
	"list":       {minArgs: 1, maxArgs: 1},
	"first":      {minArgs: 1, maxArgs: 1},
	"last":       {minArgs: 1, maxArgs: 1},
	"empty":      {minArgs: 1, maxArgs: 1},
	"not":        {minArgs: 1, maxArgs: 1},
}

// FunctionKnown reports whether name is part of the function table.
func FunctionKnown(name string) bool {
	_, ok := KnownFunctions[name]
	return ok
}

type outputKind string

const (
	outSections outputKind = "sections"
	outPage     outputKind = "page"
	outDetail   outputKind = "detail"
	outEpisodes outputKind = "episodes"
	outUpdates  outputKind = "updates"
	outComments outputKind = "comments"
	outManifest outputKind = "manifest"
)

var cardFields = fieldSet("id", "title", "kind", "year", "area", "tags", "remarks", "overview",
	"posterUrl", "backdropUrl")
var detailFields = fieldSet("id", "title", "kind", "year", "area", "tags", "overview", "posterUrl",
	"backdropUrl", "remarks", "actors", "directors", "episodeCount")
var episodeFields = fieldSet("id", "number", "title", "payload")
var sectionFields = fieldSet("id", "title", "subtitle")
var updateFields = fieldSet("id", "title", "kind", "year", "area", "tags", "remarks", "overview",
	"posterUrl", "backdropUrl", "note")
var lineFields = fieldSet("id", "label", "urlKind", "url", "ticket", "default")
var commentFields = fieldSet("at", "text", "author", "color", "episode")
var manifestFields = fieldSet("source")

var outputFields = map[outputKind]map[string]bool{
	outSections: sectionFields,
	outPage:     cardFields,
	outDetail:   detailFields,
	outEpisodes: episodeFields,
	outUpdates:  updateFields,
	outComments: commentFields,
	outManifest: manifestFields,
}

var builtinVars = map[string][]string{
	"home":     nil,
	"list":     {"page", "keyword", "kind"},
	"updates":  {"weekday", "kind"},
	"detail":   {"id"},
	"episodes": {"id"},
	"resolve":  {"id", "payload", "media", "lineId"},
	"comments": {"id", "payload"},
	"manifest": {"id", "payload"},
	"segments": {"id", "payload", "source", "document"},
}

func fieldSet(names ...string) map[string]bool {
	set := make(map[string]bool, len(names))
	for _, name := range names {
		set[name] = true
	}
	return set
}

// Validate checks the whole rule: identity, transport policy, every step and
// every mapping. Rule authoring mistakes surface here, not as empty cards.
func (r *Rule) Validate() error {
	if r.SchemaVersion != SchemaVersion {
		return fmt.Errorf("schemaVersion must be %d, got %d", SchemaVersion, r.SchemaVersion)
	}
	if !isName(r.ID) {
		return fmt.Errorf("id %q is not a usable source identifier", r.ID)
	}
	if strings.TrimSpace(r.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if err := validateBaseURL(r.BaseURL); err != nil {
		return err
	}
	if r.Sign != nil {
		if err := validateSign(r.Sign); err != nil {
			return err
		}
	}
	if r.Home == nil && r.List == nil && r.Updates == nil && r.Detail == nil && r.Episodes == nil && r.Resolve == nil && r.Comments == nil && r.Manifest == nil {
		return fmt.Errorf("rule declares no operation block")
	}
	if r.Home != nil {
		if err := validateBlock("home", r.Home.Steps, r.Home.Output, outSections); err != nil {
			return err
		}
	}
	if r.List != nil {
		if err := validateKindValues(r.List.KindValues); err != nil {
			return err
		}
		if err := validateBlock("list", r.List.Steps, r.List.Output, outPage); err != nil {
			return err
		}
	}
	if r.Updates != nil {
		if err := validateKindValues(r.Updates.KindValues); err != nil {
			return err
		}
		if r.Updates.Days.IsZero() {
			return fmt.Errorf("updates.days is required")
		}
		if err := validateBlock("updates", r.Updates.Steps, r.Updates.Output, outUpdates); err != nil {
			return err
		}
	}
	if r.Detail != nil {
		if err := validateBlock("detail", r.Detail.Steps, r.Detail.Output, outDetail); err != nil {
			return err
		}
	}
	if r.Episodes != nil {
		if err := validateBlock("episodes", r.Episodes.Steps, r.Episodes.Output, outEpisodes); err != nil {
			return err
		}
		if r.Episodes.List != nil {
			return fmt.Errorf("episodes must not declare list; only segments runs per window")
		}
	}
	if r.Resolve != nil {
		if err := validateResolve(r.Resolve); err != nil {
			return err
		}
	}
	if r.Comments != nil {
		if err := validateBlock("comments", r.Comments.Steps, r.Comments.Output, outComments); err != nil {
			return err
		}
	}
	if err := validateManifest(r.Manifest, r.Segments); err != nil {
		return err
	}
	return nil
}

// validateManifest checks the pair of blocks that reads an upstream's windowed
// comments: the manifest names where the window list lives, the segments block
// turns one window into comments. They only make sense together, so one without
// the other is a rule mistake rather than an optional shape.
func validateManifest(manifest, segments *Block) error {
	if manifest == nil && segments == nil {
		return nil
	}
	if manifest == nil {
		return fmt.Errorf("segments is declared without a manifest block to point at its windows")
	}
	if segments == nil {
		return fmt.Errorf("manifest is declared without a segments block to read its windows")
	}
	if err := validateBlock("manifest", manifest.Steps, manifest.Output, outManifest); err != nil {
		return err
	}
	if err := validateBlockSteps("segments", segments.Steps, segments.Output, outComments, false); err != nil {
		return err
	}
	return validateList("segments.list", segments.List)
}

// validateList checks a list mapping. The address is the point of it, so both
// it and the array it reads are mandatory; a window count is only there to let
// the engine stop early.
func validateList(label string, list *ListSpec) error {
	if list == nil {
		return fmt.Errorf("%s is required", label)
	}
	if _, err := ParseSelector(strings.TrimSpace(list.From)); err != nil {
		return fmt.Errorf("%s.from: %w", label, err)
	}
	if list.URL.IsZero() {
		return fmt.Errorf("%s.url is required", label)
	}
	if err := validateValue(label+".url", list.URL, nil); err != nil {
		return err
	}
	if !list.Count.IsZero() {
		if err := validateValue(label+".count", list.Count, nil); err != nil {
			return err
		}
	}
	if list.MaxSegments < 0 {
		return fmt.Errorf("%s.maxSegments must not be negative", label)
	}
	return nil
}

func validateBaseURL(raw string) error {
	parsed, err := url.Parse(raw)
	if err != nil {
		return fmt.Errorf("baseURL: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return fmt.Errorf("baseURL must be http or https, got %q", raw)
	}
	if parsed.Host == "" {
		return fmt.Errorf("baseURL needs a host, got %q", raw)
	}
	if parsed.RawQuery != "" || parsed.Fragment != "" {
		return fmt.Errorf("baseURL must not carry a query or fragment")
	}
	return nil
}

func validateKindValues(values map[string]string) error {
	for key := range values {
		switch key {
		case "movie", "series", "anime", "variety":
		default:
			return fmt.Errorf("list.kindValues: %q is not a player kind", key)
		}
	}
	return nil
}

var signVars = fieldSet("method", "path", "timestamp", "nonce")

// namesWholeAddress reports whether a path template is a complete http(s)
// address once its holes are filled, such as "{source}" standing for a file the
// catalogue handed out.
func namesWholeAddress(path string) bool {
	trimmed := strings.TrimSpace(path)
	if strings.Contains(trimmed, "{") {
		// The placeholder is the whole path, so whatever it renders to is what
		// the request asks for; checking it here would mean running the rule.
		return strings.HasPrefix(trimmed, "{") || strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
	}
	return strings.HasPrefix(trimmed, "http://") || strings.HasPrefix(trimmed, "https://")
}

func validateSign(sign *Sign) error {
	if strings.TrimSpace(sign.Secret) == "" {
		return fmt.Errorf("sign.secret is required")
	}
	if err := checkTemplate("sign.message", sign.Message, signVars); err != nil {
		return err
	}
	if sign.TimestampUnit != "ms" && sign.TimestampUnit != "s" {
		return fmt.Errorf("sign.timestampUnit must be ms or s, got %q", sign.TimestampUnit)
	}
	if sign.NonceBytes < 1 || sign.NonceBytes > 64 {
		return fmt.Errorf("sign.nonceBytes must be between 1 and 64, got %d", sign.NonceBytes)
	}
	if len(sign.Headers) == 0 {
		return fmt.Errorf("sign.headers must declare the signature headers")
	}
	signHeaderVars := fieldSet("method", "path", "timestamp", "nonce", "signature")
	for _, header := range sign.Headers {
		if strings.TrimSpace(header.Name) == "" {
			return fmt.Errorf("sign.headers: header name is required")
		}
		if err := checkTemplate("sign.headers."+header.Name, header.Value, signHeaderVars); err != nil {
			return err
		}
	}
	return nil
}

func validateBlock(name string, steps []Step, out Output, kind outputKind) error {
	return validateBlockSteps(name, steps, out, kind, true)
}

// validateBlockSteps checks a block, optionally allowing it to declare no steps
// at all. A segments block reads the window the engine already downloaded, so
// its steps refine that document rather than fetch one; a rule that needs no
// refinement leaves them out.
func validateBlockSteps(name string, steps []Step, out Output, kind outputKind, requireSteps bool) error {
	if requireSteps && len(steps) == 0 {
		return fmt.Errorf("%s.steps must not be empty", name)
	}
	declared := fieldSet(builtinVars[name]...)
	for index := range steps {
		step := steps[index]
		if err := validateStep(name, index, step, declared); err != nil {
			return err
		}
		// A conditional step is one branch of a choice, so it may name a
		// variable an earlier branch also names: only one of them runs.
		if declared[step.As] && step.When.IsZero() {
			return fmt.Errorf("%s.steps[%d]: variable %q is already bound", name, index, step.As)
		}
		declared[step.As] = true
	}
	return validateOutput(name+".output", out, kind, declared)
}

func validateStep(block string, index int, step Step, declared map[string]bool) error {
	label := fmt.Sprintf("%s.steps[%d]", block, index)
	switch step.Op {
	case OpRequest:
	case OpPaginate:
		if step.Mode != "offset" && step.Mode != "page" {
			return fmt.Errorf("%s: paginate mode must be offset or page, got %q", label, step.Mode)
		}
		if step.Size <= 0 {
			return fmt.Errorf("%s: paginate size must be positive", label)
		}
		if step.MaxPages <= 0 {
			return fmt.Errorf("%s: paginate maxPages must be positive", label)
		}
		if step.Items == "" {
			return fmt.Errorf("%s: paginate needs items", label)
		}
		if _, err := ParseSelector(step.Items); err != nil {
			return fmt.Errorf("%s: %w", label, err)
		}
		if step.HasMore != "" {
			if _, err := ParseSelector(step.HasMore); err != nil {
				return fmt.Errorf("%s: %w", label, err)
			}
		}
	default:
		return fmt.Errorf("%s: unknown op %q", label, step.Op)
	}
	if !isName(step.As) {
		return fmt.Errorf("%s: as %q is not a usable variable name", label, step.As)
	}
	scope := copySet(declared)
	if step.Op == OpPaginate {
		scope["page"] = true
		scope["offset"] = true
	}
	if !step.When.IsZero() {
		if err := validateValue(label+".when", step.When, scope); err != nil {
			return err
		}
	}
	return validateRequest(label, step.Request, scope)
}

func validateRequest(label string, req Request, vars map[string]bool) error {
	if req.Path == "" {
		return fmt.Errorf("%s: path is required", label)
	}
	// A path is either rooted at the baseURL or a whole address the template
	// fills in, which is how a step reaches a file server the catalogue names
	// rather than its own host. A bare relative path is neither, so it stays a
	// mistake.
	if !strings.HasPrefix(req.Path, "/") && !namesWholeAddress(req.Path) {
		return fmt.Errorf("%s: path must start with / or be an absolute address, got %q", label, req.Path)
	}
	switch req.Method {
	case "", "GET", "POST", "PUT", "PATCH", "DELETE":
	default:
		return fmt.Errorf("%s: unsupported method %q", label, req.Method)
	}
	switch req.Response {
	case "", ResponseJSON, ResponseRouterData:
	default:
		return fmt.Errorf("%s: unsupported response %q", label, req.Response)
	}
	if err := checkTemplate(label+".path", req.Path, vars); err != nil {
		return err
	}
	for i, pair := range req.Query {
		if strings.TrimSpace(pair.Key) == "" {
			return fmt.Errorf("%s.query[%d]: key is required", label, i)
		}
		if pair.Value == "" {
			return fmt.Errorf("%s.query[%d]: value is required", label, i)
		}
		if err := checkTemplate(fmt.Sprintf("%s.query[%d]", label, i), pair.Value, vars); err != nil {
			return err
		}
		if !pair.OmitWhen.IsZero() {
			if err := validateValue(fmt.Sprintf("%s.query[%d].omitWhen", label, i), pair.OmitWhen, vars); err != nil {
				return err
			}
		}
	}
	for _, header := range req.Headers {
		if strings.TrimSpace(header.Name) == "" {
			return fmt.Errorf("%s.headers: header name is required", label)
		}
		if err := checkTemplate(label+".headers."+header.Name, header.Value, vars); err != nil {
			return err
		}
	}
	for key, value := range req.Body {
		if strings.TrimSpace(key) == "" {
			return fmt.Errorf("%s.body: field name is required", label)
		}
		if err := validateValue(label+".body."+key, value, vars); err != nil {
			return err
		}
	}
	return nil
}

// validateFrom checks a `from` path. A from may list candidates separated by
// `|`, so a block can read two response shapes; every candidate has to stand on
// its own, otherwise a typo in one branch hides behind the other.
func validateFrom(label, from string, vars map[string]bool) error {
	for _, candidate := range strings.Split(from, "|") {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" {
			return fmt.Errorf("%s.from: empty candidate in %q", label, from)
		}
		sel, err := ParseSelector(candidate)
		if err != nil {
			return fmt.Errorf("%s.from: %w", label, err)
		}
		if sel.ReadsItem() {
			// A number-only selector reads the item being walked, so it names
			// no variable and cannot be a typo.
			continue
		}
		if vars != nil && !vars[sel.Root()] {
			return fmt.Errorf("%s.from: unknown variable %q", label, sel.Root())
		}
	}
	return nil
}

func checkTemplate(label, text string, vars map[string]bool) error {
	parsed, err := ParseTemplate(text)
	if err != nil {
		return fmt.Errorf("%s: %w", label, err)
	}
	for _, sel := range parsed.Selectors() {
		if sel.ReadsItem() {
			continue
		}
		root := sel.Root()
		if !vars[root] {
			return fmt.Errorf("%s: unknown variable %q in {%s}", label, root, sel.String())
		}
	}
	return nil
}

func validateOutput(label string, out Output, kind outputKind, vars map[string]bool) error {
	if err := validateFrom(label, out.From, vars); err != nil {
		return err
	}
	allowed := outputFields[kind]
	for key, value := range out.Fields {
		if !allowed[key] {
			return fmt.Errorf("%s.fields: %q is not a %s field", label, key, kind)
		}
		if err := validateValue(label+".fields."+key, value, nil); err != nil {
			return err
		}
	}
	switch kind {
	case outSections:
		if out.Cards == nil {
			return fmt.Errorf("%s: sections need a cards mapping", label)
		}
		if _, ok := out.Fields["title"]; !ok {
			return fmt.Errorf("%s.fields: title is required", label)
		}
		if !out.Page.IsZero() || !out.HasMore.IsZero() {
			return fmt.Errorf("%s: sections must not declare page or hasMore", label)
		}
		return validateShape(label+".cards", out.Cards, cardFields)
	case outPage:
		if out.Cards == nil {
			return fmt.Errorf("%s: page needs a cards mapping", label)
		}
		if len(out.Fields) > 0 {
			return fmt.Errorf("%s.fields: page maps cards, so fields belong under cards", label)
		}
		if !out.Page.IsZero() {
			if err := validateValue(label+".page", out.Page, vars); err != nil {
				return err
			}
		}
		if !out.HasMore.IsZero() {
			if err := validateValue(label+".hasMore", out.HasMore, vars); err != nil {
				return err
			}
		}
		return validateShape(label+".cards", out.Cards, cardFields)
	case outUpdates:
		if out.Cards == nil {
			return fmt.Errorf("%s: updates need a cards mapping", label)
		}
		if len(out.Fields) > 0 {
			return fmt.Errorf("%s.fields: updates map cards, so fields belong under cards", label)
		}
		if !out.Page.IsZero() || !out.HasMore.IsZero() {
			return fmt.Errorf("%s: updates must not declare page or hasMore", label)
		}
		return validateShape(label+".cards", out.Cards, updateFields)
	default:
		if out.Cards != nil {
			return fmt.Errorf("%s: %s must not declare cards", label, kind)
		}
		if !out.Page.IsZero() || !out.HasMore.IsZero() {
			return fmt.Errorf("%s: %s must not declare page or hasMore", label, kind)
		}
		// Each mapping kind names the field it cannot do without: a detail
		// without a title, an episode without an id, a comment without text,
		// a manifest without a source to point at.
		required := "title"
		switch kind {
		case outEpisodes:
			required = "id"
		case outComments:
			required = "text"
		case outManifest:
			required = "source"
		}
		if _, ok := out.Fields[required]; !ok {
			return fmt.Errorf("%s.fields: %s is required", label, required)
		}
		return nil
	}
}

func validateShape(label string, shape *Shape, allowed map[string]bool) error {
	if shape == nil {
		return fmt.Errorf("%s: mapping is required", label)
	}
	if err := validateFrom(label, shape.From, nil); err != nil {
		return err
	}
	if len(shape.Fields) == 0 {
		return fmt.Errorf("%s.fields must not be empty", label)
	}
	for key, value := range shape.Fields {
		if !allowed[key] {
			return fmt.Errorf("%s.fields: %q is not a card field", label, key)
		}
		if err := validateValue(label+".fields."+key, value, nil); err != nil {
			return err
		}
	}
	for _, required := range []string{"id", "title"} {
		if _, ok := shape.Fields[required]; !ok {
			return fmt.Errorf("%s.fields: %s is required", label, required)
		}
	}
	return nil
}

func validateResolve(block *ResolveBlock) error {
	declared := fieldSet(builtinVars["resolve"]...)
	for index := range block.Steps {
		step := block.Steps[index]
		if err := validateStep("resolve", index, step, declared); err != nil {
			return err
		}
		declared[step.As] = true
	}
	lines := block.Lines
	if err := validateFrom("resolve.lines", lines.From, declared); err != nil {
		return err
	}
	for key, value := range lines.Fields {
		if !lineFields[key] {
			return fmt.Errorf("resolve.lines.fields: %q is not a line field", key)
		}
		if err := validateValue("resolve.lines.fields."+key, value, nil); err != nil {
			return err
		}
	}
	for _, required := range []string{"id", "url"} {
		if _, ok := lines.Fields[required]; !ok {
			return fmt.Errorf("resolve.lines.fields: %s is required", required)
		}
	}
	if !lines.ExpiresIn.IsZero() {
		if err := validateValue("resolve.lines.expiresIn", lines.ExpiresIn, declared); err != nil {
			return err
		}
	}
	if lines.MaxAttempts <= 0 {
		return fmt.Errorf("resolve.lines.maxAttempts must be positive")
	}
	if kind := lines.PreferURLKind; kind != "" && kind != "mp4" && kind != "hls" {
		return fmt.Errorf("resolve.lines.preferUrlKind: %q is not mp4 or hls", kind)
	}
	if lines.Exchange == nil {
		return nil
	}
	exchange := lines.Exchange
	if err := validateValue("resolve.lines.exchange.ticket", exchange.Ticket, nil); err != nil {
		return err
	}
	exchangeVars := copySet(declared)
	exchangeVars["ticket"] = true
	if err := validateRequest("resolve.lines.exchange", exchange.Request, exchangeVars); err != nil {
		return err
	}
	if _, err := ParseSelector(exchange.URL); err != nil {
		return fmt.Errorf("resolve.lines.exchange.url: %w", err)
	}
	if !exchange.URLKind.IsZero() {
		if err := validateValue("resolve.lines.exchange.urlKind", exchange.URLKind, nil); err != nil {
			return err
		}
	}
	return nil
}

func validateValue(label string, value Value, vars map[string]bool) error {
	switch value.Kind() {
	case "selector":
		sel, _ := value.Selector()
		if sel.ReadsItem() {
			return nil
		}
		if vars != nil && !vars[sel.Root()] {
			return fmt.Errorf("%s: unknown variable %q", label, sel.Root())
		}
		return nil
	case "template":
		tpl, _ := value.Template()
		if vars == nil {
			return nil
		}
		for _, sel := range tpl.Selectors() {
			if sel.ReadsItem() {
				continue
			}
			if !vars[sel.Root()] {
				return fmt.Errorf("%s: unknown variable %q in {%s}", label, sel.Root(), sel.String())
			}
		}
		return nil
	case "function":
		name, args, _ := value.Func()
		spec, ok := KnownFunctions[name]
		if !ok {
			return fmt.Errorf("%s: unknown function %q", label, name)
		}
		if len(args) < spec.minArgs {
			return fmt.Errorf("%s: %s needs at least %d arguments", label, name, spec.minArgs)
		}
		if spec.maxArgs > 0 && len(args) > spec.maxArgs {
			return fmt.Errorf("%s: %s takes at most %d arguments", label, name, spec.maxArgs)
		}
		for index, arg := range args {
			if err := validateValue(fmt.Sprintf("%s.args[%d]", label, index), arg, vars); err != nil {
				return err
			}
		}
		return nil
	default:
		return nil
	}
}

func copySet(source map[string]bool) map[string]bool {
	out := make(map[string]bool, len(source)+2)
	for key, value := range source {
		out[key] = value
	}
	return out
}

func isName(text string) bool {
	if text == "" || len(text) > 64 {
		return false
	}
	for _, r := range text {
		if r == '_' || (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') {
			continue
		}
		return false
	}
	if text[0] >= '0' && text[0] <= '9' {
		return false
	}
	return true
}
