package ops

import (
	"fmt"
	"os"

	wterrors "wt/internal/errors"
	"wt/internal/git"
	"wt/internal/tui"
)

// CDWorktree resolves the worktree path for a target so the caller's shell
// can cd into it (see `wt cd`). Prints ONLY the path to stdout; all
// UI/errors must go to stderr.
//
// When target is empty or interactive is set, an interactive arrow-key
// selector is shown; with exactly one worktree and no explicit selector
// request, its path is printed directly.
func CDWorktree(target string, global, interactive bool) error {
	if interactive || target == "" {
		items, err := cdSelectorItems(global)
		if err != nil {
			return err
		}
		if len(items) == 0 {
			fmt.Fprintln(os.Stderr, "Error: no worktrees found")
			return wterrors.New(wterrors.ErrWorktreeNotFound, "no worktrees found")
		}
		if len(items) == 1 && !interactive {
			fmt.Println(items[0].Value)
			return nil
		}
		// Default selection: current directory's worktree.
		defaultIdx := 0
		if cwd, err := os.Getwd(); err == nil {
			for i, it := range items {
				if samePath(it.Value, cwd) {
					defaultIdx = i
					break
				}
			}
		}
		value, ok := tui.ArrowSelect(items, "Select worktree:", defaultIdx)
		if !ok {
			return wterrors.ErrAborted
		}
		fmt.Println(value)
		return nil
	}

	t, err := ResolveWorktreeTarget(target, LookupAuto, global)
	if err != nil {
		return err
	}
	fmt.Println(t.WorktreePath)
	return nil
}

// cdSelectorItems builds the interactive selector items: repo:branch targets
// across the registry in global mode, local worktrees otherwise.
func cdSelectorItems(global bool) ([]tui.Item, error) {
	var items []tui.Item
	if global {
		for _, gb := range GlobalTargets("") {
			t, err := ResolveWorktreeTarget(gb, LookupAuto, true)
			if err != nil {
				continue
			}
			items = append(items, tui.Item{Label: gb, Value: t.WorktreePath})
		}
		return items, nil
	}
	repo, err := git.MainRepoRoot("")
	if err != nil {
		return nil, err
	}
	wts, err := git.ParseWorktrees(repo)
	if err != nil {
		return nil, err
	}
	for _, wt := range wts {
		if wt.Branch == git.DetachedBranch {
			continue
		}
		items = append(items, tui.Item{Label: wt.Branch, Value: wt.Path})
	}
	return items, nil
}
