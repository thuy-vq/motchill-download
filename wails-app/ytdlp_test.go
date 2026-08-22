package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestYouTubeURLDetection(t *testing.T) {
	youTube := []string{
		"https://www.youtube.com/watch?v=dQw4w9WgXcQ",
		"https://youtube.com/watch?v=abc&list=PL123",
		"https://m.youtube.com/watch?v=abc",
		"https://music.youtube.com/watch?v=abc",
		"https://youtu.be/abc",
		"https://www.youtube.com/playlist?list=PL123",
		"https://www.youtube-nocookie.com/embed/abc",
	}
	for _, value := range youTube {
		if !isYouTubeURL(value) {
			t.Fatalf("%s should be recognised as YouTube", value)
		}
	}
	others := []string{
		"https://motphimchill.cc/phim/gantz",
		"https://notyoutube.com/watch?v=abc",
		"https://youtube.com.evil.example/watch?v=abc",
		"https://cdn.example/index.m3u8",
		"", "not a url",
	}
	for _, value := range others {
		if isYouTubeURL(value) {
			t.Fatalf("%s must not be treated as YouTube", value)
		}
	}
}

// The logs showed two faces of the same block: a bot challenge, and a 403 on the
// media URL. Both must be recognised so the queue stops and explains itself.
func TestThrottleFailuresAreRecognised(t *testing.T) {
	blocked := []string{
		`yt-dlp: exit status 1 — ERROR: [youtube] Mvhrc0sUQHI: Sign in to confirm you're not a bot. Use --cookies-from-browser`,
		"yt-dlp: exit status 1 — ERROR: unable to download video data: HTTP Error 403: Forbidden",
		"ERROR: fragment 3 not found, 403: Forbidden",
		"Your account has been rate-limited by YouTube for up to an hour",
		"HTTP Error 429: Too Many Requests",
	}
	for _, message := range blocked {
		if !isThrottleFailure(errors.New(message)) {
			t.Fatalf("must be seen as throttling: %q", message)
		}
	}
	others := []error{
		nil,
		errors.New("yt-dlp: exit status 1 — ERROR: Video unavailable"),
		errors.New("FFmpeg treo: yt-dlp không tiến triển trong 1m30s"),
		errors.New("HTTP Error 404: Not Found"),
	}
	for _, err := range others {
		if isThrottleFailure(err) {
			t.Fatalf("must not be seen as throttling: %v", err)
		}
	}
}

func TestExplicitAccountRateLimitIsRecognised(t *testing.T) {
	for _, message := range []string{
		"Your account has been rate-limited by YouTube for up to an hour",
		"rate limited by YouTube",
		"HTTP Error 429: Too Many Requests",
	} {
		if !isAccountRateLimitFailure(errors.New(message)) {
			t.Fatalf("must be seen as an account rate limit: %q", message)
		}
	}
	for _, err := range []error{nil, errors.New("Sign in to confirm you're not a bot"), errors.New("HTTP Error 403")} {
		if isAccountRateLimitFailure(err) {
			t.Fatalf("must not be seen as an account rate limit: %v", err)
		}
	}
}

func TestThrottleHintDependsOnCookies(t *testing.T) {
	withoutCookies := throttleHint("")
	if !strings.Contains(withoutCookies, "Cookie đăng nhập") {
		t.Fatalf("the hint must point at the cookie setting: %q", withoutCookies)
	}
	withCookies := throttleHint("firefox")
	if strings.Contains(withCookies, "Hãy chọn Cookie đăng nhập") {
		t.Fatalf("with cookies set, the hint must move on: %q", withCookies)
	}
	if !strings.Contains(withCookies, "chờ") {
		t.Fatalf("the hint should suggest waiting: %q", withCookies)
	}
}

func TestPacingArgsSlowTheRequestsDown(t *testing.T) {
	args := strings.Join(ytDlpPacingArgs(), " ")
	for _, flag := range []string{"--sleep-requests 0.75", "--sleep-interval 10", "--max-sleep-interval 20", "--retries", "--extractor-retries", "extractor:exp=5:60"} {
		if !strings.Contains(args, flag) {
			t.Fatalf("%s is missing from %q", flag, args)
		}
	}
}

func TestDownloadDelayCanBeCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if waitForDownloadDelay(ctx, time.Hour) {
		t.Fatal("cancelled delay must stop immediately")
	}
	if !waitForDownloadDelay(context.Background(), time.Millisecond) {
		t.Fatal("short delay should complete")
	}
}

