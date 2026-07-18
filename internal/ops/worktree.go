package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/hooks"
	"wt/internal/launch"
	"wt/internal/registry"
	"wt/internal/session"
	"wt/internal/share"
	"wt/internal/termenv"
)

// CreateOptions parameterizes CreateWorktree.
type CreateOptions struct {
	BranchName string
	BaseBranch string // "" = current branch
	Path       string // "" = default ../<repo>-<branch>
	Term       string // "" = default launch method
	NoTerm     bool
}

// CreateWorktree creates a new worktree with a feature branch and
// launches the AI tool. Returns the worktree path.
func CreateWorktree(opts CreateOptions) (string, error) {
	repo, err := git.RepoRoot("")
	if err != nil {
		return "", err
	}

	if msg := git.BranchNameError(opts.BranchName); msg != "" {
		return "", wterrors.New(wterrors.ErrInvalidBranch,
			"invalid branch name: %s\nHint: Use alphanumeric characters, hyphens, and slashes. Avoid special characters like emojis, backslashes, or control characters.", msg)
	}

	// Branches created by wt carry a prefix so they are recognizable and
	// destructive commands (merge, pr) can refuse foreign branches.
	branch := PrefixBranch(opts.BranchName)

	// Existing worktree for this branch?
	existing, found, err := git.FindWorktreeByBranch(repo, branch)
	if err != nil {
		return "", err
	}
	if found {
		termenv.Warn("%s", termenv.Bold(fmt.Sprintf("Worktree already exists\nBranch '%s' already has a worktree at:\n  %s", branch, existing.Path)))
		if opts.NoTerm {
			termenv.Info("\n%s\n", termenv.Dim("Worktree exists at: "+existing.Path))
			return existing.Path, nil
		}
		termenv.Info("\n%s\n", termenv.Dim(fmt.Sprintf("Switching to resume mode for '%s'...", branch)))
		if err := ResumeWorktree(ResumeOptions{Worktree: branch, Term: opts.Term}); err != nil {
			return "", err
		}
		return existing.Path, nil
	}

	branchAlreadyExists := false
	isRemoteOnly := false
	localExists := git.BranchExists(repo, branch)
	remoteExists := git.RemoteBranchExists(repo, "origin", branch)

	if localExists {
		termenv.Warn("%s", termenv.Bold(fmt.Sprintf("Branch already exists\nBranch '%s' already exists locally but has no worktree.", branch)))
		if !termenv.IsNonInteractive() {
			if !termenv.Confirm("Create worktree from this existing branch?", true) {
				termenv.Info("\n%s To use a different branch name:\n  %s\n\nOr delete the existing branch first:\n  %s  (if fully merged)\n  %s  (force delete)\n",
					termenv.Yellow("Tip:"), termenv.Cyan("wt new "+branch+"-v2"),
					termenv.Cyan("git branch -d "+branch), termenv.Cyan("git branch -D "+branch))
				return "", wterrors.New(wterrors.ErrAborted, "operation cancelled")
			}
		}
		termenv.Info("%s\n", termenv.Dim(fmt.Sprintf("Creating worktree from existing branch '%s'...", branch)))
		branchAlreadyExists = true
	} else if remoteExists {
		termenv.Warn("%s", termenv.Bold(fmt.Sprintf("Remote branch found\nBranch '%s' exists on remote (origin) but not locally.", branch)))
		if !termenv.IsNonInteractive() {
			if !termenv.Confirm("Create worktree tracking this remote branch?", true) {
				termenv.Info("\n%s To create a new branch instead:\n  %s\n",
					termenv.Yellow("Tip:"), termenv.Cyan("wt new "+branch+"-v2"))
				return "", wterrors.New(wterrors.ErrAborted, "operation cancelled")
			}
		}
		termenv.Info("%s\n", termenv.Dim(fmt.Sprintf("Creating worktree tracking remote branch '%s'...", branch)))
		branchAlreadyExists = true
		isRemoteOnly = true
	}

	// Determine base branch.
	userProvidedBase := opts.BaseBranch != ""
	base := opts.BaseBranch
	if base == "" {
		base, err = git.CurrentBranch(repo)
		if err != nil {
			if isRemoteOnly {
				base = "main" // metadata fallback for remote-only
			} else {
				return "", wterrors.New(wterrors.ErrInvalidBranch,
					"cannot determine base branch. Specify with --base or checkout a branch first.")
			}
		}
	}
	if (!isRemoteOnly || userProvidedBase) && !git.BranchExists(repo, base) {
		return "", wterrors.New(wterrors.ErrInvalidBranch, "base branch '%s' not found", base)
	}

	worktreePath := opts.Path
	if worktreePath == "" {
		worktreePath = DefaultWorktreePath(repo, branch)
	} else {
		worktreePath, _ = filepath.Abs(worktreePath)
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Creating new worktree:")))
	termenv.Info("  Base branch: %s", termenv.Green(base))
	termenv.Info("  New branch:  %s", termenv.Green(branch))
	termenv.Info("  Path:        %s\n", termenv.Cyan(worktreePath))

	hookCtx := hooks.Context{
		Branch: branch, BaseBranch: base,
		WorktreePath: worktreePath, RepoPath: repo, Operation: "new",
	}
	if err := hooks.RunHooks(repo, "worktree.pre_create", hookCtx, repo); err != nil {
		return "", err
	}

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return "", err
	}
	if _, err := git.Git(repo, true, "fetch", "--all", "--prune"); err != nil {
		termenv.Warn("fetch failed (continuing): %v", err)
	}

	switch {
	case isRemoteOnly:
		if _, err := git.Git(repo, true, "worktree", "add", "-b", branch, worktreePath, "origin/"+branch); err != nil {
			return "", err
		}
	case branchAlreadyExists:
		if _, err := git.Git(repo, true, "worktree", "add", worktreePath, branch); err != nil {
			return "", err
		}
	default:
		if _, err := git.Git(repo, true, "worktree", "add", "-b", branch, worktreePath, base); err != nil {
			return "", err
		}
	}

	// Store metadata.
	if err := git.SetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch), base); err != nil {
		return "", err
	}
	if err := git.SetConfig(repo, git.MetadataKey(git.KeyBasePath, branch), repo); err != nil {
		return "", err
	}
	if err := git.SetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, branch), branch); err != nil {
		return "", err
	}

	_ = registry.Register(repo) // non-fatal

	termenv.Success("%s", termenv.Bold("Worktree created successfully"))
	fmt.Println()

	share.CopyToWorktree(repo, worktreePath)

	_ = hooks.RunHooks(repo, "worktree.post_create", hookCtx, worktreePath) // non-blocking

	if !opts.NoTerm {
		if err := launch.AITool(launch.Options{WorktreePath: worktreePath, Term: opts.Term}); err != nil {
			return worktreePath, err
		}
	}
	return worktreePath, nil
}

