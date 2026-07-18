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
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-feat-x")
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
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-conflict-branch")

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
	out, _, err := run(t, repo, "_path", "wt-path-test")
	if err != nil {
		t.Fatalf("_path: %v", err)
	}
	want := filepath.Join(filepath.Dir(repo), "myrepo-wt-path-test")
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

func TestCDCommand(t *testing.T) {
	repo := testutil.NewRepo(t)
	home := sharedHome(t)
	if _, _, err := runHome(t, repo, home, "new", "cd-test", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}

	// Resolve by full branch name.
	out, _, err := runHome(t, repo, home, "cd", "wt-cd-test")
	if err != nil {
		t.Fatalf("cd: %v", err)
	}
	want := filepath.Join(filepath.Dir(repo), "myrepo-wt-cd-test")
	got := strings.TrimSpace(out)
	gotR, _ := filepath.EvalSymlinks(got)
	wantR, _ := filepath.EvalSymlinks(want)
	if gotR != wantR {
		t.Errorf("cd = %q, want %q", got, want)
	}

	// Resolve without the internal wt- prefix (users never type it).
	out, _, err = runHome(t, repo, home, "cd", "cd-test")
	if err != nil {
		t.Fatalf("cd (no prefix): %v", err)
	}
	if strings.TrimSpace(out) != got {
		t.Errorf("cd (no prefix) = %q, want %q", strings.TrimSpace(out), got)
	}

	// repo:branch notation implies global mode (also without prefix).
	out, _, err = runHome(t, repo, home, "cd", "myrepo:cd-test")
	if err != nil {
		t.Fatalf("cd repo:branch: %v", err)
	}
	if strings.TrimSpace(out) != got {
		t.Errorf("cd repo:branch = %q, want %q", strings.TrimSpace(out), got)
	}

	// Unknown target fails with a non-zero exit code.
	if _, _, err := runHome(t, repo, home, "cd", "no-such-branch"); err == nil {
		t.Error("expected error for unknown branch")
	}
}

func TestUnprefixedTargets(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "np-test", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-np-test")

	// stash save (inside the worktree, tracked change), then apply by
	// unprefixed name. The apply resolves the target worktree even though
	// the stash ref itself may not exist in the main repo.
	testutil.WriteFile(t, wtPath, "stashed.txt", "stash me\n")
	testutil.Commit(t, wtPath, "add stashed.txt")
	testutil.WriteFile(t, wtPath, "stashed.txt", "modified\n")
	if _, _, err := run(t, wtPath, "stash", "save"); err != nil {
		t.Fatalf("stash save: %v", err)
	}
	_, _, _ = run(t, repo, "stash", "apply", "np-test")

	// shell runs a command in a worktree by unprefixed branch name.
	out, _, err := run(t, repo, "shell", "np-test", "pwd")
	if err != nil {
		t.Fatalf("shell (unprefixed): %v", err)
	}
	if !strings.Contains(out, "myrepo-wt-np-test") {
		t.Errorf("shell pwd missing worktree path:\n%s", out)
	}

	// delete by unprefixed branch name.
	if _, _, err := run(t, repo, "delete", "np-test"); err != nil {
		t.Fatalf("delete (unprefixed): %v", err)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after unprefixed delete")
	}
}

func TestUnprefixedMerge(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "np-merge", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-np-merge")
	testutil.WriteFile(t, wtPath, "merged.txt", "content\n")
	testutil.Commit(t, wtPath, "feature commit")

	// merge by unprefixed branch name: must resolve to wt-np-merge,
	// rebase that real branch, and clean up.
	out, _, err := run(t, repo, "merge", "np-merge")
	if err != nil {
		t.Fatalf("merge (unprefixed): %v\n%s", err, out)
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after unprefixed merge")
	}
	if _, err := os.Stat(filepath.Join(repo, "merged.txt")); err != nil {
		t.Error("feature commit not merged to main")
	}
}

func TestInitCommand(t *testing.T) {
	repo := testutil.NewRepo(t)

	// --print emits the snippet for each supported shell.
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		out, _, err := run(t, repo, "init", shell, "--print")
		if err != nil {
			t.Fatalf("init %s --print: %v", shell, err)
		}
		if !strings.Contains(out, "wt-cd") {
			t.Errorf("init %s snippet missing wt-cd:\n%s", shell, out)
		}
		if !strings.Contains(out, "wt completion") {
			t.Errorf("init %s snippet missing completion:\n%s", shell, out)
		}
	}

	// Unsupported shell errors out.
	if _, _, err := run(t, repo, "init", "tcsh", "--print"); err == nil {
		t.Error("expected error for unsupported shell")
	}

	// Writing to a profile under a temp HOME.
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

	out, err := runWithHome("init", "zsh")
	if err != nil {
		t.Fatalf("init zsh (write): %v\n%s", err, out)
	}
	profile := filepath.Join(home, ".zshrc")
	if data, err := os.ReadFile(profile); err != nil || !strings.Contains(string(data), "wt-cd") {
		t.Errorf("profile not written or missing wt-cd: %v", err)
	}
	// Idempotent: second run reports already installed.
	out, err = runWithHome("init", "zsh")
	if err != nil {
		t.Fatalf("init zsh (2nd): %v\n%s", err, out)
	}
	if !strings.Contains(out, "already") {
		t.Errorf("expected 'already installed' message:\n%s", out)
	}
}

func TestInitDoesNotMatchComments(t *testing.T) {
	repo := testutil.NewRepo(t)
	home := t.TempDir()
	// A profile that only mentions wt-cd in a COMMENT (and an old
	// _shell-function line) must NOT count as "already installed".
	profile := filepath.Join(home, ".zshrc")
	stale := "# wt shell integration (wt-cd)\nsource <(wt _shell-function zsh)\n"
	if err := os.WriteFile(profile, []byte(stale), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binaryPath, "init", "zsh")
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "WT_NON_INTERACTIVE=1"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(profile)
	if !strings.Contains(string(data), "# >>> wt shell integration") {
		t.Errorf("init did not install over stale comment-only profile:\n%s", data)
	}
	// The stale _shell-function line is left untouched (not ours to manage),
	// but the new block must be appended.
	if !strings.Contains(string(data), "source <(wt completion zsh)") {
		t.Errorf("init missing completion line:\n%s", data)
	}
}

func TestInitUpgradesOldBlock(t *testing.T) {
	repo := testutil.NewRepo(t)
	home := t.TempDir()
	// A profile with an OLD-version managed block gets it replaced.
	profile := filepath.Join(home, ".zshrc")
	old := "export X=1\n# >>> wt shell integration (v0) >>>\nOLD-STUFF\n# <<< wt shell integration <<<\nexport Y=2\n"
	if err := os.WriteFile(profile, []byte(old), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command(binaryPath, "init", "zsh")
	cmd.Dir = repo
	cmd.Env = []string{"PATH=" + os.Getenv("PATH"), "HOME=" + home, "WT_NON_INTERACTIVE=1"}
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("init: %v\n%s", err, out)
	}
	data, _ := os.ReadFile(profile)
	s := string(data)
	if strings.Contains(s, "OLD-STUFF") {
		t.Errorf("old block not replaced:\n%s", s)
	}
	if !strings.Contains(s, "source <(wt completion zsh)") {
		t.Errorf("new block missing:\n%s", s)
	}
	if !strings.Contains(s, "export X=1") || !strings.Contains(s, "export Y=2") {
		t.Errorf("content outside block was clobbered:\n%s", s)
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
