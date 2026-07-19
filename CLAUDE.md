# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`wt` is a git worktree manager (Go rewrite of the Python `claude-worktree`/`cw`). It creates worktrees for feature branches (auto-prefixed `wt-`), launches an AI coding assistant in them, and merges changes back. Module name is `wt` (no module path prefix); entry point is `cmd/wt/main.go`.

## Commands

```bash
go build ./...                 # build
go build -o wt ./cmd/wt        # build binary
go test ./...                  # all tests (CI adds -race)
go test ./internal/ops/ -run TestCreateWorktree -v   # single test
go vet ./... && gofmt -l .     # lint (gofmt output must be empty — CI enforces)
```

e2e tests (`e2e/`) build the real binary once in `TestMain` and run it against temp git repos. Tests on Windows need `USERPROFILE` set — `testutil.SetHome(t)` handles this plus `HOME`, `XDG_CONFIG_HOME`, `WT_NON_INTERACTIVE=1`, `WT_AI_TOOL=""` (use it in every test touching config/env; it uses `t.Setenv`, so no `t.Parallel()`).

## Architecture

Layered: `internal/cli` (cobra commands, flags, output formatting) → `internal/ops` (business logic, one file per feature area) → `internal/git` (git subprocess wrapper) + supporting packages.

- **cli/**: commands are plain constructor functions (`newCmd()`, `mergeCmd()`, ...) registered in `root.go`'s `NewRootCmd()`. `root.go` also owns global state: `globalMode` (`-g` flag) and `targetFlags` (shared `-b/--branch` vs `-w/--worktree` disambiguation used by delete/merge/pr/resume/sync/change-base).
- **ops/**: all user-facing operations. `resolve.go` centralizes target resolution (branch name vs worktree dir vs current dir, with `LookupMode`).
- **config/**: `~/.config/wt/config.json` (XDG-aware) deep-merged over `DefaultConfig`. `presets.go` defines AI tool presets (`claude`, `codex`, ...) plus their resume/merge invocation variants. `launch.go` defines the 18 launch methods (foreground/iterm/tmux/zellij/wezterm) and alias parsing for `--term` (`i-t`, `t:session`, ...).
- **aitool/**: resolves the effective AI command. Priority is always: `WT_AI_TOOL` env (empty string = disabled) > config. Three variants: launch, resume (preset-specific flags like `--continue`), merge (for `sync --ai-merge`).
- **launch/**: spawns the AI tool per launch method (OS-specific: `run_unix.go`/`run_windows.go` in ops, `remove_unix.go`/`remove_windows.go` in git).
- **hooks/**: per-repo lifecycle hooks in `<repo>/.wtconfig.json`; 12 fixed events (`worktree.post_create`, `merge.pre`, ...). Hook processes get context injected as `WT_*` env vars (`WT_BRANCH`, `WT_WORKTREE_PATH`, ...).
- **session/**: detects and stores AI session state (incl. Claude's session dirs) so `wt resume` can continue the right session.
- **registry/**: global registry (`~/.config/wt/registry.json`) backing `-g` cross-repo mode, `scan`, `prune`.
- **share/**: `.wtshare` file copying into new worktrees; prompts the user once — suppressed when stdout is not a TTY or `WT_NON_INTERACTIVE` is set. Commands whose stdout is captured (e.g. `cd`, `llm`) must be added to the skip list in `root.go`'s `PersistentPreRun`.
- **termenv/**: TTY-aware colored output. Anything printed to stdout that scripts may capture must bypass the color helpers.
- **llm command**: `cli/llm.go` contains `llmSpecs` — AI-oriented usage snippets, one per command. When adding/renaming a command or changing flags, update the matching snippet; tests assert every snippet lists its enums and has an Examples section.

## Before every commit (mandatory)

每次修改代码后、commit 之前，必须先在本地完整验证通过，禁止依赖 CI 发现问题：

1. `go build ./...` — 编译必须通过
2. `go test ./...` — 所有测试必须通过（包括 `e2e/`）
3. `go vet ./... && gofmt -l .` — gofmt 输出必须为空

任何一项不通过就不允许 commit / push。

## Conventions

- Version metadata injected via ldflags (`cli.SetVersion`); releases use GoReleaser on `v*` tags (see RELEASING.md).
- Branches from `wt new` get a `wt-` prefix; `merge`/`pr` reject non-prefixed branches without `--any` (`ops.PrefixBranch`).
- Errors: `internal/errors` wraps with typed categories; `ErrAborted` exits 0; commands that already printed their error return `exitError` to keep stdout clean.
