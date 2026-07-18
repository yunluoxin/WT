package git_test

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/git"
	"wt/internal/testutil"
)

func TestRepoRoot(t *testing.T) {
	repo := testutil.NewRepo(t)
	root, err := git.RepoRoot(repo)
	if err != nil {
		t.Fatalf("RepoRoot: %v", err)
	}
	// /tmp symlinks on macOS: compare resolved paths.
	want, _ := filepath.EvalSymlinks(repo)
	got, _ := filepath.EvalSymlinks(root)
	if got != want {
		t.Errorf("RepoRoot = %q, want %q", got, want)
	}
}

func TestRepoRootNotARepo(t *testing.T) {
	dir := t.TempDir()
	if _, err := git.RepoRoot(dir); err == nil {
		t.Fatal("expected error outside a git repository")
	}
}

func TestCurrentBranch(t *testing.T) {
	repo := testutil.NewRepo(t)
	branch, err := git.CurrentBranch(repo)
	if err != nil {
		t.Fatalf("CurrentBranch: %v", err)
	}
	if branch != "main" {
		t.Errorf("CurrentBranch = %q, want main", branch)
	}
}

func TestParseWorktrees(t *testing.T) {
	repo := testutil.NewRepo(t)
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		t.Fatalf("ParseWorktrees: %v", err)
	}
	if len(wts) != 1 {
		t.Fatalf("expected 1 worktree, got %d", len(wts))
	}
	if wts[0].Branch != "main" {
		t.Errorf("branch = %q, want main", wts[0].Branch)
	}

	// Add a second worktree.
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-feat")
	if _, err := git.Git(repo, true, "worktree", "add", "-b", "feat", wtPath, "main"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	wts, err = git.ParseWorktrees(repo)
	if err != nil {
		t.Fatalf("ParseWorktrees: %v", err)
	}
	if len(wts) != 2 {
		t.Fatalf("expected 2 worktrees, got %d", len(wts))
	}

	features, err := git.FeatureWorktrees(repo)
	if err != nil {
		t.Fatalf("FeatureWorktrees: %v", err)
	}
	if len(features) != 1 || features[0].Branch != "feat" {
		t.Errorf("FeatureWorktrees = %v", features)
	}
}

func TestFindWorktreeByBranchAndName(t *testing.T) {
	repo := testutil.NewRepo(t)
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-fix-auth")
	if _, err := git.Git(repo, true, "worktree", "add", "-b", "fix-auth", wtPath, "main"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	if _, found, _ := git.FindWorktreeByBranch(repo, "fix-auth"); !found {
		t.Error("FindWorktreeByBranch: not found")
	}
	if _, found, _ := git.FindWorktreeByBranch(repo, "nope"); found {
		t.Error("FindWorktreeByBranch: unexpected match")
	}
	if _, found, _ := git.FindWorktreeByName(repo, "myrepo-fix-auth"); !found {
		t.Error("FindWorktreeByName: not found")
	}
}

func TestFindWorktreeByIntendedBranch(t *testing.T) {
	repo := testutil.NewRepo(t)
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-feat-x")
	if _, err := git.Git(repo, true, "worktree", "add", "-b", "feat-x", wtPath, "main"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	// Store intended-branch metadata, then switch the worktree to another branch.
	if err := git.SetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, "feat-x"), "feat-x"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if _, err := git.Git(wtPath, true, "switch", "-c", "other"); err != nil {
		t.Fatalf("switch: %v", err)
	}
	wt, found, err := git.FindWorktreeByIntendedBranch(repo, "feat-x", func(s string) string { return s })
	if err != nil || !found {
		t.Fatalf("FindWorktreeByIntendedBranch: found=%v err=%v", found, err)
	}
	if filepath.Base(wt.Path) != "myrepo-feat-x" {
		t.Errorf("path = %q", wt.Path)
	}
}

func TestConfigGetSetUnset(t *testing.T) {
	repo := testutil.NewRepo(t)
	if got := git.GetConfig(repo, "wt.test.key"); got != "" {
		t.Errorf("GetConfig unset = %q", got)
	}
	if err := git.SetConfig(repo, "wt.test.key", "value"); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if got := git.GetConfig(repo, "wt.test.key"); got != "value" {
		t.Errorf("GetConfig = %q, want value", got)
	}
	git.UnsetConfig(repo, "wt.test.key")
	if got := git.GetConfig(repo, "wt.test.key"); got != "" {
		t.Errorf("GetConfig after unset = %q", got)
	}
}

func TestBranchNameValidation(t *testing.T) {
	valid := []string{"feat", "feat/auth-fix", "release-1.2", "FIX_123"}
	for _, name := range valid {
		if !git.IsValidBranchName(name) {
			t.Errorf("expected %q to be valid", name)
		}
		if msg := git.BranchNameError(name); msg != "" {
			t.Errorf("BranchNameError(%q) = %q", name, msg)
		}
	}
	invalid := []string{"", "feat..x", "-bad", "a b", "a@{b", "a.lock", "a//b", "@", "a~b", "a:b", "a\\b"}
	for _, name := range invalid {
		if git.IsValidBranchName(name) {
			t.Errorf("expected %q to be invalid", name)
		}
		if msg := git.BranchNameError(name); msg == "" {
			t.Errorf("BranchNameError(%q) empty", name)
		}
	}
}

func TestRemoveWorktreeSafe(t *testing.T) {
	repo := testutil.NewRepo(t)
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-gone")
	if _, err := git.Git(repo, true, "worktree", "add", "-b", "gone", wtPath, "main"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}
	if err := git.RemoveWorktreeSafe(repo, wtPath, true); err != nil {
		t.Fatalf("RemoveWorktreeSafe: %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree directory still exists")
	}
	wts, _ := git.ParseWorktrees(repo)
	if len(wts) != 1 {
		t.Errorf("expected 1 worktree after removal, got %d", len(wts))
	}
}
