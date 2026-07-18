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

func TestPrefixBranch(t *testing.T) {
	if got := PrefixBranch("fix-auth"); got != "wt-fix-auth" {
		t.Errorf("PrefixBranch = %q, want wt-fix-auth", got)
	}
	// Idempotent for already-prefixed names.
	if got := PrefixBranch("wt-fix-auth"); got != "wt-fix-auth" {
		t.Errorf("PrefixBranch = %q, want wt-fix-auth (unchanged)", got)
	}
	if !IsManagedBranch("wt-fix-auth") || IsManagedBranch("fix-auth") {
		t.Error("IsManagedBranch misclassified")
	}
}

func TestCustomPrefixesApplyEverywhere(t *testing.T) {
	// Simulate a user/build changing both global prefixes; branch and
	// worktree-directory prefixes must stay independent.
	oldB, oldW := BranchPrefix, WorktreePrefix
	BranchPrefix, WorktreePrefix = "cw-", "cwdir-"
	t.Cleanup(func() { BranchPrefix, WorktreePrefix = oldB, oldW })

	if got := PrefixBranch("fix"); got != "cw-fix" {
		t.Errorf("PrefixBranch = %q, want cw-fix", got)
	}
	if got := PrefixBranch("cw-fix"); got != "cw-fix" {
		t.Errorf("PrefixBranch should be idempotent, got %q", got)
	}
	repo := t.TempDir()
	myrepo := filepath.Join(repo, "myrepo")
	// Directory uses WorktreePrefix, not BranchPrefix.
	got := DefaultWorktreePath(myrepo, "cw-fix")
	want := filepath.Join(repo, "myrepo-cwdir-fix")
	if got != want {
		t.Errorf("DefaultWorktreePath = %q, want %q", got, want)
	}
}

