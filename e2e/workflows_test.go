// Package e2e runs end-to-end workflow tests against the built wt binary
// (ported from tests/e2e/test_workflows.py).
package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/testutil"
)

var binaryPath string

func TestMain(m *testing.M) {
	// Build the binary once for all e2e tests.
	tmp, err := os.MkdirTemp("", "wt-e2e-bin")
	if err != nil {
		panic(err)
	}
	binaryPath = filepath.Join(tmp, "wt")
	build := exec.Command("go", "build", "-o", binaryPath, "./cmd/wt")
	build.Dir = ".."
	if out, err := build.CombinedOutput(); err != nil {
		panic("build failed: " + string(out))
	}
	code := m.Run()
	os.RemoveAll(tmp)
	os.Exit(code)
}

// sharedHome is a per-test HOME reused across run() calls so registry
// and config state persist between invocations (as they do for real users).
func sharedHome(t *testing.T) string {
	t.Helper()
	return t.TempDir()
}

// run executes the wt binary in dir with an isolated HOME and no-op AI tool.
func run(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	return runEnv(t, dir, t.TempDir(), []string{"WT_AI_TOOL="}, args...)
}

// runHome is run with an explicit HOME (for multi-invocation state).
func runHome(t *testing.T, dir, home string, args ...string) (string, string, error) {
	t.Helper()
	return runEnv(t, dir, home, []string{"WT_AI_TOOL="}, args...)
}

// runEnv executes the wt binary with an explicit HOME plus extra env vars.
// The process environment is not inherited (except PATH) so
// developer-machine settings can't leak in.
func runEnv(t *testing.T, dir, home string, extra []string, args ...string) (string, string, error) {
	t.Helper()
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + home,
		"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
		"WT_NON_INTERACTIVE=1",
		"GIT_TERMINAL_PROMPT=0",
	}
	env = append(env, extra...)
	cmd := exec.Command(binaryPath, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

func TestFullLifecycle(t *testing.T) {
	repo := testutil.NewRepo(t)

	// Create worktree.
	out, _, err := run(t, repo, "new", "feat-x", "--no-term")
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-feat-x")
	if _, err := os.Stat(wtPath); err != nil {
		t.Fatalf("worktree not created: %v", err)
	}

	// Commit in the worktree.
	testutil.WriteFile(t, wtPath, "feature.txt", "work\n")
	testutil.Commit(t, wtPath, "feature commit")

	// List shows the worktree.
	out, _, err = run(t, repo, "list")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if !strings.Contains(out, "feat-x") {
		t.Errorf("list missing feat-x:\n%s", out)
	}

	// Status works from inside the worktree.
	out, _, err = run(t, wtPath, "status")
	if err != nil {
		t.Fatalf("status: %v", err)
	}
	if !strings.Contains(out, "feat-x") || !strings.Contains(out, "main") {
		t.Errorf("status output wrong:\n%s", out)
	}

	// Merge: rebases, merges, cleans up.
	out, _, err = run(t, wtPath, "merge")
	if err != nil {
		t.Fatalf("merge: %v\n%s", err, out)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after merge")
	}
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Error("feature commit not merged to main")
	}

	// List is back to just main.
	out, _, _ = run(t, repo, "list")
	if strings.Contains(out, "feat-x") {
		t.Errorf("list still shows feat-x:\n%s", out)
	}
}

