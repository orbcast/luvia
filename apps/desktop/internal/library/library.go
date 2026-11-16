// Package library keeps the viewer's local watch progress and follows. It
// stores no catalog data: media always come from a source rule, and only the
// viewer's own state lives here.
package library

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"sync"
	"time"

	"luvia/apps/desktop/internal/source"
)

// Progress is one resumable episode of a media.
type Progress struct {
	Media     source.Ref
	Title     string
	PosterURL string
	Episode   source.Episode
	Position  time.Duration
	Duration  time.Duration
	UpdatedAt time.Time
}

// Remaining reports the part of the episode still ahead, never negative.
func (p Progress) Remaining() time.Duration {
	left := p.Duration - p.Position
	if left < 0 {
		return 0
	}
	return left
}

// Follow is one media the viewer keeps up with.
type Follow struct {
	Media     source.Ref
	Title     string
	PosterURL string
	UpdatedAt time.Time
}

// Snapshot is an immutable copy of the stored state, newest first.
type Snapshot struct {
	Continues []Progress
	Follows   []Follow
}

// Store is a concurrency-safe view of the library file. Every change is
// written through, so a crash loses at most the change in flight.
type Store struct {
	mu     sync.Mutex
	path   string
	items  map[string]Progress
	follow map[string]Follow
}

const formatVersion = 1

type fileFormat struct {
	Version   int              `json:"version"`
	Continues []storedProgress `json:"continues,omitempty"`
	Follows   []storedFollow   `json:"follows,omitempty"`
}

type storedProgress struct {
	Media     source.Ref    `json:"media"`
	Title     string        `json:"title,omitempty"`
	PosterURL string        `json:"posterUrl,omitempty"`
	Episode   storedEpisode `json:"episode"`
	Position  int64         `json:"positionMs"`
	Duration  int64         `json:"durationMs,omitempty"`
	UpdatedAt time.Time     `json:"updatedAt"`
}

type storedEpisode struct {
	Media   source.Ref `json:"media"`
	ID      string     `json:"id,omitempty"`
	Number  int        `json:"number,omitempty"`
	Title   string     `json:"title,omitempty"`
	Payload string     `json:"payload,omitempty"`
}

type storedFollow struct {
	Media     source.Ref `json:"media"`
	Title     string     `json:"title,omitempty"`
	PosterURL string     `json:"posterUrl,omitempty"`
	UpdatedAt time.Time  `json:"updatedAt"`
}

// Open loads the library at path. A missing file is an empty library and no
// error. An unreadable or invalid file returns an empty library with the
// reason, so the app starts and the next change rewrites the file.
func Open(path string) (*Store, error) {
	s := &Store{path: path, items: map[string]Progress{}, follow: map[string]Follow{}}
	data, err := os.ReadFile(path)
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return s, fmt.Errorf("read library: %w", err)
	}
	var disk fileFormat
	if err := json.Unmarshal(data, &disk); err != nil {
		return s, fmt.Errorf("decode library: %w", err)
	}
	if disk.Version != formatVersion {
		return s, fmt.Errorf("library version %d is not %d", disk.Version, formatVersion)
	}
	for _, p := range disk.Continues {
		progress, err := p.progress()
		if err != nil {
			return s, err
		}
		if progress.Media.IsZero() {
			continue
		}
		s.items[progress.Media.String()] = progress
	}
	for _, f := range disk.Follows {
		if f.Media.IsZero() {
			continue
		}
		s.follow[f.Media.String()] = Follow{
			Media: f.Media, Title: f.Title, PosterURL: f.PosterURL, UpdatedAt: f.UpdatedAt,
		}
	}
	return s, nil
}

func (s storedProgress) progress() (Progress, error) {
	return Progress{
		Media: s.Media, Title: s.Title, PosterURL: s.PosterURL,
		Episode: source.Episode{
			Media: s.Episode.Media, ID: s.Episode.ID, Number: s.Episode.Number,
			Title: s.Episode.Title, Payload: s.Episode.Payload,
		},
		Position:  time.Duration(s.Position) * time.Millisecond,
		Duration:  time.Duration(s.Duration) * time.Millisecond,
		UpdatedAt: s.UpdatedAt,
	}, nil
}

