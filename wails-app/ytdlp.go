package main

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

// yt-dlp carries the YouTube extraction logic and refreshes it often, which is
// what keeps downloads working when YouTube changes. Releases are single files.
const (
	ytDlpWindowsURL = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp.exe"
	ytDlpMacURL     = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp_macos"
	ytDlpLinuxURL   = "https://github.com/yt-dlp/yt-dlp/releases/latest/download/yt-dlp"
	// ytDlpFreshFor is how long a self-update check is considered current.
	ytDlpFreshFor = 24 * time.Hour
	// Separate yt-dlp processes are used for playlist entries, so its internal
	// --sleep-interval cannot space the extraction of one entry from the next.
	// The queue enforces this gap between process starts as well.
	ytDlpQueueInterval = 20 * time.Second
	// YouTube's explicit account limit says it can last for up to an hour. Keep
	// the current item pending for that window instead of failing dozens of the
	// following entries in a few seconds.
	ytDlpRateLimitCooldown = time.Hour
	ytDlpRateLimitRetries  = 1
)

// engineYtDlp marks the episodes that yt-dlp downloads instead of FFmpeg.
const engineYtDlp = "ytdlp"

var (
	youTubeHostPattern = regexp.MustCompile(`(?i)^(?:www\.|m\.|music\.)?(?:youtube\.com|youtube-nocookie\.com|youtu\.be)$`)
	// Progress lines are emitted through --progress-template, so they parse
	// exactly instead of by scraping the human readable bar.
	ytDlpProgressPrefix = "MOTCHILL|"
)

// Sites whose pages hold no playable URL to parse, so yt-dlp handles them. Many
// of them need the login the user already has in their browser.
var ytDlpHostSuffixes = []string{
	"udemy.com", "vimeo.com", "dailymotion.com", "coursera.org", "skillshare.com",
	"bilibili.com", "tiktok.com", "facebook.com", "twitter.com", "x.com", "twitch.tv",
	"soundcloud.com", "nicovideo.jp", "odysee.com", "rumble.com",
}

// prefersYtDlp reports whether a link should skip the HTML extractor entirely.
func prefersYtDlp(value string) bool {
	if isYouTubeURL(value) {
		return true
	}
	parsed, err := url.Parse(strings.TrimSpace(value))
	if err != nil || parsed.Host == "" {
		return false
	}
	host := strings.ToLower(parsed.Host)
	if index := strings.Index(host, ":"); index > 0 {
		host = host[:index]
	}
	for _, suffix := range ytDlpHostSuffixes {
		if host == suffix || strings.HasSuffix(host, "."+suffix) {
			return true
		}
	}
	return false
}

// YouTube uses two different throttle replies: the generic bot/403 challenge
// and an explicit per-account rate limit. Keep them separate because the latter
// has a concrete one-hour recovery window and should be retried in place.
var (
	ytDlpBotCheck              = regexp.MustCompile(`(?i)sign in to confirm|not a bot|http error 403|403: forbidden`)
	ytDlpAccountRateLimitCheck = regexp.MustCompile(
		`(?i)account has been rate[- ]limited|rate[- ]limited by youtube|too many requests|http (?:error )?429|429: too many`,
	)
)

func isAccountRateLimitFailure(err error) bool {
	return err != nil && ytDlpAccountRateLimitCheck.MatchString(err.Error())
}

func isThrottleFailure(err error) bool {
	return err != nil && (ytDlpBotCheck.MatchString(err.Error()) || isAccountRateLimitFailure(err))
}

// throttleHint tells the user what actually helps, because the raw yt-dlp error
// is a wall of links.
func throttleHint(cookieSource string) string {
	if strings.TrimSpace(cookieSource) == "" || strings.EqualFold(cookieSource, "none") {
		return "YouTube đang chặn vì nghi tải tự động. Hãy chọn Cookie đăng nhập (trình duyệt đang đăng nhập YouTube) hoặc nạp file cookies.txt, rồi chờ vài phút và tải lại."
	}
	return "YouTube vẫn chặn dù đã có cookie. Hãy chờ vài phút, giảm số tập mỗi lượt, hoặc kiểm tra lại cookie còn hiệu lực."
}

