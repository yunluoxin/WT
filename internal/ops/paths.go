// Package ops implements the worktree business logic (create, delete,
// merge, sync, etc.) shared by CLI commands.
package ops

import (
	"path/filepath"
	"regexp"
	"strings"
)

// Naming conventions for resources created by `wt new`. The prefixes make
// wt-managed branches and worktrees recognizable at a glance, and let
// destructive commands (merge, pr) refuse foreign branches unless --any
// is given. Branch and worktree-directory prefixes are deliberately two
// separate variables so they can diverge later — change them here to
// rename everything at once.
var (
	// BranchPrefix is prepended to branch names created by `wt new`.
	BranchPrefix = "wt-"
	// WorktreePrefix is prepended to worktree directory names created by
	// `wt new` (independent of BranchPrefix).
	WorktreePrefix = "wt-"
)

// PrefixBranch returns the branch name `wt new` creates for a user-supplied
// name. Already-prefixed names are returned unchanged (idempotent).
func PrefixBranch(name string) string {
	if strings.HasPrefix(name, BranchPrefix) {
		return name
	}
	return BranchPrefix + name
}

// IsManagedBranch reports whether a branch carries BranchPrefix, i.e. was
// created by `wt new`.
func IsManagedBranch(branch string) bool {
	return strings.HasPrefix(branch, BranchPrefix)
}

// PrefixWorktreeDir returns the directory suffix `wt new` uses for a
// user-supplied name. Already-prefixed names are returned unchanged.
func PrefixWorktreeDir(name string) string {
	if strings.HasPrefix(name, WorktreePrefix) {
		return name
	}
	return WorktreePrefix + name
}

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
// <repo-parent>/<repo-name>-<sanitized-branch>. The directory name is
// derived from the branch name, which already carries BranchPrefix; when
// WorktreePrefix diverges from BranchPrefix the directory prefix is
// normalized to WorktreePrefix so both conventions stay consistent.
func DefaultWorktreePath(repo, branch string) string {
	abs, err := filepath.Abs(repo)
	if err != nil {
		abs = repo
	}
	suffix := SanitizeBranchName(branch)
	if BranchPrefix != WorktreePrefix {
		// Strip BranchPrefix from the raw name, then apply WorktreePrefix.
		suffix = PrefixWorktreeDir(SanitizeBranchName(strings.TrimPrefix(branch, BranchPrefix)))
	}
	return filepath.Join(filepath.Dir(abs), filepath.Base(abs)+"-"+suffix)
}
