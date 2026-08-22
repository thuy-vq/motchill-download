//go:build darwin || linux

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"syscall"
	"time"
)

func prepareBackgroundCommand(command *exec.Cmd) {
	// Own process group: the whole FFmpeg tree can then be signalled at once.
	command.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}

// superviseProcess has no job-object equivalent here; killProcessTree below
// covers the cleanup that Windows gets from the job object.
func superviseProcess(process *os.Process) error {
	if process == nil {
		return fmt.Errorf("không tìm thấy tiến trình FFmpeg")
	}
	return nil
}

// Here the app keeps the countdown itself, so closing it cancels the shutdown.
const systemHandlesShutdownDelay = false

func scheduleSystemShutdown(delay time.Duration) error {
	if delay > 0 {
		// The caller waits instead; only the immediate command is issued here.
		time.Sleep(delay)
	}
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		command = exec.Command("osascript", "-e", `tell application "System Events" to shut down`)
	} else {
		command = exec.Command("shutdown", "-h", "now")
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("không tắt được máy: %w — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func cancelSystemShutdown() error {
	// Nothing was handed to the system, so there is nothing to call off.
	return nil
}

func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	// A suspended process still receives SIGKILL, so pausing cannot block this.
	if err := syscall.Kill(-process.Pid, syscall.SIGKILL); err == nil {
		return nil
	}
	if err := process.Kill(); err != nil && !errors.Is(err, os.ErrProcessDone) {
		return err
	}
	return nil
}

func setProcessPaused(process *os.Process, paused bool) error {
	if process == nil {
		return errProcessGone
	}
	signal := syscall.SIGCONT
	if paused {
		signal = syscall.SIGSTOP
	}
	if err := syscall.Kill(process.Pid, signal); err != nil {
		// The episode may simply have ended between the button press and here.
		if errors.Is(err, syscall.ESRCH) {
			return errProcessGone
		}
		return fmt.Errorf("không thể điều khiển FFmpeg: %w", err)
	}
	return nil
}
