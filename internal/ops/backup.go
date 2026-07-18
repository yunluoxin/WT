package ops

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"time"

	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

// BackupsDir returns the backups root (~/.config/wt/backups/).
func BackupsDir() (string, error) {
	dir := filepath.Join(config.Dir(), "backups")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	return dir, nil
}

type backupMetadata struct {
	Branch                string `json:"branch"`
	BaseBranch            string `json:"base_branch"`
	BasePath              string `json:"base_path"`
	WorktreePath          string `json:"worktree_path"`
	BackedUpAt            string `json:"backed_up_at"`
	HasUncommittedChanges bool   `json:"has_uncommitted_changes"`
	BundleFile            string `json:"bundle_file"`
	StashFile             string `json:"stash_file,omitempty"`
}

// BackupCreate backs up worktree(s) via git bundle.
func BackupCreate(branch, output string, allWorktrees bool, global bool) error {
	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}

	type target struct {
		branch, path string
	}
	var targets []target
	if allWorktrees {
		features, err := git.FeatureWorktrees(repo)
		if err != nil {
			return err
		}
		for _, wt := range features {
			targets = append(targets, target{wt.Branch, wt.Path})
		}
	} else {
		t, err := ResolveWorktreeTarget(branch, LookupAuto, global)
		if err != nil {
			return err
		}
		targets = append(targets, target{t.Branch, t.WorktreePath})
	}

	backupsRoot := output
	if backupsRoot == "" {
		backupsRoot, err = BackupsDir()
		if err != nil {
			return err
		}
	}
	timestamp := time.Now().Format("20060102-150405")
	created := 0

	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Creating backup(s)...")))

	for _, tg := range targets {
		branchDir := filepath.Join(backupsRoot, tg.branch, timestamp)
		if err := os.MkdirAll(branchDir, 0o755); err != nil {
			return err
		}
		bundleFile := filepath.Join(branchDir, "bundle.git")

		termenv.Info("%s %s", termenv.Yellow("Backing up:"), termenv.Bold(tg.branch))

		if _, err := git.Git(tg.path, true, "bundle", "create", bundleFile, "--all"); err != nil {
			termenv.Error("Backup failed: %v", err)
			continue
		}

		baseBranch := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, tg.branch))
		basePath := git.GetConfig(repo, git.MetadataKey(git.KeyBasePath, tg.branch))

		res, _ := git.Git(tg.path, false, "status", "--porcelain")
		hasChanges := res.Stdout != "" && len(bytes.TrimSpace([]byte(res.Stdout))) > 0

		stashFile := ""
		if hasChanges {
			termenv.Info("  %s", termenv.Dim("Found uncommitted changes, creating stash..."))
			stashFile = filepath.Join(branchDir, "stash.patch")
			diffRes, err := git.Git(tg.path, true, "diff", "HEAD")
			if err != nil {
				termenv.Warn("Failed to create stash patch: %v", err)
				stashFile = ""
			} else if err := os.WriteFile(stashFile, []byte(diffRes.Stdout), 0o644); err != nil {
				termenv.Warn("Failed to write stash patch: %v", err)
				stashFile = ""
			}
		}

		meta := backupMetadata{
			Branch: tg.branch, BaseBranch: baseBranch, BasePath: basePath,
			WorktreePath: tg.path, BackedUpAt: time.Now().Format(time.RFC3339),
			HasUncommittedChanges: hasChanges, BundleFile: bundleFile, StashFile: stashFile,
		}
		data, _ := json.MarshalIndent(meta, "", "  ")
		if err := os.WriteFile(filepath.Join(branchDir, "metadata.json"), data, 0o644); err != nil {
			termenv.Error("Failed to write metadata: %v", err)
			continue
		}
		termenv.Info("  %s Backup saved to: %s", termenv.Green("*"), branchDir)
		created++
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Green(fmt.Sprintf("* Backup complete! Created %d backup(s)", created))))
	termenv.Info("%s\n", termenv.Dim("Backups saved in: "+backupsRoot))
	return nil
}

// BackupList lists available backups, optionally filtered by branch.
func BackupList(branch string) error {
	backupsDir, err := BackupsDir()
	if err != nil {
		return err
	}
	entries, err := os.ReadDir(backupsDir)
	if err != nil || len(entries) == 0 {
		termenv.Info("\n%s\n", termenv.Yellow("No backups found"))
		return nil
	}

	termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Available Backups:")))
	type backup struct {
		ts   string
		meta backupMetadata
	}
	byBranch := map[string][]backup{}
	var branches []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if branch != "" && name != branch {
			continue
		}
		tsDirs, _ := os.ReadDir(filepath.Join(backupsDir, name))
		for _, ts := range tsDirs {
			if !ts.IsDir() {
				continue
			}
			metaFile := filepath.Join(backupsDir, name, ts.Name(), "metadata.json")
			data, err := os.ReadFile(metaFile)
			if err != nil {
				continue
			}
			var meta backupMetadata
			if json.Unmarshal(data, &meta) != nil {
				continue
			}
			if _, seen := byBranch[name]; !seen {
				branches = append(branches, name)
			}
			byBranch[name] = append(byBranch[name], backup{ts.Name(), meta})
		}
	}

	if len(branches) == 0 {
		suffix := ""
		if branch != "" {
			suffix = " for branch: " + branch
		}
		termenv.Info("%s\n", termenv.Yellow("No backups found"+suffix))
		return nil
	}

	sort.Strings(branches)
	for _, b := range branches {
		termenv.Info("%s:", termenv.Bold(termenv.Green(b)))
		list := byBranch[b]
		sort.Slice(list, func(i, j int) bool { return list[i].ts > list[j].ts })
		for _, bk := range list {
			indicator := ""
			if bk.meta.HasUncommittedChanges {
				indicator = " " + termenv.Yellow("(with uncommitted changes)")
			}
			termenv.Info("  • %s - %s%s", bk.ts, bk.meta.BackedUpAt, indicator)
		}
		fmt.Println()
	}
	return nil
}

