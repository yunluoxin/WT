package config

import (
	"fmt"
	"strings"
)

// MaxSessionNameLength bounds tmux/zellij session names (Zellij's Unix
// socket path limit is the binding constraint).
const MaxSessionNameLength = 50

// LaunchMethod identifies how the AI tool is started.
type LaunchMethod string

const (
	MethodForeground    LaunchMethod = "foreground"
	MethodDetach        LaunchMethod = "detach"
	MethodItermWindow   LaunchMethod = "iterm-window"
	MethodItermTab      LaunchMethod = "iterm-tab"
	MethodItermPaneH    LaunchMethod = "iterm-pane-h"
	MethodItermPaneV    LaunchMethod = "iterm-pane-v"
	MethodTmux          LaunchMethod = "tmux"
	MethodTmuxWindow    LaunchMethod = "tmux-window"
	MethodTmuxPaneH     LaunchMethod = "tmux-pane-h"
	MethodTmuxPaneV     LaunchMethod = "tmux-pane-v"
	MethodZellij        LaunchMethod = "zellij"
	MethodZellijTab     LaunchMethod = "zellij-tab"
	MethodZellijPaneH   LaunchMethod = "zellij-pane-h"
	MethodZellijPaneV   LaunchMethod = "zellij-pane-v"
	MethodWeztermWindow LaunchMethod = "wezterm-window"
	MethodWeztermTab    LaunchMethod = "wezterm-tab"
	MethodWeztermPaneH  LaunchMethod = "wezterm-pane-h"
	MethodWeztermPaneV  LaunchMethod = "wezterm-pane-v"
)

// methodAliases maps shorthand and deprecated names to canonical methods.
var methodAliases = map[string]LaunchMethod{
	"fg": MethodForeground, "foreground": MethodForeground,
	"d": MethodDetach, "detach": MethodDetach,
	"bg": MethodDetach, "background": MethodDetach, // deprecated
	"i-w": MethodItermWindow, "i-t": MethodItermTab,
	"i-p-h": MethodItermPaneH, "i-p-v": MethodItermPaneV,
	"t": MethodTmux, "t-w": MethodTmuxWindow,
	"t-p-h": MethodTmuxPaneH, "t-p-v": MethodTmuxPaneV,
	"z": MethodZellij, "z-t": MethodZellijTab,
	"z-p-h": MethodZellijPaneH, "z-p-v": MethodZellijPaneV,
	"w-w": MethodWeztermWindow, "w-t": MethodWeztermTab,
	"w-p-h": MethodWeztermPaneH, "w-p-v": MethodWeztermPaneV,
}

// TermSpec is the parsed form of a --term option: a launch method plus an
// optional session name (syntax "t:name" / "z:name", tmux/zellij only).
type TermSpec struct {
	Method  LaunchMethod
	Session string
}

// AllMethods lists every canonical launch method (for help/completion).
func AllMethods() []LaunchMethod {
	return []LaunchMethod{
		MethodForeground, MethodDetach,
		MethodItermWindow, MethodItermTab, MethodItermPaneH, MethodItermPaneV,
		MethodTmux, MethodTmuxWindow, MethodTmuxPaneH, MethodTmuxPaneV,
		MethodZellij, MethodZellijTab, MethodZellijPaneH, MethodZellijPaneV,
		MethodWeztermWindow, MethodWeztermTab, MethodWeztermPaneH, MethodWeztermPaneV,
	}
}

// ParseTermOption parses a --term value: canonical name, alias, or
// method:session form.
func ParseTermOption(value string) (TermSpec, error) {
	methodPart, session, hasSession := strings.Cut(value, ":")
	if hasSession {
		if len(session) > MaxSessionNameLength {
			return TermSpec{}, fmt.Errorf("session name too long (max %d chars)", MaxSessionNameLength)
		}
	}
	m, err := resolveMethod(methodPart)
	if err != nil {
		return TermSpec{}, err
	}
	if hasSession && m != MethodTmux && m != MethodZellij {
		return TermSpec{}, fmt.Errorf("session names are only supported for tmux and zellij (got %q)", value)
	}
	return TermSpec{Method: m, Session: session}, nil
}

func resolveMethod(s string) (LaunchMethod, error) {
	if alias, ok := methodAliases[s]; ok {
		return alias, nil
	}
	for _, m := range AllMethods() {
		if string(m) == s {
			return m, nil
		}
	}
	return "", fmt.Errorf("unknown launch method %q", s)
}

// DefaultLaunchMethod resolves the configured/env default launch method.
// Priority: explicit term > WT_LAUNCH_METHOD > config launch.method > foreground.
func DefaultLaunchMethod(cfg map[string]any) TermSpec {
	if env := strings.TrimSpace(getenv("WT_LAUNCH_METHOD")); env != "" {
		if spec, err := ParseTermOption(env); err == nil {
			return spec
		}
	}
	if m := GetString(cfg, "launch.method"); m != "" {
		if spec, err := ParseTermOption(m); err == nil {
			return spec
		}
	}
	return TermSpec{Method: MethodForeground}
}

var getenv = func(key string) string { return strings.TrimSpace(envLookup(key)) }
