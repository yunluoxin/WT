//go:build !windows

package tui

import (
	"time"

	"golang.org/x/sys/unix"
)

// peekReady reports whether input is available on fd within d.
func peekReady(fd int, d time.Duration) bool {
	var fds unix.FdSet
	fds.Zero()
	fds.Set(fd)
	tv := unix.NsecToTimeval(d.Nanoseconds())
	n, err := unix.Select(fd+1, &fds, nil, nil, &tv)
	return err == nil && n > 0
}
