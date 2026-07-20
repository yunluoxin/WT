package cli

import (
	"bytes"
	"strings"
	"testing"
)

// Core commands that must have an llm subcommand.
var llmExpected = []string{
	"new", "list", "status", "delete", "merge", "done", "pr", "resume", "shell",
	"sync", "clean", "change-base", "doctor", "diff", "tree", "stats",
	"stash", "backup", "hook", "config", "cd", "init",
}

func runLLM(t *testing.T, args ...string) string {
	t.Helper()
	root := NewRootCmd()
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	cmd, err := root.ExecuteC()
	if err != nil {
		t.Fatalf("llm %v: %v", args, err)
	}
	// Snippets are printed directly to stdout, not through cobra's writer.
	_ = cmd
	return buf.String()
}

func TestLLMCoversAllCoreCommands(t *testing.T) {
	llm := llmCmd()
	subs := map[string]bool{}
	for _, c := range llm.Commands() {
		subs[c.Name()] = true
	}
	for _, name := range llmExpected {
		if !subs[name] {
			t.Errorf("llm missing subcommand %q", name)
		}
	}
}

func TestLLMSubcommandOutput(t *testing.T) {
	for _, name := range llmExpected {
		snippet := ""
		for _, s := range llmSpecs {
			if s.name == name {
				snippet = s.snippet
			}
		}
		if snippet == "" {
			t.Errorf("no spec for %q", name)
			continue
		}
		if !strings.Contains(snippet, "wt "+name) {
			t.Errorf("snippet for %q does not contain its usage line", name)
		}
	}
}

func TestLLMAllIncludesEveryCommand(t *testing.T) {
	all := llmAll()
	for _, name := range llmExpected {
		if !strings.Contains(all, "wt "+name) {
			t.Errorf("llmAll output missing usage for %q", name)
		}
	}
	if !strings.Contains(all, "Core workflow") {
		t.Error("llmAll output missing preamble")
	}
}

func TestLLMHookSnippetListsAllEvents(t *testing.T) {
	snippet := specFor(t, "hook")
	for _, event := range []string{
		"worktree.pre_create", "worktree.post_create",
		"worktree.pre_delete", "worktree.post_delete",
		"merge.pre", "merge.post",
		"pr.pre", "pr.post",
		"resume.pre", "resume.post",
		"sync.pre", "sync.post",
	} {
		if !strings.Contains(snippet, event) {
			t.Errorf("hook snippet missing event %q", event)
		}
	}
	if !strings.Contains(snippet, "Example") {
		t.Error("hook snippet missing examples")
	}
}

func TestLLMConfigSnippetListsAllPresets(t *testing.T) {
	snippet := specFor(t, "config")
	for _, preset := range []string{
		"no-op", "claude", "claude-yolo", "claude-remote",
		"claude-yolo-remote", "codex", "codex-yolo",
		"cursor-agent", "cursor-agent-yolo",
		"aider", "gemini", "opencode", "crush", "kimi", "qwen",
		"copilot", "goose",
	} {
		if !strings.Contains(snippet, preset) {
			t.Errorf("config snippet missing preset %q", preset)
		}
	}
}

func TestLLMConfigSnippetDocumentsSetKeys(t *testing.T) {
	snippet := specFor(t, "config")
	for _, want := range []string{
		// How to set a custom AI tool — the keys have dashes, not dots.
		"ai-tool", "ai-tool.name", "ai-tool.merge", "ai-tool.resume",
		"{prompt}",
		"launch.method", "launch.session_prefix",
		"git.default_base_branch", "session.auto_resume",
		// Env overrides.
		"WT_AI_TOOL", "WT_AI_TOOL_MERGE", "WT_AI_TOOL_RESUME",
		"WT_LAUNCH_METHOD", "WT_AUTO_RESUME",
		// Subcommands.
		"config show", "config set", "use-preset", "list-presets", "config reset",
	} {
		if !strings.Contains(snippet, want) {
			t.Errorf("config snippet missing %q", want)
		}
	}
}

func TestLLMNewSnippetListsLaunchMethods(t *testing.T) {
	snippet := specFor(t, "new")
	for _, m := range []string{
		"foreground", "detach", "iterm-tab", "tmux", "zellij", "wezterm-tab",
	} {
		if !strings.Contains(snippet, m) {
			t.Errorf("new snippet missing launch method %q", m)
		}
	}
}

func TestLLMKeySnippetsHaveExamples(t *testing.T) {
	for _, name := range []string{"new", "delete", "merge", "pr", "hook", "config", "cd", "clean"} {
		if !strings.Contains(specFor(t, name), "Example") {
			t.Errorf("snippet for %q has no examples", name)
		}
	}
}

func TestLLMStashSnippetCoversSubcommands(t *testing.T) {
	snippet := specFor(t, "stash")
	for _, want := range []string{
		"stash save", "stash list", "stash apply",
		"--include-untracked", "stash@{0}",
	} {
		if !strings.Contains(snippet, want) {
			t.Errorf("stash snippet missing %q", want)
		}
	}
}

func TestLLMBackupSnippetCoversSubcommands(t *testing.T) {
	snippet := specFor(t, "backup")
	for _, want := range []string{
		"backup create", "backup list", "backup restore",
		"--all", "--id", "--output",
	} {
		if !strings.Contains(snippet, want) {
			t.Errorf("backup snippet missing %q", want)
		}
	}
}

func TestLLMEverySnippetHasExample(t *testing.T) {
	for _, s := range llmSpecs {
		if !strings.Contains(s.snippet, "Example") {
			t.Errorf("snippet for %q has no examples", s.name)
		}
	}
}

func specFor(t *testing.T, name string) string {
	t.Helper()
	for _, s := range llmSpecs {
		if s.name == name {
			return s.snippet
		}
	}
	t.Fatalf("no spec for %q", name)
	return ""
}

func TestLLMUnknownSubcommandFails(t *testing.T) {
	root := NewRootCmd()
	root.SetOut(&bytes.Buffer{})
	root.SetErr(&bytes.Buffer{})
	root.SetArgs([]string{"llm", "does-not-exist"})
	if err := root.Execute(); err == nil {
		t.Fatal("expected error for unknown llm subcommand")
	}
}