// Snapshot copies the current state, newest first.
func (s *Store) Snapshot() Snapshot {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := Snapshot{
		Continues: make([]Progress, 0, len(s.items)),
		Follows:   make([]Follow, 0, len(s.follow)),
	}
	for _, p := range s.items {
		out.Continues = append(out.Continues, p)
	}
	sort.Slice(out.Continues, func(i, j int) bool {
		if out.Continues[i].UpdatedAt.Equal(out.Continues[j].UpdatedAt) {
			return out.Continues[i].Media.String() < out.Continues[j].Media.String()
		}
		return out.Continues[i].UpdatedAt.After(out.Continues[j].UpdatedAt)
	})
	for _, f := range s.follow {
		out.Follows = append(out.Follows, f)
	}
	sort.Slice(out.Follows, func(i, j int) bool {
		if out.Follows[i].UpdatedAt.Equal(out.Follows[j].UpdatedAt) {
			return out.Follows[i].Title < out.Follows[j].Title
		}
		return out.Follows[i].UpdatedAt.After(out.Follows[j].UpdatedAt)
	})
	return out
}

// ProgressOf returns the saved progress of ref.
func (s *Store) ProgressOf(ref source.Ref) (Progress, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	p, ok := s.items[ref.String()]
	return p, ok
}

// RecordProgress saves where playback of one episode stopped.
func (s *Store) RecordProgress(p Progress) error {
	if p.Media.IsZero() {
		return errors.New("record progress: media is empty")
	}
	if p.Position < 0 {
		p.Position = 0
	}
	if p.Duration > 0 && p.Position > p.Duration {
		p.Position = p.Duration
	}
	if p.UpdatedAt.IsZero() {
		p.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[p.Media.String()] = p
	return s.saveLocked()
}

// Forget drops the saved progress of ref.
func (s *Store) Forget(ref source.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.items, ref.String())
	return s.saveLocked()
}

// Follow adds or refreshes a followed media.
func (s *Store) Follow(f Follow) error {
	if f.Media.IsZero() {
		return errors.New("follow: media is empty")
	}
	if f.UpdatedAt.IsZero() {
		f.UpdatedAt = time.Now().UTC()
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.follow[f.Media.String()] = f
	return s.saveLocked()
}

// Unfollow stops following ref.
func (s *Store) Unfollow(ref source.Ref) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.follow, ref.String())
	return s.saveLocked()
}

// IsFollowing reports whether ref is followed.
func (s *Store) IsFollowing(ref source.Ref) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.follow[ref.String()]
	return ok
}

func (s *Store) saveLocked() error {
	if s.path == "" {
		return nil
	}
	var disk fileFormat
	disk.Version = formatVersion
	for _, p := range s.items {
		stored := storedProgress{
			Media: p.Media, Title: p.Title, PosterURL: p.PosterURL,
			Episode: storedEpisode{
				Media: p.Episode.Media, ID: p.Episode.ID, Number: p.Episode.Number,
				Title: p.Episode.Title, Payload: p.Episode.Payload,
			},
			Position:  int64(p.Position / time.Millisecond),
			Duration:  int64(p.Duration / time.Millisecond),
			UpdatedAt: p.UpdatedAt,
		}
		disk.Continues = append(disk.Continues, stored)
	}
	for _, f := range s.follow {
		disk.Follows = append(disk.Follows, storedFollow{
			Media: f.Media, Title: f.Title, PosterURL: f.PosterURL, UpdatedAt: f.UpdatedAt,
		})
	}
	data, err := json.MarshalIndent(disk, "", "  ")
	if err != nil {
		return fmt.Errorf("save library: %w", err)
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o755); err != nil {
		return fmt.Errorf("save library: %w", err)
	}
	temp := s.path + ".tmp"
	if err := os.WriteFile(temp, append(data, '\n'), 0o644); err != nil {
		return fmt.Errorf("save library: %w", err)
	}
	if err := os.Rename(temp, s.path); err != nil {
		return fmt.Errorf("save library: %w", err)
	}
	return nil
}
