package main

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestAutoLoggerWritesEveryLineImmediately(t *testing.T) {
	logger := &autoLogger{dir: filepath.Join(t.TempDir(), "logs")}
	defer logger.close()

	path, err := logger.append("Tập 01: hoàn tất")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := logger.append("Tập 02: lỗi"); err != nil {
		t.Fatal(err)
	}

	// Read while the file is still open: lines must already be on disk.
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.HasPrefix(data, []byte{0xEF, 0xBB, 0xBF}) {
		t.Fatal("auto log must start with a UTF-8 BOM")
	}
	for _, expected := range []string{"Video HTML Downloader v", "Tập 01: hoàn tất", "Tập 02: lỗi"} {
		if !bytes.Contains(data, []byte(expected)) {
			t.Fatalf("auto log is missing %q", expected)
		}
	}
}

func TestAutoLoggerStartsNewFilePerSession(t *testing.T) {
	logger := &autoLogger{dir: filepath.Join(t.TempDir(), "logs")}
	defer logger.close()

	first, err := logger.append("phiên 1")
	if err != nil {
		t.Fatal(err)
	}
	logger.startSession()
	// File names carry a second-resolution stamp, so wait for a distinct name.
	time.Sleep(1100 * time.Millisecond)
	second, err := logger.append("phiên 2")
	if err != nil {
		t.Fatal(err)
	}
	if first == second {
		t.Fatal("a new session must open a new log file")
	}
	if logger.currentPath() != second {
		t.Fatalf("currentPath = %q, want %q", logger.currentPath(), second)
	}
}

func TestAutoLoggerPrunesOldFiles(t *testing.T) {
	directory := filepath.Join(t.TempDir(), "logs")
	if err := os.MkdirAll(directory, 0o755); err != nil {
		t.Fatal(err)
	}
	for index := 0; index < 5; index++ {
		path := filepath.Join(directory, fmt.Sprintf("old-%d.log", index))
		if err := os.WriteFile(path, []byte("old"), 0o600); err != nil {
			t.Fatal(err)
		}
		stamp := time.Now().Add(-time.Duration(index+1) * time.Hour)
		if err := os.Chtimes(path, stamp, stamp); err != nil {
			t.Fatal(err)
		}
	}

	logger := &autoLogger{dir: directory}
	logger.prune(2)

	entries, err := os.ReadDir(directory)
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 2 {
		t.Fatalf("kept %d log files, want 2", len(entries))
	}
	// The two newest are old-0 and old-1.
	for _, entry := range entries {
		if entry.Name() != "old-0.log" && entry.Name() != "old-1.log" {
			t.Fatalf("pruned the wrong file, %s should have been removed", entry.Name())
		}
	}
}