// FinishOptions parameterizes FinishWorktree (merge command).
type FinishOptions struct {
	Target      string
	Push        bool
	Interactive bool
	DryRun      bool
	AIMerge     bool
	Any         bool // allow branches not created by `wt new` (no wt- prefix)
	LookupMode  LookupMode
	Global      bool
}

// FinishWorktree rebases the feature branch, fast-forward merges into the
// base branch, and cleans up the worktree. Shared by `merge` and `finish`.
func FinishWorktree(opts FinishOptions) error {
	t, err := ResolveWorktreeTarget(opts.Target, opts.LookupMode, opts.Global)
	if err != nil {
		return err
	}
	cwd, feature := t.WorktreePath, t.Branch

	baseBranch, basePath, err := WorktreeMetadata(feature, t.WorktreeRepo)
	if err != nil {
		return err
	}
	repo := basePath

	// Safety: merging a branch into itself is meaningless, and the cleanup
	// step would try to remove the main worktree and delete the base branch
	// (e.g. running `wt merge` inside the main working tree on 'main').
	if feature == baseBranch {
		return wterrors.New(wterrors.ErrProtectedWorktree,
			"cannot merge branch '%s' into itself.\nHint: 'wt merge' is meant to be run inside a feature worktree created with 'wt new'.", feature)
	}
	if samePath(cwd, basePath) {
		return wterrors.New(wterrors.ErrProtectedWorktree, "cannot merge the main repository worktree")
	}
	// By default only merge branches created by `wt new` (wt- prefix);
	// --any opts out for foreign branches/worktrees.
	if !opts.Any && !IsManagedBranch(feature) {
		return wterrors.New(wterrors.ErrProtectedWorktree,
			"branch '%s' was not created by wt (missing '%s' prefix).\nHint: use 'wt merge --any' to merge it anyway, or create worktrees with 'wt new'.",
			feature, BranchPrefix)
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Finishing worktree:")))
	termenv.Info("  Feature:     %s", termenv.Green(feature))
	termenv.Info("  Base:        %s", termenv.Green(baseBranch))
	termenv.Info("  Repo:        %s\n", termenv.Cyan(repo))

	hookCtx := hooks.Context{
		Branch: feature, BaseBranch: baseBranch,
		WorktreePath: cwd, RepoPath: repo, Operation: "merge",
	}
	if !opts.DryRun {
		if err := hooks.RunHooks(repo, "merge.pre", hookCtx, cwd); err != nil {
			return err
		}
	}

	if opts.DryRun {
		termenv.Info("%s\n", termenv.Bold(termenv.Yellow("DRY RUN MODE - No changes will be made")))
		termenv.Info("%s\n", termenv.Bold("The following operations would be performed:"))
		termenv.Info("  1. %s updates from remote", termenv.Cyan("Fetch"))
		termenv.Info("  2. %s %s onto %s", termenv.Cyan("Rebase"), feature, baseBranch)
		termenv.Info("  3. %s to %s in base repository", termenv.Cyan("Switch"), baseBranch)
		termenv.Info("  4. %s %s into %s (fast-forward)", termenv.Cyan("Merge"), feature, baseBranch)
		step := 5
		if opts.Push {
			termenv.Info("  %d. %s %s to origin", step, termenv.Cyan("Push"), baseBranch)
			step++
		}
		termenv.Info("  %d. %s worktree at %s", step, termenv.Cyan("Remove"), cwd)
		termenv.Info("  %d. %s local branch %s", step+1, termenv.Cyan("Delete"), feature)
		termenv.Info("  %d. %s metadata", step+2, termenv.Cyan("Clean up"))
		termenv.Info("\n%s\n", termenv.Dim("Run without --dry-run to execute these operations."))
		return nil
	}

	confirmStep := func(name string) (bool, error) {
		if !opts.Interactive {
			return true, nil
		}
		termenv.Info("\n%s", termenv.Bold(termenv.Yellow("Next step: "+name)))
		fmt.Print("Continue? [Y/n/q]: ")
		var response string
		fmt.Scanln(&response)
		switch strings.ToLower(strings.TrimSpace(response)) {
		case "q", "quit":
			termenv.Warn("Aborting...")
			return false, wterrors.New(wterrors.ErrAborted, "aborted by user")
		case "", "y", "yes":
			return true, nil
		}
		return false, nil
	}

	if ok, err := confirmStep(fmt.Sprintf("Rebase %s onto %s", feature, baseBranch)); err != nil {
		return err
	} else if !ok {
		termenv.Warn("Skipping rebase step...")
		return nil
	}

	fetchRes, _ := git.Git(repo, false, "fetch", "--all", "--prune")
	rebaseTarget := baseBranch
	if fetchRes.ExitCode == 0 {
		if res, _ := git.Git(cwd, false, "rev-parse", "--verify", "origin/"+baseBranch); res.ExitCode == 0 {
			rebaseTarget = "origin/" + baseBranch
		}
	}

	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Rebasing %s onto %s...", feature, rebaseTarget)))
	if _, err := git.Git(cwd, true, "rebase", rebaseTarget); err != nil {
		conflicts := conflictedFiles(cwd)
		if len(conflicts) > 0 && opts.AIMerge {
			return aiResolveConflicts(cwd, feature, rebaseTarget, conflicts, "wt merge")
		}
		_, _ = git.Git(cwd, false, "rebase", "--abort")
		return rebaseError(cwd, rebaseTarget, conflicts, !opts.AIMerge)
	}
	termenv.Success("Rebase successful")
	fmt.Println()

	if _, err := os.Stat(basePath); err != nil {
		return wterrors.New(wterrors.ErrWorktreeNotFound, "base repository not found at: %s", basePath)
	}

	if ok, err := confirmStep(fmt.Sprintf("Merge %s into %s", feature, baseBranch)); err != nil {
		return err
	} else if !ok {
		termenv.Warn("Skipping merge step...")
		return nil
	}

	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Merging %s into %s...", feature, baseBranch)))
	_, _ = git.Git(basePath, false, "fetch", "--all", "--prune")

	if cur, err := git.CurrentBranch(basePath); err != nil || cur != baseBranch {
		termenv.Info("Switching base worktree to '%s'", baseBranch)
		if _, err := git.Git(basePath, true, "switch", baseBranch); err != nil {
			return err
		}
	}
	if _, err := git.Git(basePath, true, "merge", "--ff-only", feature); err != nil {
		return wterrors.New(wterrors.ErrMergeFailed,
			"fast-forward merge failed. Manual intervention required:\n  cd %s\n  git merge %s", basePath, feature)
	}
	termenv.Success("Merged %s into %s", feature, baseBranch)
	fmt.Println()

	if opts.Push {
		if ok, err := confirmStep(fmt.Sprintf("Push %s to origin", baseBranch)); err != nil {
			return err
		} else if ok {
			termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Pushing %s to origin...", baseBranch)))
			if _, err := git.Git(basePath, true, "push", "origin", baseBranch); err != nil {
				termenv.Warn("Push failed: %v\n", err)
			} else {
				termenv.Success("Pushed to origin")
				fmt.Println()
			}
		} else {
			termenv.Warn("Skipping push step...")
		}
	}

	if ok, err := confirmStep(fmt.Sprintf("Clean up worktree and delete branch %s", feature)); err != nil {
		return err
	} else if !ok {
		termenv.Warn("Skipping cleanup step...")
		return nil
	}

	termenv.Info("%s", termenv.Yellow("Cleaning up worktree and branch..."))
	// Move to base repo before removing the worktree we might be standing in.
	if err := os.Chdir(repo); err != nil {
		return err
	}
	if err := git.RemoveWorktreeSafe(repo, cwd, true); err != nil {
		return err
	}
	if _, err := git.Git(repo, true, "branch", "-D", feature); err != nil {
		return err
	}
	unsetMetadata(repo, feature)
	_ = session.Delete(feature)

	termenv.Success("%s", termenv.Bold("Cleanup complete!"))
	fmt.Println()

	_ = hooks.RunHooks(repo, "merge.post", hookCtx, repo)
	registry.UpdateLastSeen(repo)
	return nil
}

// DeleteOptions parameterizes DeleteWorktree.
type DeleteOptions struct {
	Target       string
	KeepBranch   bool
	DeleteRemote bool
	NoForce      bool
	LookupMode   LookupMode
	Global       bool
}

// DeleteWorktree removes a worktree (and optionally its branch).
func DeleteWorktree(opts DeleteOptions) error {
	t, err := ResolveWorktreeTarget(opts.Target, opts.LookupMode, opts.Global)
	if err != nil {
		return err
	}
	worktreePath, branch := t.WorktreePath, t.Branch

	mainRepo, err := git.MainRepoRoot(t.WorktreeRepo)
	if err != nil {
		return err
	}
	// Metadata may know the real main repo.
	if branch != "" {
		if basePathStr := git.GetConfig(mainRepo, git.MetadataKey(git.KeyBasePath, branch)); basePathStr != "" {
			mainRepo = basePathStr
		}
	}

	// Safety: never delete the main repository worktree.
	if samePath(worktreePath, mainRepo) {
		return wterrors.New(wterrors.ErrProtectedWorktree, "cannot delete main repository worktree")
	}

	// Move out of the target worktree if we're inside it.
	if cwd, err := os.Getwd(); err == nil && samePath(cwd, worktreePath) {
		if err := os.Chdir(mainRepo); err != nil {
			return err
		}
	}

	baseBranch := ""
	if branch != "" {
		baseBranch = git.GetConfig(mainRepo, git.MetadataKey(git.KeyWorktreeBase, branch))
	}
	hookCtx := hooks.Context{
		Branch: branch, BaseBranch: baseBranch,
		WorktreePath: worktreePath, RepoPath: mainRepo, Operation: "delete",
	}
	if err := hooks.RunHooks(mainRepo, "worktree.pre_delete", hookCtx, mainRepo); err != nil {
		return err
	}

	termenv.Info("%s", termenv.Yellow("Removing worktree: "+worktreePath))
	if err := git.RemoveWorktreeSafe(mainRepo, worktreePath, !opts.NoForce); err != nil {
		return err
	}
	termenv.Success("Worktree removed")
	fmt.Println()

	if branch != "" && !opts.KeepBranch {
		termenv.Info("%s", termenv.Yellow("Deleting local branch: "+branch))
		if _, err := git.Git(mainRepo, true, "branch", "-D", branch); err != nil {
			return err
		}
		unsetMetadata(mainRepo, branch)
		_ = session.Delete(branch)
		termenv.Success("Local branch and metadata removed")
		fmt.Println()

		if opts.DeleteRemote {
			termenv.Info("%s", termenv.Yellow("Deleting remote branch: origin/"+branch))
			if _, err := git.Git(mainRepo, true, "push", "origin", ":"+branch); err != nil {
				termenv.Warn("Remote branch deletion failed: %v\n", err)
			} else {
				termenv.Success("Remote branch deleted")
				fmt.Println()
			}
		}
	}

	_ = hooks.RunHooks(mainRepo, "worktree.post_delete", hookCtx, mainRepo)
	registry.UpdateLastSeen(mainRepo)
	return nil
}

// ResumeOptions parameterizes ResumeWorktree.
type ResumeOptions struct {
	Worktree   string
	Term       string
	LookupMode LookupMode
	Global     bool
}

// ResumeWorktree resumes AI work in a worktree with context restoration.
func ResumeWorktree(opts ResumeOptions) error {
	t, err := ResolveWorktreeTarget(opts.Worktree, opts.LookupMode, opts.Global)
	if err != nil {
		return err
	}
	worktreePath, branch := t.WorktreePath, t.WorktreeRepo

	mainRepo, err := git.MainRepoRoot(t.WorktreeRepo)
	if err != nil {
		return err
	}
	baseBranch := git.GetConfig(mainRepo, git.MetadataKey(git.KeyWorktreeBase, branch))

	hookCtx := hooks.Context{
		Branch: branch, BaseBranch: baseBranch,
		WorktreePath: worktreePath, RepoPath: mainRepo, Operation: "resume",
	}
	if err := hooks.RunHooks(mainRepo, "resume.pre", hookCtx, worktreePath); err != nil {
		return err
	}

	if opts.Worktree != "" {
		if err := os.Chdir(worktreePath); err != nil {
			return err
		}
	}

	hasSession := session.Exists(branch) || session.ClaudeNativeSessionExists(worktreePath)
	if cfg, err := config.Load(); err == nil && !config.AutoResume(cfg) {
		hasSession = false
	}

	if ctx := session.LoadContext(branch); ctx != "" {
		termenv.Info("\n%s", termenv.Bold("Previous context:"))
		termenv.Info("%s\n", termenv.Dim(ctx))
	}
	if m, err := session.LoadMetadata(branch); err == nil {
		termenv.Info("%s %s", termenv.Dim("Last session:"), termenv.Dim(m.UpdatedAt.Format("2006-01-02 15:04")))
	}

	_ = session.Save(session.Metadata{
		Branch:       branch,
		WorktreePath: worktreePath,
	})

	err = launch.AITool(launch.Options{
		WorktreePath: worktreePath,
		Term:         opts.Term,
		Resume:       hasSession,
	})
	if err != nil {
		return err
	}

	_ = hooks.RunHooks(mainRepo, "resume.post", hookCtx, worktreePath)
	return nil
}

// conflictedFiles lists files with unresolved merge conflicts.
func conflictedFiles(dir string) []string {
	res, err := git.Git(dir, false, "diff", "--name-only", "--diff-filter=U")
	if err != nil || res.ExitCode != 0 {
		return nil
	}
	var out []string
	for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
		if line != "" {
			out = append(out, line)
		}
	}
	return out
}

