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

func TestIsClaudeTool(t *testing.T) {
	testutil.SetHome(t)
	unsetEnv(t, "WT_AI_TOOL")
	cfg := loadCfg(t)
	if !IsClaudeTool(cfg) {
		t.Error("expected claude tool by default")
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
