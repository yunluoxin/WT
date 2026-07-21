package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"wt/internal/ops"
)

func newCmd() *cobra.Command {
	var base, path, term string
	var noTerm bool
	cmd := &cobra.Command{
		Use:   "new <branch-name>",
		Short: "Create a new worktree with a feature branch",
		Long: `Create a new git worktree for a feature branch.

Default path: ../<repo>-<branch>. Automatically launches your
configured AI tool in the new worktree unless --no-term is given.`,
		Args: cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if len(args) > 0 {
				return nil, cobra.ShellCompDirectiveNoFileComp
			}
			return completeNewBranchNames(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if noTerm && term != "" {
				return errf("--no-term and --term are mutually exclusive")
			}
			_, err := ops.CreateWorktree(ops.CreateOptions{
				BranchName: args[0],
				BaseBranch: base,
				Path:       path,
				Term:       term,
				NoTerm:     noTerm,
			})
			return err
		},
	}
	cmd.Flags().StringVarP(&base, "base", "b", "", "Base branch (default: current branch)")
	cmd.Flags().StringVarP(&path, "path", "p", "", "Custom worktree path")
	cmd.Flags().BoolVar(&noTerm, "no-term", false, "Skip AI tool launch")
	cmd.Flags().StringVarP(&term, "term", "T", "", "Launch method (e.g. i-t, t:mysession, z-p-h)")
	_ = cmd.RegisterFlagCompletionFunc("term", termFlagCompletion)
	_ = cmd.RegisterFlagCompletionFunc("base", branchFlagCompletion)
	return cmd
}

func listCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "list",
		Short: "List all worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalMode {
				return ops.GlobalListWorktrees()
			}
			return ops.ListWorktrees()
		},
	}
}

func statusCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current worktree metadata and list all worktrees",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ShowStatus()
		},
	}
}

func deleteCmd() *cobra.Command {
	var keepBranch, deleteRemote, noForce bool
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "delete [target]",
		Short: "Remove a worktree by branch name or path",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := ""
			if len(args) > 0 {
				target = args[0]
			}
			return ops.DeleteWorktree(ops.DeleteOptions{
				Target:       target,
				KeepBranch:   keepBranch,
				DeleteRemote: deleteRemote,
				NoForce:      noForce,
				LookupMode:   tf.mode(),
				Global:       globalMode,
			})
		},
	}
	cmd.Flags().BoolVarP(&keepBranch, "keep-branch", "k", false, "Keep the branch, only remove worktree")
	cmd.Flags().BoolVarP(&deleteRemote, "delete-remote", "r", false, "Also delete the remote branch")
	cmd.Flags().BoolVar(&noForce, "no-force", false, "Don't use --force when removing worktree")
	tf.add(cmd)
	return cmd
}

func mergeCmd() *cobra.Command {
	var push, interactive, dryRun, any, ai bool
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "merge [target]",
		Short: "Rebase, fast-forward merge into base branch, and clean up",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.FinishWorktree(ops.FinishOptions{
				Target:      firstArg(args),
				Push:        push,
				Interactive: interactive,
				DryRun:      dryRun,
				Any:         any,
				AIMerge:     ai,
				LookupMode:  tf.mode(),
				Global:      globalMode,
			})
		},
	}
	cmd.Flags().BoolVar(&push, "push", false, "Push base branch to origin after merge")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Confirm each step")
	cmd.Flags().BoolVar(&dryRun, "dry-run", false, "Preview operations without executing")
	cmd.Flags().BoolVar(&any, "any", false, "Allow branches not created by 'wt new' (no wt- prefix)")
	cmd.Flags().BoolVar(&ai, "ai", false, "Launch AI tool to resolve rebase conflicts")
	tf.add(cmd)
	return cmd
}

func doneCmd() *cobra.Command {
	var ai, keep bool
	cmd := &cobra.Command{
		Use:   "done",
		Short: "Finish the current worktree: move uncommitted changes and commits onto the base branch, then clean up",
		Long:  "Run inside a feature worktree. Stashes uncommitted changes, rebases and fast-forward merges new commits into the base branch (like 'wt merge'), restores the stashed changes on the base branch, and removes the worktree and branch (unless --keep).",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.DoneWorktree(ops.DoneOptions{
				AI:   ai,
				Keep: keep,
			})
		},
	}
	cmd.Flags().BoolVar(&ai, "ai", false, "Launch AI tool to resolve conflicts (stash pop or rebase)")
	cmd.Flags().BoolVar(&keep, "keep", false, "Keep the worktree and branch after finishing")
	return cmd
}

func finishCmd() *cobra.Command {
	cmd := mergeCmd()
	cmd.Use = "finish [target]"
	cmd.Short = "Deprecated alias for merge"
	cmd.Hidden = true
	origRunE := cmd.RunE
	cmd.RunE = func(c *cobra.Command, args []string) error {
		c.PrintErrln("Warning: 'finish' is deprecated, use 'merge' instead")
		return origRunE(c, args)
	}
	return cmd
}

