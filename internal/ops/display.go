package ops

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

// Status represents a worktree's health.
type Status string

const (
	StatusStale    Status = "stale"
	StatusActive   Status = "active"
	StatusModified Status = "modified"
	StatusClean    Status = "clean"
)

// WorktreeStatus determines the status of a worktree.
func WorktreeStatus(path, repo string) Status {
	if _, err := os.Stat(path); err != nil {
		return StatusStale
	}
	if cwd, err := os.Getwd(); err == nil && strings.HasPrefix(cwd, path) {
		return StatusActive
	}
	if res, err := git.Git(path, false, "status", "--porcelain"); err == nil && res.ExitCode == 0 && strings.TrimSpace(res.Stdout) != "" {
		return StatusModified
	}
	return StatusClean
}

// FormatAge renders an age in days as a human string.
func FormatAge(ageDays float64) string {
	switch {
	case ageDays < 1:
		hours := int(ageDays * 24)
		if hours > 0 {
			return fmt.Sprintf("%dh ago", hours)
		}
		return "just now"
	case ageDays < 7:
		return fmt.Sprintf("%dd ago", int(ageDays))
	case ageDays < 30:
		return fmt.Sprintf("%dw ago", int(ageDays/7))
	case ageDays < 365:
		return fmt.Sprintf("%dmo ago", int(ageDays/30))
	default:
		return fmt.Sprintf("%dy ago", int(ageDays/365))
	}
}

func statusColor(s Status) func(string) string {
	switch s {
	case StatusActive:
		return termenv.Green
	case StatusClean:
		return termenv.Green
	case StatusModified:
		return termenv.Yellow
	case StatusStale:
		return termenv.Red
	}
	return func(s string) string { return s }
}

type worktreeRow struct {
	ID      string
	Branch  string
	Status  Status
	Age     string
	RelPath string
}

// collectRows gathers display data for all worktrees of a repo.
func collectRows(repo string) ([]worktreeRow, error) {
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		return nil, err
	}
	var rows []worktreeRow
	for _, wt := range wts {
		current := git.NormalizeBranchName(wt.Branch)
		status := WorktreeStatus(wt.Path, repo)
		relPath, err := filepath.Rel(repo, wt.Path)
		if err != nil {
			relPath = wt.Path
		}
		age := ""
		if info, err := os.Stat(wt.Path); err == nil {
			age = FormatAge(time.Since(info.ModTime()).Hours() / 24)
		}
		id := intendedBranchFor(repo, wt)
		if id == "" {
			id = current
		}
		rows = append(rows, worktreeRow{ID: id, Branch: current, Status: status, Age: age, RelPath: relPath})
	}
	return rows, nil
}

// intendedBranchFor finds the intended branch for a worktree (metadata or
// path naming convention).
func intendedBranchFor(repo string, wt git.Worktree) string {
	current := git.NormalizeBranchName(wt.Branch)
	if v := git.GetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, current)); v != "" {
		return v
	}
	meta := git.ConfigRegexp(repo, `^worktree\..*\.intendedBranch`)
	repoName := filepath.Base(repo)
	for key, value := range meta {
		name := strings.TrimSuffix(strings.TrimPrefix(key, "worktree."), ".intendedBranch")
		if filepath.Base(wt.Path) == repoName+"-"+SanitizeBranchName(name) {
			return value
		}
	}
	return ""
}

// ListWorktrees prints the worktree table for the current repository.
func ListWorktrees() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	rows, err := collectRows(repo)
	if err != nil {
		return err
	}
	termenv.Info("\n%s %s\n", termenv.Bold(termenv.Cyan("Worktrees for repository:")), repo)

	if termenv.Width() >= 100 {
		printWorktreeTable(rows)
	} else {
		printWorktreeCompact(rows)
	}

	featureCount := len(rows) - 1
	if featureCount > 0 {
		counts := map[Status]int{}
		for _, r := range rows {
			counts[r.Status]++
		}
		var parts []string
		for _, s := range []Status{StatusClean, StatusModified, StatusActive, StatusStale} {
			if counts[s] > 0 {
				parts = append(parts, statusColor(s)(fmt.Sprintf("%d %s", counts[s], s)))
			}
		}
		summary := fmt.Sprintf("\n%d feature worktree(s)", featureCount)
		if len(parts) > 0 {
			summary += " — " + strings.Join(parts, ", ")
		}
		termenv.Info("%s", summary)
	}
	fmt.Println()
	return nil
}

