package hooks

import (
	_ "embed"
	"os"
	"path/filepath"
	"runtime"
)

// InitScriptName is the hook script file written by Init.
const InitScriptName = "post-create.sh"

// InitScriptRel is the script path relative to the repo root.
const InitScriptRel = ".wt-hooks/" + InitScriptName

// initEvent is the lifecycle event the template registers for.
const initEvent = "worktree.post_create"

// InitCommand is the hook command registered in .wtconfig.json.
// It is invoked via sh so the script need not be executable.
const InitCommand = "sh " + InitScriptRel

// initDescription describes the installed hook in `wt hook list` output.
const initDescription = "auto-install deps for JS/Python/Go/Rust projects"

// postCreateTemplate is the hook script installed by `wt hook init`.
// It scans the new worktree for JS/Python/Go/Rust projects and installs
// their dependencies. Keep it dependency-light: POSIX sh only.
//
//go:embed templates/post-create.sh
var postCreateTemplate string

// Init installs the template post-create hook into the repo: it writes
// .wt-hooks/post-create.sh and registers it in .wtconfig.json, enabled.
// The script is written directly as the live hook (no rename needed).
// The hook is registered with a fixed ID so re-running never duplicates it;
// an existing script file is left untouched.
func Init(repo string) (scriptPath string, hookID string, err error) {
	scriptPath = filepath.Join(repo, InitScriptRel)

	if _, statErr := os.Stat(scriptPath); os.IsNotExist(statErr) {
		if err := os.MkdirAll(filepath.Dir(scriptPath), 0o755); err != nil {
			return "", "", err
		}
		perm := os.FileMode(0o644)
		if runtime.GOOS != "windows" {
			perm = 0o755
		}
		if err := os.WriteFile(scriptPath, []byte(postCreateTemplate), perm); err != nil {
			return "", "", err
		}
	} else if statErr != nil {
		return "", "", statErr
	}

	s, err := read(repo)
	if err != nil {
		return "", "", err
	}
	for _, h := range s.Hooks[initEvent] {
		if h.ID == InitScriptName {
			return scriptPath, h.ID, nil // already registered; nothing to do
		}
	}
	s.Hooks[initEvent] = append(s.Hooks[initEvent], Hook{
		ID:          InitScriptName,
		Command:     InitCommand,
		Enabled:     true,
		Description: initDescription,
	})
	if err := write(repo, s); err != nil {
		return "", "", err
	}
	return scriptPath, InitScriptName, nil
}
