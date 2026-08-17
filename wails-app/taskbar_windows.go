//go:build windows

package main

import (
	"fmt"
	"runtime"
	"sync"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// ITaskbarList3 progress states, as documented for SetProgressState.
const (
	taskbarNoProgress    = 0x0
	taskbarIndeterminate = 0x1
	taskbarNormal        = 0x2
	taskbarError         = 0x4
	taskbarPaused        = 0x8
)

// Vtable slots of ITaskbarList3: IUnknown holds 0-2, ITaskbarList 3-7,
// ITaskbarList2 adds 8, so ITaskbarList3 starts at 9.
const (
	slotHrInit           = 3
	slotSetProgressValue = 9
	slotSetProgressState = 10
)

var (
	clsidTaskbarList = ole.NewGUID("{56FDF344-FD6D-11D0-958A-006097C9A090}")
	iidTaskbarList3  = ole.NewGUID("{EA1AFB91-9E28-4B86-90E9-9E9F8A5EEFAF}")

	user32                       = windows.NewLazySystemDLL("user32.dll")
	procEnumWindows              = user32.NewProc("EnumWindows")
	procGetWindowThreadProcessID = user32.NewProc("GetWindowThreadProcessId")
	procIsWindowVisible          = user32.NewProc("IsWindowVisible")
	procGetWindowTextLength      = user32.NewProc("GetWindowTextLengthW")

	taskbarOnce  sync.Once
	taskbarQueue chan taskbarUpdate
)

type taskbarUpdate struct {
	percent int
	state   string
}

// taskbarClient owns the COM object; it must stay on the thread that created it.
type taskbarClient struct {
	unknown *ole.IUnknown
}

func newTaskbarClient() (*taskbarClient, error) {
	unknown, err := ole.CreateInstance(clsidTaskbarList, iidTaskbarList3)
	if err != nil {
		return nil, fmt.Errorf("không tạo được ITaskbarList3: %w", err)
	}
	client := &taskbarClient{unknown: unknown}
	if err := client.call(slotHrInit); err != nil {
		unknown.Release()
		return nil, fmt.Errorf("HrInit thất bại: %w", err)
	}
	return client, nil
}

func (c *taskbarClient) call(slot int, args ...uintptr) error {
	vtable := *(**[16]uintptr)(unsafe.Pointer(c.unknown))
	arguments := append([]uintptr{uintptr(unsafe.Pointer(c.unknown))}, args...)
	result, _, _ := syscall.SyscallN(vtable[slot], arguments...)
	if result != 0 {
		return ole.NewError(result)
	}
	return nil
}

// set paints the progress bar drawn over the taskbar button of a window.
func (c *taskbarClient) set(window windows.HWND, completed, total uint64, state int) error {
	if err := c.call(slotSetProgressState, uintptr(window), uintptr(state)); err != nil {
		return err
	}
	if state == taskbarNoProgress || state == taskbarIndeterminate {
		return nil
	}
	return c.call(slotSetProgressValue, uintptr(window), uintptr(completed), uintptr(total))
}

func (c *taskbarClient) close() {
	if c.unknown != nil {
		c.unknown.Release()
		c.unknown = nil
	}
}

// setTaskbarProgress is safe to call from anywhere; updates are handed to a
// single COM thread and dropped when that thread is busy.
func setTaskbarProgress(percent int, state string) {
	taskbarOnce.Do(startTaskbarWorker)
	select {
	case taskbarQueue <- taskbarUpdate{percent: percent, state: state}:
	default:
	}
}

func startTaskbarWorker() {
	taskbarQueue = make(chan taskbarUpdate, 8)
	go func() {
		// COM apartments belong to a thread, so this goroutine keeps one.
		runtime.LockOSThread()
		defer runtime.UnlockOSThread()
		// S_FALSE means the apartment already exists, which is fine.
		_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
		defer ole.CoUninitialize()
		client, err := newTaskbarClient()
		if err != nil {
			return
		}
		defer client.close()
		var window windows.HWND
		for update := range taskbarQueue {
			if window == 0 {
				// The window only exists once the frontend has been shown.
				if window = findMainWindow(); window == 0 {
					continue
				}
			}
			percent := update.percent
			if percent < 0 {
				percent = 0
			}
			if percent > 100 {
				percent = 100
			}
			_ = client.set(window, uint64(percent), 100, taskbarStateFlag(update.state))
		}
	}()
}

func taskbarStateFlag(state string) int {
	switch state {
	case "normal":
		return taskbarNormal
	case "paused":
		return taskbarPaused
	case "error":
		return taskbarError
	case "indeterminate":
		return taskbarIndeterminate
	default:
		return taskbarNoProgress
	}
}

// findMainWindow returns the visible titled top-level window of this process,
// which for a Wails app is its single window.
func findMainWindow() windows.HWND {
	current := windows.GetCurrentProcessId()
	var found windows.HWND
	callback := syscall.NewCallback(func(window windows.HWND, _ uintptr) uintptr {
		var owner uint32
		_, _, _ = procGetWindowThreadProcessID.Call(uintptr(window), uintptr(unsafe.Pointer(&owner)))
		if owner != current {
			return 1
		}
		if visible, _, _ := procIsWindowVisible.Call(uintptr(window)); visible == 0 {
			return 1
		}
		if length, _, _ := procGetWindowTextLength.Call(uintptr(window)); length == 0 {
			return 1
		}
		found = window
		return 0
	})
	_, _, _ = procEnumWindows.Call(callback, 0)
	return found
}
