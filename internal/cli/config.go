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
			termenv.Info("  command: %s", aiDisplay)
			if preset := config.PresetNameForCommand(aiCmd); preset != "" {
				termenv.Info("  preset:  %s", preset)
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
			return nil
		},
	}
}

func configSetCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "set <key> <value>",
		Short: "Set a configuration value (dot-path keys)",
		Long: `Set a configuration value. Examples:

  wt config set ai-tool "claude --dangerously-skip-permissions"
  wt config set launch.method tmux
  wt config set launch.session_prefix wt
  wt config set git.default_base_branch main`,
		Args: cobra.ExactArgs(2),
		RunE: func(cmd *cobra.Command, args []string) error {
			key, value := args[0], args[1]
			cfg, err := config.Load()
			if err != nil {
				return err
			}
			if key == "ai-tool" {
				parts := strings.Fields(value)
				if len(parts) == 0 {
					return errf("ai-tool value cannot be empty (use 'wt config use-preset no-op' to disable)")
				}
				setMap(cfg, "ai_tool", "command", parts[0])
				args := make([]any, 0, len(parts)-1)
				for _, p := range parts[1:] {
					args = append(args, p)
				}
				setMap(cfg, "ai_tool", "args", args)
			} else {
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
			if len(preset.Command) == 0 {
				setMap(cfg, "ai_tool", "command", "")
				setMap(cfg, "ai_tool", "args", []any{})
			} else {
				setMap(cfg, "ai_tool", "command", preset.Command[0])
				args := make([]any, 0, len(preset.Command)-1)
				for _, p := range preset.Command[1:] {
					args = append(args, p)
				}
				setMap(cfg, "ai_tool", "args", args)
			}
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
