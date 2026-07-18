//go:build windows

package launch

import (
	"os"
	"os/exec"
)

// runForeground runs cmd through the Windows shell with inherited stdio.
func runForeground(cmdStr, dir string) error {
	cmd := exec.Command("cmd", "/C", cmdStr)
	cmd.Dir = dir
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	return cmd.Run()
}

// runDetached starts cmd detached via CREATE_NEW_PROCESS_GROUP|DETACHED_PROCESS.
func runDetached(cmdStr, dir string) error {
	cmd := exec.Command("cmd", "/C", cmdStr)
	cmd.Dir = dir
	nul, err := os.OpenFile("NUL", os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer nul.Close()
	cmd.Stdin = nul
	cmd.Stdout = nul
	cmd.Stderr = nul
	cmd.SysProcAttr = detachedSysProcAttr()
	return cmd.Start()
}