func TestPrefersYtDlpCoversLoggedInSites(t *testing.T) {
	handled := []string{
		"https://www.udemy.com/course/backend-master-class-golang-postgresql-kubernetes/learn/lecture/25822316",
		// Udemy Business lives on a per-organisation subdomain.
		"https://lg.udemy.com/course/backend-master-class-golang-postgresql-kubernetes/learn/lecture/25822316",
		"https://vimeo.com/123456",
		"https://www.coursera.org/learn/something",
		"https://www.youtube.com/watch?v=abc",
		"https://www.tiktok.com/@user/video/123",
	}
	for _, value := range handled {
		if !prefersYtDlp(value) {
			t.Fatalf("%s should be routed to yt-dlp", value)
		}
	}
	others := []string{
		"https://motphimchill.cc/phim/gantz",
		"https://udemy.com.evil.example/course/x",
		"https://cdn.example/index.m3u8",
		"", "not a url",
	}
	for _, value := range others {
		if prefersYtDlp(value) {
			t.Fatalf("%s must stay on the HTML extractor", value)
		}
	}
}

func TestCookieArgsNeverCarryCredentials(t *testing.T) {
	if args := ytDlpAuthArgs(""); args != nil {
		t.Fatalf("no cookie source must add no arguments, got %v", args)
	}
	if args := ytDlpAuthArgs("none"); args != nil {
		t.Fatalf("\"none\" must add no arguments, got %v", args)
	}
	browser := ytDlpAuthArgs("chrome")
	if len(browser) != 2 || browser[0] != "--cookies-from-browser" || browser[1] != "chrome" {
		t.Fatalf("unexpected browser arguments: %v", browser)
	}
	file := ytDlpAuthArgs(`file:D:\cookies.txt`)
	if len(file) != 2 || file[0] != "--cookies" || file[1] != `D:\cookies.txt` {
		t.Fatalf("unexpected cookie file arguments: %v", file)
	}
	if args := ytDlpAuthArgs("file:"); args != nil {
		t.Fatalf("an empty path must be ignored, got %v", args)
	}
	// No argument may ever be a username or password flag.
	for _, source := range []string{"", "none", "chrome", "file:x"} {
		for _, argument := range ytDlpAuthArgs(source) {
			if strings.Contains(argument, "--username") || strings.Contains(argument, "--password") {
				t.Fatalf("credentials must never be passed: %v", argument)
			}
		}
	}
}

// Merging a long 4K file prints nothing for minutes; the stall watchdog used to
// kill yt-dlp in the middle of it, which failed exactly the biggest episodes.
func TestPostProcessingLinesAreRecognised(t *testing.T) {
	postProcessing := []string{
		"[Merger] Merging formats into \"D:\\Videos\\Tap 01.mp4\"",
		"[VideoRemuxer] Remuxing video from webm to mp4",
		"[FixupM3u8] Fixing MPEG-TS in MP4 container",
		"[ExtractAudio] Destination: audio.m4a",
		"[Metadata] Adding metadata to \"x.mp4\"",
	}
	for _, line := range postProcessing {
		if !isYtDlpPostProcessing(line) {
			t.Fatalf("%q must be seen as post-processing", line)
		}
	}
	for _, line := range []string{
		"[download] Destination: x.f137.mp4",
		"[youtube] abc: Downloading webpage",
		"MOTCHILL|1|2|NA|3|4|5", "",
	} {
		if isYtDlpPostProcessing(line) {
			t.Fatalf("%q must not be seen as post-processing", line)
		}
	}
}

func TestParseYtDlpPlaylistDump(t *testing.T) {
	data := []byte(`{
		"_type": "playlist",
		"id": "PL123",
		"title": "Nhạc hay 2026",
		"webpage_url": "https://www.youtube.com/playlist?list=PL123",
		"entries": [
			{"id": "aaa", "title": "Bài 1", "url": "https://www.youtube.com/watch?v=aaa", "playlist_index": 1},
			{"id": "bbb", "title": "Bài 2", "url": "bbb", "playlist_index": 2},
			{"id": "ccc", "title": "", "webpage_url": "https://www.youtube.com/watch?v=ccc"}
		]
	}`)
	result, err := parseYtDlpDump(data, "https://www.youtube.com/playlist?list=PL123")
	if err != nil {
		t.Fatal(err)
	}
	if result.Title != "Nhạc hay 2026" || len(result.Episodes) != 3 {
		t.Fatalf("unexpected result: %+v", result)
	}
	if result.Streams == nil {
		t.Fatal("streams must serialise as [] rather than null")
	}
	if result.Episodes[0].ID != "yt-aaa" || result.Episodes[0].Name != "Bài 1" || result.Episodes[0].Number != 1 {
		t.Fatalf("unexpected first episode: %+v", result.Episodes[0])
	}
	// An entry whose "url" is only the video id still becomes a watch link.
	if result.Episodes[1].PageURL != "https://www.youtube.com/watch?v=bbb" {
		t.Fatalf("unexpected second URL: %s", result.Episodes[1].PageURL)
	}
	// A missing title falls back to a numbered name, and a missing index counts.
	if result.Episodes[2].Number != 3 || result.Episodes[2].Name != "Video 03" {
		t.Fatalf("unexpected third episode: %+v", result.Episodes[2])
	}
}

