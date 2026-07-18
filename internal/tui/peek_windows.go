//go:build windows

package tui

import (
	"os"
	"time"
)

// peekReady reads one byte into a channel with a timeout. The byte is
// pushed back via a small buffered reader wrapper used by readKey.
// Note: on Windows we rely on the console delivering the whole escape
// sequence; a bare Escape simply waits out the timeout.
func peekReady(fd int, d time.Duration) bool {
	ch := make(chan struct{}, 1)
	go func() {
		buf := make([]byte, 1)
		if n, err := os.Stdin.Read(buf); err == nil && n == 1 {
			pushedBack = append(pushedBack, buf[0])
			ch <- struct{}{}
		}
	}()
	select {
	case <-ch:
		return true
	case <-time.After(d):
		return false
	}
}

// pushedBack buffers bytes consumed by peekReady.
var pushedBack []byte
