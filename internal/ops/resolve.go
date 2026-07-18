package ops

import (
	"bufio"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/registry"
	"wt/internal/termenv"
)

// LookupMode controls target resolution: "", "branch", or "worktree".
type LookupMode string

const (
	LookupAuto     LookupMode = ""
	LookupBranch   LookupMode = "branch"
	LookupWorktree LookupMode = "worktree"
)

// Target is a resolved worktree target.
type Target struct {
	WorktreePath string
	Branch       string
	WorktreeRepo string
}

// ParseRepoBranchTarget splits "repo:branch" notation.
// ':' is safe because git forbids it in branch names.
func ParseRepoBranchTarget(target string) (repoName, branch string) {
	if name, b, ok := strings.Cut(target, ":"); ok && name != "" && b != "" {
		return name, b
	}
	return "", target
}

// findWorktreeByBranchish locates a worktree by intended branch, first as
// given, then with BranchPrefix applied: users refer to wt-managed branches
// without the internal prefix. Returns the worktree and the branch name that
// matched (the intended branch, including any prefix).
func findWorktreeByBranchish(repo, target string) (git.Worktree, string, bool) {
	if wt, found, err := git.FindWorktreeByIntendedBranch(repo, target, SanitizeBranchName); err == nil && found {
		return wt, target, true
	}
	if prefixed := PrefixBranch(target); prefixed != target {
		if wt, found, err := git.FindWorktreeByIntendedBranch(repo, prefixed, SanitizeBranchName); err == nil && found {
			return wt, prefixed, true
		}
	}
	return git.Worktree{}, "", false
}

// ResolveWorktreeTarget resolves a target string (or "" for cwd) to a
// worktree path, branch name and the worktree's git root.
func ResolveWorktreeTarget(target string, mode LookupMode, global bool) (Target, error) {
	if target == "" {
		if global {
			return Target{}, wterrors.New(wterrors.ErrWorktreeNotFound,
				"global mode requires an explicit target (branch or worktree name)")
		}
		cwd, err := os.Getwd()
		if err != nil {
			return Target{}, err
		}
		branch, err := git.CurrentBranch(cwd)
		if err != nil {
			return Target{}, wterrors.New(wterrors.ErrInvalidBranch, "cannot determine current branch")
		}
		repo, err := git.RepoRoot(cwd)
		if err != nil {
			return Target{}, err
		}
		return Target{WorktreePath: cwd, Branch: branch, WorktreeRepo: repo}, nil
	}

	if global {
		return resolveGlobalTarget(target, mode)
	}

	mainRepo, err := git.MainRepoRoot("")
	if err != nil {
		return Target{}, err
	}

	var branchMatch *git.Worktree
	matchedBranch := target
	if mode != LookupWorktree {
		if wt, branch, found := findWorktreeByBranchish(mainRepo, target); found {
			branchMatch = &wt
			matchedBranch = branch
		}
	}
	var worktreeMatch *git.Worktree
	if mode != LookupBranch {
		if wt, found, err := git.FindWorktreeByName(mainRepo, target); err == nil && found {
			worktreeMatch = &wt
		}
	}
	if mode == LookupBranch && branchMatch == nil {
		return Target{}, wterrors.New(wterrors.ErrWorktreeNotFound, "no worktree found for branch '%s'", target)
	}
	if mode == LookupWorktree && worktreeMatch == nil {
		return Target{}, wterrors.New(wterrors.ErrWorktreeNotFound, "no worktree found with name '%s'", target)
	}

	path, branch, err := resolveDualMatch(target, matchedBranch, branchMatch, worktreeMatch, mainRepo)
	if err != nil {
		return Target{}, err
	}
	repo, err := git.RepoRoot(path)
	if err != nil {
		return Target{}, err
	}
	return Target{WorktreePath: path, Branch: branch, WorktreeRepo: repo}, nil
}