// aiResolveConflicts launches the AI tool to resolve rebase conflicts.
// Returns ErrAborted so the caller exits after the AI session (mirroring
// Python's sys.exit(0) — the user re-runs the command afterwards).
func aiResolveConflicts(worktreePath, branch, rebaseTarget string, conflicts []string, rerunCmd string) error {
	termenv.Info("\n%s\n", termenv.Bold(termenv.Yellow("! Rebase conflicts detected!")))
	termenv.Info("%s", termenv.Cyan("Conflicted files:"))
	for _, f := range conflicts {
		termenv.Info("  • %s", f)
	}
	fmt.Println()
	termenv.Info("\n%s\n", termenv.Cyan("Launching AI to resolve conflicts automatically..."))

	var b strings.Builder
	b.WriteString("Resolve the merge conflicts in this repository and complete the rebase.\n\n")
	b.WriteString("**Current situation:**\n")
	fmt.Fprintf(&b, "- Branch '%s' has conflicts when rebasing onto '%s'\n", branch, rebaseTarget)
	b.WriteString("- A rebase is currently in progress\n")
	fmt.Fprintf(&b, "- %d file(s) have conflicts\n\n", len(conflicts))
	b.WriteString("**Conflicted files:**\n")
	for _, f := range conflicts {
		fmt.Fprintf(&b, "- %s\n", f)
	}
	b.WriteString("\n**Your task:**\n")
	b.WriteString("1. Read each conflicted file to understand the conflicts\n")
	b.WriteString("2. Resolve the conflicts by choosing the appropriate changes or merging them\n")
	b.WriteString("3. Edit the files to remove conflict markers (<<<<<<< ======= >>>>>>>)\n")
	b.WriteString("4. Stage ALL resolved files using: `git add <file1> <file2> ...`\n")
	b.WriteString("5. Continue the rebase using: `git rebase --continue`\n")
	b.WriteString("6. If the rebase completes successfully, report back\n")
	b.WriteString("\n**Important:**\n")
	b.WriteString("- Make sure to actually execute the git commands, not just suggest them\n")
	b.WriteString("- Stage all conflicted files after resolving\n")
	b.WriteString("- Complete the entire rebase process\n")

	prompt := b.String()
	if err := session.SaveContext(branch, prompt); err != nil {
		termenv.Warn("failed to save context: %v", err)
	}
	if err := launch.AITool(launch.Options{WorktreePath: worktreePath, Prompt: prompt, Term: "foreground"}); err != nil {
		return err
	}

	termenv.Info("\n%s", termenv.Yellow("AI conflict resolution completed."))
	termenv.Info("%s\n", termenv.Yellow("Verify the resolution and re-run if needed."))
	termenv.Info("Re-run: %s to continue\n", termenv.Cyan(rerunCmd))
	return wterrors.New(wterrors.ErrAborted, "AI conflict resolution finished; re-run the command")
}

