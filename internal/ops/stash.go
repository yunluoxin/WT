package ops

import (
	"fmt"
	"os"
	"sort"
	"strings"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

// StashSave stashes changes in the current worktree with a [branch] prefix.
func StashSave(message string, includeUntracked bool) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	branch, err := git.CurrentBranch(cwd)
	if err != nil {
		return wterrors.New(wterrors.ErrInvalidBranch, "cannot determine current branch")
	}
	if message == "" {
		message = "WIP"
	}
	stashMsg := fmt.Sprintf("[%s] %s", branch, message)

	res, err := git.Git(cwd, false, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(res.Stdout) == "" {
		termenv.Warn("No changes to stash\n")
		return nil
	}

	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Stashing changes in %s...", branch)))
	args := []string{"stash", "push"}
	if includeUntracked {
		args = append(args, "--include-untracked")
	}
	args = append(args, "-m", stashMsg)
	if _, err := git.Git(cwd, true, args...); err != nil {
		return err
	}
	termenv.Success("Stashed changes: %s\n", stashMsg)
	return nil
}

// StashList lists stashes grouped by branch.
func StashList() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	res, err := git.Git(repo, true, "stash", "list")
	if err != nil {
		return err
	}
	if strings.TrimSpace(res.Stdout) == "" {
		termenv.Warn("No stashes found\n")
		return nil
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Stashes by worktree:")))
	type stashEntry struct {
		ref, msg string
	}
	byBranch := map[string][]stashEntry{}
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		parts := strings.SplitN(line, ":", 3)
		if len(parts) < 3 {
			continue
		}
		ref := strings.TrimSpace(parts[0])
		info := strings.TrimSpace(parts[1])
		msg := strings.TrimSpace(parts[2])

		branch := "unknown"
		if strings.HasPrefix(msg, "[") {
			if idx := strings.Index(msg, "]"); idx > 0 {
				branch = msg[1:idx]
				msg = strings.TrimSpace(msg[idx+1:])
			}
		} else if idx := strings.Index(info, "On "); idx >= 0 {
			branch = strings.TrimSpace(info[idx+3:])
		} else if idx := strings.Index(info, "WIP on "); idx >= 0 {
			branch = strings.TrimSpace(info[idx+7:])
		}
		byBranch[branch] = append(byBranch[branch], stashEntry{ref, msg})
	}

	branches := make([]string, 0, len(byBranch))
	for b := range byBranch {
		branches = append(branches, b)
	}
	sort.Strings(branches)
	for _, b := range branches {
		termenv.Info("%s:", termenv.Bold(termenv.Green(b)))
		for _, e := range byBranch[b] {
			termenv.Info("  %s: %s", e.ref, e.msg)
		}
		fmt.Println()
	}
	return nil
}

// StashApply applies a stash to a different worktree.
func StashApply(targetBranch, stashRef string) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	if stashRef == "" {
		stashRef = "stash@{0}"
	}
	wt, matchedBranch, found := findWorktreeByBranchish(repo, targetBranch)
	if !found {
		return wterrors.New(wterrors.ErrWorktreeNotFound,
			"no worktree found for branch '%s'. Use 'wt list' to see available worktrees.", targetBranch)
	}

	res, _ := git.Git(repo, false, "stash", "list")
	if !strings.Contains(res.Stdout, stashRef) {
		return wterrors.New(wterrors.ErrInvalidBranch,
			"stash '%s' not found. Use 'wt stash list' to see available stashes.", stashRef)
	}

	termenv.Info("\n%s", termenv.Yellow(fmt.Sprintf("Applying %s to %s...", stashRef, matchedBranch)))
	if _, err := git.Git(wt.Path, true, "stash", "apply", stashRef); err != nil {
		termenv.Error("Failed to apply stash: %v\n", err)
		termenv.Info("%s There may be conflicts. Check the worktree and resolve manually.\n", termenv.Yellow("Tip:"))
		return err
	}
	termenv.Success("Stash applied to %s\n", targetBranch)
	termenv.Info("%s\n", termenv.Dim("Worktree path: "+wt.Path))
	return nil
}
