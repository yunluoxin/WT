package launch

import (
	"os"
	"os/exec"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/termenv"
)

func requireTmux() error {
	if !git.HasCommand("tmux") {
		return wterrors.New(wterrors.ErrConfig, "tmux not installed. Install from https://tmux.github.io/")
	}
	return nil
}

func tmuxRun(args ...string) error {
	cmd := exec.Command("tmux", args...)
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

func tmuxSession(cfg map[string]any, path, cmd, toolName, sessionName string) error {
	if err := requireTmux(); err != nil {
		return err
	}
	if sessionName == "" {
		sessionName = GenerateSessionName(cfg, path)
	}
	if err := tmuxRun("new-session", "-d", "-s", sessionName, "-c", path); err != nil {
		return err
	}
	if err := tmuxRun("send-keys", "-t", sessionName, cmd, "Enter"); err != nil {
		return err
	}
	attach := exec.Command("tmux", "attach-session", "-t", sessionName)
	attach.Stdin = os.Stdin
	attach.Stdout = os.Stdout
	attach.Stderr = os.Stderr
	if err := attach.Run(); err != nil {
		return err
	}
	termenv.Success("%s ran in tmux session '%s'\n", toolName, sessionName)
	return nil
}

func tmuxWindow(path, cmd, toolName string) error {
	if os.Getenv("TMUX") == "" {
		return wterrors.New(wterrors.ErrConfig, "--term tmux-window requires running inside a tmux session")
	}
	if err := requireTmux(); err != nil {
		return err
	}
	if err := tmuxRun("new-window", "-c", path, "bash", "-lc", cmd); err != nil {
		return err
	}
	termenv.Success("%s running in new tmux window\n", toolName)
	return nil
}

func tmuxPane(path, cmd, toolName string, horizontal bool) error {
	if os.Getenv("TMUX") == "" {
		return wterrors.New(wterrors.ErrConfig, "--term tmux-pane-* requires running inside a tmux session")
	}
	if err := requireTmux(); err != nil {
		return err
	}
	flag := "-h"
	paneType := "horizontal"
	if !horizontal {
		flag = "-v"
		paneType = "vertical"
	}
	if err := tmuxRun("split-window", flag, "-c", path, "bash", "-lc", cmd); err != nil {
		return err
	}
	termenv.Success("%s running in tmux %s pane\n", toolName, paneType)
	return nil
}
