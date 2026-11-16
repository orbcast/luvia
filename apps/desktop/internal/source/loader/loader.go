// Package loader discovers source rules on disk and assembles them into ready
// sources. One broken rule never hides the others: it is skipped and reported as
// a diagnostic so a settings screen can show what went wrong and where.
package loader

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"luvia/apps/desktop/internal/source"
	"luvia/apps/desktop/internal/source/engine"
	"luvia/apps/desktop/internal/source/rule"
)

// RuleFile is the file every source directory must provide.
const RuleFile = "rule.json"

// EnvDir overrides the rule directory without a settings screen. It exists for
// development runs, where pointing at a checked-out rule directory is faster
// than copying files into the user data directory.
const EnvDir = "LUVIA_RULES_DIR"

// Diagnostic reports one rule that could not be loaded.
type Diagnostic struct {
	// Path is the rule file or directory that failed.
	Path string
	// SourceID is the rule's declared id when it was readable.
	SourceID string
	// Err is the validation or decoding failure.
	Err error
}

func (d Diagnostic) Error() string {
	where := d.Path
	if d.SourceID != "" {
		where = d.SourceID + " (" + d.Path + ")"
	}
	return where + ": " + d.Err.Error()
}

// Set is an immutable snapshot of every loadable source in one directory.
type Set struct {
	dir         string
	options     engine.Options
	sources     []source.Source
	byID        map[string]source.Source
	diagnostics []Diagnostic
}

// Load reads dir, one source per subdirectory. Directories whose name starts
// with "." or "_" are ignored without a diagnostic; a directory without a rule
// file is reported, because that usually means a renamed or half-copied rule.
func Load(dir string, options engine.Options) (*Set, error) {
	info, err := os.Stat(dir)
	if err != nil {
		return nil, fmt.Errorf("rules directory %q: %w", dir, err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("rules directory %q is not a directory", dir)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, fmt.Errorf("rules directory %q: %w", dir, err)
	}
	names := make([]string, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, ".") || strings.HasPrefix(name, "_") {
			continue
		}
		names = append(names, name)
	}
	sort.Strings(names)

	set := &Set{
		dir:     dir,
		options: options,
		byID:    make(map[string]source.Source, len(names)),
	}
	for _, name := range names {
		path := filepath.Join(dir, name, RuleFile)
		parsed, err := rule.DecodeFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				err = fmt.Errorf("missing %s", RuleFile)
			}
			set.diagnostics = append(set.diagnostics, Diagnostic{Path: path, Err: err})
			continue
		}
		if parsed.ID != name {
			set.diagnostics = append(set.diagnostics, Diagnostic{
				Path:     path,
				SourceID: parsed.ID,
				Err:      fmt.Errorf("id %q must match its directory name %q", parsed.ID, name),
			})
			continue
		}
		if _, taken := set.byID[parsed.ID]; taken {
			set.diagnostics = append(set.diagnostics, Diagnostic{
				Path:     path,
				SourceID: parsed.ID,
				Err:      fmt.Errorf("duplicate source id %q; keeping the first directory", parsed.ID),
			})
			continue
		}
		built, err := engine.New(parsed, options)
		if err != nil {
			set.diagnostics = append(set.diagnostics, Diagnostic{
				Path:     path,
				SourceID: parsed.ID,
				Err:      err,
			})
			continue
		}
		set.sources = append(set.sources, built)
		set.byID[built.ID()] = built
	}
	return set, nil
}

// Dir returns the directory this set was loaded from.
func (s *Set) Dir() string { return s.dir }

// Sources returns the loadable sources in directory order.
func (s *Set) Sources() []source.Source {
	out := make([]source.Source, len(s.sources))
	copy(out, s.sources)
	return out
}

// Get returns one source by id.
func (s *Set) Get(id string) (source.Source, bool) {
	found, ok := s.byID[id]
	return found, ok
}

// Diagnostics returns the rules that failed to load.
func (s *Set) Diagnostics() []Diagnostic {
	out := make([]Diagnostic, len(s.diagnostics))
	copy(out, s.diagnostics)
	return out
}

// DefaultDir picks the rule directory for an application run: the environment
// override, then a user data directory that already holds rules, then the copy
// bundled with the application.
func DefaultDir(userDataDir, bundledDir string) string {
	if dir := strings.TrimSpace(os.Getenv(EnvDir)); dir != "" {
		return dir
	}
	if hasRules(userDataDir) {
		return userDataDir
	}
	if strings.TrimSpace(bundledDir) != "" {
		return bundledDir
	}
	return userDataDir
}

// hasRules reports whether dir already contains at least one source directory.
func hasRules(dir string) bool {
	if strings.TrimSpace(dir) == "" {
		return false
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if !entry.IsDir() || strings.HasPrefix(entry.Name(), ".") || strings.HasPrefix(entry.Name(), "_") {
			continue
		}
		return true
	}
	return false
}

// Reload re-reads the same directory, so a settings screen can pick up edited
// rules without restarting the application.
func (s *Set) Reload() (*Set, error) {
	return Load(s.dir, s.options)
}
