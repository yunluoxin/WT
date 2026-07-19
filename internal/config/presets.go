package config

// Preset describes an AI tool preset: the full command as argv.
type Preset struct {
	Name        string
	Command     []string
	Description string
}

// AIToolPresets mirrors the Python AI_TOOL_PRESETS.
var AIToolPresets = []Preset{
	{Name: "no-op", Command: []string{}, Description: "Disable AI tool launching"},
	{Name: "claude", Command: []string{"claude"}, Description: "Claude Code (default)"},
	{Name: "claude-yolo", Command: []string{"claude", "--dangerously-skip-permissions"}, Description: "Claude Code, skip permissions"},
	{Name: "claude-remote", Command: []string{"claude", "/remote-control"}, Description: "Claude Code with remote control"},
	{Name: "claude-yolo-remote", Command: []string{"claude", "--dangerously-skip-permissions", "/remote-control"}, Description: "Claude Code remote + skip permissions"},
	{Name: "codex", Command: []string{"codex"}, Description: "OpenAI Codex"},
	{Name: "codex-yolo", Command: []string{"codex", "--dangerously-bypass-approvals-and-sandbox"}, Description: "Codex, bypass approvals and sandbox"},
}

// ResumePresets maps preset name -> resume command.
// Presets not listed here resume by appending "--resume".
var ResumePresets = map[string][]string{
	"claude":             {"claude", "--continue"},
	"claude-yolo":        {"claude", "--dangerously-skip-permissions", "--continue"},
	"claude-remote":      {"claude", "--continue", "/remote-control"},
	"claude-yolo-remote": {"claude", "--dangerously-skip-permissions", "--continue", "/remote-control"},
	"codex":              {"codex", "resume", "--last"},
	"codex-yolo":         {"codex", "resume", "--dangerously-bypass-approvals-and-sandbox", "--last"},
}

// MergePreset describes how to invoke the AI tool for --ai conflict resolution.
type MergePreset struct {
	BaseOverride []string // replaces the base command when non-empty
	Flags        []string // appended after the base command
}

// MergePresets maps preset name -> merge invocation; the prompt is
// appended at the end. Unlisted presets append the prompt directly.
var MergePresets = map[string]MergePreset{
	"claude":             {Flags: []string{"--print", "--tools=default"}},
	"claude-yolo":        {Flags: []string{"--print", "--tools=default"}},
	"claude-remote":      {BaseOverride: []string{"claude"}, Flags: []string{"--print", "--tools=default"}},
	"claude-yolo-remote": {BaseOverride: []string{"claude", "--dangerously-skip-permissions"}, Flags: []string{"--print", "--tools=default"}},
	"codex":              {Flags: []string{"--non-interactive"}},
	"codex-yolo":         {Flags: []string{"--non-interactive"}},
}

// FindPreset returns the preset by name.
func FindPreset(name string) (Preset, bool) {
	for _, p := range AIToolPresets {
		if p.Name == name {
			return p, true
		}
	}
	return Preset{}, false
}

// PresetNameForCommand finds the preset whose command matches, if any.
func PresetNameForCommand(cmd []string) string {
	for _, p := range AIToolPresets {
		if equalStrings(p.Command, cmd) {
			return p.Name
		}
	}
	return ""
}

// IsClaudeCommand reports whether the command is a Claude tool variant.
func IsClaudeCommand(cmd []string) bool {
	return len(cmd) > 0 && cmd[0] == "claude"
}

func equalStrings(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
