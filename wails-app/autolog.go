package main

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"
)

// keptLogFiles bounds the auto-saved log folder so long term use does not leave
// hundreds of files behind.
const keptLogFiles = 20

// autoLogger writes every log line to disk as it happens, so a crash or a power
// cut never loses the download history.
type autoLogger struct {
	mu   sync.Mutex
	dir  string
	path string
	file *os.File
}

func newAutoLogger() *autoLogger {
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = os.TempDir()
	}
	return &autoLogger{dir: filepath.Join(configRoot, "MotchillDownloader", "logs")}
}

func (l *autoLogger) directory() string {
	return l.dir
}

func (l *autoLogger) currentPath() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.path
}

// startSession closes the current file so the next line opens a fresh one.
func (l *autoLogger) startSession() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeFile()
}

func (l *autoLogger) append(line string) (string, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.file == nil {
		if err := l.open(); err != nil {
			return "", err
		}
	}
	if _, err := l.file.WriteString(logLine(line)); err != nil {
		return l.path, err
	}
	return l.path, nil
}

func (l *autoLogger) open() error {
	if err := os.MkdirAll(l.dir, 0o755); err != nil {
		return err
	}
	l.prune(keptLogFiles - 1)
	now := time.Now()
	path := filepath.Join(l.dir, fmt.Sprintf("VideoHtmlDownloader-v%s-%s.log", appVersion, now.Format("20060102-150405")))
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o600)
	if err != nil {
		return err
	}
	l.file = file
	l.path = path
	header := strings.Join([]string{
		fmt.Sprintf("Video HTML Downloader v%s", appVersion),
		fmt.Sprintf("Bắt đầu: %s", now.Format("02/01/2006 15:04:05")),
		strings.Repeat("-", 60),
	}, "\n")
	// UTF-8 BOM keeps Vietnamese text readable in older Windows Notepad.
	if _, err := file.Write([]byte{0xEF, 0xBB, 0xBF}); err != nil {
		return err
	}
	_, err = file.WriteString(logLine(header))
	return err
}

// prune keeps only the newest files so the folder stays small.
func (l *autoLogger) prune(keep int) {
	entries, err := os.ReadDir(l.dir)
	if err != nil {
		return
	}
	type logFile struct {
		path     string
		modified time.Time
	}
	files := make([]logFile, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(strings.ToLower(entry.Name()), ".log") {
			continue
		}
		info, statErr := entry.Info()
		if statErr != nil {
			continue
		}
		files = append(files, logFile{path: filepath.Join(l.dir, entry.Name()), modified: info.ModTime()})
	}
	if len(files) <= keep {
		return
	}
	sort.Slice(files, func(first, second int) bool { return files[first].modified.After(files[second].modified) })
	for _, file := range files[keep:] {
		if file.path != l.path {
			_ = os.Remove(file.path)
		}
	}
}

func (l *autoLogger) close() {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.closeFile()
}

func (l *autoLogger) closeFile() {
	if l.file != nil {
		_ = l.file.Close()
		l.file = nil
	}
}

// logLine terminates a line with the newline the host platform expects.
func logLine(value string) string {
	value = strings.ReplaceAll(strings.ReplaceAll(value, "\r\n", "\n"), "\n", newlineForHost())
	return value + newlineForHost()
}

func newlineForHost() string {
	if runtime.GOOS == "windows" {
		return "\r\n"
	}
	return "\n"
}
