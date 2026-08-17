//go:build windows

package main

import (
	"os"
	"strings"
	"testing"
)

func TestEscapeForXMLKeepsToastXMLValid(t *testing.T) {
	got := escapeForXML(`Tập 1 & 2 <b>"x"</b>`)
	for _, unsafe := range []string{"<b>", `"x"`, " & "} {
		if strings.Contains(got, unsafe) {
			t.Fatalf("%q was left unescaped in %q", unsafe, got)
		}
	}
	if !strings.Contains(got, "&amp;") || !strings.Contains(got, "&lt;b&gt;") {
		t.Fatalf("unexpected escaping: %q", got)
	}
}

// TestShowNotificationPushes raises a real toast, so it only runs on request:
//
//	MOTCHILL_TOAST=1 go test -run TestShowNotificationPushes -v
func TestShowNotificationPushes(t *testing.T) {
	if os.Getenv("MOTCHILL_TOAST") != "1" {
		t.Skip("MOTCHILL_TOAST is not set")
	}
	if err := showNotification("Tải xong tất cả", "3 thành công · 0 lỗi · 1 bỏ qua"); err != nil {
		t.Fatal(err)
	}
}
