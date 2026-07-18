// Package session manages AI session metadata and detection.
package session

import (
	"encoding/json"
	"os"
	"path/filepath"
	"time"

	"wt/internal/config"
)

// Metadata describes a stored AI session.
type Metadata struct {
	Branch       string    `json:"branch"`
	AITool       string    `json:"ai_tool"`
	WorktreePath string    `json:"worktree_path"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
}

// Dir returns the session directory for a branch.
func Dir(branch string) string {
	return filepath.Join(config.Dir(), "sessions", sanitize(branch))
}

// Save writes session metadata, preserving CreatedAt when it exists.
func Save(m Metadata) error {
	dir := Dir(m.Branch)
	if existing, err := LoadMetadata(m.Branch); err == nil && !existing.CreatedAt.IsZero() {
		m.CreatedAt = existing.CreatedAt
	} else if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now().UTC()
	}
	m.UpdatedAt = time.Now().UTC()
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "metadata.json"), data, 0o644)
}

// LoadMetadata reads stored session metadata for a branch.
func LoadMetadata(branch string) (Metadata, error) {
	var m Metadata
	data, err := os.ReadFile(filepath.Join(Dir(branch), "metadata.json"))
	if err != nil {
		return m, err
	}
	return m, json.Unmarshal(data, &m)
}

// SaveContext stores free-text context (used for AI-merge prompts).
func SaveContext(branch, context string) error {
	dir := Dir(branch)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	return os.WriteFile(filepath.Join(dir, "context.txt"), []byte(context), 0o644)
}

// LoadContext reads stored context; "" when absent.
func LoadContext(branch string) string {
	data, err := os.ReadFile(filepath.Join(Dir(branch), "context.txt"))
	if err != nil {
		return ""
	}
	return string(data)
}

// Delete removes all session data for a branch.
func Delete(branch string) error {
	return os.RemoveAll(Dir(branch))
}

// List returns all stored session metadata.
func List() ([]Metadata, error) {
	base := filepath.Join(config.Dir(), "sessions")
	entries, err := os.ReadDir(base)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, err
	}
	var out []Metadata
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := os.ReadFile(filepath.Join(base, e.Name(), "metadata.json"))
		if err != nil {
			continue
		}
		var m Metadata
		if json.Unmarshal(data, &m) == nil {
			out = append(out, m)
		}
	}
	return out, nil
}
