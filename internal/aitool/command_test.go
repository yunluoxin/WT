package aitool

import (
	"testing"

	"wt/internal/config"
	"wt/internal/testutil"
)

func loadCfg(t *testing.T) map[string]any {
	t.Helper()
	cfg, err := config.Load()
	if err != nil {
		t.Fatal(err)
	}
	return cfg
}

func TestEffectiveCommandDefault(t *testing.T) {
	testutil.SetHome(t) // sets WT_AI_TOOL="" → no-op
	cfg := loadCfg(t)
	if got := EffectiveCommand(cfg); len(got) != 0 {
		t.Errorf("expected no-op via WT_AI_TOOL env, got %v", got)
	}
}

func TestEffectiveCommandFromConfig(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "") // explicit empty = no-op
	// Remove the env override to test config path.
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	got := EffectiveCommand(cfg)
	if len(got) != 1 || got[0] != "claude" {
		t.Errorf("EffectiveCommand = %v, want [claude]", got)
	}
}

func TestEnvOverride(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "codex --full-auto")
	cfg := loadCfg(t)
	got := EffectiveCommand(cfg)
	if len(got) != 2 || got[0] != "codex" || got[1] != "--full-auto" {
		t.Errorf("EffectiveCommand = %v", got)
	}
}

func TestResumeCommand(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	got := ResumeCommand(cfg)
	if len(got) != 2 || got[0] != "claude" || got[1] != "--continue" {
		t.Errorf("ResumeCommand = %v, want [claude --continue]", got)
	}
}

func TestResumeCommandEnv(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "myai --fast")
	cfg := loadCfg(t)
	got := ResumeCommand(cfg)
	want := []string{"myai", "--fast", "--resume"}
	if !equal(got, want) {
		t.Errorf("ResumeCommand = %v, want %v", got, want)
	}
}

