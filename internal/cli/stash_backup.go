package cli

import (
	"github.com/spf13/cobra"

	"wt/internal/ops"
)

func stashCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "stash",
		Short: "Manage stashes across worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var includeUntracked bool
	save := &cobra.Command{
		Use:   "save [message]",
		Short: "Stash changes in the current worktree",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.StashSave(firstArg(args), includeUntracked)
		},
	}
	save.Flags().BoolVar(&includeUntracked, "include-untracked", false, "Include untracked files")

	list := &cobra.Command{
		Use:   "list",
		Short: "List stashes grouped by branch",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.StashList()
		},
	}

	var stashRef string
	apply := &cobra.Command{
		Use:   "apply <target-branch>",
		Short: "Apply a stash to another worktree",
		Args:  cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.StashApply(args[0], stashRef)
		},
	}
	apply.Flags().StringVarP(&stashRef, "stash", "s", "stash@{0}", "Stash reference to apply")

	root.AddCommand(save, list, apply)
	return root
}

func backupCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "backup",
		Short: "Backup and restore worktrees (git bundle)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}

	var output string
	var all bool
	create := &cobra.Command{
		Use:   "create [branch]",
		Short: "Create a backup of worktree(s)",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.BackupCreate(firstArg(args), output, all, globalMode)
		},
	}
	create.Flags().StringVarP(&output, "output", "o", "", "Custom output directory")
	create.Flags().BoolVar(&all, "all", false, "Backup all worktrees")

	list := &cobra.Command{
		Use:   "list [branch]",
		Short: "List available backups",
		Args:  cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.BackupList(firstArg(args))
		},
	}

	var backupID, path string
	restore := &cobra.Command{
		Use:   "restore <branch>",
		Short: "Restore a worktree from a backup",
		Args:  cobra.ExactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.BackupRestore(args[0], backupID, path)
		},
	}
	restore.Flags().StringVar(&backupID, "id", "", "Backup timestamp to restore (default: latest)")
	restore.Flags().StringVarP(&path, "path", "p", "", "Custom restore path")

	root.AddCommand(create, list, restore)
	return root
}
