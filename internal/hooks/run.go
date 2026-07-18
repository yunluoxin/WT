package hooks

import (
	"os"
	"os/exec"
	"runtime"
	"strings"

	wterrors "wt/internal/errors"
	"wt/internal/termenv"
)

// Context carries values injected into hook processes as WT_* env vars.
type Context struct {
	Branch       string
	BaseBranch   string
	WorktreePath string
	RepoPath     string
	Event        string
	Operation    string
	PRURL        string
}

// Env returns the environment for a hook process.
func (c Context) Env() []string {
	env := os.Environ()
	set := func(k, v string) {
		if v != "" {
			env = append(env, "WT_"+k+"="+v)
		}
	}
	set("BRANCH", c.Branch)
	set("BASE_BRANCH", c.BaseBranch)
	set("WORKTREE_PATH", c.WorktreePath)
	set("REPO_PATH", c.RepoPath)
	set("EVENT", c.Event)
	set("OPERATION", c.Operation)
	set("PR_URL", c.PRURL)
	return env
}

// RunHooks executes all enabled hooks for an event. Pre-hooks (event
// contains ".pre") abort on non-zero exit; post-hook failures only warn.
func RunHooks(repo, event string, ctx Context, cwd string) error {
	hookMap, err := List(repo, event)
	if err != nil {
		return err
	}
	ctx.Event = event
	isPre := strings.Contains(event, ".pre")
	for _, h := range hookMap[event] {
		if !h.Enabled {
			continue
		}
		var cmd *exec.Cmd
		if runtime.GOOS == "windows" {
			cmd = exec.Command("cmd", "/C", h.Command)
		} else {
			cmd = exec.Command("sh", "-c", h.Command)
		}
		cmd.Dir = cwd
		cmd.Env = ctx.Env()
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr
		if err := cmd.Run(); err != nil {
			if isPre {
				return wterrors.Wrap(wterrors.ErrHookFailed, err,
					"pre-hook %q failed for %s", h.ID, event)
			}
			termenv.Warn("post-hook %q failed for %s: %v", h.ID, event, err)
		}
	}
	return nil
}
