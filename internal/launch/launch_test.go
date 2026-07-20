package launch

import (
	"os"
	"testing"

	"wt/internal/config"
	"wt/internal/testutil"
)

func TestShellQuote(t *testing.T) {
	cases := map[string]string{
		"simple":        "simple",
		"with space":    "'with space'",
		"it's":          `'it'"'"'s'`,
		"":              "''",
		"--flag=value":  "--flag=value",
		"path/to/file":  "path/to/file",
		"$VAR":          "'$VAR'",
		"a;b":           "'a;b'",
		"claude":        "claude",
		"--dangerously": "--dangerously",
	}
	for in, want := range cases {
		if got := shellQuote(in); got != want {
			t.Errorf("shellQuote(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestCommandString(t *testing.T) {
	got := CommandString([]string{"claude", "--print", "hello world"})
	want := "claude --print 'hello world'"
	if got != want {
		t.Errorf("CommandString = %q, want %q", got, want)
	}
}

func TestWithEnvOverrides(t *testing.T) {
	// Clear all passthrough vars so the base case is deterministic.
	for _, k := range passthroughEnv {
		t.Setenv(k, "__sentinel__")
		os.Unsetenv(k)
	}

	if got := withEnvOverrides("claude --continue"); got != "claude --continue" {
		t.Errorf("no env set: got %q, want unchanged command", got)
	}

	t.Setenv("WT_AI_TOOL", "codex")
	t.Setenv("WT_AI_TOOL_MERGE", "opencode run {prompt}")
	got := withEnvOverrides("claude --continue")
	want := "env WT_AI_TOOL=codex WT_AI_TOOL_MERGE='opencode run {prompt}' claude --continue"
	if got != want {
		t.Errorf("withEnvOverrides = %q, want %q", got, want)
	}
}

func TestGenerateSessionName(t *testing.T) {
	testutil.SetHome(t)
	cfg, _ := config.Load()
	if got := GenerateSessionName(cfg, "/repos/myrepo-feat"); got != "wt-myrepo-feat" {
		t.Errorf("GenerateSessionName = %q", got)
	}
	// Long names are truncated to MaxSessionNameLength.
	long := "/repos/" + string(make([]byte, 0)) + "a-very-long-worktree-directory-name-that-exceeds-fifty-chars-easily"
	got := GenerateSessionName(cfg, long)
	if len(got) > config.MaxSessionNameLength {
		t.Errorf("session name too long: %d chars", len(got))
	}
	// Custom prefix from config.
	cfg["launch"] = map[string]any{"session_prefix": "dev"}
	if got := GenerateSessionName(cfg, "/repos/x"); got != "dev-x" {
		t.Errorf("GenerateSessionName custom prefix = %q", got)
	}
}

func TestAIToolNoop(t *testing.T) {
	testutil.SetHome(t) // WT_AI_TOOL="" → no-op
	if err := AITool(Options{WorktreePath: t.TempDir()}); err != nil {
		t.Errorf("no-op launch should not error: %v", err)
	}
}

func TestAIToolInvalidTerm(t *testing.T) {
	testutil.SetHome(t)
	err := AITool(Options{WorktreePath: t.TempDir(), Term: "bogus-method"})
	if err == nil {
		t.Error("expected error for invalid term")
	}
}

func TestAIToolMissingBinary(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "definitely-not-a-real-binary-xyz")
	// Missing binary: warn and return nil (matches Python behavior).
	if err := AITool(Options{WorktreePath: t.TempDir()}); err != nil {
		t.Errorf("missing binary should warn and skip, got: %v", err)
	}
}
