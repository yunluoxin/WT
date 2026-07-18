package launch

import (
	"bytes"
	"fmt"
	"os/exec"
	"runtime"
	"strings"

	wterrors "wt/internal/errors"
	"wt/internal/termenv"
)

// applescriptEscape escapes a string for embedding inside an AppleScript
// double-quoted string literal.
func applescriptEscape(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	return s
}

func runAppleScript(script string) error {
	cmd := exec.Command("osascript", "-")
	cmd.Stdin = bytes.NewBufferString(script)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return fmt.Errorf("osascript failed: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

func requireDarwin(method string) error {
	if runtime.GOOS != "darwin" {
		return wterrors.New(wterrors.ErrConfig, "--term %s only works on macOS", method)
	}
	return nil
}

func itermWindow(path, cmd, toolName string) error {
	if err := requireDarwin("iterm-window"); err != nil {
		return err
	}
	script := fmt.Sprintf(`tell application "iTerm"
  activate
  set newWindow to (create window with default profile)
  tell current session of newWindow
    write text "cd %s && %s"
  end tell
end tell
`, applescriptEscape(shellQuote(path)), applescriptEscape(cmd))
	if err := runAppleScript(script); err != nil {
		return err
	}
	termenv.Success("%s running in new iTerm window\n", toolName)
	return nil
}

func itermTab(path, cmd, toolName string) error {
	if err := requireDarwin("iterm-tab"); err != nil {
		return err
	}
	script := fmt.Sprintf(`tell application "iTerm"
  activate
  tell current window
    create tab with default profile
    tell current session
      write text "cd %s && %s"
    end tell
  end tell
end tell
`, applescriptEscape(shellQuote(path)), applescriptEscape(cmd))
	if err := runAppleScript(script); err != nil {
		return err
	}
	termenv.Success("%s running in new iTerm tab\n", toolName)
	return nil
}

func itermPane(path, cmd, toolName string, horizontal bool) error {
	if err := requireDarwin("iterm-pane-*"); err != nil {
		return err
	}
	direction, paneType := "horizontally", "horizontal"
	if !horizontal {
		direction, paneType = "vertically", "vertical"
	}
	script := fmt.Sprintf(`tell application "iTerm"
  activate
  tell current session of current window
    split %s with default profile
  end tell
  tell last session of current tab of current window
    write text "cd %s && %s"
  end tell
end tell
`, direction, applescriptEscape(shellQuote(path)), applescriptEscape(cmd))
	if err := runAppleScript(script); err != nil {
		return err
	}
	termenv.Success("%s running in iTerm %s pane\n", toolName, paneType)
	return nil
}
