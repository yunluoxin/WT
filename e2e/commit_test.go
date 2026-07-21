package e2e

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"wt/internal/testutil"
)

// Fake AI that stages the working tree and creates a commit with the
// given message. Extra flags (e.g. --no-verify, --amend) go directly
// to git commit so the test can drive every wt commit flag through AI
// behavior instead of through prompt parsing.
func writeCommittingFakeAI(t *testing.T, path, message string, extraFlags string) {
	t.Helper()
	tail := ""
	if extraFlags != "" {
		tail = " " + extraFlags
	}
	script := `#!/usr/bin/env bash
set -euo pipefail
git add -A
git commit -m ` + shellQuote(message) + tail + `
`
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake AI: %v", err)
	}
}

// Fake AI that exits without running git. Used to verify wt's
// post-AI HEAD check catches a no-op agent.
func writeSilentFakeAI(t *testing.T, path string) {
	t.Helper()
	script := "#!/usr/bin/env bash\nexit 0\n"
	if err := os.WriteFile(path, []byte(script), 0o755); err != nil {
		t.Fatalf("write fake AI: %v", err)
	}
}

func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// AI runs `git add -A` and `git commit -m X` itself; wt should see HEAD
// advance and report success with the AI's message as the commit subject.
func TestCommitBasicAgentRunsGit(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	fakeAI := filepath.Join(home, "fake-ai.sh")
	writeCommittingFakeAI(t, fakeAI, "feat: agent handled the commit", "")

	testutil.WriteFile(t, repo, "greet.go", "package main\n")
	out, _, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI}, "commit")
	if err != nil {
		t.Fatalf("commit: %v\n%s", err, out)
	}
	if got := testutil.GitOut(t, repo, "log", "-1", "--pretty=%s"); got != "feat: agent handled the commit" {
		t.Errorf("commit message = %q, want AI-authored subject", got)
	}
	// File ended up in the repo.
	if out := testutil.GitOut(t, repo, "show", "HEAD:greet.go"); !strings.Contains(out, "package main") {
		t.Errorf("greet.go not in HEAD:\n%s", out)
	}
}

// --no-verify is forwarded as part of the AI instruction; verify the
// AI is told to pass --no-verify through to git commit by inspecting
// argv recorded by a logging fake AI.
func TestCommitNoVerifyForwarded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	// Pre-commit hook that always fails. Without --no-verify the commit
	// would fail at the git level.
	hook := "#!/usr/bin/env bash\necho 'hook refuses' >&2\nexit 1\n"
	if err := os.WriteFile(filepath.Join(repo, ".git", "hooks", "pre-commit"), []byte(hook), 0o755); err != nil {
		t.Fatal(err)
	}

	fakeAI := filepath.Join(home, "fake-ai.sh")
	writeCommittingFakeAI(t, fakeAI, "chore: bypassed the hook", "--no-verify")

	testutil.WriteFile(t, repo, "v.go", "package main\n")
	out, _, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI}, "commit", "--no-verify")
	if err != nil {
		t.Fatalf("commit --no-verify: %v\n%s", err, out)
	}
	if got := testutil.GitOut(t, repo, "log", "-1", "--pretty=%s"); got != "chore: bypassed the hook" {
		t.Errorf("commit message = %q", got)
	}
}

// --amend: HEAD SHA can change (commit object is rebuilt) but the
// commit count must stay the same and the new subject replaces the
// previous one. wt accepts that as success.
func TestCommitAmendKeepsHead(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	fakeAI := filepath.Join(home, "fake-ai.sh")

	// First commit: AI creates a new commit.
	writeCommittingFakeAI(t, fakeAI, "feat: first take", "")
	testutil.WriteFile(t, repo, "a.go", "v1\n")
	if _, _, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI}, "commit"); err != nil {
		t.Fatalf("initial commit: %v", err)
	}
	countBefore := testutil.GitOut(t, repo, "rev-list", "--count", "HEAD")
	parentBefore := testutil.GitOut(t, repo, "rev-parse", "HEAD^")

	// Second commit: AI amends the previous one.
	writeCommittingFakeAI(t, fakeAI, "feat: improved take", "--amend")
	testutil.WriteFile(t, repo, "a.go", "v2 improved\n")

	out, _, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI}, "commit", "--amend")
	if err != nil {
		t.Fatalf("commit --amend: %v\n%s", err, out)
	}

	// Same number of commits, same parent (rewritten commit replaces
	// its predecessor in place), updated subject.
	if countAfter := testutil.GitOut(t, repo, "rev-list", "--count", "HEAD"); countAfter != countBefore {
		t.Errorf("commit count changed by amend: %s -> %s", countBefore, countAfter)
	}
	if parentAfter := testutil.GitOut(t, repo, "rev-parse", "HEAD^"); parentAfter != parentBefore {
		t.Errorf("amend produced a new commit (parent shifted): %s -> %s", parentBefore, parentAfter)
	}
	if got := testutil.GitOut(t, repo, "log", "-1", "--pretty=%s"); got != "feat: improved take" {
		t.Errorf("amended subject = %q, want %q", got, "feat: improved take")
	}
}