func printWorktreeTable(rows []worktreeRow) {
	maxID, maxBranch := 20, 20
	for _, r := range rows {
		if l := len(r.ID) + 2; l > maxID {
			maxID = min(l, 35)
		}
		if l := len(r.Branch) + 2; l > maxBranch {
			maxBranch = min(l, 35)
		}
	}
	termenv.Info("%-*s %-*s %-10s %-12s PATH", maxID, "WORKTREE", maxBranch, "CURRENT BRANCH", "STATUS", "AGE")
	termenv.Info("%s", strings.Repeat("─", maxID+maxBranch+72))
	for _, r := range rows {
		branchDisplay := r.Branch
		if r.ID != r.Branch {
			branchDisplay = termenv.Yellow(r.Branch + " (⚠️)")
		}
		termenv.Info("%-*s %-*s %s %-*s %s",
			maxID, r.ID, maxBranch, branchDisplay,
			statusColor(r.Status)(fmt.Sprintf("%-10s", r.Status)),
			12, r.Age, r.RelPath)
	}
}

func printWorktreeCompact(rows []worktreeRow) {
	for _, r := range rows {
		agePart := ""
		if r.Age != "" {
			agePart = "  " + r.Age
		}
		termenv.Info("  %s  %s%s", termenv.Bold(r.ID), statusColor(r.Status)(string(r.Status)), agePart)
		var details []string
		if r.ID != r.Branch {
			details = append(details, "branch: "+termenv.Yellow(r.Branch+" (⚠️)"))
		}
		details = append(details, "path: "+r.RelPath)
		termenv.Info("    %s", strings.Join(details, " · "))
	}
}

// ShowStatus prints the current worktree metadata plus the full list.
func ShowStatus() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	if branch, err := git.CurrentBranch(cwd); err == nil {
		base := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch))
		basePath := git.GetConfig(repo, git.MetadataKey(git.KeyBasePath, branch))
		or := func(s string) string {
			if s == "" {
				return "N/A"
			}
			return s
		}
		termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Current worktree:")))
		termenv.Info("  Feature:   %s", termenv.Green(branch))
		termenv.Info("  Base:      %s", termenv.Green(or(base)))
		termenv.Info("  Base path: %s\n", termenv.Cyan(or(basePath)))
	} else {
		termenv.Info("\n%s\n", termenv.Yellow("Current directory is not a feature worktree or is the main repository."))
	}
	return ListWorktrees()
}

var statusIcons = map[Status]string{
	StatusActive:   "●",
	StatusClean:    "○",
	StatusModified: "◉",
	StatusStale:    "x",
}

// ShowTree displays the worktree hierarchy as an ASCII tree.
func ShowTree() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	cwd, _ := os.Getwd()
	termenv.Info("\n%s (base repository)", termenv.Bold(termenv.Cyan(filepath.Base(repo)+"/")))
	termenv.Info("%s\n", termenv.Dim(repo))

	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		return err
	}
	if len(features) == 0 {
		termenv.Info("%s\n", termenv.Dim("  (no feature worktrees)"))
		return nil
	}
	sort.Slice(features, func(i, j int) bool { return features[i].Branch < features[j].Branch })

	for i, wt := range features {
		isLast := i == len(features)-1
		prefix := "├── "
		cont := "│   "
		if isLast {
			prefix = "└── "
			cont = "    "
		}
		status := WorktreeStatus(wt.Path, repo)
		color := statusColor(status)
		branchDisplay := color(wt.Branch)
		if strings.HasPrefix(cwd, wt.Path) {
			branchDisplay = termenv.Bold(color("★ " + wt.Branch))
		}
		termenv.Info("%s%s %s", prefix, color(statusIcons[status]), branchDisplay)
		relPath, err := filepath.Rel(filepath.Dir(repo), wt.Path)
		if err != nil {
			relPath = wt.Path
		}
		termenv.Info("%s%s", cont, termenv.Dim(relPath))
	}

	termenv.Info("\n%s", termenv.Bold("Legend:"))
	termenv.Info("  %s active (current)", termenv.Green("●"))
	termenv.Info("  %s clean", termenv.Green("○"))
	termenv.Info("  %s modified", termenv.Yellow("◉"))
	termenv.Info("  %s stale", termenv.Red("x"))
	termenv.Info("  %s currently active worktree\n", termenv.Bold(termenv.Green("★")))
	return nil
}

