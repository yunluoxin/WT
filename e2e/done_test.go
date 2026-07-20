package e2e

import (
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wt/internal/testutil"
)

// Dirty worktree, no new commits: changes are moved onto the base branch
// (main worktree) and the worktree + branch are removed.
func TestDoneNoCommitsMovesChanges(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "done-nc", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-nc")

	// Uncommitted tracked + untracked changes, no commits.
	testutil.WriteFile(t, wtPath, "README.md", "# test repo\nuncommitted change\n")
	testutil.WriteFile(t, wtPath, "untracked.txt", "new file\n")

	out, stderr, err := run(t, wtPath, "done")
	if err != nil {
		t.Fatalf("done: %v\n%s\n%s", err, out, stderr)
	}

	// Worktree and branch are gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after done")
	}
	if out := testutil.GitOut(t, repo, "branch", "--list", "wt-done-nc"); strings.TrimSpace(out) != "" {
		t.Errorf("branch wt-done-nc still exists")
	}

	// Changes landed in the main worktree, uncommitted.
	data, err := os.ReadFile(filepath.Join(repo, "README.md"))
	if err != nil || !strings.Contains(string(data), "uncommitted change") {
		t.Errorf("tracked change not moved to main worktree: %v\n%s", err, data)
	}
	if _, err := os.Stat(filepath.Join(repo, "untracked.txt")); err != nil {
		t.Error("untracked file not moved to main worktree")
	}
	if out := testutil.GitOut(t, repo, "status", "--porcelain"); strings.TrimSpace(out) == "" {
		t.Error("main worktree should be dirty after done")
	}
}

// Dirty worktree with new commits: commits are merged into base, changes are
// restored there, worktree + branch removed.
func TestDoneWithCommitsMergesAndRestores(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "done-wc", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-wc")

	testutil.WriteFile(t, wtPath, "feature.txt", "feature work\n")
	testutil.Commit(t, wtPath, "feature commit")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := run(t, wtPath, "done")
	if err != nil {
		t.Fatalf("done: %v\n%s\n%s", err, out, stderr)
	}

	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after done")
	}
	// Commit merged into base.
	if _, err := os.Stat(filepath.Join(repo, "feature.txt")); err != nil {
		t.Error("feature commit not merged to main")
	}
	// Uncommitted change restored on base (in the main worktree).
	data, err := os.ReadFile(filepath.Join(repo, "wip.txt"))
	if err != nil || string(data) != "uncommitted\n" {
		t.Errorf("stashed change not restored on base: %v", err)
	}
}

// --keep retains the worktree and branch after a successful done.
func TestDoneKeep(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "done-keep", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-keep")
	testutil.WriteFile(t, wtPath, "keep.txt", "committed\n")
	testutil.Commit(t, wtPath, "feature commit")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := run(t, wtPath, "done", "--keep")
	if err != nil {
		t.Fatalf("done --keep: %v\n%s\n%s", err, out, stderr)
	}

	if _, err := os.Stat(wtPath); err != nil {
		t.Error("worktree removed despite --keep")
	}
	if out := testutil.GitOut(t, repo, "branch", "--list", "wt-done-keep"); strings.TrimSpace(out) == "" {
		t.Error("branch deleted despite --keep")
	}
	// Commit still merged into base; change restored on base.
	if _, err := os.Stat(filepath.Join(repo, "keep.txt")); err != nil {
		t.Error("feature commit not merged to main")
	}
	if _, err := os.Stat(filepath.Join(repo, "wip.txt")); err != nil {
		t.Error("stashed change not restored on base")
	}
}

// Running done in the main worktree is refused.
func TestDoneRefusedInMainWorktree(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "done"); err == nil {
		t.Error("expected done in main worktree to fail")
	}
}

