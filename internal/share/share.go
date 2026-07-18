// Package share implements .cwshare file support: copying listed files
// into new worktrees.
package share

import (
	"bufio"
	"io"
	"os"
	"path/filepath"
	"strings"

	"wt/internal/termenv"
)

// FileName is the share-list file at the repo root.
const FileName = ".cwshare"

// Entries parses the .cwshare file: one relative path per line,
// '#' comments and blank lines ignored.
func Entries(repo string) []string {
	f, err := os.Open(filepath.Join(repo, FileName))
	if err != nil {
		return nil
	}
	defer f.Close()
	var out []string
	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		out = append(out, line)
	}
	return out
}

// HasFile reports whether the repo has a .cwshare file.
func HasFile(repo string) bool {
	_, err := os.Stat(filepath.Join(repo, FileName))
	return err == nil
}

// CopyToWorktree copies each listed file/dir from repo into the worktree.
// Missing sources and existing targets are skipped; failures warn and
// continue (non-fatal, mirroring the Python behavior).
func CopyToWorktree(repo, worktree string) {
	for _, entry := range Entries(repo) {
		src := filepath.Join(repo, entry)
		dst := filepath.Join(worktree, entry)
		info, err := os.Lstat(src)
		if err != nil {
			continue // missing source: skip silently
		}
		if _, err := os.Lstat(dst); err == nil {
			continue // target exists: skip
		}
		if err := copyEntry(src, dst, info); err != nil {
			termenv.Warn("failed to copy %s to worktree: %v", entry, err)
		}
	}
}

func copyEntry(src, dst string, info os.FileInfo) error {
	switch {
	case info.Mode()&os.ModeSymlink != 0:
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
			return err
		}
		return os.Symlink(target, dst)
	case info.IsDir():
		return copyDir(src, dst)
	default:
		return copyFile(src, dst, info.Mode())
	}
}

func copyDir(src, dst string) error {
	return filepath.Walk(src, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, p)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		switch {
		case info.Mode()&os.ModeSymlink != 0:
			linkTarget, err := os.Readlink(p)
			if err != nil {
				return err
			}
			return os.Symlink(linkTarget, target)
		case info.IsDir():
			return os.MkdirAll(target, info.Mode())
		default:
			return copyFile(p, target, info.Mode())
		}
	})
}

func copyFile(src, dst string, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	in, err := os.Open(src)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.OpenFile(dst, os.O_CREATE|os.O_WRONLY|os.O_EXCL, mode)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		out.Close()
		return err
	}
	return out.Close()
}
