package hooks

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wt/internal/testutil"
)

func TestAddListRemove(t *testing.T) {
	repo := testutil.NewRepo(t)
	id, err := Add(repo, "worktree.post_create", "npm install", "", "install deps")
	if err != nil {
		t.Fatalf("Add: %v", err)
	}
	if id != GenerateID("npm install") {
		t.Errorf("id = %q, want %q", id, GenerateID("npm install"))
	}
	if !strings.HasPrefix(id, "hook-") {
		t.Errorf("id missing prefix: %q", id)
	}

	hookMap, err := List(repo, "")
	if err != nil {
		t.Fatal(err)
	}
	found := hookMap["worktree.post_create"]
	if len(found) != 1 || found[0].ID != id || !found[0].Enabled || found[0].Description != "install deps" {
		t.Errorf("List wrong: %+v", found)
	}

	// Duplicate ID rejected.
	if _, err := Add(repo, "worktree.post_create", "other", id, ""); err == nil {
		t.Error("expected duplicate ID error")
	}

	if err := Remove(repo, "worktree.post_create", id); err != nil {
		t.Fatalf("Remove: %v", err)
	}
	hookMap, _ = List(repo, "worktree.post_create")
	if len(hookMap["worktree.post_create"]) != 0 {
		t.Error("hook not removed")
	}
	if err := Remove(repo, "worktree.post_create", id); err == nil {
		t.Error("expected not-found error")
	}
}

func TestEnableDisable(t *testing.T) {
	repo := testutil.NewRepo(t)
	id, _ := Add(repo, "merge.pre", "echo hi", "", "")
	if err := SetEnabled(repo, "merge.pre", id, false); err != nil {
		t.Fatal(err)
	}
	hookMap, _ := List(repo, "merge.pre")
	if hookMap["merge.pre"][0].Enabled {
		t.Error("disable failed")
	}
	if err := SetEnabled(repo, "merge.pre", id, true); err != nil {
		t.Fatal(err)
	}
	hookMap, _ = List(repo, "merge.pre")
	if !hookMap["merge.pre"][0].Enabled {
		t.Error("enable failed")
	}
}

func TestInvalidEvent(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, err := Add(repo, "bogus.event", "echo", "", ""); err == nil {
		t.Error("expected invalid event error")
	}
}

func TestRunHooksEnvAndExecution(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script assertions are unix-specific")
	}
	repo := testutil.NewRepo(t)
	outFile := filepath.Join(t.TempDir(), "out.txt")
	cmd := "echo \"$WT_BRANCH|$WT_BASE_BRANCH|$WT_EVENT|$WT_OPERATION\" > " + outFile
	id, err := Add(repo, "worktree.post_create", cmd, "", "")
	if err != nil {
		t.Fatal(err)
	}
	ctx := Context{Branch: "feat-1", BaseBranch: "main", Operation: "new"}
	if err := RunHooks(repo, "worktree.post_create", ctx, repo); err != nil {
		t.Fatalf("RunHooks: %v", err)
	}
	data, err := os.ReadFile(outFile)
	if err != nil {
		t.Fatalf("hook did not run: %v", err)
	}
	got := strings.TrimSpace(string(data))
	want := "feat-1|main|worktree.post_create|new"
	if got != want {
		t.Errorf("env injection = %q, want %q", got, want)
	}
	_ = id
}

func TestPreHookAborts(t *testing.T) {
	repo := testutil.NewRepo(t)
	failCmd := "exit 1"
	if runtime.GOOS == "windows" {
		failCmd = "exit /b 1"
	}
	if _, err := Add(repo, "merge.pre", failCmd, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := RunHooks(repo, "merge.pre", Context{}, repo); err == nil {
		t.Error("expected pre-hook failure to abort")
	}
}

func TestPostHookWarnOnly(t *testing.T) {
	repo := testutil.NewRepo(t)
	failCmd := "exit 1"
	if runtime.GOOS == "windows" {
		failCmd = "exit /b 1"
	}
	if _, err := Add(repo, "merge.post", failCmd, "", ""); err != nil {
		t.Fatal(err)
	}
	if err := RunHooks(repo, "merge.post", Context{}, repo); err != nil {
		t.Errorf("post-hook failure should not abort: %v", err)
	}
}

func TestDisabledHookSkipped(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("shell script assertions are unix-specific")
	}
	repo := testutil.NewRepo(t)
	marker := filepath.Join(t.TempDir(), "marker")
	id, _ := Add(repo, "merge.post", "touch "+marker, "", "")
	if err := SetEnabled(repo, "merge.post", id, false); err != nil {
		t.Fatal(err)
	}
	if err := RunHooks(repo, "merge.post", Context{}, repo); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(marker); !os.IsNotExist(err) {
		t.Error("disabled hook ran")
	}
}
