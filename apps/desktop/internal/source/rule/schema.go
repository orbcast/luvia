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
	Detail   *Block        `json:"detail,omitempty"`
	Episodes *Block        `json:"episodes,omitempty"`
	Resolve  *ResolveBlock `json:"resolve,omitempty"`
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

// Block is one operation: a step pipeline plus the mapping of its result.
type Block struct {
	Steps  []Step `json:"steps"`
	Output Output `json:"output"`
}

// ListBlock is the listing operation, which also maps player kinds onto the
// upstream's own category values.
type ListBlock struct {
	KindValues map[string]string `json:"kindValues,omitempty"`
	Steps      []Step            `json:"steps"`
	Output     Output            `json:"output"`
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
}

// Pair is one ordered query parameter. Pairs whose value renders empty are
// omitted, which is how optional filters such as keyword and kind disappear.
type Pair struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Step is one element of a pipeline. Exactly one operation is chosen by Op.
type Step struct {
	Op string `json:"op"`
	As string `json:"as,omitempty"`
	Request

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
