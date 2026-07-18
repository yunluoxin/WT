package cli

import (
	"github.com/spf13/cobra"

	"wt/internal/ops"
)

func syncCmd() *cobra.Command {
	var all, fetchOnly, aiMerge bool
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "sync [target]",
		Short: "Synchronize worktree(s) with base branch changes",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.SyncWorktrees(ops.SyncOptions{
				Target:     firstArg(args),
				All:        all,
				FetchOnly:  fetchOnly,
				AIMerge:    aiMerge,
				LookupMode: tf.mode(),
				Global:     globalMode,
			})
		},
	}
	cmd.Flags().BoolVar(&all, "all", false, "Sync all worktrees (topological order)")
	cmd.Flags().BoolVar(&fetchOnly, "fetch-only", false, "Only fetch updates without rebasing")
	cmd.Flags().BoolVar(&aiMerge, "ai-merge", false, "Launch AI tool to resolve rebase conflicts")
	tf.add(cmd)
	return cmd
}

func cleanCmd() *cobra.Command {
	var merged, interactive, dryRun bool
	var olderThan int
	var olderThanSet bool
	cmd := &cobra.Command{
		Use:   "clean",
		Short: "Batch cleanup of worktrees by criteria",
		Args:  cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, args []string) error {
			olderThanSet = cmd.Flags().Changed("older-than")
			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			var older *int
			if olderThanSet {
				older = &olderThan
			}
			return ops.CleanWorktrees(ops.CleanOptions{
				Merged:      merged,
				OlderThan:   older,
				Interactive: interactive,
				DryRun:      dryRun,
			})
		},
	}
	cmd.Flags().BoolVar(&merged, "merged", false, "Delete worktrees for branches merged to base")
	cmd.Flags().IntVar(&olderThan, "older-than", 0, "Delete worktrees older than N days")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive selection")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Show what would be deleted")
	return cmd
}

func changeBaseCmd() *cobra.Command {
	var target string
	var interactive, dryRun bool
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "change-base <new-base>",
		Short: "Change the base branch for a worktree and rebase onto it",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeAllBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ChangeBase(ops.ChangeBaseOptions{
				NewBase:     args[0],
				Target:      target,
				Interactive: interactive,
				DryRun:      dryRun,
				LookupMode:  tf.mode(),
				Global:      globalMode,
			})
		},
	}
	cmd.Flags().StringVarP(&target, "target", "t", "", "Worktree to change (default: current directory)")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Use interactive rebase")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview changes without executing")
	tf.add(cmd)
	return cmd
}

func doctorCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "doctor",
		Short: "Health check for all worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.Doctor()
		},
	}
}

func diffCmd() *cobra.Command {
	var summary, files bool
	cmd := &cobra.Command{
		Use:   "diff <branch1> <branch2>",
		Short: "Compare two branches",
		Args:  cobra.ExactArgs(2),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeAllBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.DiffWorktrees(args[0], args[1], summary, files)
		},
	}
	cmd.Flags().BoolVarP(&summary, "summary", "s", false, "Show diff statistics only")
	cmd.Flags().BoolVarP(&files, "files", "f", false, "Show changed files only")
	cmd.MarkFlagsMutuallyExclusive("summary", "files")
	return cmd
}

func treeCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "tree",
		Short: "Display worktree hierarchy as a tree",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ShowTree()
		},
	}
}

func statsCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "stats",
		Short: "Display worktree usage analytics",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ShowStats()
		},
	}
}

func scanCmd() *cobra.Command {
	var dir string
	cmd := &cobra.Command{
		Use:   "scan",
		Short: "Discover repositories with worktrees and register them",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ScanRepos(dir)
		},
	}
	cmd.Flags().StringVarP(&dir, "dir", "d", "", "Directory to scan (default: home directory)")
	return cmd
}

func pruneCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "prune",
		Short: "Remove stale entries from the global registry",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.PruneRegistry()
		},
	}
}
