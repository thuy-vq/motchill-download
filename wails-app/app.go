package main

import (
	"archive/zip"
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

const ffmpegDownloadURL = "https://www.gyan.dev/ffmpeg/builds/ffmpeg-release-essentials.zip"

type App struct {
	ctx          context.Context
	settings     *settingsStore
	autoLog      *autoLogger
	mu           sync.Mutex
	downloading  bool
	cancel       context.CancelFunc
	activeFFmpeg *os.Process
	paused       bool
}

func NewApp() *App {
	return &App{settings: newSettingsStore(), autoLog: newAutoLogger()}
}

func (a *App) startup(ctx context.Context) {
	a.ctx = ctx
}

func (a *App) shutdown(ctx context.Context) {
	a.CancelDownload()
	a.autoLog.close()
}

func (a *App) GetInitialState() InitialState {
	settings := a.settings.snapshot()
	ffmpeg := a.findFFmpeg()
	return InitialState{
		LastOutputDir: settings.LastOutputDir,
		FFmpegReady:   ffmpeg != "",
		FFmpegPath:    ffmpeg,
		Platform:      runtime.GOOS,
		Version:       appVersion,
		LogDir:        a.autoLog.directory(),
		LogPath:       a.autoLog.currentPath(),
	}
}

func (a *App) AnalyzeSource(source string) (AnalysisResult, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return AnalysisResult{}, fmt.Errorf("hãy nhập URL hoặc nội dung HTML")
	}
	if info, err := os.Stat(source); err == nil && !info.IsDir() {
		data, readErr := os.ReadFile(source)
		if readErr != nil {
			return AnalysisResult{}, readErr
		}
		return analyzeDocument(string(data), "", source)
	}
	if parsedStreams := extractMedia(source, source); len(parsedStreams) > 0 && !strings.Contains(source, "<") {
		return AnalysisResult{
			Title:       "Video",
			PageURL:     source,
			Streams:     parsedStreams,
			Episodes:    []Episode{{ID: "video", Name: "Video", PageURL: source, StreamURL: parsedStreams[0].URL, Current: true}},
			SourceLabel: source,
		}, nil
	}
	if strings.HasPrefix(strings.ToLower(source), "http://") || strings.HasPrefix(strings.ToLower(source), "https://") {
		htmlSource, err := fetchHTML(source)
		if err != nil {
			return AnalysisResult{}, err
		}
		return analyzeDocument(htmlSource, source, source)
	}
	return analyzeDocument(source, "", "HTML đã dán")
}

func (a *App) AnalyzeHTML(source string) (AnalysisResult, error) {
	return analyzeDocument(source, "", "HTML đã dán")
}

func analyzeDocument(source, fallbackURL, label string) (AnalysisResult, error) {
	result, analyzeErr := analyzeHTML(source, fallbackURL, label)
	pageURL := extractPageURL(source)
	if pageURL == "" {
		pageURL = fallbackURL
	}
	movieID := extractMovieID(source)
	if movieID == "" || pageURL == "" {
		return result, analyzeErr
	}
	episodes, indexErr := fetchEpisodeIndex(pageURL, movieID)
	if indexErr != nil || len(episodes) == 0 {
		return result, analyzeErr
	}
	if analyzeErr != nil {
		result = AnalysisResult{
			Title:       extractTitle(source),
			PageURL:     pageURL,
			Episodes:    episodes,
			HTMLBytes:   len(source),
			SourceLabel: label,
		}
	} else if len(episodes) >= len(result.Episodes) {
		result.Episodes = episodes
	}
	// Movie landing pages have no current player. Ignore media URLs from
	// unrelated recommendation cards rendered in the same HTML document.
	if episodeNumber(pageURL) == 0 {
		result.Streams = []MediaStream{}
	}
	return result, nil
}

