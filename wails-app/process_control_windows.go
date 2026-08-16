//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"syscall"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow       = 0x08000000
	processSuspendResume = 0x0800
)

var (
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	ntSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	ntResumeProcess  = ntdll.NewProc("NtResumeProcess")
)

func prepareBackgroundCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

func setProcessPaused(process *os.Process, paused bool) error {
	if process == nil {
		return fmt.Errorf("không tìm thấy tiến trình FFmpeg")
	}
	handle, err := windows.OpenProcess(processSuspendResume, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("không thể điều khiển FFmpeg: %w", err)
	}
	defer windows.CloseHandle(handle)
	procedure := ntResumeProcess
	operation := "tiếp tục"
	if paused {
		procedure = ntSuspendProcess
		operation = "tạm dừng"
	}
	status, _, _ := procedure.Call(uintptr(handle))
	if status != 0 {
		return fmt.Errorf("không thể %s FFmpeg (NTSTATUS 0x%x)", operation, status)
	}
	return nil
}
