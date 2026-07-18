// Package ops implements the worktree business logic (create, delete,
// merge, sync, etc.) shared by CLI commands.
package ops

import (
	"path/filepath"
	"regexp"
	"strings"
)

// SanitizeBranchName converts a branch name into a filesystem-safe
// directory suffix. Ported character-for-character from the Python
// sanitize_branch_name.
var (
	sanitizeSpecial  = regexp.MustCompile(`[/<>:"|?*\\#@&;$` + "`" + `!~%^()\[\]{}=+]+`)
	sanitizeSpace    = regexp.MustCompile(`\s+`)
	sanitizeCollapse = regexp.MustCompile(`-+`)
)

func SanitizeBranchName(branch string) string {
	s := sanitizeSpecial.ReplaceAllString(branch, "-")
	s = sanitizeSpace.ReplaceAllString(s, "-")
	s = sanitizeCollapse.ReplaceAllString(s, "-")
	s = strings.Trim(s, "-")
	if s == "" {
		return "worktree"
	}
	return s
}

// DefaultWorktreePath derives the default worktree path:
// <repo-parent>/<repo-name>-<sanitized-branch>
func DefaultWorktreePath(repo, branch string) string {
	abs, err := filepath.Abs(repo)
	if err != nil {
		abs = repo
	}
	return filepath.Join(filepath.Dir(abs), filepath.Base(abs)+"-"+SanitizeBranchName(branch))
}
