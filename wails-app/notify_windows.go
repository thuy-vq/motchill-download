//go:build windows

package main

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	toast "git.sr.ht/~jackmordaunt/go-toast/v2"
	"golang.org/x/sys/windows/registry"
)

const (
	// notificationAppID is what Windows shows as the sender of the toast.
	notificationAppID = "MotchillDownloader.VideoHtmlDownloader"
	// A stable GUID keeps the registration tied to this app across versions.
	notificationGUID = "{7E9C1A64-3B62-4D5E-9F0B-2C5F8D41A7E2}"
)

var notificationSetup sync.Once

// registerNotificationApp tells Windows the display name and icon of the sender,
// without which a toast can be accepted and then never shown.
func registerNotificationApp() {
	executable, err := os.Executable()
	if err != nil {
		executable = ""
	}
	_ = toast.SetAppData(toast.AppData{
		AppID:         notificationAppID,
		GUID:          notificationGUID,
		ActivationExe: executable,
		IconPath:      executable,
	})
	// SetAppData writes the raw AppID as the display name; the toast header reads
	// better with the product name.
	key, _, err := registry.CreateKey(registry.CURRENT_USER,
		`Software\Classes\AppUserModelId\`+notificationAppID, registry.SET_VALUE)
	if err != nil {
		return
	}
	defer key.Close()
	_ = key.SetStringValue("DisplayName", appWindowTitle)
}

// showNotification raises a Windows toast. go-toast falls back to PowerShell when
// the WinRT path is unavailable, and a last-resort PowerShell call covers the rest.
func showNotification(title, body string) error {
	notificationSetup.Do(registerNotificationApp)
	notification := toast.Notification{
		AppID:    notificationAppID,
		Title:    title,
		Body:     body,
		Duration: toast.Long,
	}
	if err := notification.Push(); err == nil {
		return nil
	} else if fallbackErr := pushToastWithPowerShell(title, body); fallbackErr != nil {
		return fmt.Errorf("không hiện được thông báo: %w", err)
	}
	return nil
}

func pushToastWithPowerShell(title, body string) error {
	// PowerShell's own AppID is registered on every Windows install, so the toast
	// is delivered even when this app has no Start Menu shortcut.
	const powerShellAppID = `{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe`
	script := strings.NewReplacer("{title}", escapeForXML(title), "{body}", escapeForXML(body),
		"{appid}", powerShellAppID).Replace(
		`[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType=WindowsRuntime] > $null;` +
			`$xml = New-Object Windows.Data.Xml.Dom.XmlDocument;` +
			`$xml.LoadXml('<toast><visual><binding template="ToastGeneric"><text>{title}</text><text>{body}</text></binding></visual></toast>');` +
			`$toast = New-Object Windows.UI.Notifications.ToastNotification $xml;` +
			`[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier('{appid}').Show($toast);`)
	command := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", script)
	prepareBackgroundCommand(command)
	if output, err := command.CombinedOutput(); err != nil {
		return fmt.Errorf("%w — %s", err, strings.TrimSpace(string(output)))
	}
	return nil
}

func escapeForXML(value string) string {
	return strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;", `"`, "&quot;", "'", "''").Replace(value)
}
