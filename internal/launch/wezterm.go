package launch

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

func requireWezterm() error {
	if !git.HasCommand("wezterm") {
		return wterrors.New(wterrors.ErrConfig, "wezterm not installed. Install from https://wezterm.org/")
	}
	return nil
}

// waitForShellReady polls the pane until non-whitespace content appears
// (shell or TUI has rendered). Best-effort: returns silently on timeout.
func waitForShellReady(paneID string, timeout time.Duration) {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		out, err := exec.CommandContext(ctx, "wezterm", "cli", "get-text", "--pane-id", paneID).Output()
		cancel()
		if err == nil && strings.TrimSpace(string(out)) != "" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
}

func weztermSendText(cfg map[string]any, paneID, cmd string) error {
	if paneID == "" {
		return wterrors.New(wterrors.ErrConfig, "failed to get pane ID from WezTerm spawn")
	}
	timeout := config.GetFloat(cfg, "launch.wezterm_ready_timeout")
	if timeout <= 0 {
		timeout = 5.0
	}
	waitForShellReady(paneID, time.Duration(timeout*float64(time.Second)))
	c := exec.Command("wezterm", "cli", "send-text", "--pane-id", paneID, "--no-paste")
	c.Stdin = strings.NewReader(cmd + "\n")
	return c.Run()
}

// weztermSpawn implements all four wezterm launch variants.
func weztermSpawn(cfg map[string]any, path, cmd, toolName, variant string) error {
	if err := requireWezterm(); err != nil {
		return err
	}
	var args []string
	var desc string
	switch variant {
	case "window":
		args = []string{"cli", "spawn", "--new-window", "--cwd", path}
		desc = "new WezTerm window"
	case "tab":
		args = []string{"cli", "spawn", "--cwd", path}
		desc = "new WezTerm tab"
	case "pane-h":
		args = []string{"cli", "split-pane", "--horizontal", "--cwd", path}
		desc = "WezTerm horizontal pane"
	case "pane-v":
		args = []string{"cli", "split-pane", "--bottom", "--cwd", path}
		desc = "WezTerm vertical pane"
	}
	out, err := exec.Command("wezterm", args...).Output()
	if err != nil {
		return wterrors.Wrap(wterrors.ErrConfig, err, "wezterm %s failed", args[1])
	}
	paneID := strings.TrimSpace(string(out))
	if err := weztermSendText(cfg, paneID, cmd); err != nil {
		return err
	}
	termenv.Success("%s running in %s\n", toolName, desc)
	return nil
}