func accountRateLimitHint() string {
	return "Tài khoản YouTube đã chạm giới hạn theo giờ. Ứng dụng giữ nguyên tập hiện tại, nghỉ 1 giờ rồi tự thử lại; không cần nạp lại cookie."
}

// ytDlpPacingArgs spaces the requests out. Forty videos fetched back to back from
// one address is what trips the bot check in the first place.
func ytDlpPacingArgs() []string {
	return []string{
		"--retries", "10",
		"--fragment-retries", "10",
		"--retry-sleep", "http:exp=1:30",
		"--extractor-retries", "5",
		"--retry-sleep", "extractor:exp=5:60",
		// These are yt-dlp's own `-t sleep` values. They slow the requests
		// inside one process; ytDlpQueueInterval covers separate entries.
		"--sleep-requests", "0.75",
		"--sleep-interval", "10",
		"--max-sleep-interval", "20",
	}
}

// waitForDownloadDelay is cancellable so pressing Dừng or closing the app does
// not leave the queue stuck in either a pacing delay or the rate-limit cooldown.
func waitForDownloadDelay(ctx context.Context, delay time.Duration) bool {
	if delay <= 0 {
		return ctx.Err() == nil
	}
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}

// ytDlpAuthArgs passes the browser session along, which is how a site the user is
// logged into — Udemy, for instance — lets the download through. Credentials are
// never handled here; only the cookies the browser already holds.
func ytDlpAuthArgs(cookieSource string) []string {
	cookieSource = strings.TrimSpace(cookieSource)
	switch {
	case cookieSource == "" || strings.EqualFold(cookieSource, "none"):
		return nil
	case strings.HasPrefix(cookieSource, "file:"):
		path := strings.TrimSpace(strings.TrimPrefix(cookieSource, "file:"))
		if path == "" {
			return nil
		}
		return []string{"--cookies", path}
	default:
		return []string{"--cookies-from-browser", cookieSource}
	}
}

func isYouTubeURL(value string) bool {
	value = strings.TrimSpace(value)
	if value == "" {
		return false
	}
	parsed, err := url.Parse(value)
	if err != nil || parsed.Host == "" {
		return false
	}
	return youTubeHostPattern.MatchString(parsed.Host)
}

// ytDlpExecutableName is the file name of the release for this platform.
func ytDlpExecutableName() string {
	if runtime.GOOS == "windows" {
		return "yt-dlp.exe"
	}
	return "yt-dlp"
}

func ytDlpDownloadURL() string {
	switch runtime.GOOS {
	case "windows":
		return ytDlpWindowsURL
	case "darwin":
		return ytDlpMacURL
	default:
		return ytDlpLinuxURL
	}
}

func (a *App) findYtDlp() string {
	settings := a.settings.snapshot()
	candidates := []string{settings.YtDlpPath, filepath.Join(toolInstallRoot(), ytDlpExecutableName())}
	if runtime.GOOS == "darwin" {
		candidates = append(candidates, "/opt/homebrew/bin/yt-dlp", "/usr/local/bin/yt-dlp")
	}
	candidates = append(candidates, filepath.Join(executableDirectory(), ytDlpExecutableName()))
	for _, candidate := range candidates {
		if candidate == "" {
			continue
		}
		if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
			return candidate
		}
	}
	if candidate, err := exec.LookPath("yt-dlp"); err == nil {
		return candidate
	}
	return ""
}

// toolInstallRoot is where the app keeps the binaries it downloads itself.
func toolInstallRoot() string {
	if runtime.GOOS == "windows" {
		if local := os.Getenv("LOCALAPPDATA"); local != "" {
			return filepath.Join(local, "MotchillDownloader")
		}
	}
	configRoot, err := os.UserConfigDir()
	if err != nil || configRoot == "" {
		configRoot = os.TempDir()
	}
	return filepath.Join(configRoot, "MotchillDownloader")
}

func (a *App) GetYtDlpStatus() ToolStatus {
	path := a.findYtDlp()
	if path == "" {
		return ToolStatus{}
	}
	return ToolStatus{Ready: true, Path: path, Version: ytDlpVersion(path), CheckedAt: a.settings.snapshot().YtDlpCheckedAt}
}

