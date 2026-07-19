package ops

import (
	"fmt"
	"os"
	"sort"

	"wt/internal/git"
	"wt/internal/hooks"
	"wt/internal/termenv"
)

// SyncOptions parameterizes SyncWorktrees.
type SyncOptions struct {
	Target     string
	All        bool
	FetchOnly  bool
	AIMerge    bool
	LookupMode LookupMode
	Global     bool
}

type syncItem struct {
	branch string
	path   string
}

// topologicalSort orders worktrees so base worktrees sync before their
// dependents (Kahn's algorithm, alphabetical tie-break).
func topologicalSort(items []syncItem, repo string) []syncItem {
	inDegree := map[string]int{}
	dependents := map[string][]string{}
	paths := map[string]string{}
	isWorktree := map[string]bool{}
	for _, it := range items {
		inDegree[it.branch] = 0
		paths[it.branch] = it.path
		isWorktree[it.branch] = true
	}
	for _, it := range items {
		base := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, it.branch))
		if base != "" && isWorktree[base] {
			dependents[base] = append(dependents[base], it.branch)
			inDegree[it.branch]++
		}
	}
	var queue []string
	for b, d := range inDegree {
		if d == 0 {
			queue = append(queue, b)
		}
	}
	var order []string
	for len(queue) > 0 {
		sort.Strings(queue)
		cur := queue[0]
		queue = queue[1:]
		order = append(order, cur)
		for _, dep := range dependents[cur] {
			inDegree[dep]--
			if inDegree[dep] == 0 {
				queue = append(queue, dep)
			}
		}
	}
	if len(order) != len(items) {
		termenv.Warn("Circular dependency detected in worktree base branches. Syncing in original order.")
		return items
	}
	var out []syncItem
	for _, b := range order {
		if isWorktree[b] {
			out = append(out, syncItem{b, paths[b]})
		}
	}
	return out
}

// SyncWorktrees rebases worktrees onto their base branches.
func SyncWorktrees(opts SyncOptions) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}

	var items []syncItem
	if opts.All {
		wts, err := git.ParseWorktrees(repo)
		if err != nil {
			return err
		}
		for _, wt := range wts {
			if wt.Branch == git.DetachedBranch {
				continue
			}
			items = append(items, syncItem{git.NormalizeBranchName(wt.Branch), wt.Path})
		}
		if len(items) == 0 {
			termenv.Warn("No worktrees found")
			return nil
		}
		items = topologicalSort(items, repo)
	} else {
		t, err := ResolveWorktreeTarget(opts.Target, opts.LookupMode, opts.Global)
		if err != nil {
			return err
		}
		items = []syncItem{{t.Branch, t.WorktreePath}}
	}

	firstBranch, firstPath := items[0].branch, items[0].path
	hookCtx := hooks.Context{
		Branch:       firstBranch,
		BaseBranch:   git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, firstBranch)),
		WorktreePath: firstPath, RepoPath: repo, Operation: "sync",
	}
	if err := hooks.RunHooks(repo, "sync.pre", hookCtx, repo); err != nil {
		return err
	}

	termenv.Info("%s", termenv.Yellow("Fetching updates from remote..."))
	fetchRes, _ := git.Git(repo, false, "fetch", "--all", "--prune")
	fetchOK := fetchRes.ExitCode == 0
	if !fetchOK {
		termenv.Warn("Fetch failed or no remote configured\n")
	}

	if opts.FetchOnly {
		termenv.Success("Fetch complete\n")
		return nil
	}

	for _, item := range items {
		if err := syncOne(item, repo, fetchOK, opts); err != nil {
			if opts.All {
				termenv.Error("%v", err)
				termenv.Warn("Continuing with remaining worktrees...")
				continue
			}
			return err
		}
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Green("* Sync complete!")))
	_ = hooks.RunHooks(repo, "sync.post", hookCtx, repo)
	return nil
}

func syncOne(item syncItem, repo string, fetchOK bool, opts SyncOptions) error {
	branch, path := item.branch, item.path
	base := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))

	if base == "" {
		// No metadata: fall back to origin/<branch>.
		if !fetchOK {
			termenv.Warn("Skipping %s: No metadata (not created with 'wt new') and fetch failed\n", branch)
			return nil
		}
		if res, _ := git.Git(path, false, "rev-parse", "--verify", "origin/"+branch); res.ExitCode != 0 {
			termenv.Info("\n%s\n", termenv.Dim(fmt.Sprintf("Skipping %s: No metadata and no origin/%s found", branch, branch)))
			return nil
		}
		termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Syncing worktree:")))
		termenv.Info("  Branch:  %s", termenv.Green(branch))
		termenv.Info("  Path:    %s\n", termenv.Cyan(path))
		termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Rebasing %s onto origin/%s...", branch, branch)))
		if _, err := git.Git(path, true, "rebase", "origin/"+branch); err != nil {
			conflicts := conflictedFiles(path)
			if len(conflicts) > 0 && opts.AIMerge {
				return aiResolveConflicts(path, branch, "origin/"+branch, conflicts, rerunHint(opts.All))
			}
			_, _ = git.Git(path, false, "rebase", "--abort")
			return rebaseError(path, "origin/"+branch, conflicts, !opts.AIMerge)
		}
		termenv.Success("Rebase successful")
		return nil
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Syncing worktree:")))
	termenv.Info("  Feature: %s", termenv.Green(branch))
	termenv.Info("  Base:    %s", termenv.Green(base))
	termenv.Info("  Path:    %s\n", termenv.Cyan(path))

	rebaseTarget := base
	if fetchOK {
		if res, _ := git.Git(path, false, "rev-parse", "--verify", "origin/"+base); res.ExitCode == 0 {
			rebaseTarget = "origin/" + base
		}
	}
	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Rebasing %s onto %s...", branch, rebaseTarget)))
	if _, err := git.Git(path, true, "rebase", rebaseTarget); err != nil {
		conflicts := conflictedFiles(path)
		if len(conflicts) > 0 && opts.AIMerge {
			return aiResolveConflicts(path, branch, rebaseTarget, conflicts, rerunHint(opts.All))
		}
		_, _ = git.Git(path, false, "rebase", "--abort")
		return rebaseError(path, rebaseTarget, conflicts, !opts.AIMerge)
	}
	termenv.Success("Rebase successful")
	return nil
}

