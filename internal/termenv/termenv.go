// Package termenv provides terminal/environment detection and small
// ANSI output helpers (replacing the Python rich console).
package termenv

import (
	"bufio"
	"fmt"
	"os"
	"strings"

	"golang.org/x/term"
)

// ciEnvVars mirror the Python non-interactive detection list.
var ciEnvVars = []string{
	"CI", "GITHUB_ACTIONS", "GITLAB_CI", "JENKINS_HOME", "CIRCLECI",
	"TRAVIS", "BUILDKITE", "DRONE", "BITBUCKET_PIPELINE", "CODEBUILD_BUILD_ID",
}

// IsNonInteractive reports whether prompts must be suppressed:
// WT_NON_INTERACTIVE=1/true/yes, stdin not a TTY, or a CI environment.
func IsNonInteractive() bool {
	switch strings.ToLower(os.Getenv("WT_NON_INTERACTIVE")) {
	case "1", "true", "yes":
		return true
	}
	if !term.IsTerminal(int(os.Stdin.Fd())) {
		return true
	}
	for _, v := range ciEnvVars {
		if os.Getenv(v) != "" {
			return true
		}
	}
	return false
}

// IsTTY reports whether stdout is a terminal.
func IsTTY() bool {
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// Width returns the terminal width, defaulting to 80.
func Width() int {
	w, _, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || w <= 0 {
		return 80
	}
	return w
}

// useColor reports whether ANSI colors should be emitted.
func useColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return IsTTY()
}

func colorize(code, s string) string {
	if !useColor() {
		return s
	}
	return "\x1b[" + code + "m" + s + "\x1b[0m"
}

// Styled output helpers.
func Bold(s string) string   { return colorize("1", s) }
func Dim(s string) string    { return colorize("2", s) }
func Green(s string) string  { return colorize("32", s) }
func Red(s string) string    { return colorize("31", s) }
func Yellow(s string) string { return colorize("33", s) }
func Cyan(s string) string   { return colorize("36", s) }

// Info prints a neutral status line.
func Info(format string, args ...any) {
	fmt.Printf(format+"\n", args...)
}

// Success prints a green-check status line.
func Success(format string, args ...any) {
	fmt.Printf("%s %s\n", Green("✓"), fmt.Sprintf(format, args...))
}

// Warn prints a yellow warning line.
func Warn(format string, args ...any) {
	fmt.Printf("%s %s\n", Yellow("!"), fmt.Sprintf(format, args...))
}

// Error prints a red error line to stderr.
func Error(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "%s %s\n", Red("✗"), fmt.Sprintf(format, args...))
}

// Confirm asks a yes/no question. defaultYes picks the answer on empty input.
// Returns the default without prompting in non-interactive mode.
func Confirm(prompt string, defaultYes bool) bool {
	if IsNonInteractive() {
		return defaultYes
	}
	hint := "[y/N]"
	if defaultYes {
		hint = "[Y/n]"
	}
	fmt.Fprintf(os.Stderr, "%s %s ", prompt, hint)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil && line == "" {
		return defaultYes
	}
	switch strings.ToLower(strings.TrimSpace(line)) {
	case "":
		return defaultYes
	case "y", "yes":
		return true
	default:
		return false
	}
}

// ConfirmExact requires typing the exact string (e.g. "yes") to proceed.
func ConfirmExact(prompt, expected string) bool {
	if IsNonInteractive() {
		return false
	}
	fmt.Fprintf(os.Stderr, "%s (type %q to confirm): ", prompt, expected)
	reader := bufio.NewReader(os.Stdin)
	line, err := reader.ReadString('\n')
	if err != nil {
		return false
	}
	return strings.TrimSpace(line) == expected
}