func TestMergeCommand(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix conflicts")
	want := []string{"claude", "--print", "--tools=default", "fix conflicts"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

func TestMergeCommandRemoteStripsFlag(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	// Configure claude-remote preset.
	cfg["ai_tool"] = map[string]any{"command": "claude", "args": []any{"/remote-control"}}
	got := MergeCommand(cfg, "fix")
	want := []string{"claude", "--print", "--tools=default", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// WT_AI_TOOL set to a preset NAME expands to that preset's command, so the
// preset resume/merge variants apply (e.g. cursor-agent-yolo --force and its
// --print --trust --force merge invocation).
func TestEnvPresetNameExpands(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "cursor-agent-yolo")
	cfg := loadCfg(t)

	if got := EffectiveCommand(cfg); !equal(got, []string{"cursor-agent", "--force"}) {
		t.Errorf("EffectiveCommand = %v, want [cursor-agent --force]", got)
	}
	if got := ResumeCommand(cfg); !equal(got, []string{"cursor-agent", "--force", "resume"}) {
		t.Errorf("ResumeCommand = %v, want [cursor-agent --force resume]", got)
	}
	got := MergeCommand(cfg, "fix conflicts")
	want := []string{"cursor-agent", "--print", "--trust", "--force", "fix conflicts"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// A preset name that is also a plain command (claude) still expands via the
// preset table, picking up the merge flags.
func TestEnvPresetNameClaude(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "claude")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix")
	want := []string{"claude", "--print", "--tools=default", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// A non-preset WT_AI_TOOL value is still split verbatim and does not gain
// preset merge flags.
func TestEnvNonPresetUnchanged(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "myai --fast")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix")
	want := []string{"myai", "--fast", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

func TestIsClaudeTool(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	if !IsClaudeTool(cfg) {
		t.Error("expected claude tool by default")
	}
}

func TestApplyPrompt(t *testing.T) {
	// Placeholder in the middle.
	got := applyPrompt([]string{"aider", "--message", "{prompt}"}, "fix it")
	if want := []string{"aider", "--message", "fix it"}; !equal(got, want) {
		t.Errorf("applyPrompt = %v, want %v", got, want)
	}
	// No placeholder: prompt is appended.
	got = applyPrompt([]string{"opencode", "run"}, "fix it")
	if want := []string{"opencode", "run", "fix it"}; !equal(got, want) {
		t.Errorf("applyPrompt = %v, want %v", got, want)
	}
	// Placeholder embedded in a flag value is replaced in place.
	got = applyPrompt([]string{"tool", "--msg={prompt}"}, "fix it")
	if want := []string{"tool", "--msg=fix it"}; !equal(got, want) {
		t.Errorf("applyPrompt = %v, want %v", got, want)
	}
}

// Explicit ai_tool.merge_command/merge_args in config win over preset
// inference, and {prompt} is substituted.
func TestMergeCommandFromConfig(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	cfg["ai_tool"] = map[string]any{
		"command":       "claude",
		"args":          []any{},
		"merge_command": "aider",
		"merge_args":    []any{"--yes-always", "--message", "{prompt}"},
	}
	got := MergeCommand(cfg, "fix conflicts")
	want := []string{"aider", "--yes-always", "--message", "fix conflicts"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// merge_args without merge_command extend the launch command.
func TestMergeCommandArgsExtendLaunch(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	cfg["ai_tool"] = map[string]any{
		"command":    "gemini",
		"args":       []any{},
		"merge_args": []any{"-p", "{prompt}"},
	}
	got := MergeCommand(cfg, "fix")
	want := []string{"gemini", "-p", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// Explicit ai_tool.resume_command/resume_args in config win over preset
// inference.
func TestResumeCommandFromConfig(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	cfg["ai_tool"] = map[string]any{
		"command":        "myai",
		"args":           []any{"--fast"},
		"resume_command": "myai",
		"resume_args":    []any{"session", "resume"},
	}
	got := ResumeCommand(cfg)
	want := []string{"myai", "session", "resume"}
	if !equal(got, want) {
		t.Errorf("ResumeCommand = %v, want %v", got, want)
	}
}

// WT_AI_TOOL_MERGE overrides everything for the merge variant.
func TestMergeCommandEnvOverride(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "claude")
	t.Setenv("WT_AI_TOOL_MERGE", "opencode run {prompt}")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix")
	want := []string{"opencode", "run", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// WT_AI_TOOL_MERGE set to a preset name expands to that preset's merge
// command.
func TestMergeCommandEnvPresetName(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "claude")
	t.Setenv("WT_AI_TOOL_MERGE", "codex")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix")
	want := []string{"codex", "exec", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

// WT_AI_TOOL_RESUME overrides the resume variant.
func TestResumeCommandEnvOverride(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "claude")
	t.Setenv("WT_AI_TOOL_RESUME", "crush --continue")
	cfg := loadCfg(t)
	got := ResumeCommand(cfg)
	want := []string{"crush", "--continue"}
	if !equal(got, want) {
		t.Errorf("ResumeCommand = %v, want %v", got, want)
	}
}

// codex preset merges via `codex exec <prompt>` (the old --non-interactive
// flag does not exist on current codex).
func TestMergeCommandCodexPreset(t *testing.T) {
	testutil.SetHome(t)
	t.Setenv("WT_AI_TOOL", "codex")
	cfg := loadCfg(t)
	got := MergeCommand(cfg, "fix")
	want := []string{"codex", "exec", "fix"}
	if !equal(got, want) {
		t.Errorf("MergeCommand = %v, want %v", got, want)
	}
}

func TestMergeUsesStdin(t *testing.T) {
	testutil.SetHome(t)
	cfg := loadCfg(t)
	if MergeUsesStdin(cfg) {
		t.Error("merge_stdin should default to false")
	}
	cfg["ai_tool"] = map[string]any{"command": "gemini", "merge_stdin": true}
	if !MergeUsesStdin(cfg) {
		t.Error("merge_stdin = true not picked up")
	}
}

func unsetEnv(t *testing.T, key string) {
	t.Helper()
	// t.Setenv can't unset; use os.Unsetenv with manual restore.
	if v, ok := lookupEnv(key); ok {
		t.Cleanup(func() { setEnv(key, v) })
	}
	unsetEnvImpl(key)
}

func equal(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