func prCmd() *cobra.Command {
	var noPush, draft, any bool
	var title, body string
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "pr [target]",
		Short: "Create a GitHub Pull Request (rebase, push, gh pr create)",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.CreatePR(ops.PROptions{
				Target:     firstArg(args),
				NoPush:     noPush,
				Title:      title,
				Body:       body,
				Draft:      draft,
				Any:        any,
				LookupMode: tf.mode(),
				Global:     globalMode,
			})
		},
	}
	cmd.Flags().BoolVar(&noPush, "no-push", false, "Don't push to remote before creating PR")
	cmd.Flags().StringVarP(&title, "title", "t", "", "PR title (skips AI generation)")
	cmd.Flags().StringVar(&body, "body", "", "PR body")
	cmd.Flags().BoolVar(&draft, "draft", false, "Create as draft PR")
	cmd.Flags().BoolVar(&any, "any", false, "Allow branches not created by 'wt new' (no wt- prefix)")
	tf.add(cmd)
	return cmd
}

func resumeCmd() *cobra.Command {
	var term string
	var tf targetFlags
	cmd := &cobra.Command{
		Use:   "resume [worktree]",
		Short: "Resume AI work in a worktree with context restoration",
		Args:  cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.ResumeWorktree(ops.ResumeOptions{
				Worktree:   firstArg(args),
				Term:       term,
				LookupMode: tf.mode(),
				Global:     globalMode,
			})
		},
	}
	cmd.Flags().StringVarP(&term, "term", "T", "", "Launch method (e.g. i-t, t:mysession, z-p-h)")
	_ = cmd.RegisterFlagCompletionFunc("term", termFlagCompletion)
	tf.add(cmd)
	return cmd
}

// cdCmd prints the path of a worktree so a shell wrapper can cd into it.
// Prints ONLY the path to stdout; all UI/errors go to stderr.
func cdCmd() *cobra.Command {
	var globalFlag, interactive bool
	cmd := &cobra.Command{
		Use:   "cd [branch|repo:branch]",
		Short: "Print the path of a worktree",
		Long: `Print the absolute path of a worktree.

A subprocess cannot change the parent shell's directory, so pair this
with your shell, e.g.: cd "$(wt cd <branch>)". Without a target, or
with --interactive, an interactive selector is shown.`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			if globalFlag {
				return completeGlobalTargets(toComplete), cobra.ShellCompDirectiveNoFileComp
			}
			return completeWorktreeBranches(toComplete), cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			target := firstArg(args)
			// repo:branch notation implies global mode.
			if !globalFlag {
				if r, _ := ops.ParseRepoBranchTarget(target); r != "" {
					globalFlag = true
				}
			}
			if err := ops.CDWorktree(target, globalFlag, interactive); err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return &exitError{code: 1}
			}
			return nil
		},
	}
	cmd.Flags().BoolVarP(&globalFlag, "global", "g", false, "Search across all registered repositories")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive arrow-key selector")
	return cmd
}

func shellCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "shell [worktree] [command...]",
		Short: "Open a shell or run a command in a worktree",
		Long: `Open an interactive shell in a worktree, or execute a command there.

If the first positional argument is not a valid worktree, all arguments
are treated as the command to execute in the current worktree.`,
		DisableFlagParsing: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			worktree := ""
			command := args
			if len(args) > 0 {
				if _, found, err := resolveIfWorktree(args[0]); err == nil && found {
					worktree = args[0]
					command = args[1:]
				}
			}
			code, err := ops.ShellWorktree(worktree, command)
			if err != nil {
				return err
			}
			if code != 0 {
				return &exitError{code: code}
			}
			return nil
		},
	}
	return cmd
}

func firstArg(args []string) string {
	if len(args) > 0 {
		return args[0]
	}
	return ""
}

// commitCmd is the AI-powered git commit. There is no --ai flag because the
// command itself is the AI wrapper — 'git commit -m "..."' already covers
// the literal case.
//
// The AI tool runs as an agent with git access and is responsible for
// staging and committing; wt just verifies the result. The --amend and
// --no-verify flags are passed through as part of the AI instructions.
func commitCmd() *cobra.Command {
	var noVerify, amend bool
	var term, model string
	cmd := &cobra.Command{
		Use:   "commit",
		Short: "Hand the working tree to the AI tool so it can stage and commit",
		Long: `Hand the current working tree to the configured AI tool so it can
run 'git add -A' and 'git commit' itself. wt then verifies a commit
was created. Run from inside any git repository — main or feature
worktree.

This command is the AI wrapper around 'git commit': there is no '--ai'
flag, the AI is on by default. For literal commits, use
'git commit -m "..."' directly. If no AI tool is configured, 'wt commit'
refuses — configure one with 'wt config use-preset <name>' or by setting
WT_AI_TOOL.

Because the AI tool runs git itself, --amend and --no-verify are
forwarded as natural-language instructions to the AI (e.g. "use
'git commit --amend -m ...'") rather than as raw git arguments.
Prefer tools that run in agent / exec mode (Claude Code, Codex) —
aider's --message mode and other pure-message tools will not commit.`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			return ops.CommitChanges(ops.CommitOptions{
				NoVerify: noVerify,
				Amend:    amend,
				Model:    model,
				Term:     term,
			})
		},
	}
	cmd.Flags().BoolVar(&noVerify, "no-verify", false, "Tell the AI to pass --no-verify to git commit (skip pre-commit hooks)")
	cmd.Flags().BoolVar(&amend, "amend", false, "Tell the AI to amend the previous commit instead of creating a new one")
	cmd.Flags().StringVar(&model, "model", "", "AI model id (forwarded as --model <id> to the AI tool)")
	cmd.Flags().StringVarP(&term, "term", "T", "", "Launch method (e.g. i-t, t:mysession, z-p-h); default from launch.method config")
	_ = cmd.RegisterFlagCompletionFunc("term", termFlagCompletion)
	return cmd
}