func ytDlpVersion(path string) string {
	command := exec.Command(path, "--version")
	prepareBackgroundCommand(command)
	output, err := command.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(output))
}

// InstallYtDlp fetches the current release for this platform.
func (a *App) InstallYtDlp() (ToolStatus, error) {
	root := toolInstallRoot()
	if err := os.MkdirAll(root, 0o755); err != nil {
		return ToolStatus{}, err
	}
	target := filepath.Join(root, ytDlpExecutableName())
	temporary := target + ".new"
	defer os.Remove(temporary)

	response, err := http.Get(ytDlpDownloadURL())
	if err != nil {
		return ToolStatus{}, fmt.Errorf("không tải được yt-dlp: %w", err)
	}
	defer response.Body.Close()
	if response.StatusCode < 200 || response.StatusCode >= 300 {
		return ToolStatus{}, fmt.Errorf("máy chủ yt-dlp trả về HTTP %d", response.StatusCode)
	}
	file, err := os.OpenFile(temporary, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o755)
	if err != nil {
		return ToolStatus{}, err
	}
	reader := &progressReader{reader: response.Body, total: response.ContentLength, emit: func(downloaded, total int64) {
		a.emit("ytdlp:progress", map[string]int64{"downloaded": downloaded, "total": total})
	}}
	_, copyErr := io.Copy(file, reader)
	closeErr := file.Close()
	if copyErr != nil {
		return ToolStatus{}, copyErr
	}
	if closeErr != nil {
		return ToolStatus{}, closeErr
	}
	if version := ytDlpVersion(temporary); version == "" {
		return ToolStatus{}, fmt.Errorf("bản yt-dlp tải về không chạy được")
	}
	_ = os.Remove(target)
	if err := os.Rename(temporary, target); err != nil {
		return ToolStatus{}, err
	}
	if err := a.settings.setYtDlpPath(target); err != nil {
		return ToolStatus{}, err
	}
	_ = a.settings.setYtDlpCheckedAt(time.Now().Format(time.RFC3339))
	return a.GetYtDlpStatus(), nil
}

// UpdateYtDlp runs the self-update built into yt-dlp, which is how the YouTube
// logic stays current without shipping a new build of this app.
func (a *App) UpdateYtDlp() (ToolStatus, error) {
	path := a.findYtDlp()
	if path == "" {
		return a.InstallYtDlp()
	}
	command := exec.Command(path, "-U")
	prepareBackgroundCommand(command)
	output, err := command.CombinedOutput()
	message := strings.TrimSpace(string(output))
	if err != nil {
		// A copy installed by a package manager refuses to self-update; fetching
		// our own copy keeps the feature working.
		if strings.Contains(strings.ToLower(message), "not writable") ||
			strings.Contains(strings.ToLower(message), "please use that to update") {
			return a.InstallYtDlp()
		}
		return a.GetYtDlpStatus(), fmt.Errorf("cập nhật yt-dlp thất bại: %w — %s", err, lastLine(message))
	}
	_ = a.settings.setYtDlpCheckedAt(time.Now().Format(time.RFC3339))
	if message != "" {
		a.emit("download:log", "yt-dlp: "+lastLine(message))
	}
	return a.GetYtDlpStatus(), nil
}

// EnsureYtDlpFresh self-updates at most once a day, so a queue that starts after
// YouTube changed something still picks up the new extraction logic.
func (a *App) EnsureYtDlpFresh() (ToolStatus, error) {
	status := a.GetYtDlpStatus()
	if !status.Ready {
		return status, fmt.Errorf("chưa có yt-dlp")
	}
	checked, err := time.Parse(time.RFC3339, status.CheckedAt)
	if err == nil && time.Since(checked) < ytDlpFreshFor {
		return status, nil
	}
	return a.UpdateYtDlp()
}

func lastLine(value string) string {
	lines := strings.Split(strings.TrimSpace(value), "\n")
	for index := len(lines) - 1; index >= 0; index-- {
		if line := strings.TrimSpace(lines[index]); line != "" {
			return line
		}
	}
	return ""
}

type ytDlpEntry struct {
	ID            string  `json:"id"`
	Title         string  `json:"title"`
	URL           string  `json:"url"`
	WebpageURL    string  `json:"webpage_url"`
	Duration      float64 `json:"duration"`
	PlaylistIndex int     `json:"playlist_index"`
}

