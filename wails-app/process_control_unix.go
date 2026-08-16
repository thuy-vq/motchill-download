//go:build darwin || linux

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"
)

func prepareBackgroundCommand(command *exec.Cmd) {}

func setProcessPaused(process *os.Process, paused bool) error {
	if process == nil {
		return fmt.Errorf("không tìm thấy tiến trình FFmpeg")
	}
	signal := syscall.SIGCONT
	if paused {
		signal = syscall.SIGSTOP
	}
	if err := syscall.Kill(process.Pid, signal); err != nil {
		return fmt.Errorf("không thể điều khiển FFmpeg: %w", err)
	}
	return nil
}
