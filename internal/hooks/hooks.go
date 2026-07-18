// Package hooks manages per-repository lifecycle hooks stored in
// .cwconfig.json at the repository root.
package hooks

import (
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// LocalConfigFile is the per-repo hooks config file name.
const LocalConfigFile = ".cwconfig.json"

// Events lists all supported hook events.
var Events = []string{
	"worktree.pre_create", "worktree.post_create",
	"worktree.pre_delete", "worktree.post_delete",
	"merge.pre", "merge.post",
	"pr.pre", "pr.post",
	"resume.pre", "resume.post",
	"sync.pre", "sync.post",
}

// Hook is a single registered hook.
type Hook struct {
	ID          string `json:"id"`
	Command     string `json:"command"`
	Enabled     bool   `json:"enabled"`
	Description string `json:"description,omitempty"`
}

// fileSchema mirrors the on-disk .cwconfig.json structure.
type fileSchema struct {
	Hooks map[string][]Hook `json:"hooks"`
}

// ValidEvent reports whether event is a known hook event.
func ValidEvent(event string) bool {
	for _, e := range Events {
		if e == event {
			return true
		}
	}
	return false
}

// GenerateID derives a stable hook ID from the command.
func GenerateID(command string) string {
	sum := md5.Sum([]byte(command))
	return "hook-" + hex.EncodeToString(sum[:])[:8]
}

func configPath(repo string) string {
	return filepath.Join(repo, LocalConfigFile)
}

func read(repo string) (fileSchema, error) {
	var s fileSchema
	data, err := os.ReadFile(configPath(repo))
	if err != nil {
		if os.IsNotExist(err) {
			return fileSchema{Hooks: map[string][]Hook{}}, nil
		}
		return s, err
	}
	if err := json.Unmarshal(data, &s); err != nil {
		return s, fmt.Errorf("invalid JSON in %s: %w", configPath(repo), err)
	}
	if s.Hooks == nil {
		s.Hooks = map[string][]Hook{}
	}
	return s, nil
}

func write(repo string, s fileSchema) error {
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(configPath(repo), append(data, '\n'), 0o644)
}

// Add registers a hook; returns the ID (generated when id is "").
func Add(repo, event, command, id, description string) (string, error) {
	if !ValidEvent(event) {
		return "", fmt.Errorf("unknown hook event %q", event)
	}
	if id == "" {
		id = GenerateID(command)
	}
	s, err := read(repo)
	if err != nil {
		return "", err
	}
	for _, h := range s.Hooks[event] {
		if h.ID == id {
			return "", fmt.Errorf("hook %q already exists for %s", id, event)
		}
	}
	s.Hooks[event] = append(s.Hooks[event], Hook{
		ID: id, Command: command, Enabled: true, Description: description,
	})
	return id, write(repo, s)
}

// Remove deletes a hook by ID.
func Remove(repo, event, id string) error {
	s, err := read(repo)
	if err != nil {
		return err
	}
	hooks := s.Hooks[event]
	for i, h := range hooks {
		if h.ID == id {
			s.Hooks[event] = append(hooks[:i], hooks[i+1:]...)
			return write(repo, s)
		}
	}
	return fmt.Errorf("hook %q not found for %s", id, event)
}

// SetEnabled toggles a hook.
func SetEnabled(repo, event, id string, enabled bool) error {
	s, err := read(repo)
	if err != nil {
		return err
	}
	for i, h := range s.Hooks[event] {
		if h.ID == id {
			s.Hooks[event][i].Enabled = enabled
			return write(repo, s)
		}
	}
	return fmt.Errorf("hook %q not found for %s", id, event)
}

// List returns hooks for one event, or all events when event is "".
func List(repo, event string) (map[string][]Hook, error) {
	s, err := read(repo)
	if err != nil {
		return nil, err
	}
	if event != "" {
		return map[string][]Hook{event: s.Hooks[event]}, nil
	}
	// stable output: include all known events
	out := map[string][]Hook{}
	for _, e := range Events {
		if len(s.Hooks[e]) > 0 {
			out[e] = s.Hooks[e]
		}
	}
	return out, nil
}

// SortedEvents returns event names in stable order for display.
func SortedEvents(m map[string][]Hook) []string {
	out := make([]string, 0, len(m))
	for e := range m {
		out = append(out, e)
	}
	sort.Strings(out)
	return out
}
