//go:build windows

package main

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"testing"
	"time"

	"golang.org/x/sys/windows"
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

// The app used to leave FFmpeg running after a crash or a forced close. The job
// object must take the supervised child down with the process that started it.
func TestSupervisedProcessDiesWithItsParent(t *testing.T) {
	switch os.Getenv("MOTCHILL_JOB_ROLE") {
	case "child":
		time.Sleep(60 * time.Second)
		return
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=TestSupervisedProcessDiesWithItsParent")
		child.Env = append(os.Environ(), "MOTCHILL_JOB_ROLE=child")
		prepareBackgroundCommand(child)
		if err := child.Start(); err != nil {
			return
		}
		if err := superviseProcess(child.Process); err != nil {
			return
		}
		fmt.Printf("CHILDPID:%d\n", child.Process.Pid)
		time.Sleep(60 * time.Second)
		return
	}

	parent := exec.Command(os.Args[0], "-test.run=TestSupervisedProcessDiesWithItsParent")
	parent.Env = append(os.Environ(), "MOTCHILL_JOB_ROLE=parent")
	prepareBackgroundCommand(parent)
	output, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = parent.Process.Kill()
		_, _ = parent.Process.Wait()
	}()

	childPid := 0
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		if value, found := strings.CutPrefix(scanner.Text(), "CHILDPID:"); found {
			childPid, _ = strconv.Atoi(strings.TrimSpace(value))
			break
		}
	}
	if childPid == 0 {
		t.Fatal("supervised child never reported its pid")
	}
	if !processIsRunning(childPid) {
		t.Fatal("supervised child was expected to be running")
	}

	// Force close the parent the way Task Manager or a crash would.
	if err := parent.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for processIsRunning(childPid) {
		if time.Now().After(deadline) {
			_, _ = exec.Command("taskkill", "/PID", strconv.Itoa(childPid), "/F").Output()
			t.Fatal("supervised child outlived the app instead of being killed with it")
		}
		time.Sleep(100 * time.Millisecond)
	}
}

func processIsRunning(pid int) bool {
	handle, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION, false, uint32(pid))
	if err != nil {
		return false
	}
	defer windows.CloseHandle(handle)
	var code uint32
	if err := windows.GetExitCodeProcess(handle, &code); err != nil {
		return false
	}
	const stillActive = 259
	return code == stillActive
}

// The pause button used to report "NTSTATUS 0xc000010a" whenever the episode
// ended a moment before the click landed. A process that is already gone is not
// a failure the user should see.
func TestPausingAnEndedProcessIsNotAnError(t *testing.T) {
	if os.Getenv("MOTCHILL_GONE_HELPER") == "1" {
		return
	}
	command := exec.Command(os.Args[0], "-test.run=TestPausingAnEndedProcessIsNotAnError")
	command.Env = append(os.Environ(), "MOTCHILL_GONE_HELPER=1")
	prepareBackgroundCommand(command)
	if err := command.Start(); err != nil {
		t.Fatal(err)
	}
	process := command.Process
	if err := command.Wait(); err != nil {
		t.Fatal(err)
	}
	if err := setProcessPaused(process, true); !errors.Is(err, errProcessGone) {
		t.Fatalf("pausing a finished process = %v, want errProcessGone", err)
	}
	if err := setProcessPaused(process, false); !errors.Is(err, errProcessGone) {
		t.Fatalf("resuming a finished process = %v, want errProcessGone", err)
	}
}

// Stopping has to reach the grandchildren too: yt-dlp spawns its own FFmpeg, and
// killing only yt-dlp left that FFmpeg writing to the file after Dừng.
func TestKillProcessTreeReachesGrandchildren(t *testing.T) {
	switch os.Getenv("MOTCHILL_TREE_ROLE") {
	case "child":
		time.Sleep(60 * time.Second)
		return
	case "parent":
		child := exec.Command(os.Args[0], "-test.run=TestKillProcessTreeReachesGrandchildren")
		child.Env = append(os.Environ(), "MOTCHILL_TREE_ROLE=child")
		prepareBackgroundCommand(child)
		if err := child.Start(); err != nil {
			return
		}
		fmt.Printf("CHILDPID:%d\n", child.Process.Pid)
		time.Sleep(60 * time.Second)
		return
	}

	parent := exec.Command(os.Args[0], "-test.run=TestKillProcessTreeReachesGrandchildren")
	parent.Env = append(os.Environ(), "MOTCHILL_TREE_ROLE=parent")
	prepareBackgroundCommand(parent)
	output, err := parent.StdoutPipe()
	if err != nil {
		t.Fatal(err)
	}
	if err := parent.Start(); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_ = parent.Process.Kill()
		_, _ = parent.Process.Wait()
	}()

	childPid := 0
	scanner := bufio.NewScanner(output)
	for scanner.Scan() {
		if value, found := strings.CutPrefix(scanner.Text(), "CHILDPID:"); found {
			childPid, _ = strconv.Atoi(strings.TrimSpace(value))
			break
		}
	}
	if childPid == 0 {
		t.Fatal("the child never reported its pid")
	}

	if err := killProcessTree(parent.Process); err != nil {
		t.Fatal(err)
	}
	deadline := time.Now().Add(10 * time.Second)
	for processIsRunning(childPid) {
		if time.Now().After(deadline) {
			_, _ = exec.Command("taskkill", "/PID", strconv.Itoa(childPid), "/F").Output()
			t.Fatal("the child survived killProcessTree")
		}
		time.Sleep(100 * time.Millisecond)
	}
}
