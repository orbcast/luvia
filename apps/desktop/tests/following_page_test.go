package tests

import (
	"testing"
	"time"

	"github.com/egoist/mygo/i18n"
	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/following"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

// followedShow stores one followed title updated at a fixed day, so the page
// has a wall to draw.
func followedShow(t *testing.T, store *library.Store, id, title string, at time.Time) source.Ref {
	t.Helper()
	t.Helper()
	ref := source.Ref{Source: "fake", ID: id}
	err := store.Follow(library.Follow{
		Media: ref, Title: title, PosterURL: "https://cdn.example/" + id + ".jpg", UpdatedAt: at,
	})
	if err != nil {
		t.Fatal(err)
	}
	return ref
}

// TestFollowingDrawsTheLibrary checks the page against the library it reads:
// the followed shows and the continued titles both appear, and the page names
// the show it holds.
func TestFollowingDrawsTheLibrary(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	at := time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local)
	followedShow(t, store, "severance", "Severance", at)
	followedShow(t, store, "dune-prophecy", "Dune Prophecy", at.Add(-48*time.Hour))
	progress := library.Progress{
		Media:     source.Ref{Source: "fake", ID: "andor"},
		Title:     "Andor",
		Episode:   source.Episode{Number: 3, Title: "Reckoning"},
		Position:  12 * time.Minute,
		Duration:  48 * time.Minute,
		UpdatedAt: at,
	}
	if err := store.RecordProgress(progress); err != nil {
		t.Fatal(err)
	}

	page := following.New(following.Deps{Library: store})
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	// The update day under a follow is written for the viewer's locale, as the
	// discovery calendar and the update cards write theirs: an English month
	// abbreviation on a Chinese page is a date in another language than the one
	// around it.
	if !tt.HasText("Nov 12") {
		t.Fatalf("the update day is missing in %v", tt.Texts())
	}
	tt.SetLocale("zh-Hans")
	tt.Frame()
	if want := i18n.FormatDate("zh-Hans", at, i18n.DateOptions{Skeleton: "MMMd"}); !tt.HasText(want) {
		t.Fatalf("the update day is not %q on a Chinese page: %v", want, tt.Texts())
	}
	if tt.HasText("Nov 12") {
		t.Fatalf("an English month abbreviation survived the language change: %v", tt.Texts())
	}

	tt.SetLocale("en")
	tt.Frame()
	for _, want := range []string{"Following", "Followed shows", "Severance", "Dune Prophecy", "Andor", "Episode 3, Reckoning"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	// The continued title is not followed, so it belongs to the top section
	// only and the wall lists the two follows.
	if tt.HasText("2 titles") != true {
		t.Fatalf("followed count missing in %v", tt.Texts())
	}
}

// TestFollowingEmptyExplainsHowToFollow checks the page with an empty library:
// it says what a follow is instead of drawing an empty wall.
func TestFollowingEmptyExplainsHowToFollow(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	page := following.New(following.Deps{Library: store})
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	for _, want := range []string{"No followed shows yet", "Follow a title from its page"} {
		if !tt.HasText(want) {
			t.Fatalf("missing %q in %v", want, tt.Texts())
		}
	}
	if tt.HasText("Continue Watching") {
		t.Fatalf("an empty library still shows the continue section: %v", tt.Texts())
	}
}

// TestFollowingUnfollowsFromTheWall checks the switch on a card: the page lets a
// show go without leaving the wall, and the wall follows the library on the next
// frame.
func TestFollowingUnfollowsFromTheWall(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	ref := followedShow(t, store, "severance", "Severance", time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local))
	page := following.New(following.Deps{Library: store, OnDetail: func(source.Ref) {}})
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 1100, 720)
	tt.SetLocale("en")
	tt.Frame()

	if !tt.HasText("Severance") {
		t.Fatalf("follow missing in %v", tt.Texts())
	}
	if err := tt.Click("Unfollow"); err != nil {
		t.Fatalf("click the follow switch: %v", err)
	}
	tt.Frame()
	if store.IsFollowing(ref) {
		t.Fatal("the switch did not unfollow the show")
	}
	if !tt.HasText("No followed shows yet") {
		t.Fatalf("the wall did not clear in %v", tt.Texts())
	}
}

// TestFollowingFitsANarrowWindow checks the page at the narrowest window: the
// cards take the page's gutter and stay inside it.
func TestFollowingFitsANarrowWindow(t *testing.T) {
	store, err := library.Open("")
	if err != nil {
		t.Fatal(err)
	}
	followedShow(t, store, "severance", "Severance", time.Date(2026, 11, 12, 9, 0, 0, 0, time.Local))
	page := following.New(following.Deps{Library: store})
	tt := ui.NewTester(func(c *ui.Context) { page.View(c) }, 430, 820)
	tt.SetLocale("en")
	tt.Frame()

	title, ok := tt.Find("Severance")
	if !ok {
		t.Fatalf("card missing in a narrow window: %v", tt.Texts())
	}
	if title.X < 20-0.5 || title.X+title.W > 430-20+0.5 {
		t.Fatalf("card runs outside the narrow gutters: %+v", title)
	}
}
