package git

import (
	"strings"

	wterrors "wt/internal/errors"
)

// RepoRoot returns the top-level directory of the current git repository.
func RepoRoot(dir string) (string, error) {
	out, err := Output(dir, "rev-parse", "--show-toplevel")
	if err != nil {
		return "", wterrors.Wrap(wterrors.ErrNotARepo, err, "not a git repository (or any parent)")
	}
	return out, nil
}

// MainRepoRoot returns the main repository root (the first entry of
// `git worktree list --porcelain`), which works from inside any worktree.
func MainRepoRoot(dir string) (string, error) {
	out, err := Output(dir, "worktree", "list", "--porcelain")
	if err != nil {
		return "", wterrors.Wrap(wterrors.ErrNotARepo, err, "not a git repository")
	}
	for _, line := range strings.Split(out, "\n") {
		if rest, ok := strings.CutPrefix(line, "worktree "); ok {
			return rest, nil
		}
	}
	return "", wterrors.New(wterrors.ErrNotARepo, "could not determine main repository root")
}

// CurrentBranch returns the current branch name, or ErrDetachedHEAD.
func CurrentBranch(dir string) (string, error) {
	out, err := Output(dir, "rev-parse", "--abbrev-ref", "HEAD")
	if err != nil {
		return "", err
	}
	if out == "HEAD" {
		return "", wterrors.New(wterrors.ErrDetachedHEAD, "repository is in detached HEAD state")
	}
	return out, nil
}

// BranchExists reports whether the local branch exists.
func BranchExists(dir, branch string) bool {
	res, err := Git(dir, false, "rev-parse", "--verify", "refs/heads/"+branch)
	return err == nil && res.ExitCode == 0
}

// RemoteBranchExists reports whether <remote>/<branch> exists.
func RemoteBranchExists(dir, remote, branch string) bool {
	res, err := Git(dir, false, "rev-parse", "--verify", "refs/remotes/"+remote+"/"+branch)
	return err == nil && res.ExitCode == 0
}

// NormalizeBranchName strips the refs/heads/ prefix.
func NormalizeBranchName(ref string) string {
	return strings.TrimPrefix(ref, "refs/heads/")
}
