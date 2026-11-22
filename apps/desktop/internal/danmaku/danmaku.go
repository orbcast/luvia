// Package danmaku finds comments for a title somewhere other than the source
// rule it was opened from.
//
// A source rule describes a catalogue: what it lists, how it plays, what its
// pages carry. Plenty of catalogues carry no comments at all, and the ones that
// do often carry reviews rather than comments pinned to a moment, which cannot
// be laid over the picture. So the layer over the stage is fed from here
// instead, by matching the title against a site that does pin its comments.
//
// A lookup reports a reason when it finds nothing, so the player can say why
// the layer is empty instead of leaving the viewer to guess.
package danmaku

import (
	"context"

	"luvia/apps/desktop/internal/source"
)

// Reason says why a lookup produced no comments. It is a closed set rather
// than a message: the player owns the words, this package owns the finding.
type Reason int

const (
	// ReasonReady means the lookup found comments.
	ReasonReady Reason = iota
	// ReasonNoMatch means nothing the site holds looked like this title.
	ReasonNoMatch
	// ReasonNoEpisode means a title matched but carries no page for this
	// episode, so its comments belong to a different cut of the show.
	ReasonNoEpisode
	// ReasonEmpty means the matched page carries no comments of its own.
	ReasonEmpty
	// ReasonFailed means the lookup could not be completed.
	ReasonFailed
)

// Result is what a lookup found.
type Result struct {
	// Items are the comments, oldest first. They are empty when Reason is
	// anything but ReasonReady.
	Items []source.Danmaku
	// Reason says why Items is empty, or ReasonReady when it is not.
	Reason Reason
}

// Source finds the comments of a title's episode.
type Source interface {
	// Lookup returns the comments of episode number episode of the title.
	// An empty Result is not an error: a title may simply not be there.
	Lookup(ctx context.Context, title string, episode int) (Result, error)
}
