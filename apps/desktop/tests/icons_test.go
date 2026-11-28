package tests

import (
	"testing"

	"luvia/apps/desktop/internal/icons"
)

func TestIconsParseAndWeighed(t *testing.T) {
	if got := icons.Parse("does-not-exist"); got != nil {
		t.Fatalf("Parse(unknown) = %v, want nil", got)
	}
	svg1 := icons.Parse("home-linear")
	if svg1 == nil {
		t.Fatal("Parse(home-linear) = nil, want embedded SVG")
	}
	if svg2 := icons.Parse("home-linear"); svg2 != svg1 {
		t.Fatal("Parse(home-linear) did not return cached pointer")
	}

	if got := icons.Weighed("does-not-exist", 1.35); got != nil {
		t.Fatalf("Weighed(unknown) = %v, want nil", got)
	}
	w1 := icons.Weighed("play-bold", 1.35)
	if w1 == nil {
		t.Fatal("Weighed(play-bold, 1.35) = nil, want SVG")
	}
	if w2 := icons.Weighed("play-bold", 1.35); w2 != w1 {
		t.Fatal("Weighed(play-bold, 1.35) did not return cached pointer")
	}
}
