package share

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/git"
	"wt/internal/termenv"
)

// commonSharedFiles are suggested in the first-run template when detected.
var commonSharedFiles = []string{
	".env", ".env.local", ".env.development", ".env.test",
	"config/local.json", "config/local.yaml", "config/local.yml",
	".vscode/settings.json",
}

func isPrompted(repo string) bool {
	return git.GetConfig(repo, git.KeyWtsharePrompted) == "true"
}

func markPrompted(repo string) {
	_ = git.SetConfig(repo, git.KeyWtsharePrompted, "true")
}

// DetectCommonFiles returns common share-worthy files present in the repo.
func DetectCommonFiles(repo string) []string {
	var out []string
	for _, f := range commonSharedFiles {
		if _, err := os.Stat(filepath.Join(repo, f)); err == nil {
			out = append(out, f)
		}
	}
	return out
}

// CreateTemplate writes a .wtshare template with detected files commented out.
func CreateTemplate(repo string, suggested []string) error {
	var b strings.Builder
	b.WriteString(`# .wtshare - Files to copy to new worktrees
#
# Files listed here will be automatically copied when you run 'wt new'.
# Useful for environment files and local configs not tracked in git.
#
# Format:
#   - One file/directory path per line (relative to repo root)
#   - Lines starting with # are comments
#   - Empty lines are ignored
`)
	if len(suggested) > 0 {
		b.WriteString("#\n# Detected files in this repository (uncomment to enable):\n\n")
		for _, f := range suggested {
			b.WriteString("# " + f + "\n")
		}
	} else {
		b.WriteString("#\n# No common files detected. Add your own below:\n\n")
	}
	return os.WriteFile(filepath.Join(repo, FileName), []byte(b.String()), 0o644)
}

// PromptSetup asks (once per repo) whether to create a .wtshare file.
func PromptSetup() {
	repo, err := git.RepoRoot("")
	if err != nil {
		return
	}
	if termenv.IsNonInteractive() {
		return
	}
	if HasFile(repo) {
		if !isPrompted(repo) {
			markPrompted(repo)
		}
		return
	}
	if isPrompted(repo) {
		return
	}
	// All prompt output goes to stderr, never stdout: commands whose stdout
	// is captured by the shell (`source <(wt completion zsh)`,
	// `cd "$(wt cd ...)"`) still have a TTY on stdin, so a prompt here would
	// otherwise be parsed as shell code / paths.
	out := os.Stderr
	detected := DetectCommonFiles(repo)
	fmt.Fprintln(out)
	fmt.Fprintln(out, termenv.Bold(termenv.Cyan("💡 .wtshare File Setup")))
	fmt.Fprintln(out)
	fmt.Fprintf(out, "Would you like to create a %s file?\n", termenv.Cyan(".wtshare"))
	fmt.Fprintln(out, "This lets you automatically copy files to new worktrees (like .env, configs).")
	fmt.Fprintln(out)
	if len(detected) > 0 {
		fmt.Fprintln(out, termenv.Bold("Detected files that you might want to share:"))
		for _, f := range detected {
			fmt.Fprintf(out, "  %s %s\n", termenv.Dim("•"), f)
		}
		fmt.Fprintln(out)
	}
	ok := termenv.Confirm("Create .wtshare file?", true)
	markPrompted(repo)
	if ok {
		if err := CreateTemplate(repo, detected); err == nil {
			fmt.Fprintf(out, "Created %s\n", filepath.Join(repo, FileName))
			fmt.Fprintln(out)
			fmt.Fprintln(out, termenv.Bold("Next steps:"))
			fmt.Fprintln(out, "  1. Review and edit .wtshare to uncomment files you want to share")
			fmt.Fprintf(out, "  2. Add to git: %s\n", termenv.Cyan("git add .wtshare && git commit"))
			fmt.Fprintf(out, "  3. Files will be copied when you run: %s\n\n", termenv.Cyan("wt new <branch>"))
		}
	} else {
		fmt.Fprintln(out, termenv.Dim("\nYou can create .wtshare manually anytime."))
		fmt.Fprintln(out)
	}
}
