// Package rule defines the on-disk source rule format: the JSON schema, strict
// decoding, selector/template syntax and structural validation. It knows the
// shape of a rule but nothing about HTTP or the player contract, so rules can be
// loaded, validated and reported on without a running engine.
package rule

// SchemaVersion is the only rule format this build understands.
const SchemaVersion = 1

// Rule is one upstream described as data.
type Rule struct {
	SchemaVersion int    `json:"schemaVersion"`
	ID            string `json:"id"`
	Name          string `json:"name"`
	BaseURL       string `json:"baseURL"`
	HTTP          HTTP   `json:"http,omitempty"`
	Sign          *Sign  `json:"sign,omitempty"`

	Home     *Block        `json:"home,omitempty"`
	List     *ListBlock    `json:"list,omitempty"`
	Updates  *UpdatesBlock `json:"updates,omitempty"`
	Detail   *Block        `json:"detail,omitempty"`
	Episodes *Block        `json:"episodes,omitempty"`
	Resolve  *ResolveBlock `json:"resolve,omitempty"`
	Comments *Block        `json:"comments,omitempty"`
	// Manifest reads the pointer an upstream hands out for a title's pinned
	// comments, which is where an upstream that ships them as dated segments
	// says where to fetch each one from.
	Manifest *Block `json:"manifest,omitempty"`
	// Segments turns a segment document — one window of a long title's
	// comments, far too big to be read as a single list — into the comments
	// the player lays over the picture.
	Segments *Block `json:"segments,omitempty"`
}

// HTTP holds transport policy. Zero values are replaced by engine defaults
// during decoding; a rule only overrides what it needs.
type HTTP struct {
	TimeoutMs   int      `json:"timeoutMs,omitempty"`
	Retries     *int     `json:"retries,omitempty"`
	Concurrency int      `json:"concurrency,omitempty"`
	MaxPages    int      `json:"maxPages,omitempty"`
	Headers     []Header `json:"headers,omitempty"`
}

// Defaults used when a rule leaves transport policy unset.
const (
	DefaultTimeoutMs   = 30000
	DefaultRetries     = 2
	DefaultConcurrency = 4
	DefaultMaxPages    = 20
	DefaultPageSize    = 24
	// DefaultMaxAttempts caps how many line options one resolve call may try.
	DefaultMaxAttempts = 12
)

// Header is one literal request header. Values may use templates.
type Header struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// Sign describes a request signature. The engine signs with HMAC-SHA256 over
// Message and exposes the digest and its inputs through Headers.
type Sign struct {
	Secret        string   `json:"secret"`
	Message       string   `json:"message"`
	TimestampUnit string   `json:"timestampUnit,omitempty"`
	NonceBytes    int      `json:"nonceBytes,omitempty"`
	Headers       []Header `json:"headers"`
}

// Block is one operation: a step pipeline plus the mapping of its result. A
// segments block also carries the list its steps run over, one window at a
// time, which is the only block that is not a single request.
type Block struct {
	Steps  []Step    `json:"steps"`
	Output Output    `json:"output"`
	List   *ListSpec `json:"list,omitempty"`
}

// ListBlock is the listing operation, which also maps player kinds onto the
// upstream's own category values.
type ListBlock struct {
	KindValues map[string]string `json:"kindValues,omitempty"`
	// SearchIntent is the intent value the upstream needs before it matches a
	// keyword. It replaces the query parameter of that name for a search and
	// disappears from a browse, so the two modes never carry both.
	SearchIntent string `json:"searchIntent,omitempty"`
	Steps        []Step `json:"steps"`
	Output       Output `json:"output"`
}

// UpdatesBlock is the weekday schedule: the titles a source reports as
// updating on one day of the current week.
type UpdatesBlock struct {
	// KindValues maps the player kinds onto the upstream's own day filter,
	// exactly as ListBlock does for listings.
	KindValues map[string]string `json:"kindValues,omitempty"`
	// Days maps a card onto the weekday it belongs to, 1 being Monday. An
	// upstream that answers one day per request keeps it constant through a
	// literal so its selector vocabulary stays unchanged.
	Days   Value  `json:"days"`
	Steps  []Step `json:"steps"`
	Output Output `json:"output"`
}

// ResolveBlock turns an episode into playback addresses. Lines is mandatory:
// the block must map the reported lines before one of them is exchanged.
type ResolveBlock struct {
	Steps []Step `json:"steps"`
	Lines Lines  `json:"lines"`
}

