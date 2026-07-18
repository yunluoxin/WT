// Package git wraps all git subprocess invocations.
package git

import (
	"bytes"
	"os"
	"os/exec"
	"strings"

	wterrors "wt/internal/errors"
)

// Result holds the outcome of a captured git invocation.
type Result struct {
	Stdout   string
	Stderr   string
	ExitCode int
}

// Git runs git with args in dir, capturing stdout/stderr separately.
// If check is true, a non-zero exit returns a *errors.GitError.
func Git(dir string, check bool, args ...string) (Result, error) {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	res := Result{
		Stdout:   stdout.String(),
		Stderr:   stderr.String(),
		ExitCode: 0,
	}
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			res.ExitCode = exitErr.ExitCode()
		} else {
			return res, err
		}
	}
	if check && res.ExitCode != 0 {
		return res, &wterrors.GitError{
			Args:     args,
			Dir:      dir,
			ExitCode: res.ExitCode,
			Stderr:   strings.TrimSpace(res.Stderr),
		}
	}
	return res, nil
}

// Output runs git and returns trimmed stdout, failing on non-zero exit.
func Output(dir string, args ...string) (string, error) {
	res, err := Git(dir, true, args...)
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(res.Stdout), nil
}

// Run executes git with inherited stdio (for interactive commands like
// rebase -i or foreground AI tool launches).
func Run(dir string, args ...string) error {
	cmd := exec.Command("git", args...)
	if dir != "" {
		cmd.Dir = dir
	}
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// HasCommand reports whether name is found on PATH.
func HasCommand(name string) bool {
	_, err := exec.LookPath(name)
	return err == nil
}
