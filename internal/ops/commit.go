// Package ops: AI-assisted git commit. Unlike the existing --ai flag on
// merge/done/sync (which only kicks in when AI is needed to resolve a
// conflict), `wt commit` is the AI wrapper around git commit — there is
// no `--ai` flag because that would defeat the point.
//
// The AI tool runs as a full agent in the repo and is responsible for
// running `git add -A`, composing the message, and creating the commit.
// wt does NOT parse the AI's output for the message — the prompt is
// short and explicit, and the AI is given git tools (Claude Code,
// Codex, etc., all enable these by default in their exec/--print modes).
// After the AI exits, wt verifies a commit was actually created (HEAD
// advanced, or for --amend the message changed without HEAD moving).
// This sidesteps the entire class of "AI echoed the prompt back as the
// message" failures — there is no stdout extraction.
package ops

import (
	"fmt"
	"os"
	"strings"

	"wt/internal/aitool"
	"wt/internal/config"
	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/launch"
	"wt/internal/termenv"
)

// CommitOptions parameterizes CommitChanges.
type CommitOptions struct {
	NoVerify bool   // pass --no-verify to the AI as part of the instructions
	Amend    bool   // pass --amend to the AI as part of the instructions
	Model    string // optional AI model id (forwarded to the AI tool as --model <id>)
	Term     string // optional launch method (e.g. "foreground", "tmux"); "" = config default
}

// CommitChanges hands the current working tree to the configured AI
// tool and lets it run `git add -A` and `git commit` itself. wt then
// verifies a commit was created. Run from inside any git repository.
//
// A clean working tree short-circuits to a noop before the AI check
// (so `wt commit` on an idle repo never complains about missing AI
// config). When the AI tool is not configured, the command refuses —
// configure one with `wt config use-preset <name>` or by setting
// WT_AI_TOOL, or use `git commit` directly.
func CommitChanges(opts CommitOptions) error {
	cfg, _ := config.Load()

	cwd, err := repoCwd()
	if err != nil {
		return err
	}

	statusOut, err := git.Output(cwd, "status", "--porcelain")
	if err != nil {
		return err
	}
	if strings.TrimSpace(statusOut) == "" {
		termenv.Info("%s", termenv.Dim("Nothing to commit (working tree clean)"))
		return nil
	}

	if baseCmd := aitool.EffectiveCommand(cfg); len(baseCmd) == 0 {
		return wterrors.New(wterrors.ErrAIUnavailable,
			"no AI tool configured; run 'wt config use-preset <name>' or set WT_AI_TOOL — or use 'git commit -m \"...\"' directly")
	}

	branch := ""
	if b, err := git.CurrentBranch(cwd); err == nil {
		branch = b
	}

	headBefore, msgBefore, err := readHead(cwd)
	if err != nil {
		return err
	}

	countBefore, err := git.Output(cwd, "rev-list", "--count", "HEAD")
	if err != nil {
		return err
	}

	prompt := buildCommitInstruction(branch, opts)

	termenv.Info("%s", termenv.Yellow("Handing commit to AI agent..."))
	// Default to foreground so the HEAD check below runs after the AI
	// commits. Detached/tmux launches return immediately and would race
	// against the verification.
	term := opts.Term
	if term == "" {
		term = "foreground"
	}
	if err := launch.AITool(launch.Options{
		WorktreePath: cwd,
		Prompt:       prompt,
		Model:        opts.Model,
		Term:         term,
	}); err != nil {
		return wterrors.Wrap(wterrors.ErrAIUnavailable, err,
			"AI launch failed; use 'git commit -m \"...\"' to commit manually")
	}

	headAfter, msgAfter, err := readHead(cwd)
	if err != nil {
		return err
	}
	countAfter, err := git.Output(cwd, "rev-list", "--count", "HEAD")
	if err != nil {
		return err
	}

	return verifyCommit(opts, cwd, headBefore, msgBefore, countBefore, headAfter, msgAfter, countAfter)
}

