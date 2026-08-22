package main

import (
	"bufio"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	wailsruntime "github.com/wailsapp/wails/v2/pkg/runtime"
)

var (
	ffmpegTimePattern     = regexp.MustCompile(`time=\s*([^\s]+)`)
	ffmpegSpeedPattern    = regexp.MustCompile(`speed=\s*([^\s]+)`)
	ffmpegDurationPattern = regexp.MustCompile(`Duration:\s*(\d+:\d{2}:\d{2}(?:\.\d+)?)`)
	// Lines emitted by "-progress": key=value with no spaces. They would
	// otherwise flood the tail kept for error messages.
	ffmpegProgressKey = regexp.MustCompile(`^[A-Za-z0-9_]+=\S*$`)
	invalidFileChars  = regexp.MustCompile(`[<>:"/\\|?*\x00-\x1f]`)
)

const (
	// stallTimeout is how long FFmpeg may run without the decoded position
	// moving before the download is treated as hung.
	stallTimeout = 90 * time.Second
	// stallAttempts is the total number of tries per episode, retries included.
	stallAttempts = 3
	// throttleGiveUp is how many consecutive blocked episodes end the queue.
	throttleGiveUp = 3
)

// errStalled marks a download that was killed because it stopped progressing.
var errStalled = errors.New("FFmpeg treo")

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

// CancelDownload stops the queue for good. Nothing here can fail in a way that
// leaves FFmpeg running: the flag is raised first, the process tree is killed
// outright, and stopWatchdog keeps killing until the queue reports it is done.
func (a *App) CancelDownload() {
	a.mu.Lock()
	if !a.downloading {
		a.mu.Unlock()
		return
	}
	cancel := a.cancel
	process := a.activeFFmpeg
	wasPaused := a.paused
	a.paused = false
	a.stopping = true
	a.mu.Unlock()
	// A suspended process never drains its pipes, so the queue would sit in the
	// output loop forever; it is resumed before the kill.
	if wasPaused && process != nil {
		_ = setProcessPaused(process, false)
	}
	if cancel != nil {
		cancel()
	}
	if process != nil {
		_ = killProcessTree(process)
	}
	go a.stopWatchdog()
}

// stopWatchdog kills whatever FFmpeg is running until the queue actually ends.
// A single kill can miss the mark: the process may have been mid-start when Dừng
// was pressed, or a retry may have started a new one a moment later.
func (a *App) stopWatchdog() {
	deadline := time.Now().Add(30 * time.Second)
	for {
		time.Sleep(300 * time.Millisecond)
		a.mu.Lock()
		running := a.downloading
		process := a.activeFFmpeg
		a.mu.Unlock()
		if !running {
			return
		}
		if process != nil {
			_ = setProcessPaused(process, false)
			_ = killProcessTree(process)
		}
		if time.Now().After(deadline) {
			a.emit("download:log", "Hàng đợi chưa dừng hẳn sau 30 giây; hãy đóng ứng dụng nếu tình trạng này kéo dài.")
			return
		}
	}
}

// PauseDownload flips the pause the user asked for. The flag is what the queue
// obeys, so the answer never depends on catching a process at the right moment:
// if the episode just ended, the next one starts suspended instead.
func (a *App) PauseDownload() (DownloadControlStatus, error) {
	a.mu.Lock()
	if !a.downloading {
		a.mu.Unlock()
		return DownloadControlStatus{}, fmt.Errorf("không có hàng đợi đang tải")
	}
	if a.stopping {
		a.mu.Unlock()
		return DownloadControlStatus{}, fmt.Errorf("hàng đợi đang dừng")
	}
	targetPaused := !a.paused
	a.paused = targetPaused
	process := a.activeFFmpeg
	a.mu.Unlock()
	a.applyPause(process, targetPaused)
	return DownloadControlStatus{Paused: targetPaused}, nil
}

// applyPause brings one process in line with the requested state, treating a
// process that has already ended as nothing to do.
func (a *App) applyPause(process *os.Process, paused bool) {
	if process == nil {
		return
	}
	err := setProcessPaused(process, paused)
	if err == nil || errors.Is(err, errProcessGone) {
		return
	}
	a.emit("download:log", err.Error())
}

func (a *App) setActiveFFmpeg(process *os.Process) {
	a.mu.Lock()
	a.activeFFmpeg = process
	paused := a.paused
	stopping := a.stopping
	a.mu.Unlock()
	if stopping {
		// Dừng was pressed while this process was starting up.
		_ = killProcessTree(process)
		return
	}
	if paused {
		a.applyPause(process, true)
	}
}

