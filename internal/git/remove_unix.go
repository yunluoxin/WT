//go:build !windows

package git

import "os"

func removeAll(path string) error {
	return os.RemoveAll(path)
}
