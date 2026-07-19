package share

import (
	"bytes"
	"io"
	"os"
	"strings"
	"testing"
)

// captureStdout swaps os.Stdout for a pipe, runs fn, and returns what fn
// wrote to stdout.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("pipe: %v", err)
	}
	old := os.Stdout
	os.Stdout = w
	defer func() { os.Stdout = old }()

	fn()

	if err := w.Close(); err != nil {
		t.Fatalf("close pipe writer: %v", err)
	}
	var buf bytes.Buffer
	if _, err := io.Copy(&buf, r); err != nil {
		t.Fatalf("read pipe: %v", err)
	}
	return buf.String()
}

// TestPromptSetupKeepsStdoutClean is a regression test: the setup prompt
// must write ONLY to stderr, never stdout. Commands that pipe stdout
// (`source <(wt completion zsh)`, `cd "$(wt cd ...)"`) used to have the
// prompt banner parsed as shell code / paths when stdin was still a TTY
// (process substitution doesn't detach stdin).
func TestPromptSetupKeepsStdoutClean(t *testing.T) {
	// Simulate `source <(wt completion zsh)`: stdout is a pipe (not a TTY),
	// stdin untouched. We can't control the TTY check in tests, so set the
	// env override... but that would suppress the prompt entirely. Instead
	// just verify: whatever happens, stdout stays empty.
	out := captureStdout(t, PromptSetup)
	if strings.TrimSpace(out) != "" {
		t.Fatalf("PromptSetup wrote to stdout (breaks shell capture):\n%q", out)
	}
}
