//go:build windows

package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	createNoWindow       = 0x08000000
	processSuspendResume = 0x0800
	// NTSTATUS values that mean the target is already on its way out, which is a
	// normal race with an episode that has just finished, not a failure.
	statusProcessIsTerminating = 0xc000010a
	statusInvalidHandle        = 0xc0000008
)

var (
	ntdll            = windows.NewLazySystemDLL("ntdll.dll")
	ntSuspendProcess = ntdll.NewProc("NtSuspendProcess")
	ntResumeProcess  = ntdll.NewProc("NtResumeProcess")

	jobOnce   sync.Once
	jobHandle windows.Handle
	jobErr    error
)

func prepareBackgroundCommand(command *exec.Cmd) {
	command.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// Windows owns the countdown, so the machine still powers off if the app is
// closed while waiting.
const systemHandlesShutdownDelay = true

func scheduleSystemShutdown(delay time.Duration) error {
	seconds := int(delay.Seconds())
	if seconds < 0 {
		seconds = 0
	}
	command := exec.Command("shutdown", "/s", "/t", strconv.Itoa(seconds),
		"/c", "Video HTML Downloader: tải xong, máy sẽ tắt.")
	prepareBackgroundCommand(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("không hẹn được lệnh tắt máy: %w — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func cancelSystemShutdown() error {
	command := exec.Command("shutdown", "/a")
	prepareBackgroundCommand(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("không hủy được lệnh tắt máy: %w — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func killProcessTree(process *os.Process) error {
	if process == nil {
		return nil
	}
	// TerminateProcess reaches suspended processes too, so a paused download can
	// still be stopped — but it ends that one process only. yt-dlp runs its own
	// FFmpeg, which would keep writing the file after Dừng, so the whole tree is
	// taken down first, while the parent is still there to name its children.
	if process.Pid > 0 {
		command := exec.Command("taskkill", "/F", "/T", "/PID", strconv.Itoa(process.Pid))
		prepareBackgroundCommand(command)
		if err := command.Run(); err == nil {
			return nil
		}
	}
	// Windows answers "access denied" for a process that has already ended, and
	// these are our own children, so neither outcome is a real failure.
	if err := process.Kill(); err != nil &&
		!errors.Is(err, os.ErrProcessDone) && !errors.Is(err, windows.ERROR_ACCESS_DENIED) {
		return err
	}
	return nil
}

// superviseProcess ties FFmpeg to a job object that Windows tears down when the
// last handle closes, so the child can never outlive the app — not even when the
// app is force closed or crashes and leaves a download running.
func superviseProcess(process *os.Process) error {
	if process == nil {
		return fmt.Errorf("không tìm thấy tiến trình FFmpeg")
	}
	jobOnce.Do(createKillOnCloseJob)
	if jobErr != nil {
		return jobErr
	}
	handle, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(process.Pid))
	if err != nil {
		return fmt.Errorf("không thể giám sát FFmpeg: %w", err)
	}
	defer windows.CloseHandle(handle)
	if err := windows.AssignProcessToJobObject(jobHandle, handle); err != nil {
		return fmt.Errorf("không thể giám sát FFmpeg: %w", err)
	}
	return nil
}

func createKillOnCloseJob() {
	handle, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		jobErr = fmt.Errorf("không tạo được job object cho FFmpeg: %w", err)
		return
	}
	limits := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{
			LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE,
		},
	}
	if _, err := windows.SetInformationJobObject(handle, windows.JobObjectExtendedLimitInformation,
		uintptr(unsafe.Pointer(&limits)), uint32(unsafe.Sizeof(limits))); err != nil {
		_ = windows.CloseHandle(handle)
		jobErr = fmt.Errorf("không cấu hình được job object cho FFmpeg: %w", err)
		return
	}
	jobHandle = handle
}

func setProcessPaused(process *os.Process, paused bool) error {
	if process == nil {
		return fmt.Errorf("không tìm thấy tiến trình FFmpeg")
	}
	if process.Pid <= 0 {
		return errProcessGone
	}
	handle, err := windows.OpenProcess(processSuspendResume, false, uint32(process.Pid))
	if err != nil {
		// A PID that no longer exists fails here with "invalid parameter", which
		// only means the episode ended first.
		if errors.Is(err, windows.ERROR_INVALID_PARAMETER) {
			return errProcessGone
		}
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
	switch status {
	case 0:
		return nil
	case statusProcessIsTerminating, statusInvalidHandle:
		return errProcessGone
	default:
		return fmt.Errorf("không thể %s FFmpeg (NTSTATUS 0x%x)", operation, status)
	}
}
