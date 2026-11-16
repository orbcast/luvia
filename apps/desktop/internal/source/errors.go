package source

import (
	"errors"
	"fmt"
)

// ErrorKind classifies a failure so the presentation layer can localize it
// without parsing upstream text.
type ErrorKind string

const (
	// ErrNetwork means the upstream could not be reached or answered 5xx.
	ErrNetwork ErrorKind = "network"
	// ErrTimeout means the request ran out of time or the context deadline hit.
	ErrTimeout ErrorKind = "timeout"
	// ErrCanceled means the caller canceled the request.
	ErrCanceled ErrorKind = "canceled"
	// ErrUnauthorized means the upstream rejected the request credentials.
	ErrUnauthorized ErrorKind = "unauthorized"
	// ErrNotFound means the requested media, episode or line does not exist.
	ErrNotFound ErrorKind = "not_found"
	// ErrRateLimited means the upstream asked the caller to slow down.
	ErrRateLimited ErrorKind = "rate_limited"
	// ErrInvalidResponse means the upstream answered something unusable.
	ErrInvalidResponse ErrorKind = "invalid_response"
	// ErrInvalidRule means the rule does not describe a valid request or
	// mapping. It is a rule authoring bug, not a transient failure.
	ErrInvalidRule ErrorKind = "invalid_rule"
	// ErrNoPlayableLine means no reported line produced a playable address.
	ErrNoPlayableLine ErrorKind = "no_playable_line"
	// ErrUnsupported means the source has no block for this operation.
	ErrUnsupported ErrorKind = "unsupported"
)

// Error is a source failure with classification and rule diagnostics.
//
// Kind is safe to show as UI text through locales. Detail carries upstream
// diagnostics (status code, response excerpt, offending selector) and must not
// be shown verbatim to users.
type Error struct {
	Kind   ErrorKind
	Source string
	Op     string
	Step   string
	Detail string
	Cause  error
}

func (e *Error) Error() string {
	msg := fmt.Sprintf("source %q %s: %s", e.Source, e.Op, e.Kind)
	if e.Step != "" {
		msg += " at " + e.Step
	}
	if e.Detail != "" {
		msg += ": " + e.Detail
	}
	if e.Cause != nil {
		msg += ": " + e.Cause.Error()
	}
	return msg
}

// Unwrap exposes the underlying transport or decoding error.
func (e *Error) Unwrap() error { return e.Cause }

// KindOf reports the classified kind of err, if it is a source error.
func KindOf(err error) (ErrorKind, bool) {
	var target *Error
	if errors.As(err, &target) {
		return target.Kind, true
	}
	return "", false
}

// IsKind reports whether err is a source error of kind.
func IsKind(err error, kind ErrorKind) bool {
	got, ok := KindOf(err)
	return ok && got == kind
}