// Stash pop conflicts without --ai: done fails, the stash entry is kept.
func TestDoneNoCommitsPopConflict(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "done-pc", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-pc")

	// Same line modified in the worktree (uncommitted) and committed on main.
	testutil.WriteFile(t, wtPath, "README.md", "# test repo\nworktree version\n")
	testutil.WriteFile(t, repo, "README.md", "# test repo\nmain version\n")
	testutil.Commit(t, repo, "main change")

	out, stderr, err := run(t, wtPath, "done")
	if err == nil {
		t.Fatalf("expected done to fail on pop conflict:\n%s\n%s", out, stderr)
	}
	combined := out + stderr
	if !strings.Contains(combined, "stash") {
		t.Errorf("expected stash-related error:\n%s", combined)
	}
	// Stash entry kept for manual recovery.
	if out := testutil.GitOut(t, repo, "stash", "list"); !strings.Contains(out, "wt done") {
		t.Errorf("stash entry not kept after conflict:\n%s", out)
	}
}

// Merge failure with a dirty worktree: the stash is restored in the feature
// worktree so the command can be re-run.
func TestDoneMergeFailureRestoresStash(t *testing.T) {
	repo := testutil.NewRepo(t)
	if _, _, err := run(t, repo, "new", "done-mf", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-mf")

	// Diverge: same file committed on both branches, plus uncommitted work.
	testutil.WriteFile(t, wtPath, "a.txt", "feature version\n")
	testutil.Commit(t, wtPath, "feature change")
	testutil.WriteFile(t, repo, "a.txt", "main version\n")
	testutil.Commit(t, repo, "main change")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := run(t, wtPath, "done")
	if err == nil {
		t.Fatalf("expected done to fail on rebase conflict:\n%s\n%s", out, stderr)
	}

	// Worktree survives, branch intact, rebase aborted.
	if _, err := os.Stat(wtPath); err != nil {
		t.Error("worktree removed after failed done")
	}
	res, _ := exec.Command("git", "-C", wtPath, "status", "--porcelain").CombinedOutput()
	if strings.Contains(string(res), "UU") {
		t.Error("rebase not aborted, conflicts remain")
	}
	// Stash restored: uncommitted file is back in the feature worktree.
	if _, err := os.Stat(filepath.Join(wtPath, "wip.txt")); err != nil {
		t.Error("stashed changes not restored in feature worktree")
	}
	// Nothing leaked into the stash list.
	if out := testutil.GitOut(t, repo, "stash", "list"); strings.Contains(out, "wt done") {
		t.Errorf("stash entry left behind after restore:\n%s", out)
	}
}

// Base branch checked out in a non-main worktree: merge and stash restore
// happen there instead of switching the main worktree.
func TestDoneBaseInOtherWorktree(t *testing.T) {
	repo := testutil.NewRepo(t)

	// Detach the main worktree so main can be checked out elsewhere.
	testutil.GitOut(t, repo, "switch", "--detach")
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "switch", "main").Run()
	})
	// Park main in a second worktree so the base branch is "elsewhere".
	mainElsewhere := filepath.Join(filepath.Dir(repo), "myrepo-main-elsewhere")
	if out, err := exec.Command("git", "-C", repo, "worktree", "add", mainElsewhere, "main").CombinedOutput(); err != nil {
		t.Fatalf("worktree add: %v\n%s", err, out)
	}
	t.Cleanup(func() {
		_ = exec.Command("git", "-C", repo, "worktree", "remove", "--force", mainElsewhere).Run()
	})

	if out, stderr, err := runEnv(t, mainElsewhere, t.TempDir(), []string{"WT_AI_TOOL="}, "new", "done-bo", "--no-term"); err != nil {
		t.Fatalf("new: %v\n%s\n%s", err, out, stderr)
	}
	// 'wt new' names the worktree after the worktree it runs from.
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-main-elsewhere-wt-done-bo")
	testutil.WriteFile(t, wtPath, "bo.txt", "feature work\n")
	testutil.Commit(t, wtPath, "feature commit")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := run(t, wtPath, "done")
	if err != nil {
		t.Fatalf("done: %v\n%s\n%s", err, out, stderr)
	}

	// Merge happened where main is checked out.
	if _, err := os.Stat(filepath.Join(mainElsewhere, "bo.txt")); err != nil {
		t.Error("feature commit not merged into main (other worktree)")
	}
	// Stash restored there too.
	if _, err := os.Stat(filepath.Join(mainElsewhere, "wip.txt")); err != nil {
		t.Error("stashed change not restored where base is checked out")
	}
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Error("worktree not removed after done")
	}
}