func analyzeHTML(source, fallbackURL, label string) (AnalysisResult, error) {
	pageURL := extractPageURL(source)
	if pageURL == "" {
		pageURL = fallbackURL
	}
	streams := extractCurrentStreams(source, pageURL)
	if len(streams) == 0 {
		return AnalysisResult{}, fmt.Errorf("không tìm thấy luồng .m3u8, .mpd hoặc video trực tiếp")
	}
	episodes := extractEpisodeLinks(source, pageURL)
	if len(episodes) == 0 {
		number := episodeNumber(pageURL)
		name := "Video"
		if number > 0 {
			name = fmt.Sprintf("Tập %02d", number)
		}
		episodes = []Episode{{
			ID:        "current",
			Name:      name,
			Number:    number,
			PageURL:   pageURL,
			StreamURL: streams[0].URL,
			Current:   true,
		}}
	}
	return AnalysisResult{
		Title:       extractTitle(source),
		PageURL:     pageURL,
		Streams:     streams,
		Episodes:    episodes,
		HTMLBytes:   len(source),
		SourceLabel: label,
	}, nil
}

func (a *App) OpenHTMLFile() (SourceDocument, error) {
	selected, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title: "Mở mã HTML",
		Filters: []wailsruntime.FileFilter{
			{DisplayName: "HTML hoặc văn bản", Pattern: "*.html;*.htm;*.txt"},
			{DisplayName: "Tất cả file", Pattern: "*.*"},
		},
	})
	if err != nil || selected == "" {
		return SourceDocument{}, err
	}
	data, err := os.ReadFile(selected)
	if err != nil {
		return SourceDocument{}, err
	}
	return SourceDocument{Path: selected, Content: string(data)}, nil
}

func (a *App) SelectOutputDirectory() (string, error) {
	current := a.settings.snapshot().LastOutputDir
	selected, err := wailsruntime.OpenDirectoryDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:            "Chọn thư mục lưu video",
		DefaultDirectory: current,
	})
	if err != nil || selected == "" {
		return selected, err
	}
	if err := a.settings.setOutputDir(selected); err != nil {
		return "", err
	}
	return selected, nil
}

func (a *App) RememberOutputDirectory(directory string) error {
	directory = strings.TrimSpace(directory)
	if directory == "" {
		return fmt.Errorf("thư mục lưu đang trống")
	}
	if err := os.MkdirAll(directory, 0o755); err != nil {
		return err
	}
	return a.settings.setOutputDir(directory)
}

// AppendLog persists one line immediately; the frontend calls it for every line
// it shows so the nhật ký survives a crash without pressing "Lưu log".
func (a *App) AppendLog(line string) (string, error) {
	return a.autoLog.append(line)
}

// NewLogSession starts a fresh auto-saved file, one per download run.
func (a *App) NewLogSession() string {
	a.autoLog.startSession()
	return a.autoLog.directory()
}

func (a *App) GetLogPath() string {
	return a.autoLog.currentPath()
}

func (a *App) SaveLog(content string) (string, error) {
	content = strings.TrimSpace(content)
	if content == "" {
		return "", fmt.Errorf("nhật ký đang trống")
	}
	settings := a.settings.snapshot()
	filename := fmt.Sprintf("VideoHtmlDownloader-v%s-%s.log", appVersion, time.Now().Format("20060102-150405"))
	selected, err := wailsruntime.SaveFileDialog(a.ctx, wailsruntime.SaveDialogOptions{
		Title:            "Lưu nhật ký tải",
		DefaultDirectory: settings.LastOutputDir,
		DefaultFilename:  filename,
		Filters:          []wailsruntime.FileFilter{{DisplayName: "Nhật ký", Pattern: "*.log"}, {DisplayName: "Văn bản", Pattern: "*.txt"}},
	})
	if err != nil || selected == "" {
		return selected, err
	}
	if err := os.MkdirAll(filepath.Dir(selected), 0o755); err != nil {
		return "", err
	}
	if err := writeLogFile(selected, content); err != nil {
		return "", err
	}
	return selected, nil
}

func writeLogFile(path, content string) error {
	content = strings.ReplaceAll(strings.ReplaceAll(content, "\r\n", "\n"), "\n", newlineForHost())
	// UTF-8 BOM keeps Vietnamese text readable in older Windows Notepad.
	return os.WriteFile(path, append([]byte{0xEF, 0xBB, 0xBF}, []byte(content)...), 0o600)
}

func (a *App) GetFFmpegStatus() FFmpegStatus {
	path := a.findFFmpeg()
	return FFmpegStatus{Ready: path != "", Path: path}
}

