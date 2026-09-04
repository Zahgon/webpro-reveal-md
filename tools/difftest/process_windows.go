//go:build windows

package main

import (
	"errors"
	"os/exec"
)

// Windows has no process groups or signals in the POSIX sense. These stubs
// exist so the harness compiles everywhere; running it still requires a Unix
// host, because the SIGINT shutdown path it compares cannot be expressed here.
func useProcessGroup(cmd *exec.Cmd) {}

func interruptGroup(cmd *exec.Cmd) error {
	return errors.New("difftest requires a Unix host: SIGINT cannot be delivered on Windows")
}

func killGroup(cmd *exec.Cmd) {
	if cmd.Process != nil {
		_ = cmd.Process.Kill()
	}
}