func TestCreatePrefixesBranchAndPath(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	path, err := CreateWorktree(CreateOptions{BranchName: "feat-pre", NoTerm: true})
	if err != nil {
		t.Fatalf("CreateWorktree: %v", err)
	}
	if !git.BranchExists(repo, "wt-feat-pre") {
		t.Error("branch was not created with wt- prefix")
	}
	if git.BranchExists(repo, "feat-pre") {
		t.Error("unprefixed branch should not exist")
	}
	wantDir := "myrepo-" + WorktreePrefix + "feat-pre"
	if base := filepath.Base(path); base != wantDir {
		t.Errorf("worktree dir = %q, want %q", base, wantDir)
	}

	// Already-prefixed input is not double-prefixed.
	if _, err := CreateWorktree(CreateOptions{BranchName: "wt-explicit", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	if !git.BranchExists(repo, "wt-explicit") {
		t.Error("explicit wt- branch missing")
	}
	if git.BranchExists(repo, "wt-wt-explicit") {
		t.Error("branch name was double-prefixed")
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
	// Metadata stored under the prefixed branch name.
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "wt-feat-1")); got != "main" {
		t.Errorf("worktreeBase = %q, want main", got)
	}
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyIntendedBranch, "wt-feat-1")); got != "wt-feat-1" {
		t.Errorf("intendedBranch = %q", got)
	}

	// Resolve by branch.
	target, err := ResolveWorktreeTarget("wt-feat-1", LookupAuto, false)
	if err != nil {
		t.Fatalf("ResolveWorktreeTarget: %v", err)
	}
	if target.Branch != "wt-feat-1" {
		t.Errorf("branch = %q", target.Branch)
	}

	// Resolve by directory name.
	target, err = ResolveWorktreeTarget(filepath.Base(path), LookupAuto, false)
	if err != nil {
		t.Fatalf("ResolveWorktreeTarget by name: %v", err)
	}
	if target.Branch != "wt-feat-1" {
		t.Errorf("branch = %q", target.Branch)
	}

	// Duplicate creation auto-resumes: returns the existing worktree path
	// instead of failing.
	dupPath, err := CreateWorktree(CreateOptions{BranchName: "feat-1", NoTerm: true})
	if err != nil {
		t.Errorf("expected auto-resume for duplicate worktree, got error: %v", err)
	}
	if dupPath != path {
		t.Errorf("duplicate creation returned %q, want existing path %q", dupPath, path)
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
	if err := DeleteWorktree(DeleteOptions{Target: "wt-to-delete"}); err != nil {
		t.Fatalf("DeleteWorktree: %v", err)
	}
	if git.BranchExists(repo, "wt-to-delete") {
		t.Error("branch still exists after delete")
	}
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "wt-to-delete")); got != "" {
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
	if err := DeleteWorktree(DeleteOptions{Target: "wt-keepme", KeepBranch: true}); err != nil {
		t.Fatal(err)
	}
	if !git.BranchExists(repo, "wt-keepme") {
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

func TestFinishOnBaseBranchRefused(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)
	// Running merge inside the main worktree on the base branch must be
	// refused before any rebase/merge/cleanup is attempted.
	if err := FinishWorktree(FinishOptions{}); err == nil {
		t.Error("expected refusal to merge the base branch into itself")
	}
	if _, err := os.Stat(repo); err != nil {
		t.Error("main repository worktree was affected")
	}
	if !git.BranchExists(repo, "main") {
		t.Error("base branch was deleted")
	}
}

func TestFinishForeignBranchRefusedUnlessAny(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	testutil.Chdir(t, repo)

	// A worktree created outside wt (plain git) has no wt- prefix.
	foreign := filepath.Join(filepath.Dir(repo), "myrepo-foreign")
	if _, err := git.Git(repo, true, "worktree", "add", "-b", "foreign", foreign, "main"); err != nil {
		t.Fatal(err)
	}
	testutil.WriteFile(t, foreign, "f.txt", "foreign work\n")
	testutil.Commit(t, foreign, "foreign commit")

	testutil.Chdir(t, foreign)
	// Default: refused.
	if err := FinishWorktree(FinishOptions{}); err == nil {
		t.Error("expected refusal to merge a non-wt branch without --any")
	}
	if !git.BranchExists(repo, "foreign") {
		t.Error("foreign branch was deleted despite refusal")
	}
	// --any opts out and merges normally.
	if err := FinishWorktree(FinishOptions{Any: true}); err != nil {
		t.Fatalf("FinishWorktree --any: %v", err)
	}
	if git.BranchExists(repo, "foreign") {
		t.Error("foreign branch not deleted after --any merge")
	}
	if _, err := os.Stat(filepath.Join(repo, "f.txt")); err != nil {
		t.Error("foreign commit not merged into main")
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
	if git.BranchExists(repo, "wt-feat-merge") {
		t.Error("feature branch not deleted")
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Error("feature file not merged into main")
	}
}

func TestFinishDryRun(t *testing.T) {	testutil.SetHome(t)
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
	if !git.BranchExists(repo, "wt-feat-dry") {
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
	if got := git.GetConfig(repo, git.MetadataKey(git.KeyWorktreeBase, "wt-feat-cb")); got != "develop" {
		t.Errorf("base metadata = %q, want develop", got)
	}
	// wt-feat-cb should now contain develop's commit.
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
	if err := StashApply("wt-stash-src", "stash@{0}"); err != nil {
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
	if err := BackupCreate("wt-feat-backup", backupDir, false, false); err != nil {
		t.Fatalf("BackupCreate: %v", err)
	}
	SetBackupsDirOverride(backupDir)
	t.Cleanup(func() { SetBackupsDirOverride("") })

	// Delete the worktree, then restore from backup.
	if err := DeleteWorktree(DeleteOptions{Target: "wt-feat-backup"}); err != nil {
		t.Fatal(err)
	}
	restoreDir := t.TempDir()
	os.Remove(restoreDir) // restore needs a non-existent path
	if err := BackupRestore("wt-feat-backup", "", restoreDir); err != nil {
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
	wtA, _ := ResolveWorktreeTarget("wt-a", LookupAuto, false)
	if _, err := git.Git(wtA.WorktreePath, true, "switch", "-c", "intermediate"); err != nil {
		// create b from a: switch to a in its worktree first
		t.Fatal(err)
	}
	if _, err := git.Git(wtA.WorktreePath, true, "switch", "wt-a"); err != nil {
		t.Fatal(err)
	}
	testutil.Chdir(t, wtA.WorktreePath)
	if _, err := CreateWorktree(CreateOptions{BranchName: "b", BaseBranch: "wt-a", NoTerm: true}); err != nil {
		t.Fatal(err)
	}
	wtB, _ := ResolveWorktreeTarget("wt-b", LookupAuto, false)

	items := []syncItem{{"wt-b", wtB.WorktreePath}, {"wt-a", wtA.WorktreePath}}
	sorted := topologicalSort(items, repo)
	if sorted[0].branch != "wt-a" || sorted[1].branch != "wt-b" {
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