func (a *App) ChooseFFmpeg() (FFmpegStatus, error) {
	pattern := "ffmpeg.exe"
	if runtime.GOOS == "darwin" {
		pattern = "*"
	}
	selected, err := wailsruntime.OpenFileDialog(a.ctx, wailsruntime.OpenDialogOptions{
		Title:   "Chọn ffmpeg.exe",
		Filters: []wailsruntime.FileFilter{{DisplayName: "FFmpeg", Pattern: pattern}},
	})
	if err != nil || selected == "" {
		return a.GetFFmpegStatus(), err
	}
	if err := verifyFFmpeg(selected); err != nil {
		return FFmpegStatus{}, err
	}
	if err := a.settings.setFFmpegPath(selected); err != nil {
		return FFmpegStatus{}, err
	}
	return FFmpegStatus{Ready: true, Path: selected}, nil
}

func (a *App) InstallFFmpeg() (FFmpegStatus, error) {
	if runtime.GOOS == "darwin" {
		return a.installFFmpegWithHomebrew()
	}
	if runtime.GOOS != "windows" {
		return FFmpegStatus{}, fmt.Errorf("hãy cài FFmpeg bằng trình quản lý gói của hệ điều hành rồi chọn file thực thi")
	}
	installRoot := filepath.Join(os.Getenv("LOCALAPPDATA"), "MotchillDownloader")
	if installRoot == "MotchillDownloader" {
		configRoot, _ := os.UserConfigDir()
		installRoot = filepath.Join(configRoot, "MotchillDownloader")
	}
	if err := os.MkdirAll(installRoot, 0o755); err != nil {
		return FFmpegStatus{}, err
	}
	zipPath := filepath.Join(os.TempDir(), "ffmpeg-"+randomSuffix()+".zip")
	temporaryExe := filepath.Join(installRoot, "ffmpeg.exe.new")
	finalExe := filepath.Join(installRoot, "ffmpeg.exe")
	defer os.Remove(zipPath)
	defer os.Remove(temporaryExe)

	resp, err := http.Get(ffmpegDownloadURL)
	if err != nil {
		return FFmpegStatus{}, fmt.Errorf("không thể tải FFmpeg: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return FFmpegStatus{}, fmt.Errorf("máy chủ FFmpeg trả về HTTP %d", resp.StatusCode)
	}
	output, err := os.Create(zipPath)
	if err != nil {
		return FFmpegStatus{}, err
	}
	reader := &progressReader{reader: resp.Body, total: resp.ContentLength, emit: func(downloaded, total int64) {
		wailsruntime.EventsEmit(a.ctx, "ffmpeg:progress", map[string]int64{"downloaded": downloaded, "total": total})
	}}
	_, copyErr := io.Copy(output, reader)
	closeErr := output.Close()
	if copyErr != nil {
		return FFmpegStatus{}, copyErr
	}
	if closeErr != nil {
		return FFmpegStatus{}, closeErr
	}

	archive, err := zip.OpenReader(zipPath)
	if err != nil {
		return FFmpegStatus{}, err
	}
	var entry *zip.File
	for _, file := range archive.File {
		name := strings.ReplaceAll(file.Name, "\\", "/")
		if strings.HasSuffix(strings.ToLower(name), "/bin/ffmpeg.exe") {
			entry = file
			break
		}
	}
	if entry == nil {
		archive.Close()
		return FFmpegStatus{}, fmt.Errorf("gói tải về không chứa ffmpeg.exe")
	}
	input, err := entry.Open()
	if err != nil {
		archive.Close()
		return FFmpegStatus{}, err
	}
	executable, err := os.Create(temporaryExe)
	if err != nil {
		input.Close()
		archive.Close()
		return FFmpegStatus{}, err
	}
	_, err = io.Copy(executable, input)
	input.Close()
	executable.Close()
	archive.Close()
	if err != nil {
		return FFmpegStatus{}, err
	}
	if err := verifyFFmpeg(temporaryExe); err != nil {
		return FFmpegStatus{}, err
	}
	_ = os.Remove(finalExe)
	if err := os.Rename(temporaryExe, finalExe); err != nil {
		return FFmpegStatus{}, err
	}
	if err := a.settings.setFFmpegPath(finalExe); err != nil {
		return FFmpegStatus{}, err
	}
	return FFmpegStatus{Ready: true, Path: finalExe}, nil
}

func (a *App) installFFmpegWithHomebrew() (FFmpegStatus, error) {
	brew := ""
	if found, err := exec.LookPath("brew"); err == nil {
		brew = found
	}
	if brew == "" {
		for _, candidate := range []string{"/opt/homebrew/bin/brew", "/usr/local/bin/brew"} {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				brew = candidate
				break
			}
		}
	}
	if brew == "" {
		return FFmpegStatus{}, fmt.Errorf("không tìm thấy Homebrew; hãy chạy 'brew install ffmpeg' hoặc dùng nút Chọn file")
	}
	command := exec.Command(brew, "install", "ffmpeg")
	output, err := command.CombinedOutput()
	if err != nil {
		message := strings.TrimSpace(string(output))
		if len(message) > 1000 {
			message = message[len(message)-1000:]
		}
		return FFmpegStatus{}, fmt.Errorf("Homebrew không cài được FFmpeg: %w — %s", err, message)
	}
	path := a.findFFmpeg()
	if path == "" {
		return FFmpegStatus{}, fmt.Errorf("Homebrew đã chạy nhưng không tìm thấy lệnh ffmpeg")
	}
	if err := a.settings.setFFmpegPath(path); err != nil {
		return FFmpegStatus{}, err
	}
	return FFmpegStatus{Ready: true, Path: path}, nil
}

