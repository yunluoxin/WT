package ops

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/git"
	"wt/internal/testutil"
)

func TestSanitizeBranchName(t *testing.T) {
	cases := map[string]string{
		"feat/auth":      "feat-auth",
		"fix: bug":       "fix-bug",
		"a//b":           "a-b",
		"--lead-trail--": "lead-trail",
		"normal-branch":  "normal-branch",
		"emoji 🎉 branch": "emoji-🎉-branch", // emoji itself is preserved (Python parity)
		"a_b":            "a_b",
		"spaces   here":  "spaces-here",
		"weird@#$%chars": "weird-chars",
	}
	for in, want := range cases {
		if got := SanitizeBranchName(in); got != want {
			t.Errorf("SanitizeBranchName(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestDefaultWorktreePath(t *testing.T) {
	repo := t.TempDir()
	myrepo := filepath.Join(repo, "myrepo")
	got := DefaultWorktreePath(myrepo, "feat/auth")
	want := filepath.Join(repo, "myrepo-feat-auth")
	if got != want {
		t.Errorf("DefaultWorktreePath = %q, want %q", got, want)
	}
}

func TestCreateAndResolveWorktree(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	path, err := CreateWorktree(CreateOptions{BranchName: "feat-1", NoTerm: true})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}
	// Metadata stored.
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "feat-1")); got != "main" {
		t.Errorf("worktreeBase = %q, want main", got)
	}
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, "feat-1")); got != "feat-1" {
		t.Errorf("intendedBranch = %q", got)
	}

	// Resolve by branch.
	target, err := ResolveWorktreeTarget("feat-1", LookupAuto, false)
	if err != nil {
		t.Fatalf("ResolveWorktreeTarget: %v", err)
	}
	if target.Branch != "feat-1" {
		t.Errorf("branch = %q", target.Branch)
	}

	// Resolve by directory name.
	target, err = ResolveWorktreeTarget(filepath.Base(path), LookupAuto, false)
	if err != nil {
		t.Fatalf("ResolveWorktreeTarget by name: %v", err)
	}
	if target.Branch != "feat-1" {
		t.Errorf("branch = %q", target.Branch)
	}

	// Duplicate creation fails in non-interactive mode.
	if _, err := CreateWorktree(CreateOptions{BranchName: "feat-1", NoTerm: true}); err == nil {
		t.Error("expected error for duplicate worktree")
	}
}

func TestCreateInvalidBranch(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	if _, err := CreateWorktree(CreateOptions{BranchName: "bad..name", NoTerm: true}); err == nil {
		t.Error("expected error for invalid branch name")
	}
}

func TestDeleteWorktree(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	if _, err := CreateWorktree(CreateOptions{BranchName: "to-delete", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteWorktree(DeleteOptions{Target: "to-delete"}); err != nil {
		t.Fatalf("DeleteWorktree: %v", err)
	}
	if git.BranchExists(repo, "to-delete") {
		t.Error("branch still exists after delete")
	}
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "to-delete")); got != "" {
		t.Error("metadata not cleaned up")
	}
}

