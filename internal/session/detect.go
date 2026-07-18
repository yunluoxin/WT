package session

import (
	"bufio"
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strings"
)

// ClaudeSessionPrefixLength is the max encoded-path directory length Claude
// uses; longer paths get prefix-matched.
const ClaudeSessionPrefixLength = 200

var nonAlnum = regexp.MustCompile(`[^a-zA-Z0-9]`)

func sanitize(branch string) string {
	s := nonAlnum.ReplaceAllString(branch, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "worktree"
	}
	return s
}

// EncodeClaudeProjectPath encodes a filesystem path the way Claude Code
// names its project directories: all non-alphanumerics become '-'.
func EncodeClaudeProjectPath(path string) string {
	return nonAlnum.ReplaceAllString(path, "-")
}

func claudeDir() string {
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".claude")
}

// Exists reports whether a wt-tracked session exists for the branch:
// wt metadata present AND Claude's history.jsonl references the worktree path.
func Exists(branch string) bool {
	m, err := LoadMetadata(branch)
	if err != nil {
		return false
	}
	history := filepath.Join(claudeDir(), "history.jsonl")
	f, err := os.Open(history)
	if err != nil {
		return false
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	scanner.Buffer(make([]byte, 0, 64*1024), 10*1024*1024)
	for scanner.Scan() {
		line := scanner.Bytes()
		if !bytes.Contains(line, []byte(m.WorktreePath)) {
			continue
		}
		var entry struct {
			Project string `json:"project"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Project == m.WorktreePath {
			return true
		}
	}
	return false
}

// ClaudeNativeSessionExists reports whether Claude Code has native session
// files (~/.claude/projects/<encoded-path>/*.jsonl) for the given path.
func ClaudeNativeSessionExists(path string) bool {
	abs, err := filepath.Abs(path)
	if err != nil {
		abs = path
	}
	encoded := EncodeClaudeProjectPath(abs)
	projectsDir := filepath.Join(claudeDir(), "projects")

	hasJSONL := func(dir string) bool {
		matches, _ := filepath.Glob(filepath.Join(dir, "*.jsonl"))
		return len(matches) > 0
	}

	if len(encoded) <= 255 {
		if hasJSONL(filepath.Join(projectsDir, encoded)) {
			return true
		}
	}
	if len(encoded) > ClaudeSessionPrefixLength {
		prefix := encoded[:ClaudeSessionPrefixLength]
		entries, err := os.ReadDir(projectsDir)
		if err != nil {
			return false
		}
		for _, e := range entries {
			if e.IsDir() && strings.HasPrefix(e.Name(), prefix) {
				if hasJSONL(filepath.Join(projectsDir, e.Name())) {
					return true
				}
			}
		}
	}
	return false
}