func (a *App) findFFmpeg() string {
	settings := a.settings.snapshot()
	candidates := []string{settings.FFmpegPath}
	executableName := "ffmpeg"
	if runtime.GOOS == "windows" {
		executableName = "ffmpeg.exe"
		candidates = append(candidates, filepath.Join(os.Getenv("LOCALAPPDATA"), "MotchillDownloader", executableName))
	} else if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/ffmpeg", "/usr/local/bin/ffmpeg")
	}
	candidates = append(candidates, filepath.Join(executableDirectory(), executableName))
	for _, candidate := range candidates {
		if candidate != "" {
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	if candidate, err := exec.LookPath(executableName); err == nil {
		return candidate
	}
	return ""
}

func verifyFFmpeg(path string) error {
	command := exec.Command(path, "-version")
	prepareBackgroundCommand(command)
	if err := command.Run(); err != nil {
		return fmt.Errorf("ffmpeg.exe không chạy được: %w", err)
	}
	return nil
}

func executableDirectory() string {
	executable, err := os.Executable()
	if err != nil {
		return "."
	}
	return filepath.Dir(executable)
}

func randomSuffix() string {
	file, err := os.CreateTemp("", "motchill-id-")
	if err != nil {
		return fmt.Sprintf("%d", os.Getpid())
	}
	name := filepath.Base(file.Name())
	file.Close()
	os.Remove(file.Name())
	return name
}

type progressReader struct {
	reader io.Reader
	total  int64
	read   int64
	last   int64
	emit   func(int64, int64)
}

func (p *progressReader) Read(buffer []byte) (int, error) {
	count, err := p.reader.Read(buffer)
	p.read += int64(count)
	now := time.Now().UnixMilli()
	if p.emit != nil && (now-p.last >= 200 || (p.total > 0 && p.read >= p.total)) {
		p.last = now
		p.emit(p.read, p.total)
	}
	return count, err
}

func (a *App) RevealFile(path string) error {
	directory, err := folderForReveal(path)
	if err != nil {
		return err
	}
	switch runtime.GOOS {
	case "windows":
		return exec.Command("explorer.exe", directory).Start()
	case "darwin":
		return exec.Command("open", directory).Start()
	default:
		return exec.Command("xdg-open", directory).Start()
	}
}

func folderForReveal(value string) (string, error) {
	value = strings.TrimSpace(value)
	if value == "" {
		return "", fmt.Errorf("đường dẫn đầu ra đang trống")
	}
	absolute, err := filepath.Abs(value)
	if err != nil {
		return "", err
	}
	if info, statErr := os.Stat(absolute); statErr == nil {
		if info.IsDir() {
			return absolute, nil
		}
		return filepath.Dir(absolute), nil
	}
	directory := filepath.Dir(absolute)
	if info, statErr := os.Stat(directory); statErr == nil && info.IsDir() {
		return directory, nil
	}
	return "", fmt.Errorf("thư mục không còn tồn tại: %s", directory)
}
