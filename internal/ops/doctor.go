package ops

import (
	"fmt"
	"strconv"
	"strings"

	"wt/internal/git"
	"wt/internal/termenv"
)

// Doctor performs a health check on all worktrees.
func Doctor() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("🏥 wt Health Check")))

	issues, warnings := 0, 0

	// 1. Git version.
	termenv.Info("%s", termenv.Bold("1. Checking Git version..."))
	versionStr := ""
	if out, err := git.Output("", "--version"); err == nil {
		parts := strings.Fields(out)
		if len(parts) >= 3 {
			versionStr = parts[2]
		} else if len(parts) > 0 {
			versionStr = parts[len(parts)-1]
		}
		if versionOK(versionStr, "2.31.0") {
			termenv.Info("   %s Git version %s (minimum: 2.31.0)", termenv.Green("*"), versionStr)
		} else {
			termenv.Info("   %s Git version %s is too old (minimum: 2.31.0)", termenv.Red("x"), versionStr)
			issues++
		}
	} else {
		termenv.Info("   %s Could not detect Git version", termenv.Red("x"))
		issues++
	}
	fmt.Println()

	// 2. Worktree accessibility.
	termenv.Info("%s", termenv.Bold("2. Checking worktree accessibility..."))
	type wtInfo struct {
		branch, path string
		status       Status
	}
	var worktrees []wtInfo
	staleCount := 0
	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		return err
	}
	for _, wt := range features {
		status := WorktreeStatus(wt.Path, repo)
		worktrees = append(worktrees, wtInfo{wt.Branch, wt.Path, status})
		if status == StatusStale {
			staleCount++
			termenv.Info("   %s %s: Stale (directory missing)", termenv.Red("x"), wt.Branch)
			issues++
		}
	}
	if staleCount == 0 {
		termenv.Info("   %s All %d worktrees are accessible", termenv.Green("*"), len(worktrees))
	} else {
		termenv.Info("   %s %d stale worktree(s) found (use 'wt clean')", termenv.Yellow("!"), staleCount)
	}
	fmt.Println()

	// 3. Uncommitted changes.
	termenv.Info("%s", termenv.Bold("3. Checking for uncommitted changes..."))
	var dirty []string
	for _, w := range worktrees {
		if w.status == StatusModified || w.status == StatusActive {
			if res, err := git.Git(w.path, false, "status", "--porcelain"); err == nil && strings.TrimSpace(res.Stdout) != "" {
				dirty = append(dirty, w.branch)
			}
		}
	}
	if len(dirty) > 0 {
		termenv.Info("   %s %d worktree(s) with uncommitted changes:", termenv.Yellow("!"), len(dirty))
		for _, b := range dirty {
			termenv.Info("      • %s", b)
		}
		warnings++
	} else {
		termenv.Info("   %s No uncommitted changes", termenv.Green("*"))
	}
	fmt.Println()

	// 4. Behind base branch.
	termenv.Info("%s", termenv.Bold("4. Checking if worktrees are behind base branch..."))
	type behindInfo struct {
		branch, base, count string
	}
	var behind []behindInfo
	for _, w := range worktrees {
		if w.status == StatusStale {
			continue
		}
		base := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, w.branch))
		if base == "" {
			continue
		}
		_, _ = git.Git(w.path, false, "fetch", "--all", "--prune")
		mergeBase, err1 := git.Output(w.path, "merge-base", w.branch, "origin/"+base)
		baseCommit, err2 := git.Output(w.path, "rev-parse", "origin/"+base)
		if err1 != nil || err2 != nil || mergeBase == baseCommit {
			continue
		}
		if count, err := git.Output(w.path, "rev-list", "--count", w.branch+"..origin/"+base); err == nil {
			behind = append(behind, behindInfo{w.branch, base, count})
		}
	}
	if len(behind) > 0 {
		termenv.Info("   %s %d worktree(s) behind base branch:", termenv.Yellow("!"), len(behind))
		for _, b := range behind {
			termenv.Info("      • %s: %s commit(s) behind %s", b.branch, b.count, b.base)
		}
		termenv.Info("   %s", termenv.Dim("Tip: Use 'wt sync --all' to update all worktrees"))
		warnings++
	} else {
		termenv.Info("   %s All worktrees are up-to-date with base", termenv.Green("*"))
	}
	fmt.Println()

	// 5. Merge conflicts.
	termenv.Info("%s", termenv.Bold("5. Checking for merge conflicts..."))
	type conflictInfo struct {
		branch string
		files  []string
	}
	var conflicted []conflictInfo
	for _, w := range worktrees {
		if w.status == StatusStale {
			continue
		}
		if files := conflictedFiles(w.path); len(files) > 0 {
			conflicted = append(conflicted, conflictInfo{w.branch, files})
		}
	}
	if len(conflicted) > 0 {
		termenv.Info("   %s %d worktree(s) with merge conflicts:", termenv.Red("x"), len(conflicted))
		for _, c := range conflicted {
			termenv.Info("      • %s: %d conflicted file(s)", c.branch, len(c.files))
		}
		termenv.Info("   %s", termenv.Dim("Tip: Use 'wt sync --ai-merge' for AI-assisted resolution"))
		issues++
	} else {
		termenv.Info("   %s No merge conflicts detected", termenv.Green("*"))
	}
	fmt.Println()

	// Summary.
	termenv.Info("%s", termenv.Bold(termenv.Cyan("Summary:")))
	if issues == 0 && warnings == 0 {
		termenv.Info("%s\n", termenv.Bold(termenv.Green("* Everything looks healthy!")))
	} else {
		if issues > 0 {
			termenv.Info("%s", termenv.Bold(termenv.Red(fmt.Sprintf("x %d issue(s) found", issues))))
		}
		if warnings > 0 {
			termenv.Info("%s", termenv.Bold(termenv.Yellow(fmt.Sprintf("! %d warning(s) found", warnings))))
		}
		fmt.Println()
	}

	// Recommendations.
	printedHeader := false
	header := func() {
		if !printedHeader {
			termenv.Info("%s", termenv.Bold("Recommendations:"))
			printedHeader = true
		}
	}
	if staleCount > 0 {
		header()
		termenv.Info("  • Run %s to clean up stale worktrees", termenv.Cyan("wt clean"))
	}
	if len(behind) > 0 {
		header()
		termenv.Info("  • Run %s to update all worktrees", termenv.Cyan("wt sync --all"))
	}
	if len(conflicted) > 0 {
		header()
		termenv.Info("  • Resolve conflicts in conflicted worktrees")
		termenv.Info("  • Use %s for AI assistance", termenv.Cyan("wt sync --ai-merge"))
	}
	if printedHeader {
		fmt.Println()
	}
	return nil
}

// versionOK compares dotted version strings (sufficient for git versions).
func versionOK(have, want string) bool {
	parse := func(s string) []int {
		var out []int
		for _, p := range strings.Split(s, ".") {
			n, _ := strconv.Atoi(p)
			out = append(out, n)
		}
		return out
	}
	h, w := parse(have), parse(want)
	for i := 0; i < len(w); i++ {
		var hv int
		if i < len(h) {
			hv = h[i]
		}
		if hv != w[i] {
			return hv > w[i]
		}
	}
	return true
}
