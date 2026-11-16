package source

import (
	"errors"
	"testing"
)

func TestRefString(t *testing.T) {
	if got := (Ref{Source: "kanju", ID: "abc"}).String(); got != "kanju:abc" {
		t.Fatalf("String() = %q", got)
	}
	if got := (Ref{ID: "abc"}).String(); got != "abc" {
		t.Fatalf("String() without a source = %q", got)
	}
	if got := (Ref{Source: "kanju"}).String(); got != "kanju:" {
		t.Fatalf("String() without an id = %q", got)
	}
	if !(Ref{}).IsZero() || (Ref{ID: "a"}).IsZero() {
		t.Fatal("IsZero must report empty refs only")
	}
}

func TestKindSet(t *testing.T) {
	kinds := Kinds()
	if len(kinds) != 4 {
		t.Fatalf("Kinds() = %v", kinds)
	}
	for _, kind := range kinds {
		if !kind.Valid() {
			t.Fatalf("%q must be valid", kind)
		}
	}
	if Kind("cartoon").Valid() || Kind("").Valid() {
		t.Fatal("unknown kinds must be invalid")
	}
}

func TestErrorFormattingAndClassification(t *testing.T) {
	cause := errors.New("dial failed")
	plain := &Error{Kind: ErrNetwork, Source: "kanju", Op: "home", Detail: "503", Cause: cause}
	if got, want := plain.Error(), `source "kanju" home: network: 503: dial failed`; got != want {
		t.Fatalf("Error() = %q", got)
	}
	if !errors.Is(plain, cause) {
		t.Fatal("Error must unwrap to its cause")
	}
	withStep := &Error{Kind: ErrInvalidResponse, Source: "kanju", Op: "episodes", Step: "episodes.steps[0]"}
	if got, want := withStep.Error(), `source "kanju" episodes: invalid_response at episodes.steps[0]`; got != want {
		t.Fatalf("Error() = %q", got)
	}
	wrapped := errors.New("wrapper: " + withStep.Error())
	if _, ok := KindOf(wrapped); ok {
		t.Fatal("a plain error must not classify")
	}
	if kind, ok := KindOf(wrap(withStep)); !ok || kind != ErrInvalidResponse {
		t.Fatalf("KindOf = %v (%v)", kind, ok)
	}
	if !IsKind(wrap(withStep), ErrInvalidResponse) || IsKind(withStep, ErrTimeout) || IsKind(wrapped, ErrTimeout) {
		t.Fatal("IsKind must compare kinds only")
	}
}

func wrap(err error) error { return &wrappedError{inner: err} }

type wrappedError struct{ inner error }

func (w *wrappedError) Error() string { return w.inner.Error() }
func (w *wrappedError) Unwrap() error { return w.inner }

// fakeSource embeds the interface so the contract stays satisfied while the
// test only needs identity.
type fakeSource struct {
	Source
	id, name string
}

func (f fakeSource) ID() string   { return f.id }
func (f fakeSource) Name() string { return f.name }

func TestDescribe(t *testing.T) {
	if got := Describe(fakeSource{id: "kanju", name: "看剧"}); got != "看剧 (kanju)" {
		t.Fatalf("Describe() = %q", got)
	}
}
