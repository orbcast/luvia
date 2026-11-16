package library

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
	"time"

	"luvia/apps/desktop/internal/source"
)

var (
	dune = source.Ref{Source: "kanju", ID: "dune"}
	wall = source.Ref{Source: "kanju", ID: "wall-e"}
)

func open(t *testing.T) (*Store, string) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "library.json")
	store, err := Open(path)
	if err != nil {
		t.Fatalf("open empty library: %v", err)
	}
	return store, path
}

func TestOpenMissingFileStartsEmpty(t *testing.T) {
	store, path := open(t)
	snap := store.Snapshot()
	if len(snap.Continues) != 0 || len(snap.Follows) != 0 {
		t.Fatalf("new library is not empty: %+v", snap)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatal("opening must not create the file")
	}
}

func TestRecordProgressOrdersNewestFirst(t *testing.T) {
	store, _ := open(t)
	base := time.Date(2026, 11, 15, 20, 0, 0, 0, time.UTC)
	first := Progress{Media: dune, Title: "Dune", Episode: source.Episode{Media: dune, ID: "e1", Number: 1},
		Position: 10 * time.Minute, Duration: 60 * time.Minute, UpdatedAt: base}
	second := Progress{Media: wall, Title: "WALL-E", Episode: source.Episode{Media: wall, ID: "e1", Number: 1},
		Position: 5 * time.Minute, Duration: 30 * time.Minute, UpdatedAt: base.Add(time.Hour)}
	for _, p := range []Progress{first, second} {
		if err := store.RecordProgress(p); err != nil {
			t.Fatal(err)
		}
	}
	snap := store.Snapshot()
	if len(snap.Continues) != 2 || snap.Continues[0].Media != wall || snap.Continues[1].Media != dune {
		t.Fatalf("wrong order: %+v", snap.Continues)
	}
	if got := snap.Continues[0].Remaining(); got != 25*time.Minute {
		t.Fatalf("remaining %v", got)
	}
}

func TestRecordProgressNormalisesPosition(t *testing.T) {
	store, _ := open(t)
	progress := Progress{Media: dune, Position: -time.Minute, Duration: 30 * time.Minute}
	if err := store.RecordProgress(progress); err != nil {
		t.Fatal(err)
	}
	stored, ok := store.ProgressOf(dune)
	if !ok || stored.Position != 0 {
		t.Fatalf("negative position kept: %+v", stored)
	}
	if stored.UpdatedAt.IsZero() {
		t.Fatal("missing timestamp must be filled in")
	}
	over := Progress{Media: wall, Position: time.Hour, Duration: 30 * time.Minute}
	if err := store.RecordProgress(over); err != nil {
		t.Fatal(err)
	}
	stored, _ = store.ProgressOf(wall)
	if stored.Position != 30*time.Minute || stored.Remaining() != 0 {
		t.Fatalf("position past the end kept: %+v", stored)
	}
}

func TestRecordProgressRejectsEmptyMedia(t *testing.T) {
	store, _ := open(t)
	if err := store.RecordProgress(Progress{}); err == nil {
		t.Fatal("progress without media must fail")
	}
}

func TestProgressKeepsOpaquePayload(t *testing.T) {
	store, path := open(t)
	episode := source.Episode{Media: dune, ID: "e7", Number: 7, Title: "Episode 7", Payload: "token-7"}
	if err := store.RecordProgress(Progress{Media: dune, Episode: episode, Position: time.Minute}); err != nil {
		t.Fatal(err)
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	stored, ok := reopened.ProgressOf(dune)
	if !ok || stored.Episode.Payload != "token-7" || stored.Episode.Number != 7 {
		t.Fatalf("episode lost: %+v", stored)
	}
}

func TestFollowAndUnfollowPersist(t *testing.T) {
	store, path := open(t)
	follow := Follow{Media: dune, Title: "Dune", PosterURL: "https://cdn.example/dune.jpg"}
	if err := store.Follow(follow); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(Follow{}); err == nil {
		t.Fatal("follow without media must fail")
	}
	if !store.IsFollowing(dune) {
		t.Fatal("follow not recorded")
	}
	reopened, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	if !reopened.IsFollowing(dune) {
		t.Fatal("follow not persisted")
	}
	if err := reopened.Unfollow(dune); err != nil {
		t.Fatal(err)
	}
	if reopened.IsFollowing(dune) {
		t.Fatal("unfollow did not take effect")
	}
	if err := reopened.Unfollow(dune); err != nil {
		t.Fatalf("unfollowing twice must be harmless: %v", err)
	}
}

func TestFollowUpdatesInPlace(t *testing.T) {
	store, _ := open(t)
	first := Follow{Media: dune, Title: "Dune", UpdatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)}
	updated := Follow{Media: dune, Title: "Dune Part Two", UpdatedAt: first.UpdatedAt.Add(time.Hour)}
	if err := store.Follow(first); err != nil {
		t.Fatal(err)
	}
	if err := store.Follow(updated); err != nil {
		t.Fatal(err)
	}
	snap := store.Snapshot()
	if len(snap.Follows) != 1 || snap.Follows[0].Title != "Dune Part Two" {
		t.Fatalf("follow not replaced: %+v", snap.Follows)
	}
}

func TestForgetRemovesProgress(t *testing.T) {
	store, _ := open(t)
	if err := store.RecordProgress(Progress{Media: dune, Position: time.Minute}); err != nil {
		t.Fatal(err)
	}
	if err := store.Forget(dune); err != nil {
		t.Fatal(err)
	}
	if _, ok := store.ProgressOf(dune); ok {
		t.Fatal("progress survived Forget")
	}
	if err := store.Forget(dune); err != nil {
		t.Fatalf("forgetting twice must be harmless: %v", err)
	}
}

func TestCorruptFileStartsEmpty(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	if err := os.WriteFile(path, []byte("not a library"), 0o644); err != nil {
		t.Fatal(err)
	}
	store, err := Open(path)
	if err == nil {
		t.Fatal("corrupt file must be reported")
	}
	if snap := store.Snapshot(); len(snap.Continues) != 0 {
		t.Fatalf("corrupt file kept entries: %+v", snap)
	}
	if err := store.Follow(Follow{Media: dune, Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Open(path); err != nil {
		t.Fatalf("rewritten file is unreadable: %v", err)
	}
}

func TestSaveLeavesNoTemporaryFile(t *testing.T) {
	store, path := open(t)
	if err := store.Follow(Follow{Media: dune, Title: "Dune"}); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(filepath.Dir(path))
	if err != nil {
		t.Fatal(err)
	}
	for _, entry := range entries {
		if filepath.Ext(entry.Name()) == ".tmp" {
			t.Fatalf("temporary file left behind: %s", entry.Name())
		}
	}
}
