package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// What the YouTube home page carries: the visitor id of the session, and the
// account id when the request arrived with a login.
const signedInPage = `<!DOCTYPE html><html><script>
ytcfg.set({"INNERTUBE_CONTEXT":{"client":{"visitorData":"Cgt2aXNpdG9yLTEyMxjAtQ=="}},
"DATASYNC_ID":"110445566778899||","LOGGED_IN":true});
</script></html>`

const signedOutPage = `<!DOCTYPE html><html><script>
ytcfg.set({"VISITOR_DATA":"CgtBTk9OWU1PVVMtOQ%3D%3D","DATASYNC_ID":"||","LOGGED_IN":false});
</script></html>`

func TestParseSignedInSession(t *testing.T) {
	session := parseYouTubeSession(signedInPage)
	// The escapes of the JSON string must be decoded, or the visitor id yt-dlp
	// receives is not the one the token was minted for.
	if session.VisitorData != "Cgt2aXNpdG9yLTEyMxjAtQ==" {
		t.Fatalf("visitor data not decoded: %q", session.VisitorData)
	}
	if !session.signedIn() || session.DataSyncID != "110445566778899" {
		t.Fatalf("account id not read: %+v", session)
	}
	// A signed-in token is bound to the account, not to the visitor.
	if session.binding() != "110445566778899" {
		t.Fatalf("wrong binding: %q", session.binding())
	}
}

func TestParseSignedOutSession(t *testing.T) {
	session := parseYouTubeSession(signedOutPage)
	if session.VisitorData != "CgtBTk9OWU1PVVMtOQ%3D%3D" {
		t.Fatalf("visitor data missing: %q", session.VisitorData)
	}
	// "||" is the empty account id; it must not be mistaken for a login.
	if session.signedIn() {
		t.Fatalf("an empty sync id must not count as signed in: %+v", session)
	}
	if session.binding() != session.VisitorData {
		t.Fatalf("an anonymous token binds to the visitor: %q", session.binding())
	}
}

func TestSessionExpiresSoTheNextQueueOpensANewOne(t *testing.T) {
	if (youTubeSession{}).usable() {
		t.Fatal("a session without a visitor id is never usable")
	}
	fresh := youTubeSession{VisitorData: "V", OpenedAt: time.Now()}
	if !fresh.usable() {
		t.Fatal("a session just opened must be reused")
	}
	stale := youTubeSession{VisitorData: "V", OpenedAt: time.Now().Add(-youTubeSessionTTL - time.Minute)}
	if stale.usable() {
		t.Fatal("an old session must be replaced")
	}
}

// The session is opened with the cookies of the store so that a signed-in file
// yields an account session, and the cookies YouTube rotates in reply are handed
// back for saving — which is what stops a cookie file from dying after one use.
func TestOpenSessionSendsCookiesAndReturnsTheNewOnes(t *testing.T) {
	received := ""
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, request *http.Request) {
		received = request.Header.Get("Cookie")
		http.SetCookie(writer, &http.Cookie{
			Name: "VISITOR_INFO1_LIVE", Value: "rotated", Domain: ".youtube.com",
			Path: "/", Secure: true, MaxAge: 600,
		})
		_, _ = writer.Write([]byte(signedInPage))
	}))
	defer server.Close()

	stored := []netscapeCookie{
		{Domain: ".youtube.com", IncludeSub: true, Path: "/", Name: "SID", Value: "abc"},
		{Domain: ".udemy.com", IncludeSub: true, Path: "/", Name: "OTHER", Value: "no"},
		{Domain: ".youtube.com", IncludeSub: true, Path: "/", Name: "OLD", Value: "gone",
			Expires: time.Now().Add(-time.Hour).Unix()},
	}
	session, fresh, err := openYouTubeSession(server.URL, stored)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(received, "SID=abc") {
		t.Fatalf("the store cookies were not sent: %q", received)
	}
	if strings.Contains(received, "OTHER") {
		t.Fatalf("another site's cookies must stay behind: %q", received)
	}
	if strings.Contains(received, "OLD") {
		t.Fatalf("an expired cookie must not be sent: %q", received)
	}
	if !session.signedIn() {
		t.Fatalf("the page reported a login: %+v", session)
	}
	if len(fresh) != 1 || fresh[0].Name != "VISITOR_INFO1_LIVE" || fresh[0].Value != "rotated" {
		t.Fatalf("the rotated cookie was not returned: %+v", fresh)
	}
	if fresh[0].Expires <= time.Now().Unix() {
		t.Fatalf("max-age was not turned into an expiry: %+v", fresh[0])
	}
}

