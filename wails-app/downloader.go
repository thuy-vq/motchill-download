package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	ffmpegTimePattern  = regexp.MustCompile(`time=\s*([^\s]+)`)
	ffmpegSpeedPattern = regexp.MustCompile(`speed=\s*([^\s]+)`)
	invalidFileChars   = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
)

func (a *App) StartDownload(request DownloadRequest) error {
	if len(request.Items) == 0 {
		return fmt.Errorf("chưa chọn tập nào")
	}
	rememberedDirectory := strings.TrimSpace(request.OutputDir)
	for index := range request.Items {
		directory := strings.TrimSpace(request.Items[index].OutputDir)
		if directory == "" {
			directory = rememberedDirectory
			request.Items[index].OutputDir = directory
		}
		if directory == "" {
			return fmt.Errorf("%s chưa có thư mục lưu", request.Items[index].Title)
		}
		if err := os.MkdirAll(directory, 0o755); err != nil {
			return err
		}
		if rememberedDirectory == "" {
			rememberedDirectory = directory
		}
	}
	ffmpeg := a.findFFmpeg()
	if ffmpeg == "" {
		return fmt.Errorf("chưa có FFmpeg; hãy cài hoặc chọn ffmpeg.exe")
	}
	if err := a.settings.setOutputDir(rememberedDirectory); err != nil {
		return err
	}

	a.mu.Lock()
	if a.downloading {
		a.mu.Unlock()
		return fmt.Errorf("một hàng đợi khác đang chạy")
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloading = true
	a.cancel = cancel
	a.paused = false
	a.activeFFmpeg = nil
	a.mu.Unlock()

	go a.runQueue(ctx, ffmpeg, request)
	return nil
}

func (a *App) CancelDownload() {
	a.mu.Lock()
	cancel := a.cancel
	process := a.activeFFmpeg
	wasPaused := a.paused
	a.paused = false
	a.mu.Unlock()
	if wasPaused && process != nil {
		_ = setProcessPaused(process, false)
	}
	if cancel != nil {
		cancel()
	}
}

func (a *App) PauseDownload() (DownloadControlStatus, error) {
	a.mu.Lock()
	if !a.downloading {
		a.mu.Unlock()
		return DownloadControlStatus{}, fmt.Errorf("không có hàng đợi đang tải")
	}
	process := a.activeFFmpeg
	targetPaused := !a.paused
	a.mu.Unlock()
	if process == nil {
		return DownloadControlStatus{}, fmt.Errorf("đang chuyển tập, hãy thử lại sau một lát")
	}
	if err := setProcessPaused(process, targetPaused); err != nil {
		return DownloadControlStatus{}, err
	}
	a.mu.Lock()
	if a.activeFFmpeg == process {
		a.paused = targetPaused
	}
	paused := a.paused
	a.mu.Unlock()
	return DownloadControlStatus{Paused: paused}, nil
}

func (a *App) setActiveFFmpeg(process *os.Process) {
	a.mu.Lock()
	a.activeFFmpeg = process
	a.paused = false
	a.mu.Unlock()
}

func (a *App) clearActiveFFmpeg(process *os.Process) {
	a.mu.Lock()
	if a.activeFFmpeg == process {
		a.activeFFmpeg = nil
		a.paused = false
	}
	a.mu.Unlock()
}

func (a *App) runQueue(ctx context.Context, ffmpeg string, request DownloadRequest) {
	defer func() {
		a.mu.Lock()
		a.downloading = false
		a.cancel = nil
		a.activeFFmpeg = nil
		a.paused = false
		a.mu.Unlock()
	}()

	total := len(request.Items)
	completed, failed, skipped := 0, 0, 0
	usedStreams := map[string]string{}
	fingerprints := map[string]string{}
	width := len(fmt.Sprintf("%d", maxEpisodeNumber(request.Items)))
	if width < 2 {
		width = 2
	}
	for index, item := range request.Items {
		if ctx.Err() != nil {
			break
		}
		name := item.Name
		if name == "" {
			name = fmt.Sprintf("Tập %02d", item.Number)
		}
		movieTitle := strings.TrimSpace(item.Title)
		if movieTitle == "" {
			movieTitle = request.Title
		}
		baseTitle := sanitizeFileName(movieTitle)
		if baseTitle == "" {
			baseTitle = "Video"
		}
		outputName := baseTitle
		if total > 1 || item.Number > 0 {
			if item.Number > 0 {
				outputName += fmt.Sprintf(" - Tập %0*d", width, item.Number)
			} else {
				outputName += " - " + sanitizeFileName(name)
			}
		}
		outputPath := filepath.Join(item.OutputDir, outputName+".mp4")
		displayName := movieTitle + " · " + name
		event := QueueEvent{ID: item.ID, Movie: movieTitle, Index: index + 1, Total: total, Name: name, Status: "resolving", Output: outputPath, Completed: completed, Failed: failed, Skipped: skipped}
		wailsruntime.EventsEmit(a.ctx, "download:queue", event)

		if request.SkipExisting {
			if info, err := os.Stat(outputPath); err == nil && info.Size() > 0 {
				skipped++
				event.Status = "skipped"
				event.Skipped = skipped
				event.Message = "File đã tồn tại"
				wailsruntime.EventsEmit(a.ctx, "download:queue", event)
				continue
			}
		}

		streams, err := a.resolveStreams(item, request.PreferredServer)
		if err != nil || len(streams) == 0 {
			failed++
			event.Status = "failed"
			event.Failed = failed
			if err != nil {
				event.Message = err.Error()
			} else {
				event.Message = "Không tìm thấy luồng video"
			}
			wailsruntime.EventsEmit(a.ctx, "download:queue", event)
			continue
		}

		event.Status = "downloading"
		wailsruntime.EventsEmit(a.ctx, "download:queue", event)
		var lastErr error
		for _, stream := range streams {
			if ctx.Err() != nil {
				break
			}
			identity := streamIdentity(stream.URL)
			if previousEpisode, exists := usedStreams[identity]; exists && previousEpisode != displayName {
				lastErr = fmt.Errorf("server trả lại cùng link đã dùng cho %s", previousEpisode)
				wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf("%s — bỏ qua link trùng với %s", displayName, previousEpisode))
				continue
			}
			wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf("%s — thử server %s: %s", displayName, displayServer(stream), stream.URL))
			lastErr = a.downloadStream(ctx, ffmpeg, stream.URL, item.PageURL, outputPath, index+1, total, displayName)
			if lastErr == nil {
				usedStreams[identity] = displayName
				break
			}
			wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf("%s — server lỗi: %v", displayName, lastErr))
		}
		if ctx.Err() != nil {
			break
		}
		if lastErr == nil {
			fingerprint, fingerprintErr := videoFingerprint(outputPath)
			if fingerprintErr != nil {
				_ = os.Remove(outputPath)
				lastErr = fmt.Errorf("không thể kiểm tra file đầu ra: %w", fingerprintErr)
			} else if previousEpisode, exists := fingerprints[fingerprint]; exists && previousEpisode != displayName {
				_ = os.Remove(outputPath)
				lastErr = fmt.Errorf("video giống hệt %s; đã xóa bản trùng để tránh nhầm tập", previousEpisode)
			} else {
				fingerprints[fingerprint] = displayName
			}
		}
		if lastErr != nil {
			failed++
			event.Status = "failed"
			event.Failed = failed
			event.Message = lastErr.Error()
		} else {
			completed++
			event.Status = "completed"
			event.Completed = completed
		}
		wailsruntime.EventsEmit(a.ctx, "download:queue", event)
	}

	wailsruntime.EventsEmit(a.ctx, "download:done", DoneEvent{
		Total: total, Completed: completed, Failed: failed, Skipped: skipped, Cancelled: ctx.Err() != nil,
	})
}