// BackupRestore restores a worktree from a backup bundle.
func BackupRestore(branch, backupID, path string) error {
	backupsDir, err := BackupsDir()
	if err != nil {
		return err
	}
	branchDir := filepath.Join(backupsDir, branch)
	if _, err := os.Stat(branchDir); err != nil {
		return wterrors.New(wterrors.ErrWorktreeNotFound, "no backups found for branch '%s'", branch)
	}

	var backupDir string
	if backupID != "" {
		backupDir = filepath.Join(branchDir, backupID)
		if _, err := os.Stat(backupDir); err != nil {
			return wterrors.New(wterrors.ErrWorktreeNotFound, "backup '%s' not found for branch '%s'", backupID, branch)
		}
	} else {
		entries, err := os.ReadDir(branchDir)
		if err != nil {
			return err
		}
		var names []string
		for _, e := range entries {
			if e.IsDir() {
				names = append(names, e.Name())
			}
		}
		if len(names) == 0 {
			return wterrors.New(wterrors.ErrWorktreeNotFound, "no backups found for branch '%s'", branch)
		}
		sort.Sort(sort.Reverse(sort.StringSlice(names)))
		backupID = names[0]
		backupDir = filepath.Join(branchDir, backupID)
	}

	metaFile := filepath.Join(backupDir, "metadata.json")
	bundleFile := filepath.Join(backupDir, "bundle.git")
	data, err := os.ReadFile(metaFile)
	if err != nil {
		return wterrors.New(wterrors.ErrConfig, "invalid backup: missing metadata file")
	}
	if _, err := os.Stat(bundleFile); err != nil {
		return wterrors.New(wterrors.ErrConfig, "invalid backup: missing bundle file")
	}
	var meta backupMetadata
	if err := json.Unmarshal(data, &meta); err != nil {
		return wterrors.Wrap(wterrors.ErrConfig, err, "failed to read backup metadata")
	}

	termenv.Info("\n%s", termenv.Bold(termenv.Cyan("Restoring from backup:")))
	termenv.Info("  Branch: %s", termenv.Green(branch))
	termenv.Info("  Backup ID: %s", termenv.Yellow(backupID))
	termenv.Info("  Backed up at: %s\n", meta.BackedUpAt)

	repo, err := git.RepoRoot("")
	if err != nil {
		return err
	}
	worktreePath := path
	if worktreePath == "" {
		worktreePath = DefaultWorktreePath(repo, branch)
	} else {
		worktreePath, _ = filepath.Abs(worktreePath)
	}
	if _, err := os.Stat(worktreePath); err == nil {
		return wterrors.New(wterrors.ErrInvalidBranch,
			"worktree path already exists: %s\nRemove it first or specify a different path with --path", worktreePath)
	}

	cleanup := func() { _ = os.RemoveAll(worktreePath) }

	if err := os.MkdirAll(filepath.Dir(worktreePath), 0o755); err != nil {
		return err
	}
	termenv.Info("%s %s", termenv.Yellow("Restoring worktree to:"), worktreePath)
	if _, err := git.Git(filepath.Dir(repo), true, "clone", bundleFile, worktreePath); err != nil {
		cleanup()
		return wterrors.Wrap(wterrors.ErrMergeFailed, err, "restore failed")
	}
	_, _ = git.Git(worktreePath, false, "checkout", branch)

	if meta.BaseBranch != "" {
		if err := git.SetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, branch), meta.BaseBranch); err != nil {
			cleanup()
			return err
		}
		if err := git.SetConfig(repo, git.MetadataKey(git.KeyBasePath, branch), repo); err != nil {
			cleanup()
			return err
		}
	}

	stashFile := filepath.Join(backupDir, "stash.patch")
	if patch, err := os.ReadFile(stashFile); err == nil {
		termenv.Info("  %s", termenv.Dim("Restoring uncommitted changes..."))
		cmd := exec.Command("git", "apply", "--whitespace=fix")
		cmd.Dir = worktreePath
		cmd.Stdin = bytes.NewReader(patch)
		if out, err := cmd.CombinedOutput(); err != nil {
			termenv.Warn("Failed to restore uncommitted changes: %s", string(out))
		}
	}

	termenv.Success("Restore complete!")
	termenv.Info("  Worktree path: %s\n", worktreePath)
	return nil
}
