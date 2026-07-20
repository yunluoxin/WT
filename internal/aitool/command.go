// Package aitool resolves the effective AI tool commands (launch, resume,
// exec) from config, presets and environment overrides.
package aitool

import (
	"os"
	"strings"

	"wt/internal/config"
)

// promptPlaceholder marks where the prompt goes inside exec args.
const promptPlaceholder = "{prompt}"

// envOverride resolves a WT_* env override to argv. If the value matches a
// preset name (e.g. "cursor-agent-yolo"), it expands to that preset's
// variant command (picked by the variant function); otherwise the value is
// split into argv verbatim. Returns nil when the env var is not set; returns
// an empty slice when it is set but empty (AI disabled).
func envOverride(envKey string, variant func(config.Preset) []string) []string {
	v, ok := os.LookupEnv(envKey)
	if !ok {
		return nil
	}
	if p, found := config.FindPreset(strings.TrimSpace(v)); found {
		return append([]string{}, variant(p)...)
	}
	return splitCommand(v)
}

// launchVariant picks the launch command of a preset.
func launchVariant(p config.Preset) []string { return p.Command }

// resumeVariant picks the resume command of a preset, falling back to
// appending "--resume" to the launch command.
func resumeVariant(p config.Preset) []string {
	if len(p.Resume) > 0 {
		return p.Resume
	}
	return append(append([]string{}, p.Command...), "--resume")
}

// execVariant picks the headless exec command of a preset, falling back to the
// launch command (the prompt is appended later by applyPrompt).
func execVariant(p config.Preset) []string {
	if len(p.Exec) > 0 {
		return p.Exec
	}
	return p.Command
}

// envCommand resolves the WT_AI_TOOL env override to argv (launch variant).
func envCommand() []string {
	return envOverride("WT_AI_TOOL", launchVariant)
}

// EffectiveCommand returns the configured AI tool command as argv.
// Priority: WT_AI_TOOL env (empty = no-op) > config ai_tool.command+args.
func EffectiveCommand(cfg map[string]any) []string {
	if env := envCommand(); env != nil {
		return env
	}
	return configVariant(cfg, "command", "args")
}

// ResumeCommand returns the command used to resume an existing session.
// Priority: WT_AI_TOOL_RESUME > WT_AI_TOOL (preset-expanded) >
// config ai_tool.resume_command+resume_args > preset inference > base+"--resume".
func ResumeCommand(cfg map[string]any) []string {
	if env := envOverride("WT_AI_TOOL_RESUME", resumeVariant); env != nil {
		return env
	}
	if env := envCommand(); env != nil {
		if len(env) == 0 {
			return env
		}
		if preset := config.PresetNameForCommand(env); preset != "" {
			if p, ok := config.FindPreset(preset); ok && len(p.Resume) > 0 {
				return append([]string{}, p.Resume...)
			}
		}
		return append(env, "--resume")
	}
	if cmd := configVariant(cfg, "resume_command", "resume_args"); len(cmd) > 0 {
		return cmd
	}
	base := EffectiveCommand(cfg)
	if len(base) == 0 {
		return base
	}
	if preset := config.PresetNameForCommand(base); preset != "" {
		if p, ok := config.FindPreset(preset); ok && len(p.Resume) > 0 {
			return append([]string{}, p.Resume...)
		}
	}
	return append(base, "--resume")
}

// ExecCommand builds the command used for --ai conflict resolution, with
// the prompt placed at the {prompt} placeholder or appended at the end.
// Priority: WT_AI_TOOL_EXEC > WT_AI_TOOL (preset-expanded) >
// config ai_tool.exec_command+exec_args > preset inference > base+prompt.
func ExecCommand(cfg map[string]any, prompt string) []string {
	if env := envOverride("WT_AI_TOOL_EXEC", execVariant); env != nil {
		return applyPrompt(env, prompt)
	}
	if env := envCommand(); env != nil {
		if len(env) == 0 {
			return env
		}
		base := env
		if preset := config.PresetNameForCommand(env); preset != "" {
			if p, ok := config.FindPreset(preset); ok && len(p.Exec) > 0 {
				base = append([]string{}, p.Exec...)
			}
		}
		return applyPrompt(base, prompt)
	}
	if cmd := configVariant(cfg, "exec_command", "exec_args"); len(cmd) > 0 {
		return applyPrompt(cmd, prompt)
	}
	base := EffectiveCommand(cfg)
	if len(base) == 0 {
		return base
	}
	if preset := config.PresetNameForCommand(base); preset != "" {
		if p, ok := config.FindPreset(preset); ok && len(p.Exec) > 0 {
			base = append([]string{}, p.Exec...)
		}
	}
	return applyPrompt(base, prompt)
}

// ExecUsesStdin reports whether the exec prompt should be fed to the
// command via stdin instead of appended to argv (ai_tool.exec_stdin).
func ExecUsesStdin(cfg map[string]any) bool {
	return config.GetBool(cfg, "ai_tool.exec_stdin")
}

// applyPrompt inserts the prompt into argv: the first element containing
// the {prompt} placeholder is replaced wholesale; without a placeholder the
// prompt is appended as the final argument.
func applyPrompt(argv []string, prompt string) []string {
	for i, a := range argv {
		if strings.Contains(a, promptPlaceholder) {
			out := append([]string{}, argv...)
			out[i] = strings.ReplaceAll(a, promptPlaceholder, prompt)
			return out
		}
	}
	return append(argv, prompt)
}

// configVariant reads an ai_tool command+args pair (e.g. exec_command /
// exec_args). When the command is empty but args are set, the args extend
// the launch command instead.
func configVariant(cfg map[string]any, cmdKey, argsKey string) []string {
	cmd := config.GetString(cfg, "ai_tool."+cmdKey)
	args := stringSlice(config.Get(cfg, "ai_tool."+argsKey))
	if cmd != "" {
		return append([]string{cmd}, args...)
	}
	if cmdKey == "command" || len(args) == 0 {
		return args
	}
	// exec_args / resume_args without a command extend the launch command.
	base := configVariant(cfg, "command", "args")
	if len(base) == 0 {
		return nil
	}
	return append(base, args...)
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
