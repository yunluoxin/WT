//go:build !windows

package tui

import "os"

func readByteImpl(fd int) (byte, error) {
	buf := make([]byte, 1)
	_, err := os.Stdin.Read(buf)
	return buf[0], err
}
