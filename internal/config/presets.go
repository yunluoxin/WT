package config

// Preset describes an AI tool preset: the launch command plus the resume
// and merge invocation variants, all as argv.
type Preset struct {
	Name        string
	Command     []string // launch (interactive)
	Resume      []string // full resume command; empty = append "--resume" to Command
	Merge       []string // full merge command (prompt appended); empty = Command + prompt
	MergeStdin  bool     // merge prompt is fed via stdin instead of argv
	Description string
}

// AIToolPresets lists the built-in AI tool presets. Each preset is a
// recommended ai_tool configuration; applying one writes all of its
// variants into the config.
var AIToolPresets = []Preset{
	{Name: "no-op", Command: []string{}, Description: "Disable AI tool launching"},
	{
		Name:        "claude",
		Command:     []string{"claude"},
		Resume:      []string{"claude", "--continue"},
		Merge:       []string{"claude", "--print", "--tools=default"},
		Description: "Claude Code (default)",
	},
	{
		Name:        "claude-yolo",
		Command:     []string{"claude", "--dangerously-skip-permissions"},
		Resume:      []string{"claude", "--dangerously-skip-permissions", "--continue"},
		Merge:       []string{"claude", "--print", "--tools=default"},
		Description: "Claude Code, skip permissions",
	},
	{
		Name:        "claude-remote",
		Command:     []string{"claude", "/remote-control"},
		Resume:      []string{"claude", "--continue", "/remote-control"},
		Merge:       []string{"claude", "--print", "--tools=default"},
		Description: "Claude Code with remote control",
	},
	{
		Name:        "claude-yolo-remote",
		Command:     []string{"claude", "--dangerously-skip-permissions", "/remote-control"},
		Resume:      []string{"claude", "--dangerously-skip-permissions", "--continue", "/remote-control"},
		Merge:       []string{"claude", "--dangerously-skip-permissions", "--print", "--tools=default"},
		Description: "Claude Code remote + skip permissions",
	},
	{
		Name:        "codex",
		Command:     []string{"codex"},
		Resume:      []string{"codex", "resume", "--last"},
		Merge:       []string{"codex", "exec"},
		Description: "OpenAI Codex",
	},
	{
		Name:        "codex-yolo",
		Command:     []string{"codex", "--dangerously-bypass-approvals-and-sandbox"},
		Resume:      []string{"codex", "resume", "--dangerously-bypass-approvals-and-sandbox", "--last"},
		Merge:       []string{"codex", "exec", "--dangerously-bypass-approvals-and-sandbox"},
		Description: "Codex, bypass approvals and sandbox",
	},
	{
		Name:    "cursor-agent",
		Command: []string{"cursor-agent"},
		Resume:  []string{"cursor-agent", "resume"},
		// --print is headless (no TUI, prints and exits). --trust skips the
		// workspace-trust prompt that only appears in headless mode and would
		// otherwise block the run; --force auto-allows the shell commands the
		// agent runs to resolve conflicts.
		Merge:       []string{"cursor-agent", "--print", "--trust", "--force"},
		Description: "Cursor Agent CLI",
	},
	{
		Name:    "cursor-agent-yolo",
		Command: []string{"cursor-agent", "--force"},
		Resume:  []string{"cursor-agent", "--force", "resume"},
		// The yolo launch command already carries --force; the merge
		// invocation restarts from the plain binary so it is not
		// "--force --print --trust --force".
		Merge:       []string{"cursor-agent", "--print", "--trust", "--force"},
		Description: "Cursor Agent, auto-allow commands",
	},
	{
		Name:        "aider",
		Command:     []string{"aider"},
		Resume:      []string{"aider", "--restore-chat-history"},
		Merge:       []string{"aider", "--message"},
		Description: "Aider",
	},
	{
		Name:        "gemini",
		Command:     []string{"gemini"},
		Resume:      []string{"gemini", "-r", "latest"},
		Merge:       []string{"gemini", "-p"},
		Description: "Google Gemini CLI",
	},
	{
		Name:        "opencode",
		Command:     []string{"opencode"},
		Resume:      []string{"opencode", "run", "--continue"},
		Merge:       []string{"opencode", "run"},
		Description: "OpenCode",
	},
	{
		Name:        "crush",
		Command:     []string{"crush"},
		Resume:      []string{"crush", "--continue"},
		Merge:       []string{"crush", "run"},
		Description: "Crush (Charm)",
	},
	{
		Name:        "kimi",
		Command:     []string{"kimi"},
		Resume:      []string{"kimi", "--continue"},
		Merge:       []string{"kimi", "--print", "-p"},
		Description: "Kimi CLI (Moonshot)",
	},
	{
		Name:        "qwen",
		Command:     []string{"qwen"},
		Resume:      []string{"qwen", "--continue"},
		Merge:       []string{"qwen", "-p"},
		Description: "Qwen Code",
	},
	{
		Name:        "copilot",
		Command:     []string{"copilot"},
		Resume:      []string{"copilot", "--continue"},
		Merge:       []string{"copilot", "-p"},
		Description: "GitHub Copilot CLI",
	},
	{
		Name:        "goose",
		Command:     []string{"goose"},
		Resume:      []string{"goose", "session", "--resume"},
		Merge:       []string{"goose", "run", "-t"},
		Description: "Goose (Block)",
	},
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

// PresetNameForCommand finds the preset whose launch command matches, if any.
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