func TestOpenSessionFailsWhenThereIsNoVisitorData(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(writer http.ResponseWriter, _ *http.Request) {
		_, _ = writer.Write([]byte("<html>đang bảo trì</html>"))
	}))
	defer server.Close()
	// Guessing a session would mint a token bound to nothing, so this has to
	// fail and let the caller fall back.
	if _, _, err := openYouTubeSession(server.URL, nil); err == nil {
		t.Fatal("a page without a visitor id must be an error")
	}
}

func TestCookieHeaderMatchesSubdomains(t *testing.T) {
	cookies := []netscapeCookie{
		{Domain: ".youtube.com", IncludeSub: true, Path: "/", Name: "A", Value: "1"},
		{Domain: "www.youtube.com", Path: "/", Name: "B", Value: "2"},
		{Domain: "m.youtube.com", Path: "/", Name: "C", Value: "3"},
	}
	header := cookieHeader(cookies, "www.youtube.com")
	if !strings.Contains(header, "A=1") || !strings.Contains(header, "B=2") {
		t.Fatalf("matching cookies missing: %q", header)
	}
	if strings.Contains(header, "C=3") {
		t.Fatalf("a cookie of another host must not be sent: %q", header)
	}
}

// TestLiveYouTubeSession opens a real session with the cookies on this machine.
// Guarded by an environment variable so a normal run never reaches the network.
func TestLiveYouTubeSession(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_YOUTUBE") == "" {
		t.Skip("đặt MOTCHILL_LIVE_YOUTUBE=1 để chạy thử với YouTube thật")
	}
	stored := loadCookieStore()
	t.Logf("store có %d cookie, gửi đi: %d", len(stored),
		len(strings.Split(cookieHeader(stored, "www.youtube.com"), "; ")))
	session, fresh, err := openYouTubeSession(youTubeHomeURL, stored)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("visitor=%q đăng nhập=%v binding=%q cookie mới=%d",
		session.VisitorData, session.signedIn(), shortVisitor(session.binding()), len(fresh))
	for _, cookie := range fresh {
		t.Logf("  cookie mới: %s (%s)", cookie.Name, cookie.Domain)
	}
	if session.VisitorData == "" {
		t.Fatal("không đọc được visitor data từ trang thật")
	}
}

// TestLiveYouTubeResolve runs the whole chain the way an analysis does: open a
// session, mint a token bound to it, and let yt-dlp resolve a real video with
// the arguments that come out. It is the only way to see whether YouTube still
// answers "Sign in to confirm you're not a bot".
func TestLiveYouTubeResolve(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_YOUTUBE") == "" {
		t.Skip("đặt MOTCHILL_LIVE_YOUTUBE=1 để chạy thử với YouTube thật")
	}
	app := NewApp()
	ytDlp := app.findYtDlp()
	if ytDlp == "" {
		t.Skip("chưa có yt-dlp")
	}
	target := os.Getenv("MOTCHILL_LIVE_YOUTUBE_URL")
	if target == "" {
		target = "https://www.youtube.com/watch?v=jNQXAC9IVRw"
	}

	// The arguments hide why a path was taken, so the two steps behind them are
	// reported first: without this a fallback looks like a success.
	session, sessionErr := app.youTubeSessionFor(app.settings.snapshot().CookieSource)
	t.Logf("phiên: đăng nhập=%v lỗi=%v", session.signedIn(), sessionErr)
	if status := app.GetProviderStatus(); !status.Running && status.ServerInstalled {
		started, startErr := app.StartPOTokenProvider(status.Port)
		t.Logf("khởi động provider: %+v lỗi=%v", started.Running, startErr)
		defer app.StopPOTokenProvider()
	}
	token, tokenErr := app.providerToken(app.GetYtDlpTuning(), session)
	t.Logf("token: dài %d, binding khớp phiên=%v lỗi=%v",
		len(token.Token), token.Binding == session.binding(), tokenErr)

	arguments := app.ytDlpRequestArgs(target)
	for _, argument := range arguments {
		t.Logf("  tham số: %s", argument)
	}
	if len(arguments) == 0 {
		t.Fatal("không có tham số nào được dựng")
	}

	arguments = append(arguments, "--simulate", "--no-playlist", "--no-colors",
		"--print", "%(height)sp %(format_id)s", target)
	command := exec.Command(ytDlp, arguments...)
	prepareBackgroundCommand(command)
	output, err := command.CombinedOutput()
	t.Logf("yt-dlp: %s", strings.TrimSpace(string(output)))
	if err != nil {
		t.Fatalf("không resolve được video: %v", err)
	}
}

