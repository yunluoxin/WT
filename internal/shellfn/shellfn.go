// Package shellfn bundles shell integration scripts (wt-cd) via go:embed.
package shellfn

import (
	_ "embed"
	"fmt"
)

//go:embed assets/wt.bash
var bashScript string

//go:embed assets/wt.fish
var fishScript string

//go:embed assets/wt.ps1
var ps1Script string

// Script returns the shell integration script for the given shell.
// bash and zsh share the same script; powershell/pwsh use the PS1 script.
func Script(shell string) (string, error) {
	switch shell {
	case "bash", "zsh":
		return bashScript, nil
	case "fish":
		return fishScript, nil
	case "powershell", "pwsh":
		return ps1Script, nil
	}
	return "", fmt.Errorf("unsupported shell %q (supported: bash, zsh, fish, powershell, pwsh)", shell)
}