func (a *App) clearActiveFFmpeg(process *os.Process) {
	a.mu.Lock()
	if a.activeFFmpeg == process {
		a.activeFFmpeg = nil
	}
	a.mu.Unlock()
}

// waitWhilePaused holds the queue between episodes for as long as the user keeps
// it paused, and reports false when the queue was stopped instead of resumed.
func (a *App) waitWhilePaused(ctx context.Context) bool {
	for {
		if ctx.Err() != nil {
			return false
		}
		if !a.isPaused() {
			return true
		}
		select {
		case <-ctx.Done():
			return false
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func (a *App) runQueue(ctx context.Context, ffmpeg string, request DownloadRequest) {
	defer func() {
		a.mu.Lock()
		a.downloading = false
		a.cancel = nil
		a.activeFFmpeg = nil
		a.paused = false
		a.stopping = false
		a.mu.Unlock()
	}()

	total := len(request.Items)
	completed, failed, skipped := 0, 0, 0
	throttled := 0
	nextYtDlpStart := time.Time{}
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
		// Pausing between episodes holds the queue here instead of letting the
		// next FFmpeg start behind the user's back.
		if !a.waitWhilePaused(ctx) {
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
		switch {
		case item.Engine == engineYtDlp || prefersYtDlp(item.PageURL):
			// Video and lecture titles carry the meaning, so they name the file
			// instead of an episode number.
			outputName = sanitizeFileName(name)
			if item.Number > 0 && total > 1 {
				outputName = fmt.Sprintf("%0*d - %s", width, item.Number, sanitizeFileName(name))
			}
			if outputName == "" {
				outputName = baseTitle
			}
		case total > 1 || item.Number > 0:
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

		// yt-dlp handles YouTube and the other sites it knows: it picks the
		// formats, merges them with our FFmpeg and keeps up with site changes.
		if item.Engine == engineYtDlp || prefersYtDlp(item.PageURL) {
			isYouTubeItem := isYouTubeURL(item.PageURL)
			if delay := time.Until(nextYtDlpStart); isYouTubeItem && delay > time.Second {
				event.Message = fmt.Sprintf("Giãn yêu cầu YouTube thêm %s", delay.Round(time.Second))
				wailsruntime.EventsEmit(a.ctx, "download:queue", event)
				wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf(
					"%s — chờ %s trước yêu cầu kế tiếp để tránh rate limit.", displayName, delay.Round(time.Second)))
				if !waitForDownloadDelay(ctx, delay) {
					break
				}
			}
			event.Status = "downloading"
			event.Attempt = 1
			event.Message = ""
			wailsruntime.EventsEmit(a.ctx, "download:queue", event)
			var youTubeErr error
			stallAttempt, rateLimitRetry, totalAttempt := 1, 0, 1
			for {
				if stallAttempt > 1 {
					event.Attempt = totalAttempt
					event.Message = fmt.Sprintf("Tải lại lần %d sau khi treo", stallAttempt)
					wailsruntime.EventsEmit(a.ctx, "download:queue", event)
					wailsruntime.EventsEmit(a.ctx, "download:log",
						fmt.Sprintf("%s — tải lại lần %d/%d sau khi yt-dlp bị treo.", displayName, stallAttempt, stallAttempts))
				}
				if delay := time.Until(nextYtDlpStart); isYouTubeItem && delay > 0 {
					if !waitForDownloadDelay(ctx, delay) {
						break
					}
				}
				if isYouTubeItem {
					nextYtDlpStart = time.Now().Add(ytDlpQueueInterval)
				}
				youTubeErr = a.downloadWithYtDlp(ctx, ffmpeg, item, outputPath, index+1, total, request.MaxHeight, displayName)
				if youTubeErr == nil || ctx.Err() != nil {
					break
				}
				if errors.Is(youTubeErr, errStalled) && stallAttempt < stallAttempts {
					stallAttempt++
					totalAttempt++
					continue
				}
				if isYouTubeItem && isAccountRateLimitFailure(youTubeErr) && rateLimitRetry < ytDlpRateLimitRetries {
					rateLimitRetry++
					totalAttempt++
					event.Attempt = totalAttempt
					event.Message = accountRateLimitHint()
					wailsruntime.EventsEmit(a.ctx, "download:queue", event)
					retryAt := time.Now().Add(ytDlpRateLimitCooldown)
					wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf(
						"%s — YouTube giới hạn tài khoản; giữ nguyên tập này và tự thử lại lúc %s (có thể bấm Dừng).",
						displayName, retryAt.Format("15:04")))
					if !waitForDownloadDelay(ctx, ytDlpRateLimitCooldown) || !a.waitWhilePaused(ctx) {
						break
					}
					// A rate-limit retry is independent of stall retries.
					stallAttempt = 1
					continue
				}
				break
			}
			event.Message = ""
			if ctx.Err() != nil {
				break
			}
			if youTubeErr != nil {
				failed++
				event.Status = "failed"
				event.Failed = failed
				event.Message = youTubeErr.Error()
				if isYouTubeItem && isAccountRateLimitFailure(youTubeErr) {
					throttled = throttleGiveUp
					event.Message = accountRateLimitHint() + " — " + youTubeErr.Error()
				} else if isThrottleFailure(youTubeErr) {
					throttled++
					hint := throttleHint(request.CookieSource)
					event.Message = hint + " — " + youTubeErr.Error()
				} else {
					throttled = 0
				}
			} else {
				throttled = 0
				completed++
				event.Status = "completed"
				event.Completed = completed
			}
			wailsruntime.EventsEmit(a.ctx, "download:queue", event)
			// Continuing to hammer a site that is already refusing only makes the
			// block worse, so the queue stops and keeps the rest for later.
			if throttled >= throttleGiveUp {
				message := fmt.Sprintf("Dừng hàng đợi: %d tập liên tiếp bị chặn. %s", throttled, throttleHint(request.CookieSource))
				if isYouTubeItem && isAccountRateLimitFailure(youTubeErr) {
					message = "Dừng hàng đợi: tài khoản YouTube vẫn bị rate limit sau thời gian nghỉ; các tập còn lại được giữ nguyên để tải sau."
				}
				wailsruntime.EventsEmit(a.ctx, "download:log", message)
				break
			}
			continue
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
		event.Attempt = 1
		wailsruntime.EventsEmit(a.ctx, "download:queue", event)
		var lastErr error
		for attempt := 1; attempt <= stallAttempts; attempt++ {
			if attempt > 1 {
				event.Attempt = attempt
				event.Message = fmt.Sprintf("Tải lại lần %d sau khi treo", attempt)
				wailsruntime.EventsEmit(a.ctx, "download:queue", event)
				wailsruntime.EventsEmit(a.ctx, "download:log",
					fmt.Sprintf("%s — tải lại lần %d/%d sau khi FFmpeg bị treo.", displayName, attempt, stallAttempts))
			}
			lastErr = a.downloadFromStreams(ctx, ffmpeg, streams, item, outputPath, index+1, total, displayName,
				usedStreams, attempt < stallAttempts)
			if lastErr == nil || ctx.Err() != nil || !errors.Is(lastErr, errStalled) {
				break
			}
		}
		// Links from the episode index go stale (typically HTTP 404). When every
		// known server has failed, ask the episode page for fresh ones.
		if lastErr != nil && ctx.Err() == nil && len(knownStreams(item)) > 0 && item.PageURL != "" {
			fresh, refreshErr := a.resolveStreamsFromPage(item, request.PreferredServer)
			if refreshErr != nil {
				wailsruntime.EventsEmit(a.ctx, "download:log",
					fmt.Sprintf("%s — không lấy được link mới từ trang tập: %v", displayName, refreshErr))
			} else if unseen := newStreams(fresh, streams); len(unseen) > 0 {
				wailsruntime.EventsEmit(a.ctx, "download:log",
					fmt.Sprintf("%s — thử lại với %d link mới lấy từ trang tập.", displayName, len(unseen)))
				event.Message = "Thử link mới từ trang tập"
				wailsruntime.EventsEmit(a.ctx, "download:queue", event)
				lastErr = a.downloadFromStreams(ctx, ffmpeg, unseen, item, outputPath, index+1, total, displayName,
					usedStreams, false)
			}
		}
		event.Message = ""
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

// downloadFromStreams walks the servers of one episode and stops at the first
// one that produces a file. A stall is reported back so the caller can retry.
func (a *App) downloadFromStreams(ctx context.Context, ffmpeg string, streams []MediaStream, item DownloadItem,
	outputPath string, index, total int, displayName string, usedStreams map[string]string, stopOnStall bool) error {
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
		lastErr = a.downloadStream(ctx, ffmpeg, stream.URL, item.PageURL, outputPath, item.ID, index, total, displayName)
		if lastErr == nil {
			usedStreams[identity] = displayName
			return nil
		}
		wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf("%s — server lỗi: %v", displayName, lastErr))
		if stopOnStall && errors.Is(lastErr, errStalled) {
			// Let the caller restart this episode instead of burning the other
			// servers on what is usually a temporary network stall. On the last
			// attempt the remaining servers are tried instead.
			return lastErr
		}
	}
	return lastErr
}

func (a *App) resolveStreams(item DownloadItem, preferred string) ([]MediaStream, error) {
	// Links carried by the episode index cover every server of the host, so try
	// them before spending a request on the episode page.
	if candidates := knownStreams(item); len(candidates) > 0 {
		return preferServer(candidates, preferred), nil
	}
	if item.PageURL == "" {
		return nil, fmt.Errorf("tập không có URL trang")
	}
	return a.resolveStreamsFromPage(item, preferred)
}

// resolveStreamsFromPage reads the episode page itself, which is where fresh
// signed URLs come from when the ones in the index have expired.
func (a *App) resolveStreamsFromPage(item DownloadItem, preferred string) ([]MediaStream, error) {
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
		if embeds := extractEmbedLinks(source); len(embeds) > 0 {
			return nil, embedOnlyError(embeds)
		}
		return nil, fmt.Errorf("không tìm thấy video trong trang tập")
	}
	return preferServer(streams, preferred), nil
}

// knownStreams normalizes the candidates already attached to the item.
func knownStreams(item DownloadItem) []MediaStream {
	result := make([]MediaStream, 0, len(item.Streams)+1)
	for _, stream := range append(item.Streams, MediaStream{URL: item.StreamURL, Server: "", Kind: ""}) {
		if strings.TrimSpace(stream.URL) == "" {
			continue
		}
		for _, parsed := range extractMedia(stream.URL, item.PageURL) {
			if stream.Server != "" {
				parsed.Server = stream.Server
			}
			if !containsStreamURL(result, parsed.URL) {
				result = append(result, parsed)
			}
		}
	}
	return result
}

// newStreams keeps only the candidates that were not tried already.
func newStreams(candidates, tried []MediaStream) []MediaStream {
	result := make([]MediaStream, 0, len(candidates))
	for _, candidate := range candidates {
		if !containsStreamURL(tried, candidate.URL) && !containsStreamURL(result, candidate.URL) {
			result = append(result, candidate)
		}
	}
	return result
}

func preferServer(streams []MediaStream, preferred string) []MediaStream {
	if preferred == "" {
		return streams
	}
	sort.SliceStable(streams, func(i, j int) bool {
		iPreferred := strings.EqualFold(streams[i].Server, preferred)
		jPreferred := strings.EqualFold(streams[j].Server, preferred)
		return iPreferred && !jPreferred
	})
	return streams
}

func (a *App) downloadStream(ctx context.Context, ffmpeg, mediaURL, referer, outputPath, episodeID string, index, total int, name string) error {
	partial := strings.TrimSuffix(outputPath, filepath.Ext(outputPath)) + ".part.mp4"
	_ = os.Remove(partial)
	if referer == "" {
		referer = mediaURL
	}
	args := []string{
		// "info" keeps the input header, which carries the total duration used
		// for the per-episode progress bar.
		"-y", "-hide_banner", "-loglevel", "info", "-nostats", "-progress", "pipe:2",
		"-user_agent", browserUserAgent,
		"-referer", referer,
		"-rw_timeout", "20000000",
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
	if superviseErr := superviseProcess(command.Process); superviseErr != nil {
		wailsruntime.EventsEmit(a.ctx, "download:log", fmt.Sprintf("%s — %v", name, superviseErr))
	}

	watchdog := a.watchStall(command.Process, name)
	lastLines := make([]string, 0, 12)
	lastProgress := time.Time{}
	durationSeconds := 0.0
	currentSeconds := -1.0
	scanner := bufio.NewScanner(stderr)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := scanner.Text()
		if !ffmpegProgressKey.MatchString(line) {
			if len(lastLines) == cap(lastLines) {
				lastLines = lastLines[1:]
			}
			lastLines = append(lastLines, line)
		}
		if durationSeconds == 0 {
			if durationMatch := ffmpegDurationPattern.FindStringSubmatch(line); len(durationMatch) > 1 {
				durationSeconds = clockSeconds(durationMatch[1])
			}
		}
		timeMatch := ffmpegTimePattern.FindStringSubmatch(line)
		if len(timeMatch) < 2 {
			continue
		}
		// Only a moving decode position counts as progress; FFmpeg keeps
		// printing the same block while it is stuck on a dead segment.
		if seconds := clockSeconds(timeMatch[1]); seconds > currentSeconds {
			currentSeconds = seconds
			watchdog.progressed()
		}
		if time.Since(lastProgress) <= 300*time.Millisecond {
			continue
		}
		speed := ""
		if speedMatch := ffmpegSpeedPattern.FindStringSubmatch(line); len(speedMatch) > 1 {
			speed = speedMatch[1]
		}
		wailsruntime.EventsEmit(a.ctx, "download:progress", ProgressEvent{
			ID: episodeID, Index: index, Total: total, Name: name,
			Time: humanClock(currentSeconds), Speed: speed,
			Duration: humanClock(durationSeconds),
			Percent:  progressPercent(currentSeconds, durationSeconds),
		})
		lastProgress = time.Now()
	}
	err = command.Wait()
	stalled := watchdog.stop()
	if ctx.Err() != nil {
		_ = os.Remove(partial)
		return ctx.Err()
	}
	if stalled {
		_ = os.Remove(partial)
		return fmt.Errorf("%w: không có tiến triển trong %s", errStalled, stallTimeout)
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

// stallGuard kills FFmpeg when the decode position stops moving. Pausing the
// queue suspends the process on purpose, so those seconds never count.
type stallGuard struct {
	mu      sync.Mutex
	last    time.Time
	stalled bool
	done    chan struct{}
	closed  bool
}

func (a *App) watchStall(process *os.Process, name string) *stallGuard {
	guard := &stallGuard{last: time.Now(), done: make(chan struct{})}
	go func() {
		ticker := time.NewTicker(5 * time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-guard.done:
				return
			case <-ticker.C:
				if a.isPaused() {
					guard.progressed()
					continue
				}
				guard.mu.Lock()
				idle := time.Since(guard.last)
				guard.mu.Unlock()
				if idle < stallTimeout {
					continue
				}
				guard.mu.Lock()
				guard.stalled = true
				guard.mu.Unlock()
				wailsruntime.EventsEmit(a.ctx, "download:log",
					fmt.Sprintf("%s — treo %.0f giây không tiến triển, đang tắt FFmpeg để tải lại.", name, idle.Seconds()))
				_ = killProcessTree(process)
				return
			}
		}
	}()
	return guard
}

func (g *stallGuard) progressed() {
	g.mu.Lock()
	g.last = time.Now()
	g.mu.Unlock()
}

// stop ends the watchdog and reports whether it had to kill FFmpeg.
func (g *stallGuard) stop() bool {
	g.mu.Lock()
	defer g.mu.Unlock()
	if !g.closed {
		close(g.done)
		g.closed = true
	}
	return g.stalled
}

func (a *App) isPaused() bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	return a.paused
}

// clockSeconds turns an FFmpeg HH:MM:SS.mmm stamp into seconds; unusable values
// such as "N/A" or the negative placeholder emitted at startup give -1.
func clockSeconds(value string) float64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.HasPrefix(value, "-") || strings.EqualFold(value, "N/A") {
		return -1
	}
	parts := strings.Split(value, ":")
	if len(parts) != 3 {
		return -1
	}
	hours, hoursErr := strconv.ParseFloat(parts[0], 64)
	minutes, minutesErr := strconv.ParseFloat(parts[1], 64)
	seconds, secondsErr := strconv.ParseFloat(parts[2], 64)
	if hoursErr != nil || minutesErr != nil || secondsErr != nil {
		return -1
	}
	return hours*3600 + minutes*60 + seconds
}

func humanClock(seconds float64) string {
	if seconds <= 0 {
		return ""
	}
	whole := int(seconds)
	if whole >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", whole/3600, (whole%3600)/60, whole%60)
	}
	return fmt.Sprintf("%d:%02d", whole/60, whole%60)
}

// progressPercent returns -1 when the duration is unknown, so the interface can
// fall back to an indeterminate bar instead of showing a wrong number.
func progressPercent(current, duration float64) float64 {
	if current < 0 || duration <= 0 {
		return -1
	}
	percent := current / duration * 100
	if percent > 100 {
		percent = 100
	}
	return float64(int(percent*10)) / 10
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
