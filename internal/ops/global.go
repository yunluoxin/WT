package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"time"

	"wt/internal/git"
	"wt/internal/registry"
	"wt/internal/termenv"
)

// GlobalListWorktrees lists worktrees across all registered repositories.
func GlobalListWorktrees() error {
	// Auto-prune stale entries first.
	removed, _ := registry.Prune()
	if len(removed) > 0 {
		termenv.Info("%s", termenv.Dim(fmt.Sprintf("Pruned %d stale registry entr%s", len(removed), plural(len(removed), "y", "ies"))))
	}
	paths, entries, err := registry.Repositories()
	if err != nil {
		return err
	}
	_ = entries
	if len(paths) == 0 {
		termenv.Info("\n%s\n", termenv.Yellow("No repositories registered. Run 'wt scan' to discover repositories."))
		return nil
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Global worktrees (all registered repositories)")))
	totalFeatures := 0
	for _, repoPath := range paths {
		termenv.Info("%s %s", termenv.Bold(filepath.Base(repoPath)), termenv.Dim(repoPath))
		wts, err := git.ParseWorktrees(repoPath)
		if err != nil {
			termenv.Info("  %s\n", termenv.Red("(failed to read worktrees)"))
			continue
		}
		features := 0
		for i, wt := range wts {
			if i == 0 {
				continue
			}
			status := WorktreeStatus(wt.Path, repoPath)
			age := ""
			if info, err := os.Stat(wt.Path); err == nil {
				age = FormatAge(time.Since(info.ModTime()).Hours() / 24)
			}
			termenv.Info("  %-30s %s %-12s %s",
				git.NormalizeBranchName(wt.Branch),
				statusColor(status)(fmt.Sprintf("%-10s", status)),
				age, wt.Path)
			features++
		}
		if features == 0 {
			termenv.Info("  %s", termenv.Dim("(no feature worktrees)"))
		}
		totalFeatures += features
		fmt.Println()
	}
	termenv.Info("%s\n", termenv.Dim(fmt.Sprintf("%d repositories, %d feature worktrees", len(paths), totalFeatures)))
	return nil
}

// ScanRepos discovers repositories with worktrees under dir and registers them.
func ScanRepos(dir string) error {
	if dir == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return err
		}
		dir = home
	}
	termenv.Info("Scanning %s for repositories with worktrees...\n", dir)
	found, err := registry.ScanForRepos(dir)
	if err != nil {
		return err
	}
	if len(found) == 0 {
		termenv.Info("%s\n", termenv.Yellow("No repositories with worktrees found."))
		return nil
	}
	termenv.Success("Found %d repositories:", len(found))
	for _, p := range found {
		termenv.Info("  • %s", p)
	}
	fmt.Println()
	return nil
}

// PruneRegistry removes stale registry entries.
func PruneRegistry() error {
	removed, err := registry.Prune()
	if err != nil {
		return err
	}
	if len(removed) == 0 {
		termenv.Success("Registry is clean, no stale entries\n")
		return nil
	}
	termenv.Success("Removed %d stale entries:", len(removed))
	for _, p := range removed {
		termenv.Info("  • %s", p)
	}
	fmt.Println()
	return nil
}

// GlobalTargets returns "repo:branch" completion candidates across the registry.
func GlobalTargets(toComplete string) []string {
	paths, _, err := registry.Repositories()
	if err != nil {
		return nil
	}
	var out []string
	for _, repoPath := range paths {
		name := filepath.Base(repoPath)
		wts, err := git.ParseWorktrees(repoPath)
		if err != nil {
			continue
		}
		for i, wt := range wts {
			if i == 0 || wt.Branch == git.DetachedBranch {
				continue
			}
			out = append(out, name+":"+git.NormalizeBranchName(wt.Branch))
		}
	}
	sort.Strings(out)
	return out
}

func plural(n int, singular, pluralForm string) string {
	if n == 1 {
		return singular
	}
	return pluralForm
}
