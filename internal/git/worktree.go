package git

import (
	"path/filepath"
	"strings"
)

// DetachedBranch is the label used for worktrees on a detached HEAD.
const DetachedBranch = "(detached)"

// Worktree pairs a branch name with its filesystem path.
type Worktree struct {
	Branch string
	Path   string
}

// ParseWorktrees parses `git worktree list --porcelain` output.
// The first entry is always the main repository.
func ParseWorktrees(dir string) ([]Worktree, error) {
	out, err := Output(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return nil, err
	}
	var worktrees []Worktree
	var cur *Worktree
	flush := func() {
		if cur != nil {
			if cur.Branch == "" {
				cur.Branch = DetachedBranch
			}
			worktrees = append(worktrees, *cur)
			cur = nil
		}
	}
	for _, line := range strings.Split(out, "\n") {
		switch {
		case line == "":
			flush()
		case strings.HasPrefix(line, "worktree "):
			flush()
			cur = &Worktree{Path: strings.TrimPrefix(line, "worktree ")}
		case strings.HasPrefix(line, "branch "):
			if cur != nil {
				cur.Branch = NormalizeBranchName(strings.TrimPrefix(line, "branch "))
			}
		}
	}
	flush()
	return worktrees, nil
}

// FeatureWorktrees returns all worktrees except the main repo and detached ones.
func FeatureWorktrees(dir string) ([]Worktree, error) {
	wts, err := ParseWorktrees(dir)
	if err != nil {
		return nil, err
	}
	var out []Worktree
	for i, wt := range wts {
		if i == 0 || wt.Branch == DetachedBranch {
			continue
		}
		out = append(out, wt)
	}
	return out, nil
}

// FindWorktreeByBranch finds a worktree whose checked-out branch matches.
func FindWorktreeByBranch(dir, branch string) (Worktree, bool, error) {
	wts, err := ParseWorktrees(dir)
	if err != nil {
		return Worktree{}, false, err
	}
	for _, wt := range wts {
		if wt.Branch == branch {
			return wt, true, nil
		}
	}
	return Worktree{}, false, nil
}

// FindWorktreeByName finds a worktree whose directory base name matches.
func FindWorktreeByName(dir, name string) (Worktree, bool, error) {
	wts, err := ParseWorktrees(dir)
	if err != nil {
		return Worktree{}, false, err
	}
	for _, wt := range wts {
		if filepath.Base(wt.Path) == name {
			return wt, true, nil
		}
	}
	return Worktree{}, false, nil
}

// RemoveWorktreeSafe removes a worktree, with a Windows fallback that
// force-removes the directory and prunes when git reports "Directory not empty".
func RemoveWorktreeSafe(repo, path string, force bool) error {
	args := []string{"worktree", "remove"}
	if force {
		args = append(args, "--force")
	}
	args = append(args, path)
	res, err := Git(repo, false, args...)
	if err != nil {
		return err
	}
	if res.ExitCode == 0 {
		return nil
	}
	if strings.Contains(res.Stderr, "Directory not empty") || strings.Contains(res.Stdout, "Directory not empty") {
		if err := removeAll(path); err != nil {
			return err
		}
		_, err := Git(repo, false, "worktree", "prune")
		return err
	}
	return &worktreeRemoveError{path: path, stderr: strings.TrimSpace(res.Stderr)}
}

type worktreeRemoveError struct {
	path, stderr string
}

func (e *worktreeRemoveError) Error() string {
	return "failed to remove worktree " + e.path + ": " + e.stderr
}
