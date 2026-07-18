package registry

import (
	"os"
	"path/filepath"
	"strings"

	"wt/internal/git"
)

// maxScanDepth bounds the recursive repository scan.
const maxScanDepth = 5

// skipDirs are never descended into during scans.
var skipDirs = map[string]bool{
	"node_modules": true, ".cache": true, "Library": true, ".npm": true,
	".cargo": true, ".rustup": true, "go": false, // "go" may contain repos; descend
	".local": true, ".vscode": true, ".idea": true, "vendor": true,
	"dist": true, "build": true, ".next": true, ".nuxt": true, "out": true,
	"target": true, "Pods": true, "DerivedData": true, ".gradle": true,
	".m2": true, "__pycache__": true, ".venv": true, "venv": true,
	".tox": true, ".pytest_cache": true, ".git": true, ".hg": true, ".svn": true,
	"Applications": true, "Pictures": true, "Movies": true, "Music": true,
	"Public": true, "Downloads": false, // Downloads may contain repos; descend
	".Trash": true, "tmp": true, "Temp": true,
}

// ScanForRepos recursively finds repositories that use worktrees
// (.git is a directory and the repo has more than one worktree) and
// registers them. Returns the newly found repo paths.
func ScanForRepos(root string) ([]string, error) {
	var found []string
	root, err := filepath.Abs(root)
	if err != nil {
		return nil, err
	}
	walk(root, 0, &found)
	for _, repo := range found {
		if err := Register(repo); err != nil {
			return found, err
		}
	}
	return found, nil
}

func walk(dir string, depth int, found *[]string) {
	if depth > maxScanDepth {
		return
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return
	}
	// A qualifying repo: .git is a real directory (not a worktree's file)
	gitPath := filepath.Join(dir, ".git")
	if info, err := os.Stat(gitPath); err == nil && info.IsDir() {
		if wts, err := git.ParseWorktrees(dir); err == nil && len(wts) > 1 {
			*found = append(*found, dir)
		}
		return // don't descend into repos
	}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if strings.HasPrefix(name, ".") {
			continue
		}
		if skip, ok := skipDirs[name]; ok && skip {
			continue
		}
		walk(filepath.Join(dir, name), depth+1, found)
	}
}
