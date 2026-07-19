# wt — git worktree manager (Go)

`wt` is a Go rewrite of [claude-worktree](https://github.com/DaveDev42/claude-worktree) (`cw`).
It creates isolated git worktrees for feature branches, launches your AI
coding assistant (Claude Code, Codex, or custom) in them, and cleanly
merges changes back.

Compared to the Python version: single static binary, faster startup, no
runtime dependencies.

## Install

### Prebuilt binaries

Download an archive for your platform from the
[latest release](https://github.com/yunluoxin/WT/releases/latest),
extract it, and put `wt` on your `PATH`:

```bash
# macOS (Apple Silicon) — swap darwin_arm64 for your platform:
#   darwin_amd64 | linux_amd64 | linux_arm64 | windows_amd64 | windows_arm64
curl -LO https://github.com/yunluoxin/WT/releases/latest/download/wt_1.0.0_darwin_arm64.tar.gz
tar xzf wt_1.0.0_darwin_arm64.tar.gz
sudo install wt /usr/local/bin/      # or any directory on your PATH

# Verify the download against the published checksums
curl -LO https://github.com/yunluoxin/WT/releases/latest/download/checksums.txt
shasum -a 256 -c checksums.txt --ignore-missing
```

Windows: download `wt_*_windows_<arch>.zip`, unzip, and add the folder
containing `wt.exe` to your `PATH`.

Note: `releases/latest/download/<asset>` URLs pin the exact version in
the asset name, so replace `1.0.0` in the filename with the version
shown on the releases page when a newer one exists (the version inside
the archive name always matches the tag).

### From source

```bash
go install ./cmd/wt

# Or build locally
go build -o wt ./cmd/wt
```

## Quick start

```bash
wt new fix-auth        # create branch wt-fix-auth + worktree ../<repo>-wt-fix-auth, launch AI tool
wt list                # list worktrees
wt resume wt-fix-auth  # resume AI session in a worktree
wt pr                  # rebase, push, create GitHub PR (requires gh)
wt merge               # rebase + fast-forward merge into base + cleanup
```

Branches created by `wt new` carry a `wt-` prefix so wt-managed worktrees
are recognizable at a glance. `wt merge` and `wt pr` refuse branches
without the prefix (e.g. worktrees created by plain `git worktree add`)
unless you pass `--any`.

## Commands

| Group | Commands |
|---|---|
| Core workflow | `new`, `resume`, `pr`, `merge`, `finish` (deprecated alias), `shell` |
| Worktree management | `list`, `status`, `delete`, `clean`, `sync`, `change-base` |
| Global (cross-repo) | `-g/--global` flag, `scan`, `prune` |
| Inspection | `doctor`, `diff`, `tree`, `stats` |
| Stash | `stash save`, `stash list`, `stash apply` |
| Backup | `backup create`, `backup list`, `backup restore` |
| Hooks | `hook init/add/remove/list/enable/disable/run` (12 lifecycle events, `.wtconfig.json`) |
| Config | `config show/set/use-preset/list-presets/reset`, `export`, `import` |
| Shell integration | `cd`, `init`, `completion <shell>` |
| AI integration | `llm [command]` (machine-oriented usage docs, see below) |

### AI agent integration (`wt llm`)

`wt llm` prints usage instructions written for AI coding agents (Claude
Code, Codex, …) — exact syntax, fixed enum values (hook events, launch
methods, presets), concrete examples, and non-interactive usage notes.
Output is plain text on stdout with no side effects, safe to capture.

```bash
wt llm          # full instruction block for every command
wt llm merge    # just one command
```

Feed it into your agent's context so it can drive wt directly:

```bash
# e.g. append to CLAUDE.md / AGENTS.md, or paste into a system prompt
wt llm >> CLAUDE.md
```

### AI tool presets

`wt config use-preset <name>`: `claude` (default), `claude-yolo`,
`claude-remote`, `claude-yolo-remote`, `codex`, `codex-yolo`, `no-op`.

### Launch methods (`--term`)

`foreground` (default), `detach`, `iterm-window/tab/pane-h/pane-v`,
`tmux[:name]/tmux-window/tmux-pane-h/pane-v`,
`zellij[:name]/zellij-tab/zellij-pane-h/pane-v`,
`wezterm-window/tab/pane-h/pane-v`. Short aliases: `fg`, `d`, `i-w`,
`i-t`, `t`, `z`, `w-w`, …

### Configuration

- Config file: `~/.config/wt/config.json` (honors `XDG_CONFIG_HOME`)
- Sessions: `~/.config/wt/sessions/<branch>/`
- Backups: `~/.config/wt/backups/`
- Registry (global mode): `~/.config/wt/registry.json`
- Per-repo hooks: `<repo>/.wtconfig.json` (scripts conventionally in `<repo>/.wt-hooks/`)
- Shared files to copy into new worktrees: `<repo>/.wtshare`
- Env overrides: `WT_AI_TOOL`, `WT_LAUNCH_METHOD`, `WT_NON_INTERACTIVE`

### Hooks

Hooks are shell commands registered per repository and fired at 12
lifecycle points (`worktree.post_create`, `merge.pre`, …). To get started,
`wt hook init` writes `.wt-hooks/post-create.sh` from a built-in template
that detects common project types — JS (npm/pnpm/yarn/bun), Python
(poetry/uv/pipenv/venv), Go, Rust, Swift/SPM, CocoaPods, Gradle (Android
/JVM), Flutter/Dart, Ruby, PHP, .NET, Maven — and installs their
dependencies (frozen/locked modes where the tool supports them, skipping
projects that declare no dependencies), and enables it as a
`worktree.post_create` hook — live immediately, no rename needed.
Re-running is safe: an existing script is left untouched. (The template
ships inside the binary via `go:embed`; its source lives at
`internal/hooks/templates/post-create.sh`.)

```bash
wt hook init                                        # install the template
wt hook add worktree.post_create "npm install"      # or register your own
wt hook list                                        # see what's registered
wt hook run worktree.post_create --dry-run          # preview
```

Hook processes receive `WT_BRANCH`, `WT_BASE_BRANCH`, `WT_WORKTREE_PATH`,
`WT_REPO_PATH`, `WT_EVENT`, `WT_OPERATION`, `WT_PR_URL` in their
environment. Pre-hooks abort the operation on failure; post-hooks only
warn.

### Shell integration (wt cd + completion)

`wt cd <branch>` prints the worktree path (with `-g`/`repo:branch` for
cross-repo lookup, or an interactive selector when no target is given).
A subprocess can't change the parent shell's directory, so the shell
side needs a one-line function plus completion. `wt init` installs both:

```bash
wt init            # auto-detect shell, append snippet to your profile
wt init zsh        # explicit shell: bash | zsh | fish | powershell
wt init --print    # print the snippet instead of writing the profile
```

The snippet is deliberately minimal — all logic lives in the binary:

```bash
# bash / zsh
wt-cd() { cd "$(wt cd "$@")"; }
source <(wt completion bash)   # or: source <(wt completion zsh)

# fish
function wt-cd; cd (wt cd $argv); end
wt completion fish | source

# powershell
function wt-cd { Set-Location (wt cd @args) }
wt completion powershell | Out-String | Invoke-Expression
```

After restarting your shell: `wt-cd <branch>` jumps to a worktree,
`wt-cd` alone opens an interactive selector, `wt-cd -g repo:branch`
works across repositories, and `<TAB>` completes branch names and flags
for every `wt` subcommand.

## Differences from the Python version

- No `upgrade` command / update checks (no PyPI for Go binaries)
- New config location (`~/.config/wt/`); Python config is not migrated
- Env var prefix is `WT_*` (was `CW_*`), including hook env injection
- Deprecated hidden flags (`--bg`, `--iterm`, `--iterm-tab`, `--tmux`) removed — use `--term`
- Fixed: `launch.session_prefix` is now honored consistently

## Development

```bash
go build ./...
go test ./... -race
```

Releases: see [RELEASING.md](RELEASING.md).
