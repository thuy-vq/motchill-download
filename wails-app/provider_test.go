package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSafeJoinBlocksEscapingEntries(t *testing.T) {
	target := filepath.Join(t.TempDir(), "out")
	for _, name := range []string{"../evil.txt", "..\\evil.txt", "/etc/passwd", "a/../../evil"} {
		if _, err := safeJoin(target, name); err == nil {
			t.Fatalf("%q must be refused", name)
		}
	}
	good, err := safeJoin(target, "server/build/main.js")
	if err != nil {
		t.Fatal(err)
	}
	if !strings.HasPrefix(good, target) {
		t.Fatalf("safe entry landed outside the target: %s", good)
	}
}

func TestProviderPathsSitUnderTheToolFolder(t *testing.T) {
	root := toolInstallRoot()
	for _, path := range []string{providerRoot(), providerServerDir(), providerPluginDir(), providerBuiltEntry()} {
		if !strings.HasPrefix(path, root) {
			t.Fatalf("%s must stay under %s", path, root)
		}
	}
}

func TestProviderStatusWithoutAnInstall(t *testing.T) {
	// A missing install must read as "not installed", never as an error.
	app := NewApp()
	status := app.GetProviderStatus()
	if status.Port != providerDefaultPort {
		t.Fatalf("default port = %d, want %d", status.Port, providerDefaultPort)
	}
	if status.Running {
		t.Fatal("nothing was started, so nothing may report as running")
	}
}

// TestLiveProviderInstall downloads and builds the real provider, so it needs
// network access, Node and npm:
//
//	MOTCHILL_LIVE_PROVIDER=1 go test -run TestLiveProviderInstall -v -timeout 900s
func TestLiveProviderInstall(t *testing.T) {
	if os.Getenv("MOTCHILL_LIVE_PROVIDER") == "" {
		t.Skip("MOTCHILL_LIVE_PROVIDER is not set")
	}
	app := NewApp()
	status, err := app.InstallPOTokenProvider()
	if err != nil {
		t.Fatalf("cài provider thất bại: %v", err)
	}
	t.Logf("plugin=%v server=%v version=%s node=%s",
		status.PluginInstalled, status.ServerInstalled, status.Version, status.NodeVersion)
	if !status.PluginInstalled || !status.ServerInstalled {
		t.Fatalf("chưa cài đủ: %+v", status)
	}

	started, err := app.StartPOTokenProvider(0)
	if err != nil {
		t.Fatalf("chạy provider thất bại: %v", err)
	}
	defer app.StopPOTokenProvider()
	if !started.Running {
		t.Fatalf("provider không chạy: %+v", started)
	}
	t.Logf("provider đang chạy tại http://127.0.0.1:%d", started.Port)

	if tuning := app.GetYtDlpTuning(); tuning.PluginDir == "" || tuning.ProviderURL == "" {
		t.Fatalf("cấu hình chưa được ghi lại: %+v", tuning)
	}

	// The yt-dlp .exe build cannot load plugins, so what matters is the token the
	// app mints over HTTP and the download it enables.
	session, sessionErr := app.youTubeSessionFor(app.settings.snapshot().CookieSource)
	if sessionErr != nil {
		t.Logf("không mở được phiên YouTube, mint ẩn danh: %v", sessionErr)
	} else {
		t.Logf("phiên YouTube: visitor %d ký tự, đăng nhập=%v", len(session.VisitorData), session.signedIn())
	}
	token, err := app.providerToken(app.GetYtDlpTuning(), session)
	if err != nil {
		t.Fatalf("không lấy được PO token: %v", err)
	}
	t.Logf("token dài %d ký tự, binding %d ký tự, hạn %s",
		len(token.Token), len(token.Binding), token.Expires.Local().Format("15:04:05"))

	target := os.Getenv("MOTCHILL_LIVE_PROVIDER_URL")
	if target == "" {
		return
	}
	directory := t.TempDir()
	output := filepath.Join(directory, "probe.mp4")
	item := DownloadItem{ID: "probe", Name: "probe", PageURL: target, Engine: engineYtDlp}
	if err := app.downloadWithYtDlp(context.Background(), app.findFFmpeg(), item, output, 1, 1, 240, "probe"); err != nil {
		t.Fatalf("tải bằng PO token thất bại: %v", err)
	}
	info, err := os.Stat(output)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("đã tải %.1f MB bằng PO token", float64(info.Size())/1024/1024)
	if info.Size() < 1<<20 {
		t.Fatalf("file quá nhỏ: %d bytes", info.Size())
	}
}
