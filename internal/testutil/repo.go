// Package testutil provides fixtures for tests: real temporary git
// repositories and isolated HOME environments (classicist style, mirroring
// the Python conftest.py).
package testutil

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"wt/internal/git"
)

// SetHome isolates HOME/USERPROFILE/XDG_CONFIG_HOME to a temp dir and
// forces non-interactive mode with AI launching disabled.
// Note: uses t.Setenv, so tests calling this cannot use t.Parallel().
func SetHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(home, ".config"))
	t.Setenv("WT_NON_INTERACTIVE", "1")
	t.Setenv("WT_AI_TOOL", "")
	t.Setenv("GIT_CONFIG_NOSYSTEM", "1")
	t.Setenv("GIT_TERMINAL_PROMPT", "0")
	// Neutralize CI env vars that would otherwise force non-interactive.
	for _, v := range []string{"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_HOME", "CIRCLECI", "TRAVIS", "BUILDKITE", "DRONE", "BITBUCKET_PIPELINE", "CODEBUILD_BUILD_ID"} {
		t.Setenv(v, "")
	}
	return home
}

// NewRepo creates a real git repository in a temp dir with an initial
// commit on branch main. Returns the repo path.
func NewRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	repo := filepath.Join(dir, "myrepo")
	if err := os.MkdirAll(repo, 0o755); err != nil {
		t.Fatalf("mkdir repo: %v", err)
	}
	mustGit(t, repo, "init", "-b", "main")
	mustGit(t, repo, "config", "user.name", "Test User")
	mustGit(t, repo, "config", "user.email", "test@example.com")
	mustGit(t, repo, "config", "commit.gpgsign", "false")
	WriteFile(t, repo, "README.md", "# test repo\n")
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", "initial commit")
	t.Cleanup(func() {
		// Best-effort removal of any worktrees created during the test.
		_, _ = git.Git(repo, false, "worktree", "prune")
	})
	return repo
}

// Chdir changes the working directory for the duration of the test.
func Chdir(t *testing.T, dir string) {
	t.Helper()
	orig, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(orig) })
}

// WriteFile writes a file inside the repo, creating parent dirs.
func WriteFile(t *testing.T, repo, name, content string) {
	t.Helper()
	p := filepath.Join(repo, name)
	if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
		t.Fatalf("mkdir for %s: %v", name, err)
	}
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", name, err)
	}
}

// Commit stages everything and commits.
func Commit(t *testing.T, repo, message string) {
	t.Helper()
	mustGit(t, repo, "add", ".")
	mustGit(t, repo, "commit", "-m", message)
}

// GitOut runs git in the repo and returns trimmed stdout, failing on error.
func GitOut(t *testing.T, repo string, args ...string) string {
	t.Helper()
	out, err := git.Output(repo, args...)
	if err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
	return out
}

func mustGit(t *testing.T, dir string, args ...string) {
	t.Helper()
	if _, err := git.Git(dir, true, args...); err != nil {
		t.Fatalf("git %s: %v", strings.Join(args, " "), err)
	}
}
