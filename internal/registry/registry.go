// Package registry tracks repositories that use wt, enabling global (-g)
// operations across all registered repos.
package registry

import (
	"encoding/json"
	"os"
	"path/filepath"
	"sort"
	"time"

	"wt/internal/config"
)

// Entry describes a registered repository.
type Entry struct {
	Name         string    `json:"name"`
	RegisteredAt time.Time `json:"registered_at"`
	LastSeen     time.Time `json:"last_seen"`
}

type fileSchema struct {
	Version      int              `json:"version"`
	Repositories map[string]Entry `json:"repositories"`
}

// Path returns the registry file path.
func Path() string {
	return filepath.Join(config.Dir(), "registry.json")
}

func read() (fileSchema, error) {
	var s fileSchema
	data, err := os.ReadFile(Path())
	if err != nil {
		if os.IsNotExist(err) {
			return fileSchema{Version: 1, Repositories: map[string]Entry{}}, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, err
	}
	if s.Repositories == nil {
		s.Repositories = map[string]Entry{}
	}
	return s, nil
}

func write(s fileSchema) error {
	if err := os.MkdirAll(config.Dir(), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	tmp := Path() + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, Path())
}

// Register adds (or refreshes) a repository in the registry.
func Register(repoPath string) error {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	s, err := read()
	if err != nil {
		return err
	}
	entry, ok := s.Repositories[abs]
	if !ok {
		entry = Entry{Name: filepath.Base(abs), RegisteredAt: time.Now().UTC()}
	}
	entry.LastSeen = time.Now().UTC()
	s.Repositories[abs] = entry
	return write(s)
}

// UpdateLastSeen refreshes a repo's last_seen timestamp; unregistered
// paths are silently ignored.
func UpdateLastSeen(repoPath string) {
	abs, err := filepath.Abs(repoPath)
	if err != nil {
		abs = repoPath
	}
	s, err := read()
	if err != nil {
		return
	}
	entry, ok := s.Repositories[abs]
	if !ok {
		return
	}
	entry.LastSeen = time.Now().UTC()
	s.Repositories[abs] = entry
	_ = write(s)
}

// Repositories returns all registered repos sorted by path.
func Repositories() ([]string, map[string]Entry, error) {
	s, err := read()
	if err != nil {
		return nil, nil, err
	}
	paths := make([]string, 0, len(s.Repositories))
	for p := range s.Repositories {
		paths = append(paths, p)
	}
	sort.Strings(paths)
	return paths, s.Repositories, nil
}

// Prune drops entries whose path or .git no longer exists.
// Returns the removed paths.
func Prune() ([]string, error) {
	s, err := read()
	if err != nil {
		return nil, err
	}
	var removed []string
	for p := range s.Repositories {
		if _, err := os.Stat(filepath.Join(p, ".git")); err != nil {
			delete(s.Repositories, p)
			removed = append(removed, p)
		}
	}
	if len(removed) > 0 {
		if err := write(s); err != nil {
			return removed, err
		}
	}
	sort.Strings(removed)
	return removed, nil
}
