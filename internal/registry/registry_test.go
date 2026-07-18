package registry

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/git"
	"wt/internal/testutil"
)

func TestRegisterAndRepositories(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	if err := Register(repo); err != nil {
		t.Fatalf("Register: %v", err)
	}
	paths, entries, err := Repositories()
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) != 1 {
		t.Fatalf("expected 1 repo, got %d", len(paths))
	}
	entry := entries[paths[0]]
	if entry.Name != "myrepo" {
		t.Errorf("name = %q, want myrepo", entry.Name)
	}
	if entry.RegisteredAt.IsZero() || entry.LastSeen.IsZero() {
		t.Error("timestamps not set")
	}
}

func TestUpdateLastSeen(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	Register(repo)
	paths, entries, _ := Repositories()
	first := entries[paths[0]].LastSeen
	UpdateLastSeen(repo)
	_, entries2, _ := Repositories()
	if entries2[paths[0]].LastSeen.Before(first) {
		t.Error("last_seen moved backwards")
	}
	// Unknown path is a no-op.
	UpdateLastSeen("/nonexistent/repo")
}

func TestPrune(t *testing.T) {
	testutil.SetHome(t)
	repo := testutil.NewRepo(t)
	Register(repo)
	stale := filepath.Join(t.TempDir(), "gone")
	// Manually plant a stale entry.
	Register(stale)
	paths, _, _ := Repositories()
	if len(paths) != 2 {
		t.Fatalf("setup: expected 2 repos, got %d", len(paths))
	}
	removed, err := Prune()
	if err != nil {
		t.Fatal(err)
	}
	if len(removed) != 1 {
		t.Errorf("expected 1 pruned, got %d: %v", len(removed), removed)
	}
	paths, _, _ = Repositories()
	if len(paths) != 1 {
		t.Errorf("expected 1 repo after prune, got %d", len(paths))
	}
}

func TestScanForRepos(t *testing.T) {
	testutil.SetHome(t)
	// Build a directory tree with a repo that has worktrees and one without.
	root := t.TempDir()
	withWT := filepath.Join(root, "projects", "with-worktrees")
	os.MkdirAll(withWT, 0o755)
	initRepo(t, withWT)
	wtPath := filepath.Join(root, "projects", "with-worktrees-feat")
	if _, err := git.Git(withWT, true, "worktree", "add", "-b", "feat", wtPath, "main"); err != nil {
		t.Fatalf("worktree add: %v", err)
	}

	withoutWT := filepath.Join(root, "projects", "plain")
	os.MkdirAll(withoutWT, 0o755)
	initRepo(t, withoutWT)

	// Hidden dir should be skipped.
	hidden := filepath.Join(root, ".hidden", "repo")
	os.MkdirAll(hidden, 0o755)
	initRepo(t, hidden)

	found, err := ScanForRepos(root)
	if err != nil {
		t.Fatal(err)
	}
	if len(found) != 1 {
		t.Fatalf("expected 1 repo found, got %d: %v", len(found), found)
	}
	if filepath.Base(found[0]) != "with-worktrees" {
		t.Errorf("found %q", found[0])
	}
	// Should be registered now.
	paths, _, _ := Repositories()
	if len(paths) != 1 {
		t.Errorf("expected 1 registered repo, got %d", len(paths))
	}
}

func initRepo(t *testing.T, dir string) {
	t.Helper()
	if _, err := git.Git(dir, true, "init", "-b", "main"); err != nil {
		t.Fatalf("init: %v", err)
	}
	git.Git(dir, true, "config", "user.name", "T")
	git.Git(dir, true, "config", "user.email", "t@t")
	os.WriteFile(filepath.Join(dir, "f.txt"), []byte("x"), 0o644)
	git.Git(dir, true, "add", ".")
	if _, err := git.Git(dir, true, "commit", "-m", "init"); err != nil {
		t.Fatalf("commit: %v", err)
	}
}
