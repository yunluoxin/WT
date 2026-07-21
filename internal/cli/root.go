// Package cli wires all wt commands with cobra.
package cli

import (
	"errors"
	"fmt"
	"os"

	"github.com/spf13/cobra"

	wterrors "wt/internal/errors"
	"wt/internal/ops"
	"wt/internal/share"
)

var (
	version = "dev"
	commit  = "none"
)

// SetVersion injects build metadata (via ldflags).
func SetVersion(v, c string) { version, commit = v, c }

// globalFlags holds root-level persistent flags.
var globalMode bool

// targetFlags holds the shared --branch/--worktree disambiguation flags.
type targetFlags struct {
	branch   bool
	worktree bool
}

func (t *targetFlags) add(cmd *cobra.Command) {
	cmd.Flags().BoolVarP(&t.branch, "branch", "b", false, "Treat target as branch name")
	cmd.Flags().BoolVarP(&t.worktree, "worktree", "w", false, "Treat target as worktree directory name")
	cmd.MarkFlagsMutuallyExclusive("branch", "worktree")
}

func (t *targetFlags) mode() ops.LookupMode {
	if t.branch {
		return ops.LookupBranch
	}
	if t.worktree {
		return ops.LookupWorktree
	}
	return ops.LookupAuto
}

// NewRootCmd builds the root command.
func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "wt",
		Short:         "Git worktree manager with AI assistant integration",
		Long:          "wt creates isolated git worktrees for feature branches, launches your AI coding assistant in them, and cleanly merges changes back.",
		Version:       version,
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			// Skip side effects for internal commands and commands whose
			// output is captured (llm prints AI-oriented instructions).
			switch cmd.Name() {
			case "_path", "cd", "init", "completion", "llm":
				return
			}
			if cmd.Parent() != nil && cmd.Parent().Name() == "llm" {
				return
			}
			share.PromptSetup()
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			if globalMode {
				return ops.GlobalListWorktrees()
			}
			return cmd.Help()
		},
	}
	root.PersistentFlags().BoolVarP(&globalMode, "global", "g", false, "Operate across all registered repositories")
	root.SetVersionTemplate("wt version {{.Version}}\n")

	root.AddCommand(
		newCmd(), listCmd(), statusCmd(), deleteCmd(), mergeCmd(), finishCmd(),
		doneCmd(),
		prCmd(), resumeCmd(), shellCmd(), configCmd(),
		syncCmd(), cleanCmd(), changeBaseCmd(), doctorCmd(), diffCmd(),
		treeCmd(), statsCmd(), stashCmd(), exportCmd(), importCmd(),
		backupCmd(), hookCmd(), scanCmd(), pruneCmd(),
		pathCmd(), cdCmd(), initCmd(), llmCmd(), commitCmd(),
	)
	return root
}

// Execute runs the CLI.
func Execute() {
	root := NewRootCmd()
	if err := root.Execute(); err != nil {
		if errors.Is(err, wterrors.ErrAborted) {
			os.Exit(0)
		}
		// Commands that already printed their error (e.g. to keep stdout
		// clean for shell capture) signal it with exitError.
		var ee *exitError
		if errors.As(err, &ee) {
			os.Exit(ee.code)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