// With --ai, a rebase conflict resolved by the AI tool should make done
// continue automatically (no manual re-run): merge completes, the worktree
// is cleaned up, and the stash is restored on the base branch.
func TestDoneAIRebaseConflictAutoContinues(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	// Fake AI tool: resolves every conflicted file by keeping both sides
	// (dropping the conflict markers), stages it, and continues the rebase.
	fakeAI := filepath.Join(home, "fake-ai.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
export GIT_EDITOR=true
export GIT_SEQUENCE_EDITOR=true
for f in $(git diff --name-only --diff-filter=U); do
  grep -v -E '^(<<<<<<<|=======|>>>>>>>)' "$f" > "$f.resolved"
  mv "$f.resolved" "$f"
  git add "$f"
done
git rebase --continue
`
	if err := os.WriteFile(fakeAI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runHome(t, repo, home, "new", "done-ai", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-ai")

	// Diverge: same file committed on both branches, plus uncommitted work.
	testutil.WriteFile(t, wtPath, "s.txt", "from feature\n")
	testutil.Commit(t, wtPath, "feature change")
	testutil.WriteFile(t, repo, "s.txt", "from main\n")
	testutil.Commit(t, repo, "main change")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := runEnv(t, wtPath, home, []string{"WT_AI_TOOL=" + fakeAI}, "done", "--ai")
	if err != nil {
		t.Fatalf("done --ai: %v\n%s\n%s", err, out, stderr)
	}

	// The rebase was resolved by the fake AI and done continued on its own:
	// merge completed and the worktree + branch are gone.
	if _, err := os.Stat(wtPath); !os.IsNotExist(err) {
		t.Errorf("worktree not removed after done --ai auto-continue:\n%s\n%s", out, stderr)
	}
	if out := testutil.GitOut(t, repo, "branch", "--list", "wt-done-ai"); strings.TrimSpace(out) != "" {
		t.Error("branch wt-done-ai still exists")
	}
	// Feature commit merged into base with both sides kept.
	data, err := os.ReadFile(filepath.Join(repo, "s.txt"))
	if err != nil {
		t.Fatalf("s.txt not merged to main: %v", err)
	}
	if !strings.Contains(string(data), "from main") || !strings.Contains(string(data), "from feature") {
		t.Errorf("resolved s.txt should keep both sides, got:\n%s", data)
	}
	// Stash restored on the base branch.
	if _, err := os.Stat(filepath.Join(repo, "wip.txt")); err != nil {
		t.Error("stashed change not restored on base after auto-continue")
	}
	// Not left mid-rebase.
	if _, err := os.Stat(filepath.Join(repo, ".git", "REBASE_HEAD")); !os.IsNotExist(err) {
		t.Error("repository left mid-rebase")
	}
}

// With --ai and WT_AI_TOOL_MERGE set, the merge invocation must use the
// override command (prompt appended), and the WT_* overrides must be
// visible inside the AI tool process (env passthrough down the chain).
func TestDoneAIUsesMergeEnvOverride(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	// Fake AI: records its argv and the WT_* env it sees, then resolves the
	// rebase like the auto-continue helper.
	logFile := filepath.Join(home, "fake-ai-merge.log")
	fakeAI := filepath.Join(home, "fake-ai-merge.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
{
  echo "argv: $*"
  echo "WT_AI_TOOL=$WT_AI_TOOL"
  echo "WT_AI_TOOL_MERGE=$WT_AI_TOOL_MERGE"
} > "` + logFile + `"
export GIT_EDITOR=true
export GIT_SEQUENCE_EDITOR=true
for f in $(git diff --name-only --diff-filter=U); do
  grep -v -E '^(<<<<<<<|=======|>>>>>>>)' "$f" > "$f.resolved"
  mv "$f.resolved" "$f"
  git add "$f"
done
git rebase --continue
`
	if err := os.WriteFile(fakeAI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runHome(t, repo, home, "new", "done-merge-env", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-merge-env")

	testutil.WriteFile(t, wtPath, "s.txt", "from feature\n")
	testutil.Commit(t, wtPath, "feature change")
	testutil.WriteFile(t, repo, "s.txt", "from main\n")
	testutil.Commit(t, repo, "main change")

	out, stderr, err := runEnv(t, wtPath, home, []string{
		"WT_AI_TOOL=" + fakeAI,
		"WT_AI_TOOL_MERGE=" + fakeAI + " exec --headless",
	}, "done", "--ai")
	if err != nil {
		t.Fatalf("done --ai: %v\n%s\n%s", err, out, stderr)
	}

	data, err := os.ReadFile(logFile)
	if err != nil {
		t.Fatalf("fake AI log not written: %v", err)
	}
	log := string(data)
	// The merge override took effect: extra flags first, prompt appended last.
	if !strings.Contains(log, "argv: exec --headless ") {
		t.Errorf("merge override argv missing, log:\n%s", log)
	}
	if !strings.Contains(log, "Resolve the merge conflicts") {
		t.Errorf("prompt not appended to merge command, log:\n%s", log)
	}
	// Passthrough: the tool process sees both WT_* overrides.
	if !strings.Contains(log, "WT_AI_TOOL="+fakeAI) {
		t.Errorf("WT_AI_TOOL not passed through, log:\n%s", log)
	}
	if !strings.Contains(log, "WT_AI_TOOL_MERGE="+fakeAI+" exec --headless") {
		t.Errorf("WT_AI_TOOL_MERGE not passed through, log:\n%s", log)
	}
}

// When the AI session is cancelled (exits non-zero without finishing the
// rebase), done must abort the unfinished rebase, restore the stash, and fail
// — leaving the worktree clean so the command can be re-run.
func TestDoneAICancelledMidRebase(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	// Fake AI that gives up immediately without resolving anything (simulates
	// Ctrl+C / a failed AI session). Exits non-zero.
	fakeAI := filepath.Join(home, "fake-ai-cancel.sh")
	if err := os.WriteFile(fakeAI, []byte("#!/usr/bin/env bash\nexit 1\n"), 0o755); err != nil {
		t.Fatal(err)
	}

	if _, _, err := runHome(t, repo, home, "new", "done-cancel", "--no-term"); err != nil {
		t.Fatalf("new: %v", err)
	}
	wtPath := filepath.Join(filepath.Dir(repo), "myrepo-wt-done-cancel")

	testutil.WriteFile(t, wtPath, "s.txt", "from feature\n")
	testutil.Commit(t, wtPath, "feature change")
	testutil.WriteFile(t, repo, "s.txt", "from main\n")
	testutil.Commit(t, repo, "main change")
	testutil.WriteFile(t, wtPath, "wip.txt", "uncommitted\n")

	out, stderr, err := runEnv(t, wtPath, home, []string{"WT_AI_TOOL=" + fakeAI}, "done", "--ai")
	if err == nil {
		t.Fatalf("expected done to fail when AI is cancelled:\n%s\n%s", out, stderr)
	}

	// The unfinished rebase is aborted: no conflict markers, not mid-rebase.
	res, _ := exec.Command("git", "-C", wtPath, "status", "--porcelain").CombinedOutput()
	if strings.Contains(string(res), "UU") {
		t.Errorf("rebase not aborted after AI cancel, conflicts remain:\n%s", res)
	}
	if out := testutil.GitOut(t, wtPath, "rev-parse", "--git-path", "rebase-merge"); true {
		p := out
		if !filepath.IsAbs(p) {
			p = filepath.Join(wtPath, p)
		}
		if _, err := os.Stat(p); !os.IsNotExist(err) {
			t.Error("worktree left mid-rebase after AI cancel")
		}
	}
	// Worktree and branch survive for a re-run.
	if _, err := os.Stat(wtPath); err != nil {
		t.Error("worktree removed after cancelled AI")
	}
	// Stash restored: uncommitted file is back, no stash entry leaked.
	if _, err := os.Stat(filepath.Join(wtPath, "wip.txt")); err != nil {
		t.Error("stashed changes not restored after AI cancel")
	}
	if out := testutil.GitOut(t, repo, "stash", "list"); strings.Contains(out, "wt done") {
		t.Errorf("stash entry left behind after restore:\n%s", out)
	}
}
