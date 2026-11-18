// Package source defines the contract between the player and the upstream
// sources it reads from. It carries no HTTP, JSON or rule knowledge: drivers
// live in the rule, engine and loader packages and only have to satisfy the
// Source interface.
package source

import (
	"context"
	"fmt"
	"time"
)

// Kind is the closed set of media categories the player understands. Upstream
// granularity that falls outside the set belongs in Card.Tags.
type Kind string

const (
	Movie   Kind = "movie"
	Series  Kind = "series"
	Anime   Kind = "anime"
	Variety Kind = "variety"
)

// Kinds lists every valid Kind in presentation order.
func Kinds() []Kind {
	return []Kind{Movie, Series, Anime, Variety}
}

// Valid reports whether k is one of the known kinds.
func (k Kind) Valid() bool {
	switch k {
	case Movie, Series, Anime, Variety:
		return true
	default:
		return false
	}
}

// Ref identifies one media item on one source. Source is the stable source
// identifier, ID is whatever the upstream uses natively; both are needed
// because ids are only unique inside their source.
type Ref struct {
	Source string
	ID     string
}

// IsZero reports whether the reference carries no identity at all.
func (r Ref) IsZero() bool { return r.Source == "" && r.ID == "" }

// String returns the durable "source:id" form used for progress, favorites and
// cache keys. It is a display/persistence helper, not an input for parsing.
func (r Ref) String() string {
	switch {
	case r.Source == "":
		return r.ID
	case r.ID == "":
		return r.Source + ":"
	default:
		return r.Source + ":" + r.ID
	}
}

// Card is the compact item shown in home sections, grids and search results.
type Card struct {
	Ref
	Title       string
	Kind        Kind
	Year        int
	Area        string
	Tags        []string
	Remarks     string
	Overview    string
	PosterURL   string
	BackdropURL string
}

// Section is one named group of cards on the home page.
type Section struct {
	ID       string
	Title    string
	Subtitle string
	Cards    []Card
}

// Home is the whole home page contributed by one source.
type Home struct {
	Sections []Section
}

// ListQuery asks for one page of a browse or search listing. A query with a
// Keyword searches, an empty Keyword lists the latest catalog.
type ListQuery struct {
	Keyword string
	Kind    Kind
	Page    int
}

// Page is one page of listing results.
type Page struct {
	Cards   []Card
	Page    int
	HasMore bool
}

// Day asks for the titles a source reports as updating on one weekday of the
// week in progress. Weekday is Monday-based, 1 through 7; a zero value means
// "every day", which the engine reports as ErrInvalidRule because no rule can
// answer it.
type Day struct {
	Weekday int
	Kind    Kind
}

// Update is one title a source schedules for a day, with the status line that
// explains why it is there ("updated to episode 26" and the like).
type Update struct {
	Card
	Note string
}

// Detail is the full record for one media item.
type Detail struct {
	Ref
	Title        string
	Kind         Kind
	Year         int
	Area         string
	Tags         []string
	Overview     string
	PosterURL    string
	BackdropURL  string
	Remarks      string
	Actors       []string
	Directors    []string
	EpisodeCount int
}

// Episode is one playable unit. Payload belongs to the source: it carries
// whatever the upstream needs later, for example a playback token, and the
// caller must treat it as opaque.
type Episode struct {
	Media   Ref
	ID      string
	Number  int
	Title   string
	Payload string
}

// Line is one playback route offered for an episode.
type Line struct {
	ID      string
	Label   string
	URLKind string
	Default bool
}

// Playback is a resolved media address plus the lines the source reported, so
// the caller can offer switching without resolving again from scratch.
type Playback struct {
	URL       string
	URLKind   string
	Line      Line
	Lines     []Line
	ExpiresIn time.Duration
}

// Source is one upstream the player can read from.
type Source interface {
	// ID returns the stable identifier used in Ref.Source.
	ID() string
	// Name returns the human readable upstream name.
	Name() string

	// Home returns the curated home page. ErrUnsupported means the source has
	// no home block.
	Home(ctx context.Context) (Home, error)
	// List returns one page of browse or search results.
	List(ctx context.Context, query ListQuery) (Page, error)
	// Updates returns the titles a source reports as updating on day. A
	// source without an updates block returns ErrUnsupported, and a caller
	// then shows nothing rather than making a calendar up.
	Updates(ctx context.Context, day Day) ([]Update, error)
	// Detail returns the full record for ref.
	Detail(ctx context.Context, ref Ref) (Detail, error)
	// Episodes returns every episode of ref, already flattened.
	Episodes(ctx context.Context, ref Ref) ([]Episode, error)
	// Resolve returns a playable address for episode. lineID is a preference;
	// an empty value lets the source pick. It returns ErrNotFound when the
	// requested line does not exist.
	Resolve(ctx context.Context, episode Episode, lineID string) (Playback, error)
}

// Describe is a small helper for diagnostics.
func Describe(s Source) string { return fmt.Sprintf("%s (%s)", s.Name(), s.ID()) }
