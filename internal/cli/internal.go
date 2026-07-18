package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/git"
	"wt/internal/ops"
	"wt/internal/shellfn"
	"wt/internal/termenv"
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

// shellFunctionCmd prints the bundled shell integration script.
func shellFunctionCmd() *cobra.Command {
	return &cobra.Command{
		Use:    "_shell-function <shell>",
		Short:  "Print shell integration script (internal)",
		Hidden: true,
		Args:   cobra.ExactArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"bash", "zsh", "fish", "powershell", "pwsh"}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			script, err := shellfn.Script(args[0])
			if err != nil {
				return err
			}
			fmt.Print(script)
			return nil
		},
	}
}

// shellSetupCmd appends sourcing lines to the user's shell profile.
func shellSetupCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "shell-setup",
		Short: "Install shell integration (wt-cd) into your shell profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			shell := detectShell()
			switch shell {
			case "powershell", "pwsh":
				termenv.Info("\n%s\n", termenv.Bold(termenv.Cyan("PowerShell setup")))
				termenv.Info("Add the following line to your PowerShell profile:\n")
				termenv.Info("  %s\n", termenv.Cyan("wt _shell-function powershell | Out-String | Invoke-Expression"))
				termenv.Info("To find your profile path, run: %s\n", termenv.Cyan("$PROFILE"))
				return nil
			case "":
				return errf("could not detect your shell. Supported: bash, zsh, fish, powershell")
			}

			profile := shellProfile(shell)
			line := shellSourceLine(shell)

			existing, _ := os.ReadFile(profile)
			if strings.Contains(string(existing), "wt _shell-function") || strings.Contains(string(existing), "wt-cd") {
				termenv.Success("Shell integration already installed in %s\n", profile)
				return nil
			}

			f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := f.WriteString("\n# wt shell integration (wt-cd)\n" + line + "\n"); err != nil {
				return err
			}
			termenv.Success("Added wt shell integration to %s\n", profile)
			termenv.Info("Restart your shell or run: %s\n", termenv.Cyan("source "+profile))
			return nil
		},
	}
}

// detectShell identifies the current shell from $SHELL / environment.
func detectShell() string {
	if psModule := os.Getenv("PSModulePath"); psModule != "" && os.Getenv("SHELL") == "" {
		return "powershell"
	}
	shell := filepath.Base(os.Getenv("SHELL"))
	switch shell {
	case "bash", "zsh", "fish":
		return shell
	case "pwsh", "powershell":
		return "powershell"
	}
	return ""
}

func shellProfile(shell string) string {
	home, _ := os.UserHomeDir()
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc")
	case "zsh":
		return filepath.Join(home, ".zshrc")
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish")
	}
	return ""
}

func shellSourceLine(shell string) string {
	switch shell {
	case "bash", "zsh":
		return `source <(wt _shell-function ` + shell + `)`
	case "fish":
		return `wt _shell-function fish | source`
	}
	return ""
}
