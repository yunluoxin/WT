package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"wt/internal/git"
	"wt/internal/ops"
	"wt/internal/tui"
)

// pathCmd resolves worktree paths for shell integration.
// Prints ONLY the path to stdout; all UI/errors go to stderr.
func pathCmd() *cobra.Command {
	var globalFlag, listBranches, interactive bool
	cmd := &cobra.Command{
		Use:    "_path [branch]",
		Short:  "Print the path of a worktree (internal)",
		Hidden: true,
		Args:   cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			if listBranches {
				var branches []string
				if globalFlag {
					branches = ops.GlobalTargets("")
				} else {
					repo, err := git.MainRepoRoot("")
					if err != nil {
						return err
					}
					wts, err := git.ParseWorktrees(repo)
					if err != nil {
						return err
					}
					for _, wt := range wts {
						if wt.Branch != git.DetachedBranch {
							branches = append(branches, wt.Branch)
						}
					}
				}
				for _, b := range branches {
					fmt.Println(b)
				}
				return nil
			}

			target := firstArg(args)

			if interactive || target == "" {
				// Interactive selector.
				var items []tui.Item
				if globalFlag {
					for _, gb := range ops.GlobalTargets("") {
						t, err := ops.ResolveWorktreeTarget(gb, ops.LookupAuto, true)
						if err != nil {
							continue
						}
						items = append(items, tui.Item{Label: gb, Value: t.WorktreePath})
					}
				} else {
					repo, err := git.MainRepoRoot("")
					if err != nil {
						return err
					}
					wts, err := git.ParseWorktrees(repo)
					if err != nil {
						return err
					}
					for _, wt := range wts {
						label := wt.Branch
						if label == git.DetachedBranch {
							continue
						}
						items = append(items, tui.Item{Label: label, Value: wt.Path})
					}
				}
				if len(items) == 0 {
					fmt.Fprintln(os.Stderr, "Error: no worktrees found")
					return &exitError{code: 1}
				}
				if len(items) == 1 && !interactive {
					fmt.Println(items[0].Value)
					return nil
				}
				// Default selection: current directory's worktree.
				defaultIdx := 0
				if cwd, err := os.Getwd(); err == nil {
					for i, it := range items {
						if samePathStr(it.Value, cwd) {
							defaultIdx = i
							break
						}
					}
				}
				value, ok := tui.ArrowSelect(items, "Select worktree:", defaultIdx)
				if !ok {
					return &exitError{code: 1}
				}
				fmt.Println(value)
				return nil
			}

			t, err := ops.ResolveWorktreeTarget(target, ops.LookupAuto, globalFlag)
			if err != nil {
				fmt.Fprintf(os.Stderr, "Error: %v\n", err)
				return &exitError{code: 1}
			}
			fmt.Println(t.WorktreePath)
			return nil
		},
	}
	cmd.Flags().BoolVarP(&globalFlag, "global", "g", false, "Search across all registered repositories")
	cmd.Flags().BoolVar(&listBranches, "list-branches", false, "List all worktree branches")
	cmd.Flags().BoolVarP(&interactive, "interactive", "i", false, "Interactive arrow-key selector")
	return cmd
}

func samePathStr(a, b string) bool {
	ra, errA := filepath.EvalSymlinks(a)
	rb, errB := filepath.EvalSymlinks(b)
	if errA == nil && errB == nil {
		return ra == rb
	}
	aa, _ := filepath.Abs(a)
	bb, _ := filepath.Abs(b)
	return filepath.Clean(aa) == filepath.Clean(bb)
}
