package git

import (
	"fmt"
	"strings"
)

// IsValidBranchName reports whether name passes `git check-ref-format --branch`.
// Note: git's check-ref-format accepts "@" alone, which is not a usable
// branch name, so we reject it explicitly (as does the Python version).
func IsValidBranchName(name string) bool {
	if name == "@" {
		return false
	}
	res, err := Git("", false, "check-ref-format", "--branch", name)
	return err == nil && res.ExitCode == 0
}

// BranchNameError returns a human-readable explanation for why a branch
// name is invalid, or "" if it is valid.
func BranchNameError(name string) string {
	if IsValidBranchName(name) {
		return ""
	}
	switch {
	case name == "":
		return "branch name cannot be empty"
	case name == "@":
		return "branch name cannot be '@' alone"
	case strings.HasSuffix(name, ".lock"):
		return "branch name cannot end with '.lock'"
	case strings.HasPrefix(name, "/") || strings.HasSuffix(name, "/"):
		return "branch name cannot start or end with '/'"
	case strings.Contains(name, "//"):
		return "branch name cannot contain consecutive slashes '//'"
	case strings.Contains(name, ".."):
		return "branch name cannot contain consecutive dots '..'"
	case strings.Contains(name, "@{"):
		return "branch name cannot contain '@{'"
	case strings.ContainsAny(name, "~^:?*["):
		return "branch name cannot contain any of: ~ ^ : ? * ["
	case strings.ContainsAny(name, " \t"):
		return "branch name cannot contain spaces"
	case strings.HasPrefix(name, "-"):
		return "branch name cannot start with '-'"
	case strings.HasPrefix(name, ".") || strings.HasSuffix(name, "."):
		return "branch name cannot start or end with '.'"
	case strings.Contains(name, "\\"):
		return `branch name cannot contain '\'`
	default:
		return fmt.Sprintf("%q is not a valid branch name", name)
	}
}
