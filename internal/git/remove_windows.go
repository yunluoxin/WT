//go:build windows

package git

import (
	"os"
	"path/filepath"
)

// removeAll force-removes a directory tree on Windows: clears the
// read-only attribute on files that resist deletion, then retries.
func removeAll(path string) error {
	_ = filepath.Walk(path, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		_ = os.Chmod(p, 0o666)
		return nil
	})
	return os.RemoveAll(path)
}
