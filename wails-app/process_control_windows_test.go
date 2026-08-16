//go:build windows

package main

import (
	"os"
	"os/exec"
	"testing"
	"time"
)

func TestProcessCanBePausedAndResumed(t *testing.T) {
	if os.Getenv("MOTCHILL_PAUSE_HELPER") == "1" {
		time.Sleep(30 * time.Second)
		return
	}
	command := exec.Command(os.Args[0], "-test.run=TestProcessCanBePausedAndResumed")
	command.Env = append(os.Environ(), "MOTCHILL_PAUSE_HELPER=1")
	prepareBackgroundCommand(command)
	if command.SysProcAttr == nil || !command.SysProcAttr.HideWindow || command.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatal("FFmpeg process must be configured without a visible console window")
	}
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = command.Process.Kill()
		_, _ = command.Process.Wait()
	}()
	if err := setProcessPaused(command.Process, true); err != nil {
		t.Fatal(err)
	}
	if err := setProcessPaused(command.Process, false); err != nil {
		t.Fatal(err)
	}
}
