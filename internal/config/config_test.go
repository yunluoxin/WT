package config

import (
	"os"
	"path/filepath"
	"testing"

	"wt/internal/testutil"
)

func TestDefaults(t *testing.T) {
	testutil.SetHome(t)
	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := GetString(cfg, "ai_tool.command"); got != "claude" {
		t.Errorf("ai_tool.command = %q, want claude", got)
	}
	if got := GetString(cfg, "launch.session_prefix"); got != "wt" {
		t.Errorf("launch.session_prefix = %q, want wt", got)
	}
	if got := GetString(cfg, "git.default_base_branch"); got != "main" {
		t.Errorf("git.default_base_branch = %q, want main", got)
	}
	if got := GetFloat(cfg, "launch.wezterm_ready_timeout"); got != 5.0 {
		t.Errorf("wezterm_ready_timeout = %v, want 5.0", got)
	}
}

func TestSetAndBoolCoercion(t *testing.T) {
	testutil.SetHome(t)
	cfg, _ := Load()
	Set(cfg, "launch.method", "tmux")
	if got := GetString(cfg, "launch.method"); got != "tmux" {
		t.Errorf("launch.method = %q", got)
	}
	Set(cfg, "update.auto_check", "TRUE")
	if got := GetBool(cfg, "update.auto_check"); got != true {
		t.Errorf("bool coercion failed: %v", Get(cfg, "update.auto_check"))
	}
	Set(cfg, "update.auto_check", "false")
	if got := GetBool(cfg, "update.auto_check"); got != false {
		t.Errorf("bool coercion failed")
	}
	// Creates intermediate maps.
	Set(cfg, "brand.new.key", "x")
	if got := GetString(cfg, "brand.new.key"); got != "x" {
		t.Errorf("nested set failed: %q", got)
	}
}

func TestSaveLoadRoundTrip(t *testing.T) {
	home := testutil.SetHome(t)
	cfg, _ := Load()
	Set(cfg, "launch.method", "zellij")
	if err := Save(cfg); err != nil {
		t.Fatalf("Save: %v", err)
	}
	if _, err := os.Stat(filepath.Join(home, ".config", "wt", "config.json")); err != nil {
		t.Fatalf("config file not written: %v", err)
	}
	cfg2, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if got := GetString(cfg2, "launch.method"); got != "zellij" {
		t.Errorf("round trip failed: %q", got)
	}
	// Defaults still present after merge.
	if got := GetString(cfg2, "ai_tool.command"); got != "claude" {
		t.Errorf("default lost after merge: %q", got)
	}
}

func TestInvalidJSON(t *testing.T) {
	testutil.SetHome(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(Path(), []byte("{invalid"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(); err == nil {
		t.Fatal("expected error for invalid JSON")
	}
}

func TestDeepMergePreservesDefaults(t *testing.T) {
	testutil.SetHome(t)
	if err := os.MkdirAll(Dir(), 0o755); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(Path(), []byte(`{"launch": {"session_prefix": "custom"}}`), 0o644)
	cfg, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if got := GetString(cfg, "launch.session_prefix"); got != "custom" {
		t.Errorf("session_prefix = %q", got)
	}
	if got := GetFloat(cfg, "launch.wezterm_ready_timeout"); got != 5.0 {
		t.Errorf("default lost: %v", got)
	}
}

func TestParseTermOption(t *testing.T) {
	cases := []struct {
		in      string
		method  LaunchMethod
		session string
		wantErr bool
	}{
		{"foreground", MethodForeground, "", false},
		{"fg", MethodForeground, "", false},
		{"d", MethodDetach, "", false},
		{"bg", MethodDetach, "", false},
		{"iterm-window", MethodItermWindow, "", false},
		{"i-t", MethodItermTab, "", false},
		{"t", MethodTmux, "", false},
		{"t:mysession", MethodTmux, "mysession", false},
		{"tmux:work", MethodTmux, "work", false},
		{"z:dev", MethodZellij, "dev", false},
		{"w-w", MethodWeztermWindow, "", false},
		{"z-p-h", MethodZellijPaneH, "", false},
		{"foreground:name", "", "", true}, // session only for tmux/zellij
		{"bogus", "", "", true},
	}
	for _, c := range cases {
		spec, err := ParseTermOption(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("ParseTermOption(%q): expected error", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("ParseTermOption(%q): %v", c.in, err)
			continue
		}
		if spec.Method != c.method || spec.Session != c.session {
			t.Errorf("ParseTermOption(%q) = %+v, want %v/%q", c.in, spec, c.method, c.session)
		}
	}
}

func TestPresets(t *testing.T) {
	p, ok := FindPreset("claude-yolo")
	if !ok || len(p.Command) != 2 || p.Command[1] != "--dangerously-skip-permissions" {
		t.Errorf("claude-yolo preset wrong: %+v", p)
	}
	if _, ok := FindPreset("nope"); ok {
		t.Error("unexpected preset match")
	}
	if got := PresetNameForCommand([]string{"codex"}); got != "codex" {
		t.Errorf("PresetNameForCommand = %q", got)
	}
	if got := PresetNameForCommand([]string{"unknown", "--x"}); got != "" {
		t.Errorf("PresetNameForCommand unknown = %q", got)
	}
	if !IsClaudeCommand([]string{"claude", "--print"}) {
		t.Error("IsClaudeCommand false negative")
	}
	if IsClaudeCommand([]string{"codex"}) {
		t.Error("IsClaudeCommand false positive")
	}
}

func TestResumeAndMergePresets(t *testing.T) {
	if got := ResumePresets["claude"]; len(got) != 2 || got[1] != "--continue" {
		t.Errorf("claude resume preset wrong: %v", got)
	}
	if got := ResumePresets["codex"]; len(got) != 3 || got[1] != "resume" || got[2] != "--last" {
		t.Errorf("codex resume preset wrong: %v", got)
	}
	mp := MergePresets["claude-remote"]
	if len(mp.BaseOverride) != 1 || mp.BaseOverride[0] != "claude" {
		t.Errorf("claude-remote merge base override wrong: %v", mp.BaseOverride)
	}
}
