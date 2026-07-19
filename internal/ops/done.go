package ops

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/launch"
	"wt/internal/session"
	"wt/internal/termenv"
)

// DoneOptions parameterizes DoneWorktree (done command).
type DoneOptions struct {
	AI   bool // launch the AI tool to resolve conflicts (stash pop or rebase)
	Keep bool // keep the worktree and branch after finishing
}

// DoneWorktree finishes the current worktree: stashes uncommitted changes,
// merges new commits into the base branch (rebase + fast-forward, via
// FinishWorktree), restores the stashed changes on the base branch, and
// removes the worktree and branch unless Keep is set.
//
// It must be run inside a linked feature worktree (not the main worktree).
func DoneWorktree(opts DoneOptions) error {
	cwd, err := os.Getwd()
	if err != nil {
		return err
	}
	repo, err := git.RepoRoot(cwd)
	if err != nil {
		return err // ErrNotARepo
	}
	mainRepo, err := git.MainRepoRoot(cwd)
	if err != nil {
		return err
	}
	if samePath(repo, mainRepo) {
		return wterrors.New(wterrors.ErrProtectedWorktree,
			"'wt done' must be run inside a feature worktree created with 'wt new', not the main repository worktree")
	}
	feature, err := git.CurrentBranch(cwd)
	if err != nil {
		return err // ErrDetachedHEAD
	}
	baseBranch, _, err := WorktreeMetadata(feature, repo)
	if err != nil {
		return err
	}
	if feature == baseBranch {
		return wterrors.New(wterrors.ErrProtectedWorktree,
			"current branch '%s' is the base branch; nothing to do", feature)
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Finishing worktree (done):")))
	termenv.Info("  Feature:     %s", termenv.Green(feature))
	termenv.Info("  Base:        %s", termenv.Green(baseBranch))
	termenv.Info("  Repo:        %s\n", termenv.Cyan(mainRepo))

	// Step 1: stash uncommitted changes (stash refs are shared across
	// worktrees, so they can be popped later wherever the base branch lives).
	stashed, err := doneStash(cwd, feature)
	if err != nil {
		return err
	}

	// Step 2: does the feature branch have commits the base branch lacks?
	ahead, err := aheadCount(cwd, baseBranch, feature)
	if err != nil {
		return err
	}

	if ahead == 0 {
		return doneNoCommits(cwd, mainRepo, feature, baseBranch, stashed, opts)
	}
	return doneWithCommits(cwd, mainRepo, feature, baseBranch, stashed, opts)
}

// doneStash stashes uncommitted changes (including untracked files) in dir.
// Reports whether anything was stashed.
func doneStash(dir, branch string) (bool, error) {
	res, err := git.Git(dir, false, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(res.Stdout) == "" {
		return false, nil
	}
	termenv.Info("%s", termenv.Yellow("Stashing uncommitted changes..."))
	if _, err := git.Git(dir, true, "stash", "push", "--include-untracked",
		"-m", fmt.Sprintf("[%s] wt done", branch)); err != nil {
		return false, err
	}
	termenv.Success("Stashed changes")
	return true, nil
}

// aheadCount counts commits on branch that are not on base.
func aheadCount(dir, base, branch string) (int, error) {
	out, err := git.Output(dir, "rev-list", "--count", base+".."+branch)
	if err != nil {
		return 0, err
	}
	var n int
	if _, err := fmt.Sscanf(out, "%d", &n); err != nil {
		return 0, wterrors.Wrap(wterrors.ErrInvalidBranch, err,
			"cannot parse ahead count for %s..%s", base, branch)
	}
	return n, nil
}

// stashPop pops the top stash in dir. On conflict git keeps the stash entry.
func stashPop(dir string) error {
	_, err := git.Git(dir, true, "stash", "pop")
	return err
}

// rebaseInProgress reports whether dir is in the middle of a rebase. It checks
// for the rebase-merge/rebase-apply state directories rather than REBASE_HEAD,
// which git leaves behind after a rebase completes. Paths are resolved via
// --git-path so linked worktrees (where .git is a file) are handled.
func rebaseInProgress(dir string) bool {
	for _, state := range []string{"rebase-merge", "rebase-apply"} {
		p, err := git.Output(dir, "rev-parse", "--git-path", state)
		if err != nil {
			continue
		}
		if !filepath.IsAbs(p) {
			p = filepath.Join(dir, p)
		}
		if st, err := os.Stat(p); err == nil && st.IsDir() {
			return true
		}
	}
	return false
}

// doneNoCommits handles the case where the feature branch has no commits of
// its own: the stashed changes are moved onto the base branch directly.
func doneNoCommits(cwd, mainRepo, feature, baseBranch string, stashed bool, opts DoneOptions) error {
	popDir := ""
	keepWorktree := opts.Keep

	if stashed {
		// Pop the stash where the base branch is checked out. If some
		// worktree already has it, pop there without touching any branch;
		// otherwise switch this worktree onto the base branch and pop here.
		if wt, found, err := git.FindWorktreeByBranch(mainRepo, baseBranch); err == nil && found {
			popDir = wt.Path
		} else {
			termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Switching this worktree to '%s'...", baseBranch)))
			if _, err := git.Git(cwd, true, "switch", baseBranch); err != nil {
				// Best effort: pop the stash back so the worktree returns to
				// its original dirty state on the feature branch. If that
				// also fails, the stash entry is kept (see 'wt stash list').
				termenv.Info("%s", termenv.Yellow("Restoring stashed changes..."))
				if popErr := stashPop(cwd); popErr != nil {
					termenv.Warn("Could not restore stashed changes automatically: %v", popErr)
					termenv.Warn("Your changes are kept in the stash (see 'wt stash list').")
				}
				return err
			}
			popDir = cwd
			// The worktree now holds the uncommitted changes on the base
			// branch — removing it would lose them, so keep it regardless.
			keepWorktree = true
		}
		if err := donePopStash(popDir, baseBranch, opts.AI); err != nil {
			return err
		}
	}

	// The feature branch has no unique commits; only the worktree/branch
	// cleanup remains.
	if keepWorktree {
		if popDir == cwd && !opts.Keep {
			termenv.Info("This worktree now holds your uncommitted changes on '%s'; keeping it.", baseBranch)
			if _, err := git.Git(mainRepo, true, "branch", "-D", feature); err != nil {
				return err
			}
			unsetMetadata(mainRepo, feature)
			_ = session.Delete(feature)
		} else {
			termenv.Info("Keeping worktree and branch '%s' (--keep)", feature)
		}
		termenv.Success("%s", termenv.Bold("Done!"))
		return nil
	}

	if err := os.Chdir(mainRepo); err != nil {
		return err
	}
	if err := git.RemoveWorktreeSafe(mainRepo, cwd, true); err != nil {
		return err
	}
	if _, err := git.Git(mainRepo, true, "branch", "-D", feature); err != nil {
		return err
	}
	unsetMetadata(mainRepo, feature)
	_ = session.Delete(feature)
	termenv.Success("%s", termenv.Bold("Done!"))
	return nil
}

// doneWithCommits handles the case where the feature branch has new commits:
// FinishWorktree rebases and fast-forward merges them into the base branch,
// then the stashed changes are popped onto the base branch. The stashed flag
// reports whether step 1 stashed uncommitted changes that still need to be
// restored on the base branch.
func doneWithCommits(cwd, mainRepo, feature, baseBranch string, stashed bool, opts DoneOptions) error {
	err := FinishWorktree(FinishOptions{
		Target:  feature,
		AIMerge: opts.AI,
		Keep:    opts.Keep,
	})
	if err != nil {
		// The AI tool resolved the rebase conflicts and completed the rebase;
		// it signals this with ErrAborted. `wt done` continues automatically
		// (rather than stopping for a manual re-run as `wt merge`/`wt sync`
		// do): re-run FinishWorktree, which now rebases cleanly and merges.
		// The stashed flag is preserved across the retry so the changes are
		// still restored afterwards. Guard against the AI not actually
		// finishing the rebase, which would loop forever.
		if opts.AI && errors.Is(err, wterrors.ErrAborted) {
			if rebaseInProgress(cwd) {
				termenv.Warn("AI session ended but the rebase is still in progress; resolve it and re-run 'wt done'.")
				return err
			}
			termenv.Info("%s", termenv.Cyan("Rebase resolved; continuing merge..."))
			return doneWithCommits(cwd, mainRepo, feature, baseBranch, stashed, opts)
		}
		// The merge did not complete. FinishWorktree aborts a failed rebase
		// itself; restore the stashed changes here so the worktree is back
		// to its original state and `wt done` can simply be re-run.
		if stashed {
			termenv.Info("%s", termenv.Yellow("Restoring stashed changes in the feature worktree..."))
			if popErr := stashPop(cwd); popErr != nil {
				termenv.Warn("Could not restore stashed changes automatically: %v", popErr)
				termenv.Warn("Your changes are kept in the stash (see 'wt stash list').")
			}
		}
		return err
	}

	if stashed {
		// Merge succeeded: restore the changes where the base branch lives
		// (usually the main worktree; FinishWorktree merges there too).
		popDir := mainRepo
		if wt, found, ferr := git.FindWorktreeByBranch(mainRepo, baseBranch); ferr == nil && found {
			popDir = wt.Path
		}
		termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Restoring stashed changes on '%s'...", baseBranch)))
		if err := donePopStash(popDir, baseBranch, opts.AI); err != nil {
			return wterrors.Wrap(wterrors.ErrMergeFailed, err,
				"the merge completed successfully, but restoring your stashed changes failed")
		}
	}
	return nil
}

// donePopStash pops the stash in popDir, handling conflicts per the --ai flag.
func donePopStash(popDir, baseBranch string, ai bool) error {
	if err := stashPop(popDir); err == nil {
		termenv.Success("Restored uncommitted changes on '%s' (%s)", baseBranch, popDir)
		return nil
	}
	conflicts := conflictedFiles(popDir)
	if ai && len(conflicts) > 0 {
		return aiResolveStashConflicts(popDir, baseBranch, conflicts)
	}
	return wterrors.New(wterrors.ErrMergeFailed,
		"stash pop produced conflicts in %s.\nConflicted files:\n  %s\nResolve them manually, then run 'git stash drop' (the stash entry was kept).",
		popDir, strings.Join(conflicts, "\n  "))
}

// aiResolveStashConflicts launches the AI tool to resolve stash pop
// conflicts. Returns ErrAborted so the caller exits after the AI session.
func aiResolveStashConflicts(dir, baseBranch string, conflicts []string) error {
	termenv.Info("\n%s\n", termenv.Bold(termenv.Yellow("! Stash pop conflicts detected!")))
	termenv.Info("%s", termenv.Cyan("Conflicted files:"))
	for _, f := range conflicts {
		termenv.Info("  • %s", f)
	}
	fmt.Println()
	termenv.Info("\n%s\n", termenv.Cyan("Launching AI to resolve conflicts automatically..."))

	var b strings.Builder
	b.WriteString("Resolve the conflicts from a 'git stash pop' in this repository.\n\n")
	b.WriteString("**Current situation:**\n")
	fmt.Fprintf(&b, "- Uncommitted changes were popped onto branch '%s' and produced conflicts\n", baseBranch)
	fmt.Fprintf(&b, "- %d file(s) have conflicts\n", len(conflicts))
	b.WriteString("- The stash entry was kept in the stash list\n\n")
	b.WriteString("**Conflicted files:**\n")
	for _, f := range conflicts {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\n**Your task:**\n")
	b.WriteString("1. Read each conflicted file to understand the conflicts\n")
	b.WriteString("2. Resolve the conflicts by choosing the appropriate changes or merging them\n")
	b.WriteString("3. Edit the files to remove conflict markers (<<<<<<< ======= >>>>>>>)\n")
	b.WriteString("4. Stage ALL resolved files using: `git add <file1> <file2> ...`\n")
	b.WriteString("5. Drop the stash entry using: `git stash drop`\n")
	b.WriteString("\n**Important:**\n")
	b.WriteString("- Make sure to actually execute the git commands, not just suggest them\n")
	b.WriteString("- Only run `git stash drop` after every conflict is resolved\n")

	prompt := b.String()
	if err := session.SaveContext(baseBranch, prompt); err != nil {
		termenv.Warn("failed to save context: %v", err)
	}

	if err := launch.AITool(launch.Options{
		WorktreePath: dir,
		Prompt:       prompt,
		Term:         "foreground",
	}); err != nil {
		return err
	}
	return wterrors.New(wterrors.ErrAborted, "AI conflict resolution finished; re-run the command if needed")
}
