package launch

import (
	"os"
	"os/exec"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

func requireZellij() error {
	if !git.HasCommand("zellij") {
		return wterrors.New(wterrors.ErrConfig, "zellij not installed. Install from https://zellij.dev/")
	}
	return nil
}

func zellijSession(cfg map[string]any, path, cmd, toolName, sessionName string) error {
	if err := requireZellij(); err != nil {
		return err
	}
	if sessionName == "" {
		sessionName = GenerateSessionName(cfg, path)
	}
	c := exec.Command("zellij", "-s", sessionName, "--", "bash", "-lc", cmd)
	c.Dir = path
	c.Stdin = os.Stdin
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	termenv.Success("%s ran in Zellij session '%s'\n", toolName, sessionName)
	return nil
}

func zellijTab(path, cmd, toolName string) error {
	if os.Getenv("ZELLIJ") == "" {
		return wterrors.New(wterrors.ErrConfig, "--term zellij-tab requires running inside a Zellij session")
	}
	if err := requireZellij(); err != nil {
		return err
	}
	c := exec.Command("zellij", "action", "new-tab", "--cwd", path, "--", "bash", "-lc", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	termenv.Success("%s running in new Zellij tab\n", toolName)
	return nil
}

func zellijPane(path, cmd, toolName string, horizontal bool) error {
	if os.Getenv("ZELLIJ") == "" {
		return wterrors.New(wterrors.ErrConfig, "--term zellij-pane-* requires running inside a Zellij session")
	}
	if err := requireZellij(); err != nil {
		return err
	}
	direction, paneType := "right", "horizontal"
	if !horizontal {
		direction, paneType = "down", "vertical"
	}
	c := exec.Command("zellij", "action", "new-pane", "-d", direction, "--cwd", path, "--", "bash", "-lc", cmd)
	c.Stdout = os.Stdout
	c.Stderr = os.Stderr
	if err := c.Run(); err != nil {
		return err
	}
	termenv.Success("%s running in Zellij %s pane\n", toolName, paneType)
	return nil
}