// Lines maps the upstream line array and optionally exchanges a ticket for the
// final address.
type Lines struct {
	From        string           `json:"from"`
	Fields      map[string]Value `json:"fields"`
	Exchange    *Exchange        `json:"exchange,omitempty"`
	ExpiresIn   Value            `json:"expiresIn,omitempty"`
	MaxAttempts int              `json:"maxAttempts,omitempty"`
	// PreferURLKind makes the engine walk the remaining lines until one yields
	// this url kind (mp4 or hls). Without it the first playable line wins.
	PreferURLKind string `json:"preferUrlKind,omitempty"`
	// List compresses an answer that names every part in one document into
	// the parts the engine reads through, so a block need not know whether
	// the upstream paginates its episodes.
	List *ListSpec `json:"list,omitempty"`
}

// ListSpec names the array a flattened answer keeps its parts in.
//
// An upstream that answers one document per window of comments hands out the
// windows as a list; the engine reads them one by one, each a request of its
// own, so the rule describes one window instead of repeating itself N times.
//
// The upstream's file may be served without its content encoding honoured by
// anything but a browser, and a window may be sent either way, so a body that
// arrives compressed as gzip or deflate is unpacked before it is parsed. A body
// that arrives as plain JSON is read as it stands, because an upstream that
// names its windows `.json.gz` and serves them uncompressed is the normal case
// rather than a broken one.
type ListSpec struct {
	// From is the selector the step's answer resolves to, for the array of
	// windows.
	From string `json:"from"`
	// URL is the mapping of one window's address, relative to the document the
	// request asked for.
	URL Value `json:"url"`
	// Count is the mapping of how many comments a window holds, so the engine
	// stops before it has read more than the caller asked for.
	Count Value `json:"count,omitempty"`
	// MaxSegments caps how many windows one call reads.
	MaxSegments int `json:"maxSegments,omitempty"`
}

// Exchange is the second phase of line resolution: a ticket from the first
// response is traded for a playable address.
type Exchange struct {
	Ticket  Value   `json:"ticket"`
	Request Request `json:"request"`
	URL     string  `json:"url"`
	URLKind Value   `json:"urlKind,omitempty"`
}

// Request describes one HTTP call. Path, query, headers and body values may use
// templates over the variables bound by earlier steps.
type Request struct {
	Method  string           `json:"method,omitempty"`
	Path    string           `json:"path"`
	Query   []Pair           `json:"query,omitempty"`
	Headers []Header         `json:"headers,omitempty"`
	Body    map[string]Value `json:"body,omitempty"`
	Sign    *bool            `json:"sign,omitempty"`
	// Response names the shape the answer arrives in. Empty and "json" read
	// the body as JSON; "routerData" reads a page whose JSON sits in a
	// `_ROUTER_DATA = {...}` assignment, which is how a server-rendered site
	// ships the same payload it serves over its JSON endpoints.
	Response string `json:"response,omitempty"`
}

// Response shapes a request may declare.
const (
	ResponseJSON       = "json"
	ResponseRouterData = "routerData"
)

// Pair is one ordered query parameter. Values are templates so a constant
// cannot be mistaken for a selector; pairs that render empty are omitted,
// which is how optional filters such as keyword and kind disappear.
type Pair struct {
	Key      string `json:"key"`
	Value    string `json:"value"`
	OmitWhen Value  `json:"omitWhen,omitempty"`
}

// Step is one element of a pipeline. Exactly one operation is chosen by Op.
type Step struct {
	Op string `json:"op"`
	As string `json:"as,omitempty"`
	Request

	// When skips the step unless the value is true. A step that carries no
	// condition always runs, which is how a block picks between the request
	// that fits a keyword search and the one that browses a category.
	When Value `json:"when,omitempty"`

	// paginate only.
	Mode     string `json:"mode,omitempty"`
	Size     int    `json:"size,omitempty"`
	Start    int    `json:"start,omitempty"`
	MaxPages int    `json:"maxPages,omitempty"`
	Items    string `json:"items,omitempty"`
	HasMore  string `json:"hasMore,omitempty"`
}

// Step operations.
const (
	OpRequest  = "request"
	OpPaginate = "paginate"
)

// Output maps the bound variables of a block onto the player contract.
type Output struct {
	From    string           `json:"from"`
	Fields  map[string]Value `json:"fields,omitempty"`
	Page    Value            `json:"page,omitempty"`
	HasMore Value            `json:"hasMore,omitempty"`
	Cards   *Shape           `json:"cards,omitempty"`
}

// Shape maps an array of upstream items onto cards.
type Shape struct {
	From   string           `json:"from"`
	Fields map[string]Value `json:"fields"`
}
