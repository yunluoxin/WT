// Package launch starts the AI coding assistant in worktrees via
// foreground, detached, or terminal-multiplexer launch methods.
package launch

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/aitool"
	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/session"
	"wt/internal/termenv"
)

// Options parameterizes AITool.
type Options struct {
	WorktreePath string
	Term         string // "" = default launch method
	Resume       bool   // use resume command
	Prompt       string // non-empty: merge command with prompt appended
}

// CommandString joins argv into a safely quoted shell command line.
func CommandString(argv []string) string {
	quoted := make([]string, len(argv))
	for i, a := range argv {
		quoted[i] = shellQuote(a)
	}
	return strings.Join(quoted, " ")
}

// shellQuote quotes a string for POSIX shells (shlex.quote equivalent).
func shellQuote(s string) string {
	if s == "" {
		return "''"
	}
	if strings.IndexFunc(s, func(r rune) bool {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
			return false
		case strings.ContainsRune("@%_+=:,./-~^", r):
			return false
		}
		return true
	}) < 0 {
		return s
	}
	return "'" + strings.ReplaceAll(s, "'", `'"'"'`) + "'"
}

// GenerateSessionName builds a tmux/zellij session name:
// <prefix>-<dirname>, truncated to MaxSessionNameLength.
func GenerateSessionName(cfg map[string]any, path string) string {
	prefix := config.GetString(cfg, "launch.session_prefix")
	if prefix == "" {
		prefix = "wt"
	}
	name := prefix + "-" + filepath.Base(path)
	if len(name) > config.MaxSessionNameLength {
		name = name[:config.MaxSessionNameLength]
	}
	return name
}

// AITool launches the configured AI tool in the worktree.
// Empty command (no-op preset) silently skips.
func AITool(opts Options) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	// Resolve launch method.
	spec := config.DefaultLaunchMethod(cfg)
	if opts.Term != "" {
		spec, err = config.ParseTermOption(opts.Term)
		if err != nil {
			return err
		}
	}

	// Resolve command.
	var argv []string
	autoResume := config.AutoResume(cfg)
	switch {
	case opts.Prompt != "":
		argv = aitool.MergeCommand(cfg, opts.Prompt)
	case opts.Resume && autoResume:
		argv = aitool.ResumeCommand(cfg)
	default:
		// Smart --continue for Claude tools with an existing native session.
		if autoResume && aitool.IsClaudeTool(cfg) && session.ClaudeNativeSessionExists(opts.WorktreePath) {
			argv = aitool.ResumeCommand(cfg)
			termenv.Info("%s", termenv.Dim("Found existing Claude session, using --continue"))
		} else {
			argv = aitool.EffectiveCommand(cfg)
		}
	}

	if len(argv) == 0 {
		return nil // no-op
	}
	toolName := argv[0]
	if !git.HasCommand(toolName) {
		termenv.Warn("%s not detected. Install it or update your config with 'wt config set ai-tool <tool>'.\n", toolName)
		return nil
	}

	cmd := CommandString(argv)
	// merge_stdin: feed the prompt via a pipe instead of argv.
	if opts.Prompt != "" && aitool.MergeUsesStdin(cfg) {
		cmd = "printf '%s' " + shellQuote(opts.Prompt) + " | " + cmd
	}
	return dispatch(spec, opts.WorktreePath, withEnvOverrides(cmd), toolName, cfg)
}

// passthroughEnv lists the WT_* override variables propagated into the
// launched command. Terminal-multiplexer launch methods (tmux server,
// iTerm windows, wezterm spawn) do not share wt's process environment, so
// the overrides are injected into the command line itself; this keeps the
// whole chain (e.g. `wt done --ai` re-launching the tool) consistent.
var passthroughEnv = []string{
	"WT_AI_TOOL",
	"WT_AI_TOOL_MERGE",
	"WT_AI_TOOL_RESUME",
	"WT_LAUNCH_METHOD",
	"WT_AUTO_RESUME",
	"WT_NON_INTERACTIVE",
}

// withEnvOverrides prefixes cmd with `env KEY=VAL ...` for every set
// passthrough variable. A no-op when none are set.
func withEnvOverrides(cmd string) string {
	var prefix []string
	for _, k := range passthroughEnv {
		if v, ok := os.LookupEnv(k); ok {
			prefix = append(prefix, k+"="+shellQuote(v))
		}
	}
	if len(prefix) == 0 {
		return cmd
	}
	return "env " + strings.Join(prefix, " ") + " " + cmd
}

func dispatch(spec config.TermSpec, path, cmd, toolName string, cfg map[string]any) error {
	switch spec.Method {
	case config.MethodForeground:
		termenv.Info("%s\n", termenv.Cyan(fmt.Sprintf("Starting %s (Ctrl+C to exit)...", toolName)))
		return runForeground(cmd, path)
	case config.MethodDetach:
		if err := runDetached(cmd, path); err != nil {
			return err
		}
		termenv.Success("%s detached (survives terminal close)\n", toolName)
		return nil
	case config.MethodItermWindow:
		return itermWindow(path, cmd, toolName)
	case config.MethodItermTab:
		return itermTab(path, cmd, toolName)
	case config.MethodItermPaneH:
		return itermPane(path, cmd, toolName, true)
	case config.MethodItermPaneV:
		return itermPane(path, cmd, toolName, false)
	case config.MethodTmux:
		return tmuxSession(cfg, path, cmd, toolName, spec.Session)
	case config.MethodTmuxWindow:
		return tmuxWindow(path, cmd, toolName)
	case config.MethodTmuxPaneH:
		return tmuxPane(path, cmd, toolName, true)
	case config.MethodTmuxPaneV:
		return tmuxPane(path, cmd, toolName, false)
	case config.MethodZellij:
		return zellijSession(cfg, path, cmd, toolName, spec.Session)
	case config.MethodZellijTab:
		return zellijTab(path, cmd, toolName)
	case config.MethodZellijPaneH:
		return zellijPane(path, cmd, toolName, true)
	case config.MethodZellijPaneV:
		return zellijPane(path, cmd, toolName, false)
	case config.MethodWeztermWindow:
		return weztermSpawn(cfg, path, cmd, toolName, "window")
	case config.MethodWeztermTab:
		return weztermSpawn(cfg, path, cmd, toolName, "tab")
	case config.MethodWeztermPaneH:
		return weztermSpawn(cfg, path, cmd, toolName, "pane-h")
	case config.MethodWeztermPaneV:
		return weztermSpawn(cfg, path, cmd, toolName, "pane-v")
	}
	return wterrors.New(wterrors.ErrConfig, "unsupported launch method %q", spec.Method)
}