func TestParseYtDlpSingleVideoDump(t *testing.T) {
	data := []byte(`{"id":"dQw4w9WgXcQ","title":"Một video","webpage_url":"https://www.youtube.com/watch?v=dQw4w9WgXcQ"}`)
	result, err := parseYtDlpDump(data, "https://youtu.be/dQw4w9WgXcQ")
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Episodes) != 1 {
		t.Fatalf("a single video must give one episode: %+v", result.Episodes)
	}
	episode := result.Episodes[0]
	if episode.ID != "yt-dQw4w9WgXcQ" || episode.Number != 0 || !episode.Current {
		t.Fatalf("unexpected episode: %+v", episode)
	}
	if episode.PageURL != "https://www.youtube.com/watch?v=dQw4w9WgXcQ" {
		t.Fatalf("unexpected URL: %s", episode.PageURL)
	}
}

func TestParseYtDlpDumpRejectsEmptyPayloads(t *testing.T) {
	if _, err := parseYtDlpDump([]byte(`{}`), "x"); err == nil {
		t.Fatal("an empty dump must be an error")
	}
	if _, err := parseYtDlpDump([]byte(`{"_type":"playlist","entries":[]}`), "x"); err == nil {
		t.Fatal("an empty playlist must be an error")
	}
	if _, err := parseYtDlpDump([]byte(`not json`), "x"); err == nil {
		t.Fatal("broken JSON must be an error")
	}
}

func TestParseYtDlpProgressLines(t *testing.T) {
	progress, ok := parseYtDlpProgress("MOTCHILL|5242880|10485760|NA|1048576|30|12")
	if !ok {
		t.Fatal("a template line must parse")
	}
	if progress.Downloaded != 5242880 || progress.Total != 10485760 {
		t.Fatalf("unexpected bytes: %+v", progress)
	}
	if progress.Speed != "1.0 MB/s" || progress.ETA != "0:30" {
		t.Fatalf("unexpected speed/eta: %+v", progress)
	}
	if got := progressPercent(float64(progress.Downloaded), float64(progress.Total)); got != 50 {
		t.Fatalf("percent = %v, want 50", got)
	}

	// Live streams report no total, so the estimate is used instead.
	estimated, ok := parseYtDlpProgress("MOTCHILL|1000|NA|4000|None|NA|NA")
	if !ok || estimated.Total != 4000 {
		t.Fatalf("estimate fallback failed: %+v", estimated)
	}
	if estimated.Speed != "" || estimated.ETA != "" {
		t.Fatalf("unknown speed/eta must stay empty: %+v", estimated)
	}
	if _, ok := parseYtDlpProgress("[download]  50.0% of 10.00MiB"); ok {
		t.Fatal("plain yt-dlp output must not be parsed as a template line")
	}
}

func TestYtDlpFormatHonoursTheHeightCap(t *testing.T) {
	if got := ytDlpFormat(0); got != "bv*+ba/b" {
		t.Fatalf("no cap should ask for the best: %q", got)
	}
	got := ytDlpFormat(1080)
	if !strings.Contains(got, "height<=1080") || !strings.HasSuffix(got, "/bv*+ba/b") {
		t.Fatalf("capped format looks wrong: %q", got)
	}
}

