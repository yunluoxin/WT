// Package aitool resolves the effective AI tool commands (launch, resume,
// merge) from config, presets and environment overrides.
package aitool

import (
	"os"
	"strings"

	"wt/internal/config"
)

// EffectiveCommand returns the configured AI tool command as argv.
// Priority: WT_AI_TOOL env (empty = no-op) > config ai_tool.command+args.
func EffectiveCommand(cfg map[string]any) []string {
	if v, ok := os.LookupEnv("WT_AI_TOOL"); ok {
		return splitCommand(v)
	}
	cmd := config.GetString(cfg, "ai_tool.command")
	args := stringSlice(config.Get(cfg, "ai_tool.args"))
	if cmd == "" {
		return args
	}
	return append([]string{cmd}, args...)
}

// ResumeCommand returns the command used to resume an existing session.
func ResumeCommand(cfg map[string]any) []string {
	if v, ok := os.LookupEnv("WT_AI_TOOL"); ok {
		base := splitCommand(v)
		if len(base) == 0 {
			return base
		}
		return append(base, "--resume")
	}
	base := EffectiveCommand(cfg)
	if len(base) == 0 {
		return base
	}
	if preset := config.PresetNameForCommand(base); preset != "" {
		if resume, ok := config.ResumePresets[preset]; ok {
			return resume
		}
	}
	return append(base, "--resume")
}

// MergeCommand builds the command used for --ai-merge conflict resolution,
// with the prompt appended at the end.
func MergeCommand(cfg map[string]any, prompt string) []string {
	if v, ok := os.LookupEnv("WT_AI_TOOL"); ok {
		base := splitCommand(v)
		if len(base) == 0 {
			return base
		}
		return append(base, prompt)
	}
	base := EffectiveCommand(cfg)
	if len(base) == 0 {
		return base
	}
	preset := config.PresetNameForCommand(base)
	if mp, ok := config.MergePresets[preset]; ok {
		if len(mp.BaseOverride) > 0 {
			base = mp.BaseOverride
		}
		base = append(base, mp.Flags...)
	}
	return append(base, prompt)
}

// IsClaudeTool reports whether the effective command is a Claude variant.
func IsClaudeTool(cfg map[string]any) bool {
	return config.IsClaudeCommand(EffectiveCommand(cfg))
}

// splitCommand splits a command string on whitespace.
func splitCommand(s string) []string {
	return strings.Fields(s)
}

func stringSlice(v any) []string {
	arr, ok := v.([]any)
	if !ok {
		return nil
	}
	out := make([]string, 0, len(arr))
	for _, item := range arr {
		if s, ok := item.(string); ok {
			out = append(out, s)
		}
	}
	return out
}
