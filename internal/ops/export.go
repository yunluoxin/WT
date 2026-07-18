package ops

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

// ExportConfig writes worktree metadata and global config to a JSON file.
func ExportConfig(outputFile string) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	type wtInfo struct {
		Branch     string `json:"branch"`
		BaseBranch string `json:"base_branch"`
		BasePath   string `json:"base_path"`
		Path       string `json:"path"`
		Status     string `json:"status"`
	}
	export := map[string]any{
		"export_version": "1.0",
		"exported_at":    time.Now().Format(time.RFC3339),
		"repository":     repo,
		"config":         cfg,
		"worktrees":      []wtInfo{},
	}
	var wts []wtInfo
	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		return err
	}
	for _, wt := range features {
		wts = append(wts, wtInfo{
			Branch:     wt.Branch,
			BaseBranch: git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, wt.Branch)),
			BasePath:   git.GetConfig(repo, git.MetadataKey(git.KeyBasePath, wt.Branch)),
			Path:       wt.Path,
			Status:     string(WorktreeStatus(wt.Path, repo)),
		})
	}
	export["worktrees"] = wts

	if outputFile == "" {
		outputFile = fmt.Sprintf("wt-export-%s.json", time.Now().Format("20060102-150405"))
	}
	termenv.Info("\n%s %s", termenv.Yellow("Exporting configuration to:"), outputFile)
	data, err := json.MarshalIndent(export, "", "  ")
	if err != nil {
		return err
	}
	if err := os.WriteFile(outputFile, data, 0o644); err != nil {
		return wterrors.Wrap(wterrors.ErrConfig, err, "failed to write export file")
	}
	termenv.Success("Export complete!\n")
	termenv.Info("%s", termenv.Bold("Exported:"))
	termenv.Info("  • %d worktree(s)", len(wts))
	termenv.Info("  • Configuration settings")
	termenv.Info("\n%s\n", termenv.Dim("Transfer this file to another machine and use 'wt import' to restore."))
	return nil
}

// ImportConfig reads an export file and optionally applies it.
func ImportConfig(importFile string, apply bool) error {
	data, err := os.ReadFile(importFile)
	if err != nil {
		return wterrors.Wrap(wterrors.ErrConfig, err, "import file not found: %s", importFile)
	}
	var importData map[string]any
	if err := json.Unmarshal(data, &importData); err != nil {
		return wterrors.Wrap(wterrors.ErrConfig, err, "failed to read import file")
	}
	if _, ok := importData["export_version"]; !ok {
		return wterrors.New(wterrors.ErrConfig, "invalid export file format")
	}

	termenv.Info("\n%s %s\n", termenv.Yellow("Loading import file:"), importFile)
	termenv.Info("%s\n", termenv.Bold(termenv.Cyan("Import Preview:")))
	termenv.Info("%s %v", termenv.Bold("Exported from:"), importData["repository"])
	termenv.Info("%s %v", termenv.Bold("Exported at:"), importData["exported_at"])
	worktrees, _ := importData["worktrees"].([]any)
	termenv.Info("%s %d\n", termenv.Bold("Worktrees:"), len(worktrees))

	for _, w := range worktrees {
		wt, _ := w.(map[string]any)
		if wt == nil {
			continue
		}
		termenv.Info("  • %v", wt["branch"])
		termenv.Info("    Base: %v", wt["base_branch"])
		termenv.Info("    Original path: %v\n", wt["path"])
	}

	if !apply {
		termenv.Info("%s No changes made. Use --apply to import configuration.\n",
			termenv.Bold(termenv.Yellow("Preview mode:")))
		return nil
	}

	termenv.Info("%s\n", termenv.Bold(termenv.Yellow("Applying import...")))
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	imported := 0

	if cfgData, ok := importData["config"].(map[string]any); ok && len(cfgData) > 0 {
		termenv.Info("%s", termenv.Yellow("Importing global configuration..."))
		if err := config.Save(cfgData); err != nil {
			termenv.Warn("Configuration import failed: %v\n", err)
		} else {
			termenv.Success("Configuration imported\n")
		}
	}

	termenv.Info("%s\n", termenv.Yellow("Importing worktree metadata..."))
	for _, w := range worktrees {
		wt, _ := w.(map[string]any)
		if wt == nil {
			continue
		}
		branch, _ := wt["branch"].(string)
		baseBranch, _ := wt["base_branch"].(string)
		if branch == "" || baseBranch == "" {
			termenv.Warn("Skipping invalid worktree entry\n")
			continue
		}
		if !git.BranchExists(repo, branch) {
			termenv.Warn("Branch '%s' not found locally. Create it with 'wt new %s --base %s'", branch, branch, baseBranch)
			continue
		}
		if err := git.SetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch), baseBranch); err != nil {
			termenv.Warn("Failed to import %s: %v", branch, err)
			continue
		}
		if err := git.SetConfig(repo, git.MetadataKey(git.KeyBasePath, branch), repo); err != nil {
			termenv.Warn("Failed to import %s: %v", branch, err)
			continue
		}
		termenv.Success("Imported metadata for: %s", branch)
		imported++
	}
	termenv.Info("\n%s\n", termenv.Bold(termenv.Green(fmt.Sprintf("* Import complete! Imported %d worktree(s)", imported))))
	termenv.Info("%s\n", termenv.Dim("Note: This only imports metadata. Create actual worktrees with 'wt new' if they don't exist."))
	return nil
}
