package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"wt/internal/config"
	"wt/internal/git"
	"wt/internal/ops"
)

// exitError carries a command's exit code through cobra.
type exitError struct{ code int }

func (e *exitError) Error() string { return fmt.Sprintf("exit status %d", e.code) }

// ExitCode extracts a process exit code from an error chain.
func ExitCode(err error) int {
	if ee, ok := err.(*exitError); ok {
		return ee.code
	}
	return 1
}

func errf(format string, args ...any) error {
	return fmt.Errorf(format, args...)
}

// resolveIfWorktree checks whether a name resolves to a worktree (used by
// the shell command's "first arg is worktree or command" heuristic).
func resolveIfWorktree(name string) (string, bool, error) {
	repo, err := git.MainRepoRoot("")
	if err != nil {
		return "", false, err
	}
	if wt, found, err := git.FindWorktreeByIntendedBranch(repo, name, ops.SanitizeBranchName); err == nil && found {
		return wt.Path, true, nil
	}
	if wt, found, err := git.FindWorktreeByBranch(repo, name); err == nil && found {
		return wt.Path, true, nil
	}
	return "", false, nil
}

// --- shell completion helpers ---

// completeWorktreeBranches lists local worktree branches + dir names, or
// repo:branch across the registry in global mode.
func completeWorktreeBranches(toComplete string) []string {
	if globalMode {
		return completeGlobalTargets(toComplete)
	}
	repo, err := git.MainRepoRoot("")
	if err != nil {
		return nil
	}
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		return nil
	}
	var out []string
	for i, wt := range wts {
		if i == 0 {
			continue
		}
		if wt.Branch != git.DetachedBranch {
			out = append(out, wt.Branch)
		}
	}
	return out
}

func completeGlobalTargets(toComplete string) []string {
	return ops.GlobalTargets(toComplete)
}

// completeAllBranches lists all local+remote branches (remote stripped).
func completeAllBranches(toComplete string) []string {
	repo, err := git.RepoRoot("")
	if err != nil {
		return nil
	}
	out, err := git.Output(repo, "branch", "-a", "--format=%(refname:short)")
	if err != nil {
		return nil
	}
	seen := map[string]bool{}
	var branches []string
	for _, line := range splitLines(out) {
		if line == "" || line == "HEAD" {
			continue
		}
		b := line
		if rest, ok := cutPrefix(b, "origin/"); ok {
			b = rest
		}
		if !seen[b] {
			seen[b] = true
			branches = append(branches, b)
		}
	}
	return branches
}

// completeNewBranchNames lists branches without worktrees (and not current).
func completeNewBranchNames(toComplete string) []string {
	repo, err := git.RepoRoot("")
	if err != nil {
		return nil
	}
	current, _ := git.CurrentBranch(repo)
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		return nil
	}
	hasWorktree := map[string]bool{}
	for _, wt := range wts {
		hasWorktree[wt.Branch] = true
	}
	var out []string
	for _, b := range completeAllBranches("") {
		if b != current && !hasWorktree[b] {
			out = append(out, b)
		}
	}
	return out
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	if start < len(s) {
		out = append(out, s[start:])
	}
	return out
}

func cutPrefix(s, prefix string) (string, bool) {
	if len(s) >= len(prefix) && s[:len(prefix)] == prefix {
		return s[len(prefix):], true
	}
	return "", false
}

// branchFlagCompletion completes branch names for flags.
func branchFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return completeAllBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
}

// termFlagCompletion completes launch method names.
func termFlagCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	var out []string
	for _, m := range config.AllMethods() {
		out = append(out, string(m))
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}

// presetFlagCompletion completes AI tool preset names.
func presetCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var out []string
	for _, p := range config.AIToolPresets {
		out = append(out, p.Name)
	}
	return out, cobra.ShellCompDirectiveNoFileComp
}