// repoCwd returns an absolute path inside a git repository (and rejects
// detached HEAD). Reused for both "not a repo" and "detached HEAD" early
// exits, mirroring done.go.
func repoCwd() (string, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return "", err
	}
	if _, err := git.RepoRoot(cwd); err != nil {
		return "", err // wraps ErrNotARepo
	}
	if _, err := git.CurrentBranch(cwd); err != nil {
		return "", err // wraps ErrDetachedHEAD (or other branch errors)
	}
	return cwd, nil
}

// readHead captures HEAD's sha and commit-message subject; used to
// verify the AI actually created a commit.
func readHead(cwd string) (sha, msg string, err error) {
	sha, err = git.Output(cwd, "rev-parse", "HEAD")
	if err != nil {
		return "", "", err
	}
	msg, err = git.Output(cwd, "log", "-1", "--pretty=%s")
	if err != nil {
		return "", "", err
	}
	return sha, msg, nil
}

// verifyCommit checks that the AI produced a commit consistent with
// opts.Amend. For a normal run the commit count must advance by one;
// for --amend the count must stay the same (--amend rewrites the
// previous commit and produces a new SHA but no new commit object).
func verifyCommit(opts CommitOptions, cwd, headBefore, msgBefore, countBefore, headAfter, msgAfter, countAfter string) error {
	if opts.Amend {
		if headAfter == "" {
			return wterrors.New(wterrors.ErrMergeFailed,
				"AI did not modify the previous commit (no HEAD found); use 'git commit --amend' manually")
		}
		if countBefore == countAfter && msgBefore == msgAfter && headBefore == headAfter {
			return wterrors.New(wterrors.ErrMergeFailed,
				"AI finished but the previous commit was not amended; if your AI tool cannot run git, use 'wt config use-preset' to switch to one that can, or 'git commit --amend' manually")
		}
		if countBefore != countAfter {
			return wterrors.New(wterrors.ErrMergeFailed,
				"AI finished but created a new commit instead of amending; wt commit --amend must use 'git commit --amend'")
		}
		termenv.Success("Amended commit %s", termenv.Cyan(headAfter[:min(len(headAfter), 7)]))
	} else {
		if headAfter == "" || headBefore == headAfter {
			return wterrors.New(wterrors.ErrMergeFailed,
				"AI finished but no new commit was created; if your AI tool cannot run git, use 'wt config use-preset' to switch to one that can, or 'git add -A && git commit -m \"...\"' manually")
		}
		if countAfter == countBefore {
			return wterrors.New(wterrors.ErrMergeFailed,
				"AI finished but commit count did not advance")
		}
		termenv.Success("Committed on %s", termenv.Cyan(currentBranchOr(cwd, "")))
	}
	termenv.Info("%s %s\n", termenv.Dim("message:"), msgAfter)
	return nil
}

// currentBranchOr is best-effort (no error path); used by verifyCommit
// for the success line.
func currentBranchOr(cwd, fallback string) string {
	b, err := git.CurrentBranch(cwd)
	if err != nil {
		return fallback
	}
	return b
}

// buildCommitInstruction returns the short, explicit instruction passed
// to the AI tool. The AI is expected to act on git itself; we only
// hand it the flags the user passed and a couple of guard rails.
func buildCommitInstruction(branch string, opts CommitOptions) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Working tree on branch %q has uncommitted changes.\n\n", branch)

	b.WriteString("Your task:\n")
	b.WriteString("- Run `git status --porcelain` first to see what changed.\n")
	b.WriteString("- Run `git log --oneline -10` to learn the project's commit-message style.\n")
	if opts.Amend {
		b.WriteString("- Run `git add -A` then `git commit --amend -m \"...\"` to REWRITE the previous commit's message based on its diff (use `git show HEAD` for the diff).\n")
	} else {
		b.WriteString("- Run `git add -A` then `git commit -m \"...\"`.\n")
	}
	if opts.NoVerify {
		b.WriteString("- Pass `--no-verify` to `git commit` so pre-commit hooks are skipped.\n")
	}
	b.WriteString("- Use conventional-commit prefixes (feat:, fix:, refactor:, docs:, chore:, test:) when they fit.\n")
	b.WriteString("- First line <= 72 chars; optional blank-line-separated body.\n")
	b.WriteString("- Make ONE single commit. Do NOT push, do NOT touch other refs.\n\n")
	b.WriteString("Reply briefly when done (e.g. the new commit hash).\n")
	return b.String()
}
