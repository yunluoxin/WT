package cli

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"wt/internal/termenv"
)

// initMarkerBegin/initMarkerEnd delimit the managed block in the profile.
// Presence of the begin marker with the current version means "installed";
// an older/absent version means "upgrade". This replaces the previous naive
// substring check, which matched comments and stale installs alike.
const (
	initMarkerBegin = "# >>> wt shell integration (v1) >>>"
	initMarkerEnd   = "# <<< wt shell integration <<<"
)

// initSnippet returns the shell integration snippet: the wt-cd helper
// function and cobra completion sourcing. Kept minimal on purpose — all
// logic lives in the wt binary; the shell only needs to cd and complete.
func initSnippet(shell string) (string, error) {
	var body string
	switch shell {
	case "bash":
		body = `wt-cd() { cd "$(wt cd "$@")"; }
source <(wt completion bash)
`
	case "zsh":
		body = `wt-cd() { cd "$(wt cd "$@")"; }
autoload -Uz compinit && compinit
source <(wt completion zsh)
`
	case "fish":
		body = `function wt-cd; cd (wt cd $argv); end
wt completion fish | source
`
	case "powershell", "pwsh":
		body = `function wt-cd { Set-Location (wt cd @args) }
wt completion powershell | Out-String | Invoke-Expression
`
	default:
		return "", errf("unsupported shell %q (supported: bash, zsh, fish, powershell)", shell)
	}
	return initMarkerBegin + "\n" + body + initMarkerEnd + "\n", nil
}

// initProfile returns the profile file that should source the snippet.
func initProfile(shell string) (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	switch shell {
	case "bash":
		return filepath.Join(home, ".bashrc"), nil
	case "zsh":
		return filepath.Join(home, ".zshrc"), nil
	case "fish":
		return filepath.Join(home, ".config", "fish", "config.fish"), nil
	case "powershell", "pwsh":
		// Not writable cross-platform from Go without shelling out;
		// instruct the user instead.
		return "", errf("add the snippet to your PowerShell profile manually (path: $PROFILE)")
	}
	return "", errf("unsupported shell %q", shell)
}

// initCmd installs (or prints) shell integration: the wt-cd helper and
// completion. This replaces the old multi-dialect embedded scripts with a
// one-line function per shell; all logic lives in `wt cd`.
func initCmd() *cobra.Command {
	var printOnly bool
	var shell string
	cmd := &cobra.Command{
		Use:   "init [shell]",
		Short: "Install shell integration (wt-cd function + completion)",
		Long: `Install shell integration into your profile: a minimal wt-cd
function (cd into a worktree) and tab completion for all wt commands.

By default appends the snippet to your shell profile. Use --print to
write it to stdout instead (e.g. for manual sourcing).`,
		Args: cobra.MaximumNArgs(1),
		ValidArgsFunction: func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
			return []string{"bash", "zsh", "fish", "powershell"}, cobra.ShellCompDirectiveNoFileComp
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			sh := shell
			if len(args) > 0 {
				sh = args[0]
			}
			if sh == "" {
				sh = detectShell()
				if sh == "" {
					return errf("could not detect your shell; pass it explicitly: wt init <bash|zsh|fish|powershell>")
				}
			}

			snippet, err := initSnippet(sh)
			if err != nil {
				return err
			}

			if printOnly {
				fmt.Print(snippet)
				return nil
			}

			profile, err := initProfile(sh)
			if err != nil {
				// PowerShell: fall back to printing with guidance.
				fmt.Print(snippet)
				return err
			}

			existing, _ := os.ReadFile(profile)
			content := string(existing)

			switch {
			case strings.Contains(content, initMarkerBegin):
				// Current-version block already installed.
				termenv.Success("wt shell integration already present in %s\n", profile)
				return nil
			case strings.Contains(content, "# >>> wt shell integration"):
				// Older managed block: replace it with the current snippet.
				updated := replaceManagedBlock(content, snippet)
				if err := os.WriteFile(profile, []byte(updated), 0o644); err != nil {
					return err
				}
				termenv.Success("Upgraded wt shell integration in %s\n", profile)
				termenv.Info("Restart your shell or run: %s\n", termenv.Cyan("source "+profile))
				return nil
			}

			if err := os.MkdirAll(filepath.Dir(profile), 0o755); err != nil {
				return err
			}
			f, err := os.OpenFile(profile, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0o644)
			if err != nil {
				return err
			}
			defer f.Close()
			if _, err := f.WriteString("\n" + snippet); err != nil {
				return err
			}
			termenv.Success("Added wt shell integration to %s\n", profile)
			termenv.Info("Restart your shell or run: %s\n", termenv.Cyan("source "+profile))
			return nil
		},
	}
	cmd.Flags().BoolVar(&printOnly, "print", false, "Print the snippet to stdout instead of writing the profile")
	cmd.Flags().StringVar(&shell, "shell", "", "Shell to generate for (default: auto-detect)")
	return cmd
}

// replaceManagedBlock swaps the lines between the managed-block markers
// (any version) for the new snippet, preserving everything outside.
func replaceManagedBlock(content, snippet string) string {
	lines := strings.Split(content, "\n")
	var out []string
	inBlock := false
	replaced := false
	for _, ln := range lines {
		switch {
		case strings.HasPrefix(ln, "# >>> wt shell integration"):
			inBlock = true
			if !replaced {
				out = append(out, strings.TrimRight(snippet, "\n"))
				replaced = true
			}
		case strings.HasPrefix(ln, "# <<< wt shell integration"):
			inBlock = false
		case !inBlock:
			out = append(out, ln)
		}
	}
	return strings.Join(out, "\n")
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
