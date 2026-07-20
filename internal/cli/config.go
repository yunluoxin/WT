package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/aitool"
	"wt/internal/config"
	"wt/internal/termenv"
)

func configCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "config",
		Short: "Manage wt configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.AddCommand(configShowCmd(), configSetCmd(), configUsePresetCmd(), configListPresetsCmd(), configResetCmd())
	return root
}

func configShowCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "show",
		Short: "Display current configuration",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("wt configuration")))
			termenv.Info("  Config file: %s\n", termenv.Dim(config.Path()))

			aiCmd := aitool.EffectiveCommand(cfg)
			aiDisplay := "(disabled)"
			if len(aiCmd) > 0 {
				aiDisplay = strings.Join(aiCmd, " ")
			}
			termenv.Info("%s", termenv.Bold("AI tool:"))
			termenv.Info("  launch: %s", aiDisplay)
			if len(aiCmd) > 0 {
				resumeCmd := aitool.ResumeCommand(cfg)
				if len(resumeCmd) > 0 {
					termenv.Info("  resume: %s", strings.Join(resumeCmd, " "))
				}
				mergeCmd := aitool.MergeCommand(cfg, "<prompt>")
				if len(mergeCmd) > 0 {
					termenv.Info("  merge:  %s", strings.Join(mergeCmd, " "))
				}
			}
			if preset := config.PresetNameForCommand(aiCmd); preset != "" {
				termenv.Info("  preset: %s", preset)
			}
			fmt.Println()

			termenv.Info("%s", termenv.Bold("Launch:"))
			method := config.GetString(cfg, "launch.method")
			if method == "" {
				method = "(foreground)"
			}
			termenv.Info("  method:         %s", method)
			termenv.Info("  session_prefix: %s", config.GetString(cfg, "launch.session_prefix"))
			termenv.Info("  wezterm_ready_timeout: %v", config.Get(cfg, "launch.wezterm_ready_timeout"))
			fmt.Println()

			termenv.Info("%s", termenv.Bold("Git:"))
			termenv.Info("  default_base_branch: %s", config.GetString(cfg, "git.default_base_branch"))
			fmt.Println()

			termenv.Info("%s", termenv.Bold("Session:"))
			termenv.Info("  auto_resume: %v", config.AutoResume(cfg))
			fmt.Println()
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (dot-path keys)",
		Long: `Set a configuration value. Examples:

  wt config set ai-tool.name "claude --dangerously-skip-permissions"
  wt config set ai-tool.merge "codex exec {prompt}"
  wt config set ai-tool.resume "codex resume --last"
  wt config set launch.method tmux
  wt config set launch.session_prefix wt
  wt config set git.default_base_branch main

The ai-tool.* keys take a full command line split on whitespace:
  ai-tool.name    launch command (alias: ai-tool)
  ai-tool.merge   merge command for --ai conflict resolution; use {prompt}
                  where the prompt goes (appended at the end if omitted)
  ai-tool.resume  resume command for continuing a previous session
Empty values for ai-tool.merge/ai-tool.resume clear the override and
restore preset inference.`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			switch key {
			case "ai-tool", "ai-tool.name":
				if err := setAIToolVariant(cfg, value, "command", "args", true); err != nil {
					return err
				}
			case "ai-tool.merge":
				if err := setAIToolVariant(cfg, value, "merge_command", "merge_args", false); err != nil {
					return err
				}
			case "ai-tool.resume":
				if err := setAIToolVariant(cfg, value, "resume_command", "resume_args", false); err != nil {
					return err
				}
			default:
				config.Set(cfg, key, value)
			}
			if err := config.Save(cfg); err != nil {
				return err
			}
			termenv.Success("Set %s = %s\n", key, value)
			return nil
		},
	}
}

func setMap(cfg map[string]any, section, key string, value any) {
	m, ok := cfg[section].(map[string]any)
	if !ok {
		m = map[string]any{}
		cfg[section] = m
	}
	m[key] = value
}

// setAIToolPair stores an argv (from a preset) as an ai_tool command+args
// pair; an empty argv clears the pair.
func setAIToolPair(cfg map[string]any, cmdKey, argsKey string, argv []string) {
	if len(argv) == 0 {
		setMap(cfg, "ai_tool", cmdKey, "")
		setMap(cfg, "ai_tool", argsKey, []any{})
		return
	}
	setMap(cfg, "ai_tool", cmdKey, argv[0])
	args := make([]any, 0, len(argv)-1)
	for _, a := range argv[1:] {
		args = append(args, a)
	}
	setMap(cfg, "ai_tool", argsKey, args)
}

// setAIToolVariant splits value on whitespace and stores it as an
// ai_tool command+args pair. With rejectEmpty (the launch command), an
// empty value is an error; otherwise it clears the pair so preset
// inference takes over again.
func setAIToolVariant(cfg map[string]any, value, cmdKey, argsKey string, rejectEmpty bool) error {
	parts := strings.Fields(value)
	if len(parts) == 0 {
		if rejectEmpty {
			return errf("%s value cannot be empty (use 'wt config use-preset no-op' to disable)", cmdKey)
		}
		setMap(cfg, "ai_tool", cmdKey, "")
		setMap(cfg, "ai_tool", argsKey, []any{})
		return nil
	}
	setMap(cfg, "ai_tool", cmdKey, parts[0])
	args := make([]any, 0, len(parts)-1)
	for _, p := range parts[1:] {
		args = append(args, p)
	}
	setMap(cfg, "ai_tool", argsKey, args)
	return nil
}

func configUsePresetCmd() *cobra.Command {
	return &cobra.Command{
		Use:               "use-preset <name>",
		Short:             "Apply an AI tool preset",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: presetCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			preset, ok := config.FindPreset(name)
			if !ok {
				return errf("unknown preset %q. Run 'wt config list-presets' to see available presets", name)
			}
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			setAIToolPair(cfg, "command", "args", preset.Command)
			setAIToolPair(cfg, "resume_command", "resume_args", preset.Resume)
			setAIToolPair(cfg, "merge_command", "merge_args", preset.Merge)
			if err := config.Save(cfg); err != nil {
				return err
			}
			if len(preset.Command) == 0 {
				termenv.Success("Applied preset %q (AI launching disabled)\n", name)
			} else {
				termenv.Success("Applied preset %q: %s\n", name, strings.Join(preset.Command, " "))
			}
			return nil
		},
	}
}

func configListPresetsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list-presets",
		Short: "List available AI tool presets",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Available AI tool presets:")))
			for _, p := range config.AIToolPresets {
				cmdStr := "(none)"
				if len(p.Command) > 0 {
					cmdStr = strings.Join(p.Command, " ")
				}
				termenv.Info("  %-20s %s", termenv.Green(p.Name), cmdStr)
				termenv.Info("  %-20s %s", "", termenv.Dim(p.Description))
				if len(p.Resume) > 0 {
					termenv.Info("  %-20s %s", "", termenv.Dim("resume: "+strings.Join(p.Resume, " ")))
				}
				if len(p.Merge) > 0 {
					termenv.Info("  %-20s %s", "", termenv.Dim("merge:  "+strings.Join(p.Merge, " ")+" <prompt>"))
				}
			}
			fmt.Println()
			return nil
		},
	}
}

func configResetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "reset",
		Short: "Reset configuration to defaults",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fresh := map[string]any{}
			for k, v := range config.DefaultConfig {
				fresh[k] = v
			}
			if err := config.Save(fresh); err != nil {
				return err
			}
			termenv.Success("Configuration reset to defaults\n")
			return nil
		},
	}
}