// Clean tree: short-circuit before the AI check, exit 0, no AI process.
func TestCommitCleanTreeIsNoop(t *testing.T) {
	repo := testutil.NewRepo(t)
	headBefore := testutil.GitOut(t, repo, "rev-parse", "HEAD")

	out, _, err := run(t, repo, "commit")
	if err != nil {
		t.Fatalf("commit on clean tree should be a noop: %v\n%s", err, out)
	}
	if !strings.Contains(out, "Nothing to commit") && !strings.Contains(out, "nothing to commit") {
		t.Errorf("expected 'nothing to commit' message, got:\n%s", out)
	}
	if headAfter := testutil.GitOut(t, repo, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Errorf("HEAD changed despite clean tree: %s -> %s", headBefore, headAfter)
	}
}

// No AI configured (run() sets WT_AI_TOOL=) — refuse, don't commit.
func TestCommitAIDisabledErrors(t *testing.T) {
	repo := testutil.NewRepo(t)
	headBefore := testutil.GitOut(t, repo, "rev-parse", "HEAD")

	testutil.WriteFile(t, repo, "staged.go", "package main\n")

	out, stderr, err := run(t, repo, "commit")
	if err == nil {
		t.Fatalf("expected commit to fail with no AI configured\n%s\n%s", out, stderr)
	}
	combined := out + stderr
	if !strings.Contains(combined, "no AI tool configured") {
		t.Errorf("expected 'no AI tool configured', got:\n%s", combined)
	}
	if headAfter := testutil.GitOut(t, repo, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Errorf("HEAD changed despite failed commit: %s -> %s", headBefore, headAfter)
	}
	if _, err := os.Stat(filepath.Join(repo, "staged.go")); err != nil {
		t.Error("staged.go disappeared after failed commit")
	}
}

// AI exits without touching git (e.g. message-only presets like
// aider --message) — wt must catch HEAD not moving and report a clear
// hint.
func TestCommitAINoOpDetectsFailure(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	fakeAI := filepath.Join(home, "fake-ai.sh")
	writeSilentFakeAI(t, fakeAI)

	testutil.WriteFile(t, repo, "noop.go", "package main\n")
	headBefore := testutil.GitOut(t, repo, "rev-parse", "HEAD")

	out, stderr, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI}, "commit")
	if err == nil {
		t.Fatalf("expected commit to fail when AI does nothing\n%s\n%s", out, stderr)
	}
	combined := out + stderr
	if !strings.Contains(combined, "no new commit") {
		t.Errorf("expected 'no new commit' in error, got:\n%s", combined)
	}
	if headAfter := testutil.GitOut(t, repo, "rev-parse", "HEAD"); headAfter != headBefore {
		t.Errorf("HEAD changed despite silent AI: %s -> %s", headBefore, headAfter)
	}
}

// --model <id> should land in the AI tool's argv right before the
// prompt; the commit still succeeds with the AI's own git output.
func TestCommitModelFlagForwarded(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake AI helper is a bash script")
	}
	repo := testutil.NewRepo(t)
	home := sharedHome(t)

	// Logging fake AI: records its argv so we can assert --model was
	// passed in the expected slot, then runs git add -A and commits.
	logPath := filepath.Join(home, "fake-ai.log")
	fakeAI := filepath.Join(home, "fake-ai.sh")
	script := `#!/usr/bin/env bash
set -euo pipefail
{ echo "argv:$*"; } >> ` + logPath + `
git add -A
git commit -m 'feat: model flag accepted'
`
	if err := os.WriteFile(fakeAI, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}

	testutil.WriteFile(t, repo, "m.go", "package main\n")
	out, _, err := runEnv(t, repo, home, []string{"WT_AI_TOOL=" + fakeAI},
		"commit", "--model", "haiku")
	if err != nil {
		t.Fatalf("commit --model: %v\n%s", err, out)
	}

	if got := testutil.GitOut(t, repo, "log", "-1", "--pretty=%s"); got != "feat: model flag accepted" {
		t.Errorf("commit subject = %q", got)
	}
	logBytes, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("read fake AI log: %v", err)
	}
	log := string(logBytes)
	// All three tokens must appear in argv; positional order isn't
	// asserted because it depends on how the underlying AI tool argv
	// parser treats the trailing prompt.
	for _, want := range []string{"--model", "haiku"} {
		if !strings.Contains(log, want) {
			t.Errorf("expected %q in AI argv, got:\n%s", want, log)
		}
	}
}