type ytDlpDump struct {
	Type       string       `json:"_type"`
	ID         string       `json:"id"`
	Title      string       `json:"title"`
	WebpageURL string       `json:"webpage_url"`
	Uploader   string       `json:"uploader"`
	Entries    []ytDlpEntry `json:"entries"`
}

// analyzeWithYtDlp asks yt-dlp what a link contains. A playlist or course becomes
// a movie with one episode per video; a single video becomes a movie with one.
func (a *App) analyzeWithYtDlp(ctx context.Context, source string) (AnalysisResult, error) {
	path := a.findYtDlp()
	if path == "" {
		return AnalysisResult{}, fmt.Errorf("cần yt-dlp cho link này; hãy bấm Cài yt-dlp trong phần thiết lập")
	}
	arguments := append([]string{"-J", "--flat-playlist", "--no-warnings", "--no-colors"},
		a.ytDlpRequestArgs(source)...)
	arguments = append(arguments, source)
	command := exec.CommandContext(ctx, path, arguments...)
	prepareBackgroundCommand(command)
	var problems strings.Builder
	command.Stderr = &problems
	data, err := command.Output()
	if ctx.Err() != nil {
		return AnalysisResult{}, fmt.Errorf("đã dừng phân tích")
	}
	if err != nil {
		return AnalysisResult{}, fmt.Errorf("yt-dlp không đọc được link: %w — %s", err, lastLine(problems.String()))
	}
	return parseYtDlpDump(data, source)
}

func parseYtDlpDump(data []byte, source string) (AnalysisResult, error) {
	var dump ytDlpDump
	if err := json.Unmarshal(data, &dump); err != nil {
		return AnalysisResult{}, fmt.Errorf("không đọc được dữ liệu yt-dlp: %w", err)
	}
	title := strings.TrimSpace(dump.Title)
	if title == "" {
		title = "YouTube"
	}
	pageURL := firstNonEmpty(dump.WebpageURL, source)

	if len(dump.Entries) == 0 {
		if dump.ID == "" && dump.WebpageURL == "" {
			return AnalysisResult{}, fmt.Errorf("link YouTube không có video nào")
		}
		return AnalysisResult{
			Title:       title,
			PageURL:     pageURL,
			Streams:     []MediaStream{},
			SourceLabel: source,
			Episodes: []Episode{{
				ID:      youTubeEpisodeID(dump.ID),
				Name:    title,
				Number:  0,
				PageURL: firstNonEmpty(youTubeWatchURL(dump.WebpageURL, dump.ID), source),
				Engine:  engineYtDlp,
				Current: true,
			}},
		}, nil
	}

	episodes := make([]Episode, 0, len(dump.Entries))
	for index, entry := range dump.Entries {
		watch := youTubeWatchURL(firstNonEmpty(entry.WebpageURL, entry.URL), entry.ID)
		if watch == "" {
			continue
		}
		number := entry.PlaylistIndex
		if number <= 0 {
			number = index + 1
		}
		name := strings.TrimSpace(entry.Title)
		if name == "" {
			name = fmt.Sprintf("Video %02d", number)
		}
		episodes = append(episodes, Episode{
			ID:      youTubeEpisodeID(entry.ID),
			Name:    name,
			Number:  number,
			PageURL: watch,
			Engine:  engineYtDlp,
		})
	}
	if len(episodes) == 0 {
		return AnalysisResult{}, fmt.Errorf("playlist YouTube không có video tải được")
	}
	return AnalysisResult{
		Title:       title,
		PageURL:     pageURL,
		Streams:     []MediaStream{},
		Episodes:    episodes,
		SourceLabel: source,
	}, nil
}

func youTubeEpisodeID(id string) string {
	id = strings.TrimSpace(id)
	if id == "" {
		return "youtube"
	}
	return "yt-" + id
}

// youTubeWatchURL normalizes what --flat-playlist reports, where "url" is
// sometimes only the video id.
func youTubeWatchURL(value, id string) string {
	value = strings.TrimSpace(value)
	if strings.HasPrefix(strings.ToLower(value), "http") {
		return value
	}
	if id = strings.TrimSpace(id); id != "" {
		return "https://www.youtube.com/watch?v=" + id
	}
	if value != "" {
		return "https://www.youtube.com/watch?v=" + value
	}
	return ""
}

