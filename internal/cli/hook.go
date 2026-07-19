package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"wt/internal/git"
	"wt/internal/hooks"
	"wt/internal/termenv"
)

func repoForHooks() (string, error) {
	return git.RepoRoot("")
}

func hookEventCompletion(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return hooks.Events, cobra.ShellCompDirectiveNoFileComp
}

func hookCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "hook",
		Short: "Manage per-repository lifecycle hooks (.wtconfig.json)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var id, description string
	init := &cobra.Command{
		Use:   "init",
		Short: "Install a post-create hook template (auto-installs deps)",
		Long: "Write .wt-hooks/post-create.sh from the built-in template — it detects common project " +
			"types (JS, Python, Go, Rust, Swift/SPM, CocoaPods, Gradle, Flutter/Dart, Ruby, PHP, " +
			".NET, Maven) in a new worktree and installs their dependencies — and register it as an " +
			"enabled worktree.post_create hook in .wtconfig.json.\n\n" +
			"The script is live immediately (no rename needed); edit it freely. " +
			"Re-running is safe: an existing script is left untouched.",
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			scriptPath, hookID, err := hooks.Init(repo)
			if err != nil {
				return err
			}
			termenv.Success("Installed hook %s\n", hookID)
			termenv.Info("  script:  %s", scriptPath)
			termenv.Info("  event:   worktree.post_create [enabled]")
			termenv.Info("  Try it:  wt new <branch>")
			return nil
		},
	}

	add := &cobra.Command{
		Use:               "add <event> <command>",
		Short:             "Register a hook for an event",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			hookID, err := hooks.Add(repo, args[0], args[1], id, description)
			if err != nil {
				return err
			}
			termenv.Success("Added hook %s for %s\n", hookID, args[0])
			return nil
		},
	}
	add.Flags().StringVar(&id, "id", "", "Custom hook ID (default: generated from command)")
	add.Flags().StringVarP(&description, "description", "d", "", "Hook description")

	remove := &cobra.Command{
		Use:               "remove <event> <hook-id>",
		Short:             "Remove a hook",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			if err := hooks.Remove(repo, args[0], args[1]); err != nil {
				return err
			}
			termenv.Success("Removed hook %s from %s\n", args[1], args[0])
			return nil
		},
	}

	list := &cobra.Command{
		Use:               "list [event]",
		Short:             "List hooks",
		Args:              cobra.MaximumNArgs(1),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			event := firstArg(args)
			hookMap, err := hooks.List(repo, event)
			if err != nil {
				return err
			}
			termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("Hooks:")))
			any := false
			for _, e := range hooks.SortedEvents(hookMap) {
				for _, h := range hookMap[e] {
					any = true
					state := termenv.Green("enabled")
					if !h.Enabled {
						state = termenv.Red("disabled")
					}
					termenv.Info("  %s", termenv.Bold(e))
					termenv.Info("    id:      %s [%s]", h.ID, state)
					termenv.Info("    command: %s", h.Command)
					if h.Description != "" {
						termenv.Info("    desc:    %s", termenv.Dim(h.Description))
					}
				}
			}
			if !any {
				termenv.Info("  %s", termenv.Dim("(no hooks registered)"))
			}
			fmt.Println()
			return nil
		},
	}

	enable := &cobra.Command{
		Use:               "enable <event> <hook-id>",
		Short:             "Enable a hook",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			if err := hooks.SetEnabled(repo, args[0], args[1], true); err != nil {
				return err
			}
			termenv.Success("Enabled hook %s for %s\n", args[1], args[0])
			return nil
		},
	}

	disable := &cobra.Command{
		Use:               "disable <event> <hook-id>",
		Short:             "Disable a hook",
		Args:              cobra.ExactArgs(2),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			if err := hooks.SetEnabled(repo, args[0], args[1], false); err != nil {
				return err
			}
			termenv.Success("Disabled hook %s for %s\n", args[1], args[0])
			return nil
		},
	}

	var dryRun bool
	run := &cobra.Command{
		Use:               "run <event>",
		Short:             "Manually run hooks for an event",
		Args:              cobra.ExactArgs(1),
		ValidArgsFunction: hookEventCompletion,
		RunE: func(cmd *cobra.Command, args []string) error {
			repo, err := repoForHooks()
			if err != nil {
				return err
			}
			cwd, _ := os.Getwd()
			ctx := hooks.Context{
				WorktreePath: cwd, RepoPath: repo, Operation: "manual",
			}
			if dryRun {
				hookMap, err := hooks.List(repo, args[0])
				if err != nil {
					return err
				}
				termenv.Info("%s", termenv.Bold(termenv.Yellow("DRY RUN:")))
				for _, h := range hookMap[args[0]] {
					state := "would run"
					if !h.Enabled {
						state = "disabled (skipped)"
					}
					termenv.Info("  %s: %s [%s]", h.ID, h.Command, state)
				}
				return nil
			}
			return hooks.RunHooks(repo, args[0], ctx, cwd)
		},
	}
	run.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would run without executing")

	root.AddCommand(init, add, remove, list, enable, disable, run)
	return root
}
