package cli

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

// llmSpecs maps a subcommand name to an AI-oriented usage snippet. The
// snippets are written for AI coding agents (Claude Code, Codex, etc.), not
// humans: exact syntax, fixed enum values, concrete examples, and common
// failure modes. Output is plain text on stdout, safe for shell capture.
var llmSpecs = []struct {
	name    string
	snippet string
}{
	{"new", `wt new <branch-name> [--base <branch>] [--path <dir>] [--no-term] [-T <method>]
  Create a git worktree for a new feature branch and launch the configured
  AI tool in it. The branch is stored as wt-<branch-name> (prefix added
  automatically), the worktree path defaults to ../<repo>-<branch-name>.
  Flags:
    -b, --base <branch>   base branch (default: current branch)
    -p, --path <dir>      custom worktree path
        --no-term         create only, do not launch the AI tool — pass this
                          when YOU (the agent) will do the work yourself
    -T, --term <method>   launch method; one of:
                          foreground (fg), detach (d),
                          iterm-window (i-w), iterm-tab (i-t),
                          iterm-pane-h (i-p-h), iterm-pane-v (i-p-v),
                          tmux (t), tmux-window (t-w),
                          tmux-pane-h (t-p-h), tmux-pane-v (t-p-v),
                          zellij (z), zellij-tab (z-t),
                          zellij-pane-h (z-p-h), zellij-pane-v (z-p-v),
                          wezterm-window (w-w), wezterm-tab (w-t),
                          wezterm-pane-h (w-p-h), wezterm-pane-v (w-p-v)
                          tmux/zellij accept a session name: -T t:mysession
  Examples:
    wt new fix-auth --no-term
    wt new api-redesign --base main -T i-t`},
	{"list", `wt list
  List all worktrees of the current repository (branch, path, status).
  With the global flag, 'wt -g list' lists worktrees across ALL registered
  repositories. Run this first to discover existing worktrees before
  creating, deleting, or merging — most other commands take the branch
  names shown here as their target.
  Examples:
    wt list
    wt -g list`},
	{"status", `wt status
  Show metadata for the current worktree (branch, path, base branch,
  dirty/clean state) plus a list of all worktrees. Use it to answer
  "which worktree am I in and is it safe to merge?".
  Example: wt status`},
	{"delete", `wt delete [target] [-b|-w] [-k] [-r] [--no-force]
  Remove a worktree. The target is a branch name OR worktree directory name
  (auto-detected; disambiguate with -b/--branch or -w/--worktree). The local
  branch is also deleted unless -k/--keep-branch.
  Flags:
    -k, --keep-branch    remove only the worktree, keep the branch
    -r, --delete-remote  also delete the remote branch
        --no-force       refuse to remove a dirty worktree
  Omitting the target opens an interactive selector — always pass an
  explicit target in non-interactive runs.
  Examples:
    wt delete fix-auth
    wt delete fix-auth --keep-branch
    wt delete myrepo-fix-auth -w -r`},
	{"merge", `wt merge [target] [-b|-w] [--push] [--dry-run] [--any]
  Finish a worktree: rebase its branch onto the base branch, fast-forward
  merge into the base, then remove the worktree and delete the branch.
  This is the "task is done" command. Flags:
        --push      push the base branch to origin after merging
        --dry-run   preview every step without executing (use this first
                    when unsure)
        --any       allow branches not created by 'wt new' (no wt- prefix)
    -i, --interactive  confirm each step — do NOT use in non-interactive runs
  Target defaults to the current worktree; -b/-w disambiguate branch vs
  directory name.
  Examples:
    wt merge fix-auth
    wt merge fix-auth --dry-run
    wt merge --push            # merge the worktree you are standing in`},
	{"pr", `wt pr [target] [-b|-w] [-t <title>] [--body <text>] [--draft] [--no-push] [--any]
  Rebase, push, and open a GitHub Pull Request for the worktree branch via
  the 'gh' CLI. Use this instead of 'wt merge' when the change needs code
  review. Title/body are AI-generated unless given explicitly.
  Flags:
    -t, --title <title>  PR title (skips AI generation)
        --body <text>    PR body
        --draft          create as draft PR
        --no-push        don't push before creating the PR
        --any            allow branches without the wt- prefix
  Examples:
    wt pr fix-auth
    wt pr fix-auth --draft -t "Fix auth token refresh"`},
	{"resume", `wt resume [worktree] [-b|-w] [-T <method>]
  Re-open the configured AI tool inside an existing worktree, resuming its
  previous session: claude presets get --continue, codex presets get
  'resume --last', anything else gets --resume appended. With
  session.auto_resume (or WT_AUTO_RESUME=1), 'wt new' also resumes.
    -T, --term <method>   same launch-method enum as 'wt new'
                          (foreground, detach, iterm-tab, tmux, ...)
  Omitting the target opens an interactive selector — always pass a target
  in non-interactive runs.
  Examples:
    wt resume fix-auth
    wt resume fix-auth -T t:mysession`},
	{"shell", `wt shell [worktree] [command...]
  Run a command inside a worktree without cd-ing there yourself — this is
  THE way to build/test inside a worktree from outside it. Flag parsing is
  disabled: everything after the worktree name is executed verbatim. If the
  first argument is not a known worktree, all arguments run in the current
  worktree. With no command at all it opens an interactive shell — avoid
  that in non-interactive runs; always pass a command. The exit code of the
  command is propagated.
  Examples:
    wt shell fix-auth go test ./...
    wt shell fix-auth git log --oneline -5
    wt shell fix-auth make build
    wt shell npm test              # run in the current worktree`},
	{"sync", `wt sync [target] [-b|-w] [--all] [--fetch-only] [--ai-merge]
  Fetch and rebase worktree(s) onto the latest base branch. Run this when
  the base branch has moved and the worktree is behind.
  Flags:
        --all         sync every worktree in topological order
        --fetch-only  fetch updates but do not rebase
        --ai-merge    on rebase conflict, launch the configured AI tool
                      (non-interactive mode) to resolve it
  Examples:
    wt sync fix-auth
    wt sync --all
    wt sync fix-auth --ai-merge`},
	{"clean", `wt clean [--merged] [--older-than <days>] [--dry-run]
  Batch-delete worktrees by criteria.
  Flags:
        --merged          delete worktrees whose branches are merged into base
        --older-than <n>  delete worktrees idle for n+ days
        --dry-run         show what would be deleted — run this FIRST
    -i, --interactive     pick interactively — do NOT use non-interactively
  Examples:
    wt clean --merged --dry-run
    wt clean --merged
    wt clean --older-than 30`},
	{"change-base", `wt change-base <new-base> [--target <worktree>] [--dry-run] [-b|-w]
  Change the base branch of a worktree and rebase onto it. Use when a
  feature branch was started from the wrong base or must be re-targeted.
  Default target is the current directory.
  Examples:
    wt change-base develop --target fix-auth
    wt change-base release/2.0 --dry-run`},
	{"doctor", `wt doctor
  Health-check all worktrees: missing directories, stale metadata, broken
  registry entries. Run this first whenever another wt command behaves
  unexpectedly (e.g. a worktree was deleted manually with rm -rf).
  Related: 'wt prune' removes stale entries from the global registry,
  'wt scan [-d <dir>]' discovers repositories with worktrees and
  registers them (default scan directory: home).
  Examples:
    wt doctor
    wt scan --dir ~/code
    wt prune`},
	{"diff", `wt diff <branch1> <branch2> [-s|-f]
  Compare two branches. Exactly one of -s/-f may be given:
    -s, --summary   diff statistics only
    -f, --files     changed file list only
  (default: full diff output)
  Examples:
    wt diff main wt-fix-auth --summary
    wt diff wt-feat-a wt-feat-b --files`},
	{"tree", `wt tree
  Display the worktree hierarchy (base branch and its worktrees) as a tree.
  Example: wt tree`},
	{"stats", `wt stats
  Display worktree usage analytics (counts, ages, activity).
  Example: wt stats`},
	{"stash", `wt stash — stash changes in one worktree, re-apply them in another

Subcommands:
  wt stash save [message] [--include-untracked]
      Stash uncommitted changes in the CURRENT worktree (run it from inside
      the worktree). Untracked files are excluded unless
      --include-untracked is passed.
  wt stash list
      List all stashes grouped by branch — run this to find the stash
      reference (stash@{0}, stash@{1}, ...) before applying.
  wt stash apply <target-branch> [-s <ref>]
      Apply a stash onto another worktree's branch.
        -s, --stash <ref>   which stash to apply (default: stash@{0},
                            the most recent); use refs from 'wt stash list'

Examples:
  wt stash save "wip: parser rewrite"
  wt stash save --include-untracked
  wt stash list
  wt stash apply wt-fix-auth
  wt stash apply wt-fix-auth --stash "stash@{1}"`},
	{"backup", `wt backup — snapshot and restore worktrees via git bundle

Subcommands:
  wt backup create [branch] [-o <dir>] [--all]
      Create a backup of one branch (default: current branch).
        -o, --output <dir>  custom output directory for the bundle
            --all           back up ALL worktrees at once
  wt backup list [branch]
      List available backups (all, or only those for one branch). Each
      backup has a timestamp ID used by restore --id.
  wt backup restore <branch> [--id <timestamp>] [-p <path>]
      Restore a worktree from a backup.
            --id <timestamp>  which backup to restore (default: latest);
                              get IDs from 'wt backup list'
        -p, --path <path>     custom restore location

Create a backup before risky operations such as 'wt sync' on a heavily
diverged branch.

Examples:
  wt backup create wt-fix-auth
  wt backup create --all
  wt backup list wt-fix-auth
  wt backup restore wt-fix-auth
  wt backup restore wt-fix-auth --id 20260719-112233 -p /tmp/recovered`},
	{"hook", `wt hook init
wt hook add <event> <command> [--id <id>] [-d <desc>]
wt hook list [event]
wt hook remove|enable|disable <event> <hook-id>
wt hook run <event> [--dry-run]
  Manage per-repository lifecycle hooks (shell commands stored in
  .wtconfig.json, run automatically at lifecycle points).
  IMPORTANT: <event> is NOT free-form — it must be one of exactly:
    worktree.pre_create    worktree.post_create
    worktree.pre_delete    worktree.post_delete
    merge.pre              merge.post
    pr.pre                 pr.post
    resume.pre             resume.post
    sync.pre               sync.post
  An unknown event fails with: unknown hook event "<name>".
  Subcommands:
    init                             install .wt-hooks/post-create.sh (auto-
                                     installs deps for JS, Python, Go, Rust,
                                     Swift/SPM, CocoaPods, Gradle, Flutter,
                                     Ruby, PHP, .NET, Maven) and enable it;
                                     idempotent, never overwrites
    add <event> <command>            register a hook (--id custom id,
                                     -d description)
    list [event]                     list hooks (all events or one)
    remove <event> <hook-id>         remove a hook
    enable|disable <event> <hook-id> toggle a hook without removing it
    run <event> [--dry-run]          fire hooks manually
  Examples:
    wt hook init
    wt hook add worktree.post_create "npm install" -d "install deps"
    wt hook add merge.pre "make test"
    wt hook list merge.pre
    wt hook disable merge.pre <hook-id>
    wt hook run worktree.post_create --dry-run`},
	{"config", `wt config — manage wt configuration (file: ~/.config/wt/config.json)

Subcommands:
  wt config show                     display current config (AI tool, launch
                                     method, git defaults, session settings)
  wt config set <key> <value>        set one value (dot-path keys, see below)
  wt config use-preset <name>        switch the AI tool to a built-in preset
  wt config list-presets             list all presets with their commands
  wt config reset                    restore everything to defaults

=== Setting your AI tool ===
Two ways:
  1. Custom command — use the special key 'ai-tool' (with a DASH, not a
     dot). The value is split on whitespace into command + args:
       wt config set ai-tool "claude --dangerously-skip-permissions"
       wt config set ai-tool "aider --model sonnet"
       wt config set ai-tool "my-wrapper --flag value"
     To disable AI launching entirely: wt config use-preset no-op
  2. Built-in preset — <name> must be one of exactly:
       no-op                (none)                                   disable launching
       claude               claude                                    Claude Code (default)
       claude-yolo          claude --dangerously-skip-permissions
       claude-remote        claude /remote-control
       claude-yolo-remote   claude --dangerously-skip-permissions /remote-control
       codex                codex                                     OpenAI Codex
       codex-yolo           codex --dangerously-bypass-approvals-and-sandbox
     Unknown names fail with: unknown preset "<name>".

=== All 'wt config set' keys ===
  ai-tool "<cmd> [args...]"          special key (dash!): AI tool command.
                                     Empty value is rejected — use
                                     'use-preset no-op' to disable instead.
  launch.method <method>             default launch method for 'wt new' and
                                     'wt resume'; same enum as -T/--term:
                                     foreground, detach, iterm-window,
                                     iterm-tab, iterm-pane-h, iterm-pane-v,
                                     tmux, tmux-window, tmux-pane-h,
                                     tmux-pane-v, zellij, zellij-tab,
                                     zellij-pane-h, zellij-pane-v,
                                     wezterm-window, wezterm-tab,
                                     wezterm-pane-h, wezterm-pane-v
  launch.session_prefix <string>     prefix for tmux/zellij session names
                                     (default: wt)
  launch.wezterm_ready_timeout <n>   seconds to wait for wezterm panes
                                     (default: 5)
  git.default_base_branch <branch>   default base branch (default: main)
  session.auto_resume <true|false>   auto-continue the previous AI session
                                     when launching (default: false)
Values "true"/"false" (any case) are stored as booleans, numbers as numbers,
everything else as strings. Unknown dot-paths are accepted and stored, but
only the keys above have an effect.

=== Environment overrides (win over config) ===
  WT_AI_TOOL="<cmd> [args...]"   overrides the AI tool; empty string disables
  WT_LAUNCH_METHOD=<method>      overrides launch.method
  WT_AUTO_RESUME=1|true|yes      forces auto-resume on (0/false/no = off)

Examples:
  wt config show
  wt config set ai-tool "claude --dangerously-skip-permissions"
  wt config use-preset codex
  wt config set launch.method tmux
  wt config set git.default_base_branch develop
  wt config set session.auto_resume true
  wt config reset`},
	{"cd", `wt cd [branch|repo:branch] [-g]
  Print the absolute path of a worktree — ONLY the path on stdout, all UI
  and errors go to stderr, so it is safe to capture. A subprocess cannot
  change the parent shell's directory, so wrap it: cd "$(wt cd fix-auth)".
  With 'wt init' shell integration installed, 'wt cd <branch>' changes
  directory directly. Use -g or repo:branch notation to search across all
  registered repositories. Omitting the target (or passing -i) opens an
  interactive selector — always pass a target in non-interactive runs.
  Examples:
    cd "$(wt cd fix-auth)"
    cd "$(wt cd myrepo:fix-auth)"
    ls "$(wt cd fix-auth)/src"     # capture without cd-ing`},
	{"init", `wt init [shell]
  Print shell integration (a wt-cd helper so 'wt cd <branch>' changes
  directory directly, plus tab completion). <shell> is one of exactly:
  bash, zsh, fish, powershell.
  Install:
    bash:       echo 'source <(wt init bash)' >> ~/.bashrc
    zsh:        echo 'source <(wt init zsh)' >> ~/.zshrc
    fish:       wt init fish | source  (add to config.fish)
    powershell: wt init powershell | Out-String | Invoke-Expression
  Example: wt init zsh >> ~/.zshrc`},
}

func llmCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "llm <command>",
		Short: "Print AI-oriented usage instructions for wt commands",
		Long: `Print usage instructions for wt commands, written for AI coding agents.

Each subcommand prints a self-contained instruction block to stdout:
exact syntax, fixed enum values, concrete examples, and common failure
modes. Pipe or capture the output to teach an AI tool how to drive wt:

  wt llm          # all commands
  wt llm merge    # just the merge command`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Print(llmAll())
			return nil
		},
	}
	for _, spec := range llmSpecs {
		spec := spec
		root.AddCommand(&cobra.Command{
			Use:   spec.name,
			Short: fmt.Sprintf("Print AI-oriented usage for 'wt %s'", spec.name),
			Args:  cobra.NoArgs,
			RunE: func(cmd *cobra.Command, args []string) error {
				fmt.Println(spec.snippet)
				return nil
			},
		})
	}
	return root
}

// llmAll renders the full instruction block: a short preamble plus every
// command snippet.
func llmAll() string {
	var b strings.Builder
	b.WriteString("wt — git worktree manager with AI assistant integration.\n")
	b.WriteString("Core workflow: wt new <branch> --no-term → do the work in ../<repo>-<branch> → wt merge <branch> (or wt pr <branch> for review).\n")
	b.WriteString("Rules for AI agents:\n")
	b.WriteString("  - Output below is plain text; every command prints structured info to stdout and errors to stderr.\n")
	b.WriteString("  - Arguments marked <like-this> that list fixed values are enums — any other value is an error.\n")
	b.WriteString("  - Always pass explicit targets; never use -i/--interactive or selector prompts in non-interactive runs.\n")
	b.WriteString("  - Prefer --dry-run before destructive commands (merge, clean) when state is uncertain.\n")
	b.WriteString("  - Global -g flag operates across all registered repositories.\n\n")
	for i, spec := range llmSpecs {
		if i > 0 {
			b.WriteString("\n\n")
		}
		b.WriteString(spec.snippet)
	}
	b.WriteString("\n")
	return b.String()
}