// ytDlpFormat picks the best video and audio within a height cap, falling back to
// whatever single file is available.
func ytDlpFormat(maxHeight int) string {
	if maxHeight <= 0 {
		return "bv*+ba/b"
	}
	return fmt.Sprintf("bv*[height<=%d]+ba/b[height<=%d]/bv*+ba/b", maxHeight, maxHeight)
}

// Post-processing phases produce no progress lines: merging video and audio,
// remuxing or fixing a container can take minutes on a long 4K file.
var ytDlpPostProcessPrefixes = []string{
	"[merger]", "[videoremuxer]", "[videoconvertor]", "[extractaudio]",
	"[fixupm3u8]", "[fixupm4a]", "[fixuptimestamp]", "[fixup", "[metadata]",
}

func isYtDlpPostProcessing(line string) bool {
	lower := strings.ToLower(line)
	for _, prefix := range ytDlpPostProcessPrefixes {
		if strings.HasPrefix(lower, prefix) {
			return true
		}
	}
	return false
}

type ytDlpProgress struct {
	Downloaded int64
	Total      int64
	Speed      string
	ETA        string
	Fragment   string
}

// parseYtDlpProgress reads one line produced by our --progress-template.
func parseYtDlpProgress(line string) (ytDlpProgress, bool) {
	if !strings.HasPrefix(line, ytDlpProgressPrefix) {
		return ytDlpProgress{}, false
	}
	fields := strings.Split(strings.TrimPrefix(line, ytDlpProgressPrefix), "|")
	if len(fields) < 5 {
		return ytDlpProgress{}, false
	}
	progress := ytDlpProgress{
		Downloaded: parseYtDlpNumber(fields[0]),
		Total:      parseYtDlpNumber(fields[1]),
		Speed:      humanSpeed(parseYtDlpNumber(fields[3])),
		ETA:        humanClock(float64(parseYtDlpNumber(fields[4]))),
	}
	if progress.Total <= 0 {
		progress.Total = parseYtDlpNumber(fields[2])
	}
	if len(fields) > 5 {
		progress.Fragment = strings.TrimSpace(fields[5])
	}
	return progress, true
}

func parseYtDlpNumber(value string) int64 {
	value = strings.TrimSpace(value)
	if value == "" || strings.EqualFold(value, "na") || strings.EqualFold(value, "none") {
		return -1
	}
	if number, err := strconv.ParseFloat(value, 64); err == nil {
		return int64(number)
	}
	return -1
}

func humanSpeed(bytesPerSecond int64) string {
	if bytesPerSecond <= 0 {
		return ""
	}
	value := float64(bytesPerSecond)
	for _, unit := range []string{"B/s", "KB/s", "MB/s", "GB/s"} {
		if value < 1024 || unit == "GB/s" {
			return fmt.Sprintf("%.1f %s", value, unit)
		}
		value /= 1024
	}
	return ""
}

