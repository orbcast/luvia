package tests

import (
	"testing"

	"github.com/egoist/mygo/ui"

	"luvia/apps/desktop/internal/design"
	"luvia/apps/desktop/internal/detail"
	"luvia/apps/desktop/internal/library"
	"luvia/apps/desktop/internal/source"
)

func TestDetailErrorRetryAndActions(t *testing.T) {
	store, _ := library.Open("")

	// A failing source shows the error state and retries without panicking.
	failRun := make(chan func(), 4)
	failPage := detail.New(detail.Deps{
		Source: failingSource{}, Library: store, Run: func(fn func()) { failRun <- fn },
	})
	failPage.Open(source.Ref{Source: "fail", ID: "x"})
	apply(t, failRun)
	failTester := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		failPage.View(c)
	}, 1100, 800)
	failTester.SetLocale("en")
	failTester.Frame()
	if !failTester.HasText("This title could not load") {
		t.Fatalf("error state missing: %v", failTester.Texts())
	}
	failPage.Retry()
	apply(t, failRun)
	failTester.Frame()

	// A loaded title exercises follow, expand and tab switching.
	src, ref := sampleTitle()
	run := make(chan func(), 8)
	page := detail.New(detail.Deps{
		Source: src, Library: store, Run: func(fn func()) { run <- fn },
		OnPlay: func(_ source.Detail, _ []source.Episode, _ int) {},
	})
	page.Open(ref)
	apply(t, run)

	if page.IsFollowing() {
		t.Fatal("a fresh title must not be followed")
	}
	page.ToggleFollow()
	if !page.IsFollowing() {
		t.Fatal("follow did not stick")
	}
	page.ToggleFollow()
	if page.IsFollowing() {
		t.Fatal("unfollow did not stick")
	}

	for tab := 0; tab < 4; tab++ {
		page.SelectTab(tab)
	}
	page.SelectTab(-1)
	page.SelectTab(99)
	page.ToggleExpanded()
	page.ToggleExpanded()

	// Out-of-range plays are ignored; the in-range one opens the selected episode.
	page.Play(-1)
	page.Play(99)
	page.PlaySelected()

	tt := ui.NewTester(func(c *ui.Context) {
		theme := design.Theme(ui.DarkTheme())
		c.SetTheme(&theme)
		page.View(c)
	}, 1280, 900)
	tt.SetLocale("en")
	tt.Frame()
	if !tt.HasText("Cyberpunk Edgerunners 2") {
		t.Fatalf("title missing: %v", tt.Texts())
	}
}