// rebaseError builds a descriptive RebaseError.
func rebaseError(worktreePath, rebaseTarget string, conflicts []string, showTip bool) error {
	var b strings.Builder
	fmt.Fprintf(&b, "rebase failed. Please resolve conflicts manually:\n  cd %s\n  git rebase %s", worktreePath, rebaseTarget)
	if len(conflicts) > 0 {
		fmt.Fprintf(&b, "\n\nConflicted files (%d):", len(conflicts))
		for _, f := range conflicts {
			fmt.Fprintf(&b, "\n  • %s", f)
		}
		if showTip {
			b.WriteString("\n\nTip: Use --ai-merge flag to get AI assistance with conflicts")
		}
	}
	return wterrors.New(wterrors.ErrRebaseFailed, "%s", b.String())
}

// unsetMetadata removes all wt metadata keys for a branch.
func unsetMetadata(repo, branch string) {
	git.UnsetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))
	git.UnsetConfig(repo, git.MetadataKey(git.KeyBasePath, branch))
	git.UnsetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, branch))
}

// CleanOptions parameterizes CleanWorktrees.
type CleanOptions struct {
	Merged      bool
	OlderThan   *int
	Interactive bool
	DryRun      bool
}

// CleanWorktrees batch-deletes worktrees by criteria.
func CleanWorktrees(opts CleanOptions) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	if !opts.Merged && opts.OlderThan == nil && !opts.Interactive {
		return wterrors.New(wterrors.ErrInvalidBranch,
			"please specify at least one cleanup criterion:\n  --merged, --older-than, or -i/--interactive")
	}

	type deletion struct {
		branch, path, reason string
	}
	var toDelete []deletion
	var ghUnavailable []string
	hasGH := git.HasCommand("gh")

	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		return err
	}

	for _, wt := range features {
		branch := wt.Branch
		shouldDelete := false
		var reasons []string

		if opts.Merged {
			base := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))
			if base != "" {
				mergedGit := false
				if res, err := git.Git(repo, false, "branch", "--merged", base, "--format=%(refname:short)"); err == nil && res.ExitCode == 0 {
					for _, line := range strings.Split(strings.TrimSpace(res.Stdout), "\n") {
						if strings.TrimSpace(line) == branch {
							mergedGit = true
							shouldDelete = true
							reasons = append(reasons, "merged into "+base)
							break
						}
					}
				}
				if !mergedGit {
					switch IsBranchMergedViaGH(branch, base, repo) {
					case GHMerged:
						shouldDelete = true
						reasons = append(reasons, "merged into "+base+" (detected via GitHub PR)")
					case GHUnknown:
						if res, _ := git.Git(repo, false, "ls-remote", "--heads", "origin", branch); res.ExitCode == 0 && strings.TrimSpace(res.Stdout) == "" {
							ghUnavailable = append(ghUnavailable, branch)
						}
					}
				}
			}
		}

		if opts.OlderThan != nil {
			if info, err := os.Stat(wt.Path); err == nil {
				ageDays := time.Since(info.ModTime()).Hours() / 24
				if ageDays > float64(*opts.OlderThan) {
					shouldDelete = true
					reasons = append(reasons, fmt.Sprintf("older than %d days (%.1f days)", *opts.OlderThan, ageDays))
				}
			}
		}

		if shouldDelete {
			toDelete = append(toDelete, deletion{branch, wt.Path, strings.Join(reasons, ", ")})
		}
	}

	if len(toDelete) == 0 && !opts.Interactive {
		termenv.Success("No worktrees match the cleanup criteria")
		fmt.Println()
		if len(ghUnavailable) > 0 && !hasGH {
			termenv.Warn("Found worktrees with deleted remote branches:\n")
			for _, b := range ghUnavailable {
				termenv.Info("  • %s", b)
			}
			termenv.Info("\n%s", termenv.Dim("These branches may have been merged via squash/rebase merge."))
			termenv.Info("%s", termenv.Dim("Install GitHub CLI (gh) to automatically detect squash/rebase merges:"))
			termenv.Info("%s\n", termenv.Dim("  https://cli.github.com/"))
		}
		return nil
	}

	if opts.Interactive {
		termenv.Info("%s\n", termenv.Bold(termenv.Cyan("Available worktrees:")))
		var all []deletion
		for _, wt := range features {
			status := string(WorktreeStatus(wt.Path, repo))
			all = append(all, deletion{wt.Branch, wt.Path, status})
			termenv.Info("  [%-8s] %-30s %s", status, wt.Branch, wt.Path)
		}
		fmt.Println()
		termenv.Info("Enter branch names to delete (space-separated), or 'all' for all:")
		fmt.Print("> ")
		var input string
		fmt.Scanln(&input)
		input = strings.TrimSpace(input)
		if strings.EqualFold(input, "all") {
			toDelete = nil
			for _, d := range all {
				toDelete = append(toDelete, deletion{d.branch, d.path, "user selected"})
			}
		} else {
			selected := map[string]bool{}
			for _, s := range strings.Fields(input) {
				selected[s] = true
			}
			toDelete = nil
			for _, d := range all {
				if selected[d.branch] {
					toDelete = append(toDelete, deletion{d.branch, d.path, "user selected"})
				}
			}
		}
		if len(toDelete) == 0 {
			termenv.Warn("No worktrees selected for deletion")
			return nil
		}
	}

	prefix := ""
	if opts.DryRun {
		prefix = "DRY RUN: "
	}
	termenv.Info("\n%s\n", termenv.Bold(termenv.Yellow(prefix+"Worktrees to delete:")))
	for _, d := range toDelete {
		termenv.Info("  • %-30s (%s)", d.branch, d.reason)
		termenv.Info("    Path: %s", d.path)
	}
	fmt.Println()

	if opts.DryRun {
		termenv.Info("%s", termenv.Bold(termenv.Cyan(fmt.Sprintf("Would delete %d worktree(s)", len(toDelete)))))
		termenv.Info("Run without --dry-run to actually delete them")
		return nil
	}

	if opts.Interactive || len(toDelete) > 3 {
		termenv.Info("%s", termenv.Bold(termenv.Red(fmt.Sprintf("Delete %d worktree(s)?", len(toDelete)))))
		if !termenv.ConfirmExact("", "yes") {
			termenv.Warn("Deletion cancelled")
			return nil
		}
	}

	fmt.Println()
	deleted := 0
	for _, d := range toDelete {
		termenv.Info("%s", termenv.Yellow("Deleting "+d.branch+"..."))
		if err := DeleteWorktree(DeleteOptions{Target: d.branch}); err != nil {
			termenv.Error("Failed to delete %s: %v", d.branch, err)
		} else {
			termenv.Success("Deleted %s", d.branch)
			deleted++
		}
	}
	termenv.Info("\n%s\n", termenv.Bold(termenv.Green(fmt.Sprintf("* Cleanup complete! Deleted %d worktree(s)", deleted))))

	termenv.Info("%s", termenv.Dim("Pruning stale worktree metadata..."))
	if _, err := git.Git(repo, false, "worktree", "prune"); err != nil {
		termenv.Warn("Failed to prune: %v", err)
	} else {
		termenv.Info("%s\n", termenv.Dim("* Prune complete"))
	}
	return nil
}