// downloadWithYtDlp hands the download to yt-dlp, which merges the best video and
// audio with our FFmpeg, and reports progress through the same queue events.
func (a *App) downloadWithYtDlp(ctx context.Context, ffmpeg string, item DownloadItem, outputPath string,
	index, total, maxHeight int, displayName string) error {
	path := a.findYtDlp()
	if path == "" {
		return fmt.Errorf("chưa có yt-dlp")
	}
	base := strings.TrimSuffix(outputPath, filepath.Ext(outputPath))
	arguments := []string{
		"--no-colors", "--newline", "--no-playlist", "--no-warnings",
		"--progress-template", "download:" + ytDlpProgressPrefix +
			"%(progress.downloaded_bytes)s|%(progress.total_bytes)s|%(progress.total_bytes_estimate)s|" +
			"%(progress.speed)s|%(progress.eta)s|%(progress.fragment_index)s",
		"-f", ytDlpFormat(maxHeight),
		"--merge-output-format", "mp4",
		"-o", base + ".%(ext)s",
	}
	if isYouTubeURL(item.PageURL) {
		arguments = append(arguments, ytDlpPacingArgs()...)
	}
	// The PO token, minted from the local provider, is what makes YouTube hand
	// over media URLs instead of answering 403.
	arguments = append(arguments, a.ytDlpRequestArgs(item.PageURL)...)
	if ffmpeg != "" {
		arguments = append(arguments, "--ffmpeg-location", ffmpeg)
	}
	arguments = append(arguments, item.PageURL)

	command := exec.CommandContext(ctx, path, arguments...)
	prepareBackgroundCommand(command)
	output, err := command.StdoutPipe()
	if err != nil {
		return err
	}
	var problems strings.Builder
	command.Stderr = &problems
	if err := command.Start(); err != nil {
		return err
	}
	a.setActiveFFmpeg(command.Process)
	defer a.clearActiveFFmpeg(command.Process)
	if superviseErr := superviseProcess(command.Process); superviseErr != nil {
		a.emit("download:log", fmt.Sprintf("%s — %v", displayName, superviseErr))
	}

	watchdog := a.watchStall(command.Process, displayName)
	lastEmit := time.Time{}
	lastDownloaded := int64(-1)
	scanner := bufio.NewScanner(output)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		// Any output at all means yt-dlp is alive, which matters because the
		// stall watchdog would otherwise fire between phases.
		if line != "" {
			watchdog.progressed()
		}
		if isYtDlpPostProcessing(line) {
			// Merging a long 4K file runs for minutes without printing anything,
			// so the watchdog is retired and stopping is left to the user.
			watchdog.stop()
			a.emit("download:log", fmt.Sprintf("%s — %s", displayName, line))
			continue
		}
		progress, ok := parseYtDlpProgress(line)
		if !ok {
			continue
		}
		if progress.Downloaded > lastDownloaded {
			lastDownloaded = progress.Downloaded
			watchdog.progressed()
		}
		if time.Since(lastEmit) <= 300*time.Millisecond {
			continue
		}
		lastEmit = time.Now()
		percent := -1.0
		if progress.Total > 0 && progress.Downloaded >= 0 {
			percent = progressPercent(float64(progress.Downloaded), float64(progress.Total))
		}
		a.emit("download:progress", ProgressEvent{
			ID: item.ID, Index: index, Total: total, Name: displayName,
			Time: humanBytes(progress.Downloaded), Duration: humanBytes(progress.Total),
			Speed: progress.Speed, Percent: percent,
		})
	}
	waitErr := command.Wait()
	stalled := watchdog.stop()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if stalled {
		return fmt.Errorf("%w: yt-dlp không tiến triển trong %s", errStalled, stallTimeout)
	}
	if waitErr != nil {
		return fmt.Errorf("yt-dlp: %v — %s", waitErr, lastLine(problems.String()))
	}
	produced, err := findProducedFile(base, outputPath)
	if err != nil {
		return err
	}
	if produced != outputPath {
		_ = os.Remove(outputPath)
		if err := os.Rename(produced, outputPath); err != nil {
			return err
		}
	}
	return nil
}

func humanBytes(value int64) string {
	if value <= 0 {
		return ""
	}
	size := float64(value)
	for _, unit := range []string{"B", "KB", "MB", "GB"} {
		if size < 1024 || unit == "GB" {
			return fmt.Sprintf("%.1f %s", size, unit)
		}
		size /= 1024
	}
	return ""
}

// findProducedFile locates what yt-dlp wrote, because the container depends on
// the formats YouTube served.
func findProducedFile(base, preferred string) (string, error) {
	if info, err := os.Stat(preferred); err == nil && info.Size() > 0 {
		return preferred, nil
	}
	matches, err := filepath.Glob(base + ".*")
	if err != nil {
		return "", err
	}
	best, bestSize := "", int64(0)
	for _, match := range matches {
		if strings.HasSuffix(match, ".part") || strings.Contains(match, ".part-") {
			continue
		}
		info, statErr := os.Stat(match)
		if statErr != nil || info.IsDir() || info.Size() <= bestSize {
			continue
		}
		best, bestSize = match, info.Size()
	}
	if best == "" {
		return "", fmt.Errorf("yt-dlp không tạo được file video")
	}
	return best, nil
}
