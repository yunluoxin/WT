//go:build !windows

package launch

import (
	"os"
	"os/exec"
)

// runForeground runs cmd via `bash -lc` with inherited stdio.
func runForeground(cmdStr, dir string) error {
	cmd := exec.Command("bash", "-lc", cmdStr)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDetached starts cmd fully detached: new session, stdio to /dev/null.
// The child is never Wait()ed.
func runDetached(cmdStr, dir string) error {
	cmd := exec.Command("bash", "-lc", cmdStr)
	cmd.Dir = dir
	devnull, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer devnull.Close()
	cmd.Stdin = devnull
	cmd.Stdout = devnull
	cmd.Stderr = devnull
	cmd.SysProcAttr = detachedSysProcAttr()
	return cmd.Start()
}
