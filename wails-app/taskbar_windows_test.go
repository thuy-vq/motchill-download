//go:build windows

package main

import (
	"runtime"
	"syscall"
	"testing"
	"unsafe"

	"github.com/go-ole/go-ole"
	"golang.org/x/sys/windows"
)

// The COM plumbing is verified against a real window of this process, so a wrong
// vtable slot or GUID fails here instead of silently doing nothing at runtime.
func TestTaskbarClientAcceptsProgressUpdates(t *testing.T) {
	runtime.LockOSThread()
	defer runtime.UnlockOSThread()
	_ = ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	defer ole.CoUninitialize()

	window, cleanup := createTestWindow(t)
	defer cleanup()

	client, err := newTaskbarClient()
	if err != nil {
		t.Fatal(err)
	}
	defer client.close()
	for _, testCase := range []struct {
		name      string
		state     int
		completed uint64
	}{
		{"normal", taskbarNormal, 42},
		{"paused", taskbarPaused, 42},
		{"error", taskbarError, 100},
		{"indeterminate", taskbarIndeterminate, 0},
		{"cleared", taskbarNoProgress, 0},
	} {
		if err := client.set(window, testCase.completed, 100, testCase.state); err != nil {
			t.Fatalf("%s: %v", testCase.name, err)
		}
	}
}

type testWindowClass struct {
	Size       uint32
	Style      uint32
	WndProc    uintptr
	ClsExtra   int32
	WndExtra   int32
	Instance   windows.Handle
	Icon       windows.Handle
	Cursor     windows.Handle
	Background windows.Handle
	MenuName   *uint16
	ClassName  *uint16
	IconSmall  windows.Handle
}

// createTestWindow puts a real taskbar-visible window on screen so the COM calls
// operate on the same kind of handle they get in the app.
func createTestWindow(t *testing.T) (windows.HWND, func()) {
	t.Helper()
	const (
		wsOverlappedWindow = 0x00CF0000
		wsExAppWindow      = 0x00040000
		swShowMinNoActive  = 7
	)
	defWindowProc := user32.NewProc("DefWindowProcW")
	className, err := windows.UTF16PtrFromString("MotchillTaskbarProgressTest")
	if err != nil {
		t.Fatal(err)
	}
	windowName, err := windows.UTF16PtrFromString("Motchill taskbar test")
	if err != nil {
		t.Fatal(err)
	}
	moduleHandle, _, moduleErr := windows.NewLazySystemDLL("kernel32.dll").NewProc("GetModuleHandleW").Call(0)
	if moduleHandle == 0 {
		t.Skipf("cannot resolve the module handle: %v", moduleErr)
	}
	instance := windows.Handle(moduleHandle)
	class := testWindowClass{
		Size: uint32(unsafe.Sizeof(testWindowClass{})),
		WndProc: syscall.NewCallback(func(window windows.HWND, message uint32, wparam, lparam uintptr) uintptr {
			result, _, _ := defWindowProc.Call(uintptr(window), uintptr(message), wparam, lparam)
			return result
		}),
		Instance:  instance,
		ClassName: className,
	}
	atom, _, callErr := user32.NewProc("RegisterClassExW").Call(uintptr(unsafe.Pointer(&class)))
	if atom == 0 {
		t.Skipf("cannot register a window class here: %v", callErr)
	}
	handle, _, callErr := user32.NewProc("CreateWindowExW").Call(
		wsExAppWindow, uintptr(unsafe.Pointer(className)), uintptr(unsafe.Pointer(windowName)),
		wsOverlappedWindow, 0, 0, 320, 200, 0, 0, uintptr(instance), 0)
	if handle == 0 {
		user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
		t.Skipf("cannot create a window here: %v", callErr)
	}
	// Minimised but shown, so the taskbar button exists without stealing focus.
	user32.NewProc("ShowWindow").Call(handle, swShowMinNoActive)
	return windows.HWND(handle), func() {
		user32.NewProc("DestroyWindow").Call(handle)
		user32.NewProc("UnregisterClassW").Call(uintptr(unsafe.Pointer(className)), uintptr(instance))
	}
}

func TestTaskbarStateFlagMapping(t *testing.T) {
	cases := map[string]int{
		"normal": taskbarNormal, "paused": taskbarPaused, "error": taskbarError,
		"indeterminate": taskbarIndeterminate, "none": taskbarNoProgress, "": taskbarNoProgress,
	}
	for state, expected := range cases {
		if got := taskbarStateFlag(state); got != expected {
			t.Fatalf("taskbarStateFlag(%q) = %#x, want %#x", state, got, expected)
		}
	}
}

func TestFindMainWindowOnlyReturnsOurOwnWindow(t *testing.T) {
	created, cleanup := createTestWindow(t)
	defer cleanup()

	window := findMainWindow()
	if window == 0 {
		t.Fatalf("the window %#x of this process was not found", created)
	}
	var owner uint32
	_, _, _ = procGetWindowThreadProcessID.Call(uintptr(window), uintptr(unsafe.Pointer(&owner)))
	if owner != windows.GetCurrentProcessId() {
		t.Fatalf("found a window of process %d, want %d", owner, windows.GetCurrentProcessId())
	}
}

// setTaskbarProgress must never block or panic, even with no window yet.
func TestSetTaskbarProgressIsSafeToSpam(t *testing.T) {
	for percent := 0; percent <= 100; percent += 10 {
		setTaskbarProgress(percent, "normal")
	}
	setTaskbarProgress(-5, "paused")
	setTaskbarProgress(150, "error")
	setTaskbarProgress(0, "none")
}