func rerunHint(all bool) string {
	if all {
		return "wt sync --all --ai"
	}
	return "wt sync"
}

// ChangeBaseOptions parameterizes ChangeBase.
type ChangeBaseOptions struct {
	NewBase     string
	Target      string
	Interactive bool
	DryRun      bool
	LookupMode  LookupMode
	Global      bool
}

// ChangeBase rebases a worktree onto a new base branch and updates metadata.
func ChangeBase(opts ChangeBaseOptions) error {
	t, err := ResolveWorktreeTarget(opts.Target, opts.LookupMode, opts.Global)
	if err != nil {
		return err
	}
	path, branch := t.WorktreePath, t.Branch

	repo := t.WorktreeRepo
	currentBase := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))
	if currentBase == "" {
		return fmt.Errorf("no base branch metadata found for '%s'.\nWas this worktree created with 'wt new'?\nOnly worktrees created by wt can change their base branch", branch)
	}
	if !git.BranchExists(repo, opts.NewBase) {
		return fmt.Errorf("base branch '%s' not found", opts.NewBase)
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Changing base branch:")))
	termenv.Info("  Feature:      %s", termenv.Green(branch))
	termenv.Info("  Current base: %s", termenv.Yellow(currentBase))
	termenv.Info("  New base:     %s\n", termenv.Green(opts.NewBase))

	if opts.DryRun {
		termenv.Info("%s\n", termenv.Bold(termenv.Yellow("DRY RUN MODE - No changes will be made")))
		termenv.Info("%s\n", termenv.Bold("The following operations would be performed:"))
		termenv.Info("  1. %s updates from remote", termenv.Cyan("Fetch"))
		termenv.Info("  2. %s %s onto %s", termenv.Cyan("Rebase"), branch, opts.NewBase)
		termenv.Info("  3. %s base branch metadata to %s", termenv.Cyan("Update"), opts.NewBase)
		termenv.Info("\n%s\n", termenv.Dim("Run without --dry-run to execute these operations."))
		return nil
	}

	_, _ = git.Git(repo, false, "fetch", "--all", "--prune")
	rebaseTarget := opts.NewBase
	if res, _ := git.Git(path, false, "rev-parse", "--verify", "origin/"+opts.NewBase); res.ExitCode == 0 {
		rebaseTarget = "origin/" + opts.NewBase
	}

	termenv.Info("%s", termenv.Yellow(fmt.Sprintf("Rebasing %s onto %s...", branch, rebaseTarget)))
	if opts.Interactive {
		if err := git.Run(path, "rebase", "--interactive", rebaseTarget); err != nil {
			_, _ = git.Git(path, false, "rebase", "--abort")
			conflicts := conflictedFiles(path)
			return rebaseError(path, rebaseTarget, conflicts, false)
		}
	} else if _, err := git.Git(path, true, "rebase", rebaseTarget); err != nil {
		_, _ = git.Git(path, false, "rebase", "--abort")
		conflicts := conflictedFiles(path)
		return rebaseError(path, rebaseTarget, conflicts, false)
	}

	if err := git.SetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch), opts.NewBase); err != nil {
		return err
	}
	termenv.Success("Base branch changed to '%s'", opts.NewBase)
	fmt.Println()
	return nil
}

// ShellWorktree opens an interactive shell or runs a command in a worktree.
func ShellWorktree(worktree string, command []string) (int, error) {
	_, err := git.RepoRoot("")
	if err != nil {
		return 1, err
	}
	var targetPath string
	branchName := worktree
	if worktree != "" {
		t, err := ResolveWorktreeTarget(worktree, LookupAuto, false)
		if err != nil {
			return 1, err
		}
		targetPath = t.WorktreePath
		branchName = t.Branch
	} else {
		targetPath, err = os.Getwd()
		if err != nil {
			return 1, err
		}
		if _, err := git.CurrentBranch(targetPath); err != nil {
			return 1, fmt.Errorf("not in a git repository or worktree")
		}
	}
	if _, err := os.Stat(targetPath); err != nil {
		return 1, fmt.Errorf("worktree directory does not exist: %s", targetPath)
	}

	if len(command) > 0 {
		termenv.Info("%s %v\n", termenv.Cyan(fmt.Sprintf("Executing in %s:", targetPath)), command)
		return runInDir(targetPath, command[0], command[1:]...)
	}

	if branchName == "" {
		branchName, _ = git.CurrentBranch(targetPath)
	}
	termenv.Info("%s %s\n%s\n%s\n",
		termenv.Bold(termenv.Cyan("Opening shell in worktree:")), branchName,
		termenv.Dim("Path: "+targetPath),
		termenv.Dim("Type 'exit' to return"))
	shell := os.Getenv("SHELL")
	if shell == "" {
		shell = defaultShell()
	}
	code, err := runInDir(targetPath, shell)
	return code, err
}