func (a *App) resolveStreams(item DownloadItem, preferred string) ([]MediaStream, error) {
	if item.StreamURL != "" {
		streams := extractMedia(item.StreamURL, item.PageURL)
		if len(streams) == 0 {
			return nil, fmt.Errorf("URL video không hợp lệ")
		}
		return streams, nil
	}
	if item.PageURL == "" {
		return nil, fmt.Errorf("tập không có URL trang")
	}
	source, err := fetchHTML(item.PageURL)
	if err != nil {
		return nil, err
	}
	resolvedPage := extractPageURL(source)
	expectedNumber := item.Number
	actualNumber := episodeNumber(resolvedPage)
	if expectedNumber > 0 && actualNumber > 0 && actualNumber != expectedNumber {
		return nil, fmt.Errorf("yêu cầu %s nhưng máy chủ trả về trang tập %d", item.Name, actualNumber)
	}
	streams := extractCurrentStreams(source, item.PageURL)
	if len(streams) == 0 {
		return nil, fmt.Errorf("không tìm thấy video trong trang tập")
	}
	if preferred != "" {
		sort.SliceStable(streams, func(i, j int) bool {
			iPreferred := strings.EqualFold(streams[i].Server, preferred)
			jPreferred := strings.EqualFold(streams[j].Server, preferred)
			return iPreferred && !jPreferred
		})
	}
	return streams, nil
}

