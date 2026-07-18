//go:build windows

package tui

import "os"

func readByteImpl(fd int) (byte, error) {
	if len(pushedBack) > 0 {
		b := pushedBack[0]
		pushedBack = pushedBack[1:]
		return b, nil
	}
	buf := make([]byte, 1)
	_, err := os.Stdin.Read(buf)
	return buf[0], err
}