// samePath reports whether two paths refer to the same directory.
func samePath(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA == nil && errB == nil {
		return ra == rb
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return filepath.Clean(aa) == filepath.Clean(bb)
}

// branchForWorktree finds the intended branch for a worktree path.
func branchForWorktree(repo, worktreePath string) string {
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		return ""
	}
	for _, wt := range wts {
		if samePath(wt.Path, worktreePath) {
			if wt.Branch == git.DetachedBranch {
				return ""
			}
			return wt.Branch
		}
	}
	return ""
}

func resolveDualMatch(target, matchedBranch string, branchMatch, worktreeMatch *git.Worktree, mainRepo string) (string, string, error) {
	switch {
	case branchMatch != nil && worktreeMatch != nil:
		if samePath(branchMatch.Path, worktreeMatch.Path) {
			return branchMatch.Path, matchedBranch, nil
		}
		if termenv.IsNonInteractive() {
			return "", "", wterrors.New(wterrors.ErrAmbiguousTarget,
				"ambiguous target '%s' matches both a branch and a worktree name.\n  Branch '%s' → %s\n  Worktree '%s' → %s\nUse --branch (-b) or --worktree (-w) flag to specify which one.",
				target, matchedBranch, branchMatch.Path, filepath.Base(worktreeMatch.Path), worktreeMatch.Path)
		}
		fmt.Printf("\n%s\n", termenv.Yellow(fmt.Sprintf("Multiple matches found for '%s':", target)))
		fmt.Printf("  [1] Branch '%s' → %s\n", matchedBranch, branchMatch.Path)
		fmt.Printf("  [2] Worktree '%s' → %s\n", filepath.Base(worktreeMatch.Path), worktreeMatch.Path)
		fmt.Println()
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Print("Which one? [1/2]: ")
			choice, _ := reader.ReadString('\n')
			switch strings.TrimSpace(choice) {
			case "1":
				return branchMatch.Path, matchedBranch, nil
			case "2":
				branch := branchForWorktree(mainRepo, worktreeMatch.Path)
				if branch == "" {
					branch = target
				}
				return worktreeMatch.Path, branch, nil
			default:
				fmt.Println(termenv.Red("Please enter 1 or 2"))
			}
		}
	case branchMatch != nil:
		return branchMatch.Path, matchedBranch, nil
	case worktreeMatch != nil:
		branch := branchForWorktree(mainRepo, worktreeMatch.Path)
		if branch == "" {
			branch = target
		}
		return worktreeMatch.Path, branch, nil
	default:
		return "", "", wterrors.New(wterrors.ErrWorktreeNotFound,
			"no worktree found for '%s'. Try: full path, branch name (--branch), or worktree name (--worktree).", target)
	}
}

type globalMatch struct {
	WorktreePath string
	Branch       string
	MainRepo     string
	RepoName     string
}