func (a *App) downloadStream(ctx context.Context, ffmpeg, mediaURL, referer, outputPath string, index, total int, name string) error {
	partial := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".part.mp4"
	_ = os.Remove(partial)
	if referer == "" {
		referer = mediaURL
	}
	args := []string{
		"-y", "-hide_banner", "-loglevel", "warning", "-nostats", "-progress", "pipe:2",
		"-user_agent", browserUserAgent,
		"-referer", referer,
		"-i", mediaURL,
		"-map", "0:v:0?", "-map", "0:a?", "-c", "copy", "-movflags", "+faststart", partial,
	}
	command := exec.CommandContext(ctx, ffmpeg, args...)
	prepareBackgroundCommand(command)
	stderr, err := command.StderrPipe()
	if err != nil {
		return err
	}
	if err := command.Start(); err != nil {
		return err
	}
	a.setActiveFFmpeg(command.Process)
	defer a.clearActiveFFmpeg(command.Process)
	lastLines := make([]string, 0, 12)
	lastProgress := time.Time{}
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if len(lastLines) == cap(lastLines) {
			lastLines = lastLines[1:]
		}
		lastLines = append(lastLines, line)
		if time.Since(lastProgress) > 300*time.Millisecond {
			timeMatch := ffmpegTimePattern.FindStringSubmatch(line)
			if len(timeMatch) > 1 {
				speed := ""
				if speedMatch := ffmpegSpeedPattern.FindStringSubmatch(line); len(speedMatch) > 1 {
					speed = speedMatch[1]
				}
				wailsruntime.EventsEmit(a.ctx, "download:progress", ProgressEvent{Index: index, Total: total, Name: name, Time: timeMatch[1], Speed: speed})
				lastProgress = time.Now()
			}
		}
	}
	err = command.Wait()
	if ctx.Err() != nil {
		_ = os.Remove(partial)
		return ctx.Err()
	}
	if err != nil {
		_ = os.Remove(partial)
		message := strings.Join(lastLines, " | ")
		if len(message) > 700 {
			message = message[len(message)-700:]
		}
		return fmt.Errorf("FFmpeg: %v — %s", err, message)
	}
	info, err := os.Stat(partial)
	if err != nil || info.Size() == 0 {
		_ = os.Remove(partial)
		return fmt.Errorf("FFmpeg không tạo được file video")
	}
	_ = os.Remove(outputPath)
	if err := os.Rename(partial, outputPath); err != nil {
		return err
	}
	return nil
}

func displayServer(stream MediaStream) string {
	if stream.Server != "" {
		return stream.Server
	}
	return stream.Kind
}

func maxEpisodeNumber(items []DownloadItem) int {
	max := 0
	for _, item := range items {
		if item.Number > max {
			max = item.Number
		}
	}
	return max
}

func sanitizeFileName(value string) string {
	value = invalidFileChars.ReplaceAllString(value, " ")
	value = strings.Join(strings.Fields(value), " ")
	value = strings.TrimRight(strings.TrimSpace(value), ".")
	runes := []rune(value)
	if len(runes) > 120 {
		value = strings.TrimSpace(string(runes[:120]))
	}
	return value
}

func streamIdentity(value string) string {
	parsed, err := url.Parse(value)
	if err != nil {
		return value
	}
	parsed.RawQuery = ""
	parsed.Fragment = ""
	return strings.ToLower(parsed.String())
}

func videoFingerprint(path string) (string, error) {
	file, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer file.Close()
	info, err := file.Stat()
	if err != nil {
		return "", err
	}
	hash := sha256.New()
	_, _ = fmt.Fprintf(hash, "%d:", info.Size())
	const sampleSize int64 = 1024 * 1024
	if _, err := io.CopyN(hash, file, minInt64(sampleSize, info.Size())); err != nil && err != io.EOF {
		return "", err
	}
	if info.Size() > sampleSize {
		if _, err := file.Seek(maxInt64(0, info.Size()-sampleSize), io.SeekStart); err != nil {
			return "", err
		}
		if _, err := io.Copy(hash, file); err != nil {
			return "", err
		}
	}
	return hex.EncodeToString(hash.Sum(nil)), nil
}

func minInt64(a, b int64) int64 {
	if a < b {
		return a
	}
	return b
}

func maxInt64(a, b int64) int64 {
	if a > b {
		return a
	}
	return b
}