func TestRebaseConflictAbort(t *testing.T) {
	repo := testutil.NewRepo(t)

	out, _, err := run(t, repo, "new", "conflict-branch", "--no-term")
	if err != nil {
		t.Fatalf("new: %v\n%s", err, out)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-conflict-branch")

	// Diverge: same file changed on both branches.
	testutil.WriteFile(t, wtPath, "a.txt", "feature version\n")
	testutil.Commit(t, wtPath, "feature change")
	testutil.WriteFile(t, repo, "README.md", "# test repo\nmain version\n")
	testutil.WriteFile(t, repo, "a.txt", "main version\n")
	testutil.Commit(t, repo, "main change")

	// Merge must fail with a rebase error, leaving the worktree intact.
	out, stderr, err := run(t, wtPath, "merge")
	if err == nil {
		t.Fatalf("expected merge to fail:\n%s\n%s", out, stderr)
	}
	combined := out + stderr
	if !strings.Contains(combined, "ebase") {
		t.Errorf("expected rebase error:\n%s", combined)
	}
	// Worktree survives and is not mid-rebase (aborted).
	if _, err := os.Stat(wtPath); err != nil {
		t.Error("worktree removed after failed merge")
	}
	res, _ := exec.Command("git", "-C", wtPath, "status", "--porcelain").CombinedOutput()
	if strings.Contains(string(res), "UU") {
		t.Error("rebase not aborted, conflicts remain")
	}
}

func TestErrorCases(t *testing.T) {
	repo := testutil.NewRepo(t)

	// Duplicate worktree creation fails (non-interactive).
	if _, _, err := run(t, repo, "new", "dup", "--no-term"); err != nil {
		t.Fatalf("first new: %v", err)
	}
	if _, stderr, err := run(t, repo, "new", "dup", "--no-term"); err == nil {
		t.Errorf("expected duplicate creation to fail\n%s", stderr)
	}

	// Delete nonexistent worktree fails.
	if _, _, err := run(t, repo, "delete", "no-such-branch"); err == nil {
		t.Error("expected delete of missing worktree to fail")
	}

	// Invalid branch name fails.
	if _, _, err := run(t, repo, "new", "bad..name", "--no-term"); err == nil {
		t.Error("expected invalid branch name to fail")
	}

	// Deleting the main repository worktree is refused.
	if _, _, err := run(t, repo, "delete", repo); err == nil {
		t.Error("expected main-repo deletion to be refused")
	}
}

func TestConfigWorkflow(t *testing.T) {
	repo := testutil.NewRepo(t)
	home := t.TempDir()

	runWithHome := func(args ...string) (string, error) {
		cmd := exec.Command(binaryPath, args...)
		cmd.Dir = repo
		cmd.Env = []string{
			"PATH=" + os.Getenv("PATH"),
			"HOME=" + home,
			"XDG_CONFIG_HOME=" + filepath.Join(home, ".config"),
			"WT_NON_INTERACTIVE=1",
		}
		out, err := cmd.CombinedOutput()
		return string(out), err
	}

	out, err := runWithHome("config", "use-preset", "codex")
	if err != nil {
		t.Fatalf("use-preset: %v\n%s", err, out)
	}
	out, _ = runWithHome("config", "show")
	if !strings.Contains(out, "codex") {
		t.Errorf("config show missing codex:\n%s", out)
	}
	if _, err := runWithHome("config", "set", "launch.method", "tmux"); err != nil {
		t.Fatalf("config set: %v", err)
	}
	out, _ = runWithHome("config", "show")
	if !strings.Contains(out, "tmux") {
		t.Errorf("config show missing tmux:\n%s", out)
	}
	if _, err := runWithHome("config", "reset"); err != nil {
		t.Fatalf("config reset: %v", err)
	}
	out, _ = runWithHome("config", "show")
	if !strings.Contains(out, "claude") {
		t.Errorf("config after reset missing claude:\n%s", out)
	}
}

func TestPathCommand(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "path-test", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	out, _, err := run(t, repo, "_path", "path-test")
	if err != nil {
		t.Fatalf("_path: %v", err)
	}
	want := filepath.Join(filepath.Dir(repo), "myrepo-path-test")
	got := strings.TrimSpace(out)
	// Resolve symlinks for macOS /tmp.
	gotR, _ := filepath.EvalSymlinks(got)
	wantR, _ := filepath.EvalSymlinks(want)
	if gotR != wantR {
		t.Errorf("_path = %q, want %q", got, want)
	}

	// --list-branches
	out, _, err = run(t, repo, "_path", "--list-branches")
	if err != nil {
		t.Fatalf("_path --list-branches: %v", err)
	}
	if !strings.Contains(out, "path-test") {
		t.Errorf("list-branches missing path-test:\n%s", out)
	}
}

func TestShellFunctionOutput(t *testing.T) {
	repo := testutil.NewRepo(t)
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		out, _, err := run(t, repo, "_shell-function", shell)
		if err != nil {
			t.Fatalf("_shell-function %s: %v", shell, err)
		}
		if !strings.Contains(out, "wt-cd") {
			t.Errorf("_shell-function %s missing wt-cd", shell)
		}
	}
	if _, _, err := run(t, repo, "_shell-function", "tcsh"); err == nil {
		t.Error("expected error for unsupported shell")
	}
}

func TestGlobalModeListAndScan(t *testing.T) {
	repo := testutil.NewRepo(t)
	home := sharedHome(t)
	if _, _, err := runHome(t, repo, home, "new", "glob-1", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	// Registering happened during `new`. Global list shows the repo.
	out, _, err := runHome(t, repo, home, "-g", "list")
	if err != nil {
		t.Fatalf("global list: %v", err)
	}
	if !strings.Contains(out, "glob-1") {
		t.Errorf("global list missing glob-1:\n%s", out)
	}
	// Prune on a healthy registry removes nothing.
	out, _, err = runHome(t, repo, home, "prune")
	if err != nil {
		t.Fatalf("prune: %v", err)
	}
	if !strings.Contains(out, "clean") {
		t.Errorf("unexpected prune output:\n%s", out)
	}
}