// TestLiveClientsWithoutToken is the check to rerun when YouTube changes again:
// it downloads one adaptive format per player client and reports which of them
// still hand over media URLs that work. web_embedded is the current answer.
func TestLiveClientsWithoutToken(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_YOUTUBE") == "" {
		t.Skip("đặt MOTCHILL_LIVE_YOUTUBE=1 để chạy thử với YouTube thật")
	}
	app := NewApp()
	ytDlp := app.findYtDlp()
	if ytDlp == "" {
		t.Skip("chưa có yt-dlp")
	}
	videoID := os.Getenv("MOTCHILL_LIVE_VIDEO_ID")
	if videoID == "" {
		videoID = "QcZqImRCNrg"
	}
	target := "https://www.youtube.com/watch?v=" + videoID
	cookies := app.youTubeCookieArgs(app.settings.snapshot().CookieSource)

	for _, client := range []string{"", "android_vr", "tv", "web_embedded", "ios"} {
		for _, withCookies := range []bool{true, false} {
			arguments := append([]string{}, jsRuntimeArgs()...)
			if withCookies {
				arguments = append(arguments, cookies...)
			}
			if client != "" {
				arguments = append(arguments, "--extractor-args", "youtube:player_client="+client)
			}
			arguments = append(arguments, "--no-colors", "--newline", "--no-playlist",
				"-f", "bestaudio", "-o", filepath.Join(t.TempDir(), "probe.%(ext)s"), target)
			ctx, cancel := context.WithTimeout(context.Background(), 90*time.Second)
			command := exec.CommandContext(ctx, ytDlp, arguments...)
			prepareBackgroundCommand(command)
			output, runErr := command.CombinedOutput()
			cancel()
			text := strings.ReplaceAll(string(output), "\r", "\n")
			problem := ""
			for _, line := range strings.Split(text, "\n") {
				if strings.Contains(line, "ERROR") {
					problem = strings.TrimSpace(line)
				}
			}
			name := client
			if name == "" {
				name = "(mặc định)"
			}
			t.Logf("%-13s cookie=%-5v → %-5v | %s", name, withCookies, runErr == nil, problem)
		}
	}
}

// TestLiveEmbeddedDownload runs the queue's own format spec through web_embedded
// for a minute. mweb was cut off with a 403 a few percent in, so anything that
// keeps climbing past that point is the answer.
func TestLiveEmbeddedDownload(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_YOUTUBE") == "" {
		t.Skip("đặt MOTCHILL_LIVE_YOUTUBE=1 để chạy thử với YouTube thật")
	}
	app := NewApp()
	ytDlp := app.findYtDlp()
	if ytDlp == "" {
		t.Skip("chưa có yt-dlp")
	}
	videoID := os.Getenv("MOTCHILL_LIVE_VIDEO_ID")
	if videoID == "" {
		videoID = "QcZqImRCNrg"
	}
	target := "https://www.youtube.com/watch?v=" + videoID

	// Exactly what the queue runs, so the wiring is covered along with the client.
	arguments := append([]string{}, app.ytDlpRequestArgs(target)...)
	arguments = append(arguments, "--no-colors", "--newline", "--no-playlist",
		"-f", ytDlpFormat(2160), "--merge-output-format", "mp4",
		"--ffmpeg-location", app.findFFmpeg(),
		"-o", filepath.Join(t.TempDir(), "probe.%(ext)s"), target)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()
	command := exec.CommandContext(ctx, ytDlp, arguments...)
	prepareBackgroundCommand(command)
	output, _ := command.CombinedOutput()

	text := strings.ReplaceAll(string(output), "\r", "\n")
	reached, chosen, problem := "", "", ""
	for _, line := range strings.Split(text, "\n") {
		switch {
		case strings.Contains(line, "% of"):
			reached = strings.TrimSpace(line)
		case strings.Contains(line, "Downloading 1 format"), strings.Contains(line, "Downloading 2 format"):
			chosen = strings.TrimSpace(line)
		case strings.Contains(line, "ERROR"):
			problem = strings.TrimSpace(line)
		}
	}
	t.Logf("chọn : %s", chosen)
	t.Logf("tới  : %s", reached)
	if problem != "" {
		t.Fatalf("tải bị chặn: %s", problem)
	}
}