func TestFindProducedFilePicksTheRealOutput(t *testing.T) {
	directory := t.TempDir()
	base := filepath.Join(directory, "Video")
	preferred := base + ".mp4"

	// yt-dlp settled on webm, and left a fragment behind.
	if err := os.WriteFile(base+".webm", []byte("0123456789"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(base+".f140.mp4.part", []byte("012"), 0o600); err != nil {
		t.Fatal(err)
	}
	found, err := findProducedFile(base, preferred)
	if err != nil {
		t.Fatal(err)
	}
	if found != base+".webm" {
		t.Fatalf("found %s, want the webm file", found)
	}

	// Once the mp4 exists it wins, because that is what the queue expects.
	if err := os.WriteFile(preferred, []byte("01234"), 0o600); err != nil {
		t.Fatal(err)
	}
	if found, err = findProducedFile(base, preferred); err != nil || found != preferred {
		t.Fatalf("found %s (%v), want %s", found, err, preferred)
	}

	if _, err := findProducedFile(filepath.Join(directory, "Missing"), filepath.Join(directory, "Missing.mp4")); err == nil {
		t.Fatal("a download that produced nothing must be an error")
	}
}

func TestHumanBytesAndSpeed(t *testing.T) {
	cases := map[int64]string{0: "", -1: "", 512: "512.0 B", 2048: "2.0 KB", 5 * 1024 * 1024: "5.0 MB"}
	for value, expected := range cases {
		if got := humanBytes(value); got != expected {
			t.Fatalf("humanBytes(%d) = %q, want %q", value, got, expected)
		}
	}
	if got := humanSpeed(1536); got != "1.5 KB/s" {
		t.Fatalf("humanSpeed = %q", got)
	}
	if got := humanSpeed(0); got != "" {
		t.Fatalf("unknown speed must be empty, got %q", got)
	}
}

// TestLiveYtDlp installs yt-dlp and drives a real YouTube download, so it needs
// network access and stays skipped by default:
//
//	MOTCHILL_LIVE_YTDLP=1 go test -run TestLiveYtDlp -v
//
// Point MOTCHILL_LIVE_YT_VIDEO / MOTCHILL_LIVE_YT_PLAYLIST at other links to
// check them instead.
func TestLiveYtDlp(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_YTDLP") == "" {
		t.Skip("MOTCHILL_LIVE_YTDLP is not set")
	}
	app := NewApp()

	status, err := app.InstallYtDlp()
	if err != nil {
		t.Fatalf("cài yt-dlp thất bại: %v", err)
	}
	if !status.Ready || status.Version == "" {
		t.Fatalf("yt-dlp chưa sẵn sàng: %+v", status)
	}
	t.Logf("yt-dlp %s tại %s", status.Version, status.Path)

	video := os.Getenv("MOTCHILL_LIVE_YT_VIDEO")
	if video == "" {
		// 19 seconds long, the smallest public video that always exists.
		video = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	}
	single, err := app.analyzeWithYtDlp(context.Background(), video)
	if err != nil {
		t.Fatalf("phân tích video thất bại: %v", err)
	}
	t.Logf("video: title=%q episodes=%d first=%s", single.Title, len(single.Episodes), single.Episodes[0].PageURL)
	if len(single.Episodes) != 1 || single.Episodes[0].PageURL == "" {
		t.Fatalf("một video phải cho một tập: %+v", single.Episodes)
	}

	if playlist := os.Getenv("MOTCHILL_LIVE_YT_PLAYLIST"); playlist != "" {
		result, err := app.analyzeWithYtDlp(context.Background(), playlist)
		if err != nil {
			t.Fatalf("phân tích playlist thất bại: %v", err)
		}
		t.Logf("playlist: title=%q episodes=%d", result.Title, len(result.Episodes))
		for index, episode := range result.Episodes {
			if index >= 3 {
				break
			}
			t.Logf("  #%d %s → %s", episode.Number, episode.Name, episode.PageURL)
		}
		if len(result.Episodes) < 2 {
			t.Fatalf("playlist phải có nhiều tập: %+v", result.Episodes)
		}
	}

	// Download the smallest rendition into a temporary folder.
	directory := t.TempDir()
	outputPath := filepath.Join(directory, "live.mp4")
	item := DownloadItem{ID: "live", Name: single.Episodes[0].Name, PageURL: single.Episodes[0].PageURL}
	if err := app.downloadWithYtDlp(context.Background(), app.findFFmpeg(), item, outputPath, 1, 1, 144, "live"); err != nil {
		t.Fatalf("tải video thất bại: %v", err)
	}
	info, err := os.Stat(outputPath)
	if err != nil {
		t.Fatalf("không thấy file đầu ra: %v", err)
	}
	if info.Size() < 10*1024 {
		t.Fatalf("file đầu ra quá nhỏ: %d bytes", info.Size())
	}
	t.Logf("đã tải %s (%.1f KB)", filepath.Base(outputPath), float64(info.Size())/1024)
}

func TestYouTubeFilesAreNamedAfterTheVideo(t *testing.T) {
	// Sanity check on the naming rule used by the queue for YouTube items.
	if got := sanitizeFileName(`Bài 1: "Xin chào" / 2026`); strings.ContainsAny(got, `<>:"/\|?*`) {
		t.Fatalf("sanitised name still holds invalid characters: %q", got)
	}
}
