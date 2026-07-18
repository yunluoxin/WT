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
	return git.GetConfig(repo, git.KeyCwsharePrompted) == "true"
}

func markPrompted(repo string) {
	_ = git.SetConfig(repo, git.KeyCwsharePrompted, "true")
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

// CreateTemplate writes a .cwshare template with detected files commented out.
func CreateTemplate(repo string, suggested []string) error {
	var b strings.Builder
	b.WriteString(`# .cwshare - Files to copy to new worktrees
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

// PromptSetup asks (once per repo) whether to create a .cwshare file.
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
	detected := DetectCommonFiles(repo)
	fmt.Println()
	fmt.Println(termenv.Bold(termenv.Cyan("💡 .cwshare File Setup")))
	fmt.Println()
	fmt.Printf("Would you like to create a %s file?\n", termenv.Cyan(".cwshare"))
	fmt.Println("This lets you automatically copy files to new worktrees (like .env, configs).")
	fmt.Println()
	if len(detected) > 0 {
		fmt.Println(termenv.Bold("Detected files that you might want to share:"))
		for _, f := range detected {
			fmt.Printf("  %s %s\n", termenv.Dim("•"), f)
		}
		fmt.Println()
	}
	ok := termenv.Confirm("Create .cwshare file?", true)
	markPrompted(repo)
	if ok {
		if err := CreateTemplate(repo, detected); err == nil {
			termenv.Success("Created %s", filepath.Join(repo, FileName))
			fmt.Println()
			fmt.Println(termenv.Bold("Next steps:"))
			fmt.Println("  1. Review and edit .cwshare to uncomment files you want to share")
			fmt.Printf("  2. Add to git: %s\n", termenv.Cyan("git add .cwshare && git commit"))
			fmt.Printf("  3. Files will be copied when you run: %s\n\n", termenv.Cyan("wt new <branch>"))
		}
	} else {
		fmt.Println(termenv.Dim("\nYou can create .cwshare manually anytime."))
		fmt.Println()
	}
}