// ShowStats displays usage analytics for worktrees.
func ShowStats() error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		return err
	}
	type stat struct {
		branch  string
		status  Status
		ageDays float64
		commits int
	}
	var data []stat
	for _, wt := range features {
		info, err := os.Stat(wt.Path)
		if err != nil {
			continue
		}
		age := time.Since(info.ModTime()).Hours() / 24
		commits := 0
		if out, err := git.Output(wt.Path, "rev-list", "--count", wt.Branch); err == nil {
			fmt.Sscanf(out, "%d", &commits)
		}
		data = append(data, stat{wt.Branch, WorktreeStatus(wt.Path, repo), age, commits})
	}
	if len(data) == 0 {
		termenv.Info("\n%s\n", termenv.Yellow("No feature worktrees found"))
		return nil
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("📊 Worktree Statistics")))
	counts := map[Status]int{}
	for _, d := range data {
		counts[d.status]++
	}
	termenv.Info("%s", termenv.Bold("Overview:"))
	termenv.Info("  Total worktrees: %d", len(data))
	termenv.Info("  Status: %s, %s, %s, %s\n",
		termenv.Green(fmt.Sprintf("%d clean", counts[StatusClean])),
		termenv.Yellow(fmt.Sprintf("%d modified", counts[StatusModified])),
		termenv.Bold(termenv.Green(fmt.Sprintf("%d active", counts[StatusActive]))),
		termenv.Red(fmt.Sprintf("%d stale", counts[StatusStale])))

	var totalAge, oldest, newest float64
	newest = -1
	var totalCommits, maxCommits, commitWorktrees int
	for _, d := range data {
		totalAge += d.ageDays
		if d.ageDays > oldest {
			oldest = d.ageDays
		}
		if newest < 0 || d.ageDays < newest {
			newest = d.ageDays
		}
		if d.commits > 0 {
			totalCommits += d.commits
			commitWorktrees++
			if d.commits > maxCommits {
				maxCommits = d.commits
			}
		}
	}
	termenv.Info("%s", termenv.Bold("Age Statistics:"))
	termenv.Info("  Average age: %.1f days", totalAge/float64(len(data)))
	termenv.Info("  Oldest: %.1f days", oldest)
	termenv.Info("  Newest: %.1f days\n", newest)

	if commitWorktrees > 0 {
		termenv.Info("%s", termenv.Bold("Commit Statistics:"))
		termenv.Info("  Total commits across all worktrees: %d", totalCommits)
		termenv.Info("  Average commits per worktree: %.1f", float64(totalCommits)/float64(commitWorktrees))
		termenv.Info("  Most commits in a worktree: %d\n", maxCommits)
	}

	termenv.Info("%s", termenv.Bold("Oldest Worktrees:"))
	sorted := append([]stat{}, data...)
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].ageDays > sorted[j].ageDays })
	for i, d := range sorted {
		if i >= 5 {
			break
		}
		termenv.Info("  %s %-30s %s", statusColor(d.status)(statusIcons[d.status]), d.branch, FormatAge(d.ageDays))
	}
	fmt.Println()

	termenv.Info("%s", termenv.Bold("Most Active Worktrees (by commits):"))
	sort.Slice(sorted, func(i, j int) bool { return sorted[i].commits > sorted[j].commits })
	for i, d := range sorted {
		if i >= 5 || d.commits == 0 {
			break
		}
		termenv.Info("  %s %-30s %d commits", statusColor(d.status)(statusIcons[d.status]), d.branch, d.commits)
	}
	fmt.Println()
	return nil
}

// DiffWorktrees compares two branches.
func DiffWorktrees(branch1, branch2 string, summary, files bool) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	if !git.BranchExists(repo, branch1) {
		return wterrors.New(wterrors.ErrInvalidBranch, "branch '%s' not found", branch1)
	}
	if !git.BranchExists(repo, branch2) {
		return wterrors.New(wterrors.ErrInvalidBranch, "branch '%s' not found", branch2)
	}
	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Comparing branches:")))
	termenv.Info("  %s %s %s\n", branch1, termenv.Yellow("..."), branch2)

	switch {
	case files:
		out, err := git.Output(repo, "diff", "--name-status", branch1, branch2)
		if err != nil {
			return err
		}
		termenv.Info("%s\n", termenv.Bold("Changed files:"))
		if out == "" {
			termenv.Info("  %s", termenv.Dim("No differences found"))
			return nil
		}
		for _, line := range strings.Split(out, "\n") {
			code, file, _ := strings.Cut(line, "\t")
			if file == "" {
				continue
			}
			var color func(string) string
			var name string
			switch code[0] {
			case 'M':
				color, name = termenv.Yellow, "Modified"
			case 'A':
				color, name = termenv.Green, "Added"
			case 'D':
				color, name = termenv.Red, "Deleted"
			case 'R':
				color, name = termenv.Cyan, "Renamed"
			case 'C':
				color, name = termenv.Cyan, "Copied"
			default:
				color, name = func(s string) string { return s }, "Changed"
			}
			termenv.Info("  %s  %s (%s)", color(code), file, name)
		}
	case summary:
		out, err := git.Output(repo, "diff", "--stat", branch1, branch2)
		if err != nil {
			return err
		}
		termenv.Info("%s\n", termenv.Bold("Diff summary:"))
		if out == "" {
			termenv.Info("  %s", termenv.Dim("No differences found"))
		} else {
			termenv.Info("%s", out)
		}
	default:
		out, err := git.Output(repo, "diff", branch1, branch2)
		if err != nil {
			return err
		}
		if out == "" {
			termenv.Info("%s\n", termenv.Dim("No differences found"))
		} else {
			termenv.Info("%s", out)
		}
	}
	return nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}
