//go:build darwin || linux

package main

// Neither macOS nor Linux exposes a per-window progress bar the way the Windows
// taskbar does; the percentage in the window title carries the information there.
func setTaskbarProgress(percent int, state string) {}