func TestDeleteKeepBranch(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	if _, err := CreateWorktree(CreateOptions{BranchName: "keepme", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	if err := DeleteWorktree(DeleteOptions{Target: "keepme", KeepBranch: true}); err != nil {
		t.Fatal(err)
	}
	if !git.BranchExists(repo, "keepme") {
		t.Error("branch deleted despite --keep-branch")
	}
}

func TestDeleteMainRepoRefused(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	if err := DeleteWorktree(DeleteOptions{Target: repo}); err == nil {
		t.Error("expected refusal to delete main repository worktree")
	}
}

func TestFinishWorktreeMergesAndCleansUp(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	wtPath, err := CreateWorktree(CreateOptions{BranchName: "feat-merge", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	// Commit on the feature branch.
	testutil.WriteFile(t, wtPath, "feature.txt", "feature work\n")
	testutil.Commit(t, wtPath, "add feature")

	testutil.Chdir(t, wtPath)
	if err := FinishWorktree(FinishOptions{}); err != nil {
		t.Fatalf("FinishWorktree: %v", err)
	}
	// Worktree removed, branch deleted, commit on main.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed")
	}
	if git.BranchExists(repo, "feat-merge") {
		t.Error("feature branch not deleted")
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Error("feature file not merged into main")
	}
}

func TestFinishDryRun(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	wtPath, err := CreateWorktree(CreateOptions{BranchName: "feat-dry", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, wtPath)
	if err := FinishWorktree(FinishOptions{DryRun: true}); err != nil {
		t.Fatalf("dry run: %v", err)
	}
	if _, err := os.Stat(wtPath); err != nil {
		t.Error("dry run removed the worktree")
	}
	if !git.BranchExists(repo, "feat-dry") {
		t.Error("dry run deleted the branch")
	}
}

func TestChangeBase(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	// Create a develop branch with an extra commit.
	if _, err := git.Git(repo, true, "switch", "-c", "develop"); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, repo, "dev.txt", "develop work\n")
	testutil.Commit(t, repo, "develop commit")
	if _, err := git.Git(repo, true, "switch", "main"); err != nil {
		t.Fatal(err)
	}

	wtPath, err := CreateWorktree(CreateOptions{BranchName: "feat-cb", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, wtPath, "feat.txt", "feat\n")
	testutil.Commit(t, wtPath, "feat commit")

	testutil.Chdir(t, wtPath)
	if err := ChangeBase(ChangeBaseOptions{NewBase: "develop"}); err != nil {
		t.Fatalf("ChangeBase: %v", err)
	}
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "feat-cb")); got != "develop" {
		t.Errorf("base metadata = %q, want develop", got)
	}
	// feat-cb should now contain develop's commit.
	out := testutil.GitOut(t, wtPath, "log", "--oneline")
	if !contains(out, "develop commit") {
		t.Errorf("rebase onto develop failed, log:\n%s", out)
	}
}

func TestStashSaveListApply(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	wtPath, err := CreateWorktree(CreateOptions{BranchName: "stash-src", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, wtPath)
	testutil.WriteFile(t, wtPath, "wip.txt", "work in progress\n")

	if err := StashSave("my work", true); err != nil {
		t.Fatalf("StashSave: %v", err)
	}
	// Working tree clean after stash.
	res, _ := git.Git(wtPath, false, "status", "--porcelain")
	if res.Stdout != "" {
		t.Error("working tree not clean after stash")
	}

	testutil.Chdir(t, repo)
	if err := StashList(); err != nil {
		t.Fatalf("StashList: %v", err)
	}
	if err := StashApply("stash-src", "stash@{0}"); err != nil {
		t.Fatalf("StashApply: %v", err)
	}
	if _, err := os.Stat(filepath.Join(wtPath, "wip.txt")); err != nil {
		t.Error("stash not applied")
	}
}

func TestBackupRestoreRoundTrip(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	wtPath, err := CreateWorktree(CreateOptions{BranchName: "feat-backup", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, wtPath, "backed.txt", "backup me\n")
	testutil.Commit(t, wtPath, "backup commit")

	backupDir := t.TempDir()
	testutil.Chdir(t, repo)
	if err := BackupCreate("feat-backup", backupDir, false, false); err != nil {
		t.Fatalf("BackupCreate: %v", err)
	}
	SetBackupsDirOverride(backupDir)
	t.Cleanup(func() { SetBackupsDirOverride("") })

	// Delete the worktree, then restore from backup.
	if err := DeleteWorktree(DeleteOptions{Target: "feat-backup"}); err != nil {
		t.Fatal(err)
	}
	restoreDir := t.TempDir()
	os.Remove(restoreDir) // restore needs a non-existent path
	if err := BackupRestore("feat-backup", "", restoreDir); err != nil {
		t.Fatalf("BackupRestore: %v", err)
	}
	if _, err := os.Stat(filepath.Join(restoreDir, "backed.txt")); err != nil {
		t.Error("restored worktree missing file")
	}
}

func TestWorktreeStatus(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	wtPath, err := CreateWorktree(CreateOptions{BranchName: "status-test", NoTerm: true})
	if err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, repo)
	if got := WorktreeStatus(wtPath, repo); got != StatusClean {
		t.Errorf("status = %q, want clean", got)
	}
	// Modify a file.
	testutil.WriteFile(t, wtPath, "dirty.txt", "dirty\n")
	if got := WorktreeStatus(wtPath, repo); got != StatusModified {
		t.Errorf("status = %q, want modified", got)
	}
	// Missing dir = stale.
	if got := WorktreeStatus(filepath.Join(repo, "does-not-exist"), repo); got != StatusStale {
		t.Errorf("status = %q, want stale", got)
	}
}

func TestFormatAge(t *testing.T) {
	cases := []struct {
		days float64
		want string
	}{
		{0, "just now"},
		{0.5, "12h ago"},
		{3, "3d ago"},
		{14, "2w ago"},
		{60, "2mo ago"},
		{400, "1y ago"},
	}
	for _, c := range cases {
		if got := FormatAge(c.days); got != c.want {
			t.Errorf("FormatAge(%v) = %q, want %q", c.days, got, c.want)
		}
	}
}

func TestSyncFetchOnly(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	if _, err := CreateWorktree(CreateOptions{BranchName: "sync-me", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, repo)
	if err := SyncWorktrees(SyncOptions{All: true, FetchOnly: true}); err != nil {
		t.Fatalf("SyncWorktrees fetch-only: %v", err)
	}
}

func TestTopologicalSort(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	// a is base of b.
	if _, err := CreateWorktree(CreateOptions{BranchName: "a", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	wtA, _ := ResolveWorktreeTarget("a", LookupAuto, false)
	if _, err := git.Git(wtA.WorktreePath, true, "switch", "-c", "intermediate"); err != nil {
		// create b from a: switch to a in its worktree first
		t.Fatal(err)
	}
	if _, err := git.Git(wtA.WorktreePath, true, "switch", "a"); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, wtA.WorktreePath)
	if _, err := CreateWorktree(CreateOptions{BranchName: "b", BaseBranch: "a", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	wtB, _ := ResolveWorktreeTarget("b", LookupAuto, false)

	items := []syncItem{{"b", wtB.WorktreePath}, {"a", wtA.WorktreePath}}
	sorted := topologicalSort(items, repo)
	if sorted[0].branch != "a" || sorted[1].branch != "b" {
		t.Errorf("topological sort wrong: %v", sorted)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || indexOf(s, sub) >= 0)
}

func indexOf(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