func resolveGlobalTarget(target string, mode LookupMode) (Target, error) {
	repoName, branchTarget := ParseRepoBranchTarget(target)
	paths, _, err := registry.Repositories()
	if err != nil {
		return Target{}, err
	}
	var matches []globalMatch
	for _, repoPath := range paths {
		name := filepath.Base(repoPath)
		if repoName != "" && name != repoName {
			continue
		}
		if _, err := os.Stat(repoPath); err != nil {
			continue
		}
		var branchMatch, worktreeMatch *git.Worktree
		matchedBranch := branchTarget
		if mode != LookupWorktree {
			if wt, branch, found := findWorktreeByBranchish(repoPath, branchTarget); found {
				branchMatch = &wt
				matchedBranch = branch
			}
		}
		if mode != LookupBranch {
			if wt, found, err := git.FindWorktreeByName(repoPath, branchTarget); err == nil && found {
				worktreeMatch = &wt
			}
		}
		switch {
		case branchMatch != nil && worktreeMatch != nil:
			matches = append(matches, globalMatch{branchMatch.Path, matchedBranch, repoPath, name})
			if !samePath(branchMatch.Path, worktreeMatch.Path) {
				b := branchForWorktree(repoPath, worktreeMatch.Path)
				if b == "" {
					b = branchTarget
				}
				matches = append(matches, globalMatch{worktreeMatch.Path, b, repoPath, name})
			}
		case branchMatch != nil:
			matches = append(matches, globalMatch{branchMatch.Path, matchedBranch, repoPath, name})
		case worktreeMatch != nil:
			b := branchForWorktree(repoPath, worktreeMatch.Path)
			if b == "" {
				b = branchTarget
			}
			matches = append(matches, globalMatch{worktreeMatch.Path, b, repoPath, name})
		}
	}
	if len(matches) == 0 {
		return Target{}, wterrors.New(wterrors.ErrWorktreeNotFound,
			"'%s' not found in any registered repository. Run 'wt scan' to register repos.", target)
	}
	m := matches[0]
	if len(matches) > 1 {
		if termenv.IsNonInteractive() {
			var b strings.Builder
			fmt.Fprintf(&b, "ambiguous target '%s' found in multiple repositories:", target)
			for i, m := range matches {
				fmt.Fprintf(&b, "\n  [%d] %s:%s → %s", i+1, m.RepoName, m.Branch, m.WorktreePath)
			}
			b.WriteString("\nUse 'repo:branch' notation to specify directly.")
			return Target{}, wterrors.New(wterrors.ErrAmbiguousTarget, "%s", b.String())
		}
		fmt.Printf("\n%s\n", termenv.Yellow(fmt.Sprintf("Multiple matches found for '%s':", target)))
		for i, m := range matches {
			fmt.Printf("  [%d] %s:%s → %s\n", i+1, m.RepoName, m.Branch, m.WorktreePath)
		}
		fmt.Println(termenv.Dim("Tip: Use 'repo:branch' notation to specify directly."))
		fmt.Println()
		reader := bufio.NewReader(os.Stdin)
		for {
			fmt.Printf("Which one? [1-%d]: ", len(matches))
			choice, _ := reader.ReadString('\n')
			var idx int
			if _, err := fmt.Sscanf(strings.TrimSpace(choice), "%d", &idx); err == nil && idx >= 1 && idx <= len(matches) {
				m = matches[idx-1]
				break
			}
			fmt.Println(termenv.Red(fmt.Sprintf("Please enter a number between 1 and %d", len(matches))))
		}
	}
	repo, err := git.RepoRoot(m.WorktreePath)
	if err != nil {
		return Target{}, err
	}
	return Target{WorktreePath: m.WorktreePath, Branch: m.Branch, WorktreeRepo: repo}, nil
}

// WorktreeMetadata returns the base branch and base repo path for a feature
// branch, inferring them when metadata is missing.
func WorktreeMetadata(branch, repo string) (baseBranch, basePath string, err error) {
	baseBranch = git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))
	basePathStr := git.GetConfig(repo, git.MetadataKey(git.KeyBasePath, branch))
	if baseBranch != "" && basePathStr != "" {
		return baseBranch, basePathStr, nil
	}

	termenv.Warn("Metadata missing for branch '%s'", branch)
	termenv.Info("%s", termenv.Dim("Attempting to infer metadata automatically..."))

	wts, err := git.ParseWorktrees(repo)
	if err != nil || len(wts) == 0 {
		return "", "", wterrors.New(wterrors.ErrInvalidBranch,
			"cannot infer base repository path for branch '%s'. Please use 'wt new' to create worktrees.", branch)
	}
	basePath = wts[0].Path
	for _, candidate := range []string{"main", "master", "develop"} {
		if git.BranchExists(basePath, candidate) {
			baseBranch = candidate
			break
		}
	}
	if baseBranch == "" && wts[0].Branch != git.DetachedBranch {
		baseBranch = wts[0].Branch
	}
	if baseBranch == "" {
		return "", "", wterrors.New(wterrors.ErrInvalidBranch,
			"cannot infer base branch for '%s'. Please specify manually or use 'wt new' to create worktrees.", branch)
	}
	termenv.Info("  %s", termenv.Dim("Inferred base branch: "+baseBranch))
	termenv.Info("  %s", termenv.Dim("Inferred base path: "+basePath))
	termenv.Info("%s", termenv.Dim("Tip: Use 'wt new' to create worktrees with proper metadata."))
	return baseBranch, basePath, nil
}
