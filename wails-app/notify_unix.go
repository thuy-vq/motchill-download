//go:build darwin || linux

package main

import (
	"fmt"
	"os/exec"
	"runtime"
	"strings"
)

// showNotification uses the notification centre of the desktop it runs on.
func showNotification(title, body string) error {
	var command *exec.Cmd
	if runtime.GOOS == "darwin" {
		script := fmt.Sprintf("display notification %q with title %q", body, title)
		command = exec.Command("osascript", "-e", script)
	} else {
		command = exec.Command("notify-send", title, body)
	}
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("không hiện được thông báo: %w — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}
